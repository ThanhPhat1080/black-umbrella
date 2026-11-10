import type { Sql } from "./sql";
import { sha256Hex } from "./sha256";
import { normalizeDisplayName, validPin, validPublicKey, validUsername } from "./validate";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    public extra?: Record<string, unknown>,
  ) {
    super(code);
  }
}

export interface User {
  username: string;
  displayName: string;
  publicKey: string;
  joinedAt: number;
  online: boolean;
  lastSeen: number | null;
}

export interface UserRow {
  username: string;
  display_name: string;
  public_key: string | null;
  joined_at: number;
  last_seen: number | null;
}

export function userView(row: UserRow, online: boolean): User {
  return {
    username: row.username,
    displayName: row.display_name,
    publicKey: row.public_key ?? "",
    joinedAt: row.joined_at,
    online,
    lastSeen: row.last_seen ?? null,
  };
}

const PBKDF2_ITERS = 100_000;
const MAX_FAILS = 5;
const LOCK_BASE_MS = 15 * 60_000;
const SESSION_MS = 7 * 86_400_000;

function b64url(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
function randomB64url(n: number): string {
  return b64url(crypto.getRandomValues(new Uint8Array(n)));
}
function hex(buf: ArrayBuffer | Uint8Array): string {
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}
async function pbkdf2(pin: string, salt: Uint8Array): Promise<string> {
  const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(pin), "PBKDF2", false, ["deriveBits"]);
  const bits = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt, iterations: PBKDF2_ITERS }, key, 256);
  return hex(bits);
}
function fromHex(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  return out;
}
function constEq(a: string, b: string): boolean {
  let d = a.length ^ b.length;
  const n = Math.max(a.length, b.length);
  for (let i = 0; i < n; i++) d |= (a.charCodeAt(i) || 0) ^ (b.charCodeAt(i) || 0);
  return d === 0;
}

export class Auth {
  constructor(
    private sql: Sql,
    private now: () => number,
  ) {}

  createInvite(username?: string): string {
    const code = randomB64url(16);
    this.sql.exec("INSERT INTO invites(code, username, created_at) VALUES (?,?,?)", code, username ?? null, this.now());
    return code;
  }

  private async newSession(username: string): Promise<string> {
    const token = randomB64url(32);
    const t = this.now();
    this.sql.exec("DELETE FROM sessions WHERE expires_at <= ?", t);
    this.sql.exec("INSERT INTO sessions(token_hash, username, expires_at) VALUES (?,?,?)", sha256Hex(token), username, t + SESSION_MS);
    return token;
  }

  async register(b: Record<string, unknown>): Promise<{ token: string; user: User }> {
    const { username, pin, publicKey } = b;
    if (!validUsername(username)) throw new ApiError(400, "invalid_username");
    const displayName = normalizeDisplayName(b.displayName);
    if (displayName === null) throw new ApiError(400, "invalid_display_name");
    if (!validPin(pin)) throw new ApiError(400, "invalid_pin");
    if (!validPublicKey(publicKey)) throw new ApiError(400, "invalid_public_key");

    const inv = this.sql.exec<{ username: string | null }>("SELECT username FROM invites WHERE code = ?", String(b.invite ?? ""))[0];
    if (!inv || (inv.username !== null && inv.username !== username)) throw new ApiError(403, "invalid_invite");
    const existing = this.sql.exec<UserRow>("SELECT * FROM users WHERE username = ?", username)[0];
    if (inv.username === null && existing) throw new ApiError(409, "username_taken");

    const salt = crypto.getRandomValues(new Uint8Array(16));
    const hash = await pbkdf2(pin, salt);
    const invite = String(b.invite);
    // After the await, re-check everything and write atomically: a conflict leaves the invite unused.
    this.sql.tx(() => {
      const cur = this.sql.exec<{ username: string | null }>("SELECT username FROM invites WHERE code = ?", invite)[0];
      if (!cur || (cur.username !== null && cur.username !== username)) throw new ApiError(403, "invalid_invite");
      const user = this.sql.exec<UserRow>("SELECT * FROM users WHERE username = ?", username)[0];
      if (cur.username === null && user) throw new ApiError(409, "username_taken");
      this.sql.exec("DELETE FROM invites WHERE code = ?", invite);
      if (cur.username !== null && user) {
        this.sql.exec(
          "UPDATE users SET pin_hash=?, pin_salt=?, public_key=?, fails=0, locks=0, locked_until=0 WHERE username=?",
          hash, hex(salt), publicKey, username,
        );
      } else {
        this.sql.exec(
          "INSERT INTO users(username, display_name, pin_hash, pin_salt, public_key, joined_at) VALUES (?,?,?,?,?,?)",
          username, displayName, hash, hex(salt), publicKey, this.now(),
        );
      }
    });
    const row = this.sql.exec<UserRow>("SELECT * FROM users WHERE username = ?", username)[0];
    return { token: await this.newSession(username), user: userView(row, false) };
  }

  async login(username: string, pin: string): Promise<string> {
    const row = this.sql.exec<UserRow & { pin_hash: string | null; pin_salt: string | null; fails: number; locks: number; locked_until: number }>(
      "SELECT * FROM users WHERE username = ?", username,
    )[0];
    if (!row || !row.pin_hash || !row.pin_salt) throw new ApiError(401, "bad_credentials");
    const t = this.now();
    if (row.locked_until > t) throw new ApiError(423, "locked", { until: row.locked_until });
    const ok = validPin(pin) && constEq(await pbkdf2(pin, fromHex(row.pin_salt)), row.pin_hash);
    // Everything below is synchronous, so concurrent attempts cannot lose updates.
    const t2 = this.now();
    const cur = this.sql.exec<{ locked_until: number }>("SELECT locked_until FROM users WHERE username = ?", username)[0];
    if (cur && cur.locked_until > t2) throw new ApiError(423, "locked", { until: cur.locked_until });
    if (!ok) {
      const r = this.sql.exec<{ fails: number }>("UPDATE users SET fails = fails + 1 WHERE username = ? RETURNING fails", username)[0];
      if (r && r.fails >= MAX_FAILS) {
        const l = this.sql.exec<{ locks: number }>("UPDATE users SET locks = locks + 1, fails = 0 WHERE username = ? RETURNING locks", username)[0];
        this.sql.exec("UPDATE users SET locked_until = ? WHERE username = ?", t2 + LOCK_BASE_MS * 2 ** (l.locks - 1), username);
      }
      throw new ApiError(401, "bad_credentials");
    }
    this.sql.exec("UPDATE users SET fails=0, locks=0, locked_until=0 WHERE username=?", username);
    return this.newSession(username);
  }

  userForToken(token: string): string | null {
    const r = this.sql.exec<{ username: string }>(
      "SELECT username FROM sessions WHERE token_hash = ? AND expires_at > ?", sha256Hex(token), this.now(),
    )[0];
    return r ? r.username : null;
  }

  reset(username: string): string {
    const u = this.sql.exec("SELECT username FROM users WHERE username = ?", username);
    if (u.length === 0) throw new ApiError(404, "user_not_found");
    this.sql.exec("UPDATE users SET pin_hash=NULL, pin_salt=NULL, public_key=NULL, fails=0, locks=0, locked_until=0 WHERE username=?", username);
    this.sql.exec("DELETE FROM sessions WHERE username = ?", username);
    this.sql.exec("DELETE FROM invites WHERE username = ?", username);
    return this.createInvite(username);
  }

  unlock(username: string): void {
    const u = this.sql.exec("SELECT username FROM users WHERE username = ?", username);
    if (u.length === 0) throw new ApiError(404, "user_not_found");
    this.sql.exec("UPDATE users SET fails=0, locks=0, locked_until=0 WHERE username=?", username);
  }
}
