import type { Sql } from "./sql";
import type { Auth, UserRow } from "./auth";
import { userView } from "./auth";
import { normalizeDisplayName, parseConv } from "./validate";

export const EXPIRY_MS = 86_400_000;
const MAX_CT = 24_000;

export interface Outbox {
  send(username: string, msg: object): void;
  online(): string[];
}

interface MsgRow {
  id: string;
  conv: string;
  sender: string;
  parent_id: string | null;
  created_at: number;
  kept: number;
  nonce: string;
  ct: string;
}

function messageView(m: MsgRow, key: string) {
  return {
    id: m.id,
    conv: m.conv,
    sender: m.sender,
    parentId: m.parent_id,
    createdAt: m.created_at,
    kept: m.kept === 1,
    nonce: m.nonce,
    ct: m.ct,
    key,
  };
}

export class Core {
  constructor(
    private sql: Sql,
    private auth: Auth,
    private out: Outbox,
    private now: () => number,
  ) {}

  private userRow(username: string): UserRow | undefined {
    return this.sql.exec<UserRow>("SELECT * FROM users WHERE username = ?", username)[0];
  }

  private broadcastUser(username: string, onlineOverride?: boolean): void {
    const row = this.userRow(username);
    if (!row) return;
    const online = onlineOverride ?? this.out.online().includes(username);
    const msg = { t: "user", user: userView(row, online) };
    for (const u of new Set(this.out.online())) this.out.send(u, msg);
  }

  hello(username: string): object {
    const online = new Set(this.out.online());
    online.add(username);
    const users = this.sql
      .exec<UserRow>("SELECT * FROM users ORDER BY username")
      .map((r) => userView(r, online.has(r.username)));
    const rows = this.sql.exec<MsgRow & { wrapped: string }>(
      `SELECT m.*, k.wrapped FROM messages m JOIN message_keys k ON k.message_id = m.id
       WHERE k.username = ? ORDER BY m.created_at, m.id`,
      username,
    );
    return { t: "hello", me: username, users, messages: rows.map((r) => messageView(r, r.wrapped)) };
  }

  onConnect(username: string, firstSocket: boolean): void {
    if (firstSocket) this.broadcastUser(username, true);
  }

  onDisconnect(username: string, lastSocket: boolean): void {
    if (!lastSocket) return;
    this.sql.exec("UPDATE users SET last_seen = ? WHERE username = ?", this.now(), username);
    this.broadcastUser(username, false);
  }

  userJoined(username: string): void {
    this.broadcastUser(username);
  }

  onMessage(username: string, raw: string): void {
    let m: Record<string, unknown>;
    try {
      const p = JSON.parse(raw);
      if (!p || typeof p !== "object") return;
      m = p;
    } catch {
      return;
    }
    switch (m.t) {
      case "send":
        return this.onSend(username, m);
      case "keep":
        return this.onKeep(username, m);
      case "typing":
        return this.onTyping(username, m);
      case "name":
        return this.onName(username, m);
    }
  }

  private err(username: string, ref: unknown, error: string): void {
    const msg: Record<string, unknown> = { t: "error", error };
    if (typeof ref === "string") msg.ref = ref;
    this.out.send(username, msg);
  }

  /** Users that must hold a wrapped key for a conv, or null when the conv is invalid for `username`. */
  private recipients(username: string, conv: string): string[] | null {
    const c = parseConv(conv);
    if (!c) return null;
    if (c.kind === "hell") {
      return this.sql
        .exec<{ username: string }>("SELECT username FROM users WHERE public_key IS NOT NULL ORDER BY username")
        .map((r) => r.username);
    }
    if (username !== c.a && username !== c.b) return null;
    if (!this.userRow(c.a) || !this.userRow(c.b)) return null;
    return [c.a, c.b];
  }

  private onSend(username: string, m: Record<string, unknown>): void {
    const ref = m.ref;
    const conv = typeof m.conv === "string" ? m.conv : "";
    const want = this.recipients(username, conv);
    if (!want) return this.err(username, ref, "bad_conv");
    if (typeof m.ct !== "string" || typeof m.nonce !== "string" || !m.ct || !m.nonce) return this.err(username, ref, "bad_request");
    if (m.ct.length > MAX_CT) return this.err(username, ref, "too_large");
    let parentId: string | null = null;
    if (m.parentId !== undefined && m.parentId !== null) {
      const p = typeof m.parentId === "string"
        ? this.sql.exec<{ conv: string; parent_id: string | null }>("SELECT conv, parent_id FROM messages WHERE id = ?", m.parentId)[0]
        : undefined;
      if (!p || p.conv !== conv || p.parent_id !== null) return this.err(username, ref, "bad_parent");
      parentId = m.parentId as string;
    }
    const keys = m.keys && typeof m.keys === "object" ? (m.keys as Record<string, unknown>) : {};
    const have = Object.keys(keys);
    if (
      have.length !== want.length ||
      !want.every((u) => typeof keys[u] === "string" && (keys[u] as string).length > 0)
    ) {
      return this.err(username, ref, "recipients_mismatch");
    }
    const createdAt = this.now();
    const rnd = [...crypto.getRandomValues(new Uint8Array(4))].map((b) => b.toString(16).padStart(2, "0")).join("");
    const id = createdAt.toString(36).padStart(9, "0") + rnd;
    this.sql.exec(
      "INSERT INTO messages(id, conv, sender, parent_id, created_at, kept, nonce, ct) VALUES (?,?,?,?,?,0,?,?)",
      id, conv, username, parentId, createdAt, m.nonce, m.ct,
    );
    for (const u of want) this.sql.exec("INSERT INTO message_keys(message_id, username, wrapped) VALUES (?,?,?)", id, u, keys[u]);
    const row: MsgRow = { id, conv, sender: username, parent_id: parentId, created_at: createdAt, kept: 0, nonce: m.nonce, ct: m.ct };
    for (const u of want) {
      const frame: Record<string, unknown> = { t: "msg", message: messageView(row, keys[u] as string) };
      if (u === username && typeof ref === "string") frame.ref = ref;
      this.out.send(u, frame);
    }
  }

  private msgRecipients(id: string): string[] {
    return this.sql.exec<{ username: string }>("SELECT username FROM message_keys WHERE message_id = ?", id).map((r) => r.username);
  }

  private deleteMessages(ids: string[]): void {
    for (const id of ids) {
      this.sql.exec("DELETE FROM message_keys WHERE message_id = ?", id);
      this.sql.exec("DELETE FROM messages WHERE id = ?", id);
    }
  }

  private onKeep(username: string, m: Record<string, unknown>): void {
    if (typeof m.id !== "string" || typeof m.kept !== "boolean") return;
    const row = this.sql.exec<MsgRow>("SELECT * FROM messages WHERE id = ?", m.id)[0];
    if (!row) return this.err(username, undefined, "not_found");
    if (row.sender !== username) return this.err(username, undefined, "not_owner");
    const recipients = this.msgRecipients(row.id);
    if (!m.kept && row.created_at + EXPIRY_MS <= this.now()) {
      this.deleteMessages([row.id]);
      for (const u of recipients) this.out.send(u, { t: "expired", ids: [row.id] });
      return;
    }
    this.sql.exec("UPDATE messages SET kept = ? WHERE id = ?", m.kept ? 1 : 0, row.id);
    for (const u of recipients) this.out.send(u, { t: "kept", id: row.id, kept: m.kept });
  }

  private onTyping(username: string, m: Record<string, unknown>): void {
    const conv = typeof m.conv === "string" ? m.conv : "";
    const c = parseConv(conv);
    if (!c) return;
    let targets: string[];
    if (c.kind === "hell") targets = this.out.online();
    else if (username === c.a) targets = [c.b];
    else if (username === c.b) targets = [c.a];
    else return;
    for (const u of new Set(targets)) if (u !== username) this.out.send(u, { t: "typing", conv, username });
  }

  private onName(username: string, m: Record<string, unknown>): void {
    const name = normalizeDisplayName(m.displayName);
    if (name === null) return this.err(username, undefined, "invalid_display_name");
    this.sql.exec("UPDATE users SET display_name = ? WHERE username = ?", name, username);
    this.broadcastUser(username);
  }

  sweep(): string[] {
    const cutoff = this.now() - EXPIRY_MS;
    const rows = this.sql.exec<{ id: string }>("SELECT id FROM messages WHERE kept = 0 AND created_at <= ?", cutoff);
    const ids = rows.map((r) => r.id);
    const perUser = new Map<string, string[]>();
    for (const id of ids) {
      for (const u of this.msgRecipients(id)) {
        if (!perUser.has(u)) perUser.set(u, []);
        perUser.get(u)!.push(id);
      }
    }
    this.deleteMessages(ids);
    for (const [u, list] of perUser) this.out.send(u, { t: "expired", ids: list });
    return ids;
  }
}
