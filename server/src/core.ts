import type { Sql } from "./sql";
import type { Auth, UserRow } from "./auth";
import { userView } from "./auth";
import { normalizeDisplayName, parseConv, validRoomName, validUsername } from "./validate";

export const EXPIRY_MS = 86_400_000;
const MAX_CT = 32_768;
const MAX_KEPT_PER_USER = 200;
const NONCE_LEN = 32; // 24 bytes in base64
const WRAPPED_LEN = 96; // 72 bytes (24 nonce + 32 key + 16 tag) in base64
const MAX_ROOM_MEMBERS = 20;
const MAX_ROOMS_PER_USER = 10;
const B64 = /^[A-Za-z0-9+/]+={0,2}$/;

export interface Outbox {
  /** `rooms: true` delivers only to sockets that declared room support. */
  send(username: string, msg: object, opts?: { rooms?: boolean }): void;
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
  sender_key: string | null;
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
    senderKey: m.sender_key ?? "",
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

  hello(username: string, rooms = false): object {
    const online = new Set(this.out.online());
    online.add(username);
    const users = this.sql
      .exec<UserRow>("SELECT * FROM users ORDER BY username")
      .map((r) => userView(r, online.has(r.username)));
    const rows = this.sql.exec<MsgRow & { wrapped: string }>(
      `SELECT m.*, k.wrapped FROM messages m JOIN message_keys k ON k.message_id = m.id
       WHERE k.username = ? ${rooms ? "" : "AND m.conv NOT LIKE 'room:%'"} ORDER BY m.created_at, m.id`,
      username,
    );
    const hello: Record<string, unknown> = { t: "hello", me: username, users, messages: rows.map((r) => messageView(r, r.wrapped)) };
    if (rooms) {
      hello.rooms = this.sql
        .exec<{ room_id: string }>("SELECT room_id FROM room_members WHERE username = ? ORDER BY joined_at, room_id", username)
        .map((r) => this.roomView(r.room_id));
    }
    return hello;
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
      case "room_create":
        return this.onRoomCreate(username, m);
      case "room_invite":
        return this.onRoomInvite(username, m);
      case "room_kick":
        return this.onRoomKick(username, m);
      case "room_leave":
        return this.onRoomLeave(username, m);
      case "room_close":
        return this.onRoomClose(username, m);
      case "room_name":
        return this.onRoomName(username, m);
    }
  }

  private err(username: string, ref: unknown, error: string, detail?: string): void {
    const msg: Record<string, unknown> = { t: "error", error };
    if (detail !== undefined) msg.detail = detail;
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
    if (c.kind === "room") {
      if (!this.isMember(c.id, username)) return null;
      return this.sql
        .exec<{ username: string }>(
          `SELECT u.username FROM room_members r JOIN users u ON u.username = r.username
           WHERE r.room_id = ? AND u.public_key IS NOT NULL ORDER BY u.username`,
          c.id,
        )
        .map((r) => r.username);
    }
    if (username !== c.a && username !== c.b) return null;
    const have = this.sql.exec("SELECT username FROM users WHERE username IN (?, ?) AND public_key IS NOT NULL", c.a, c.b);
    if (have.length !== 2) return null;
    return [c.a, c.b];
  }

  private onSend(username: string, m: Record<string, unknown>): void {
    const ref = m.ref;
    const conv = typeof m.conv === "string" ? m.conv : "";
    const want = this.recipients(username, conv);
    if (!want) return this.err(username, ref, "bad_conv");
    if (typeof m.ct !== "string" || typeof m.nonce !== "string" || !m.ct || !m.nonce) return this.err(username, ref, "bad_request");
    if (m.ct.length > MAX_CT) return this.err(username, ref, "too_large");
    if (m.nonce.length !== NONCE_LEN || !B64.test(m.nonce)) return this.err(username, ref, "bad_request");
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
    if (!want.every((u) => (keys[u] as string).length === WRAPPED_LEN && B64.test(keys[u] as string))) {
      return this.err(username, ref, "bad_request");
    }
    const createdAt = this.now();
    const senderKey = this.userRow(username)?.public_key ?? null;
    const rnd = [...crypto.getRandomValues(new Uint8Array(4))].map((b) => b.toString(16).padStart(2, "0")).join("");
    const id = createdAt.toString(36).padStart(9, "0") + rnd;
    this.sql.exec(
      "INSERT INTO messages(id, conv, sender, parent_id, created_at, kept, nonce, ct, sender_key) VALUES (?,?,?,?,?,0,?,?,?)",
      id, conv, username, parentId, createdAt, m.nonce, m.ct, senderKey,
    );
    for (const u of want) this.sql.exec("INSERT INTO message_keys(message_id, username, wrapped) VALUES (?,?,?)", id, u, keys[u]);
    const row: MsgRow = { id, conv, sender: username, parent_id: parentId, created_at: createdAt, kept: 0, nonce: m.nonce, ct: m.ct, sender_key: senderKey };
    const roomOnly = conv.startsWith("room:") ? { rooms: true } : undefined;
    for (const u of want) {
      const frame: Record<string, unknown> = { t: "msg", message: messageView(row, keys[u] as string) };
      if (u === username && typeof ref === "string") frame.ref = ref;
      this.out.send(u, frame, roomOnly);
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
    if (m.kept && row.kept !== 1) {
      const n = this.sql.exec<{ n: number }>("SELECT COUNT(*) AS n FROM messages WHERE sender = ? AND kept = 1", username)[0].n;
      if (n >= MAX_KEPT_PER_USER) return this.err(username, undefined, "keep_limit");
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
    else if (c.kind === "room") {
      if (!this.isMember(c.id, username)) return;
      targets = this.members(c.id);
    } else if (username === c.a) targets = [c.b];
    else if (username === c.b) targets = [c.a];
    else return;
    for (const u of new Set(targets)) if (u !== username) this.out.send(u, { t: "typing", conv, username }, c.kind === "room" ? { rooms: true } : undefined);
  }

  private onName(username: string, m: Record<string, unknown>): void {
    const name = normalizeDisplayName(m.displayName);
    if (name === null) return this.err(username, undefined, "invalid_display_name");
    this.sql.exec("UPDATE users SET display_name = ? WHERE username = ?", name, username);
    this.broadcastUser(username);
  }

  // ---- rooms ----

  /** Members ordered by join time (ties by name). */
  private members(id: string): string[] {
    return this.sql
      .exec<{ username: string }>("SELECT username FROM room_members WHERE room_id = ? ORDER BY joined_at, username", id)
      .map((r) => r.username);
  }

  private isMember(id: string, username: string): boolean {
    return this.sql.exec("SELECT 1 FROM room_members WHERE room_id = ? AND username = ?", id, username).length > 0;
  }

  private roomView(id: string) {
    const r = this.sql.exec<{ name: string; owner: string }>("SELECT name, owner FROM rooms WHERE id = ?", id)[0];
    return { id, name: r.name, owner: r.owner, members: this.members(id) };
  }

  private roomCount(username: string): number {
    return this.sql.exec<{ n: number }>("SELECT COUNT(*) AS n FROM room_members WHERE username = ?", username)[0].n;
  }

  /** The room when `username` belongs to it. Non-members get the same answer as for a missing room. */
  private memberRoom(username: string, m: Record<string, unknown>): string | null {
    const id = typeof m.room === "string" ? m.room : "";
    if (parseConv(id)?.kind !== "room" || !this.isMember(id, username)) {
      this.err(username, m.ref, "not_found");
      return null;
    }
    return id;
  }

  private emitRoom(id: string, note: Record<string, unknown>, to?: { user: string; ref: unknown }): void {
    const room = this.roomView(id);
    for (const u of room.members) {
      const frame: Record<string, unknown> = { t: "room", room, note };
      if (to && u === to.user && typeof to.ref === "string") frame.ref = to.ref;
      this.out.send(u, frame, { rooms: true });
    }
  }

  private newRoomId(): string {
    return "room:" + [...crypto.getRandomValues(new Uint8Array(6))].map((b) => b.toString(16).padStart(2, "0")).join("");
  }

  /** Cleaned list of invitees, or null after replying with an error. */
  private inviteList(username: string, m: Record<string, unknown>, existing: string[]): string[] | null {
    if (!Array.isArray(m.users ?? m.members) ) { this.err(username, m.ref, "bad_request"); return null; }
    const raw = (m.users ?? m.members) as unknown[];
    const out: string[] = [];
    for (const u of raw) {
      if (typeof u !== "string" || !validUsername(u)) { this.err(username, m.ref, "unknown_user", String(u).slice(0, 20)); return null; }
      const row = this.userRow(u);
      if (!row || !row.public_key) { this.err(username, m.ref, "unknown_user", u); return null; }
      if (existing.includes(u)) {
        if (u === username) continue;
        this.err(username, m.ref, "already_member", u);
        return null;
      }
      if (!out.includes(u)) out.push(u);
    }
    if (existing.length + out.length > MAX_ROOM_MEMBERS) { this.err(username, m.ref, "room_full"); return null; }
    for (const u of out) {
      if (this.roomCount(u) >= MAX_ROOMS_PER_USER) { this.err(username, m.ref, "user_room_limit", u); return null; }
    }
    return out;
  }

  private onRoomCreate(username: string, m: Record<string, unknown>): void {
    const name = validRoomName(m.name);
    if (name === null) return this.err(username, m.ref, "invalid_room_name");
    if (m.members === undefined) m.members = [];
    if (this.roomCount(username) >= MAX_ROOMS_PER_USER) return this.err(username, m.ref, "room_limit");
    const invitees = this.inviteList(username, m, [username]);
    if (!invitees) return;
    const id = this.newRoomId();
    const now = this.now();
    this.sql.tx(() => {
      this.sql.exec("INSERT INTO rooms(id, name, owner, created_at) VALUES (?,?,?,?)", id, name, username, now);
      for (const u of [username, ...invitees]) this.sql.exec("INSERT INTO room_members(room_id, username, joined_at) VALUES (?,?,?)", id, u, now);
    });
    this.emitRoom(id, { kind: "created", by: username, users: invitees }, { user: username, ref: m.ref });
  }

  private onRoomInvite(username: string, m: Record<string, unknown>): void {
    const id = this.memberRoom(username, m);
    if (!id) return;
    const invitees = this.inviteList(username, m, this.members(id));
    if (!invitees || invitees.length === 0) return invitees ? this.err(username, m.ref, "bad_request") : undefined;
    const now = this.now();
    this.sql.tx(() => {
      for (const u of invitees) this.sql.exec("INSERT INTO room_members(room_id, username, joined_at) VALUES (?,?,?)", id, u, now);
    });
    this.emitRoom(id, { kind: "added", by: username, users: invitees }, { user: username, ref: m.ref });
  }

  /** Remove `who` from the room (leave or kick); deletes the room when it empties and hands over ownership if needed. */
  private removeMember(id: string, who: string, by: string, kind: "left" | "kicked"): void {
    let newOwner: string | undefined;
    this.sql.tx(() => {
      this.sql.exec("DELETE FROM room_members WHERE room_id = ? AND username = ?", id, who);
      this.sql.exec(
        "DELETE FROM message_keys WHERE username = ? AND message_id IN (SELECT id FROM messages WHERE conv = ?)",
        who, id,
      );
      const rest = this.members(id);
      if (rest.length === 0) {
        this.dropRoom(id);
        return;
      }
      if (this.sql.exec<{ owner: string }>("SELECT owner FROM rooms WHERE id = ?", id)[0].owner === who) {
        newOwner = rest[0];
        this.sql.exec("UPDATE rooms SET owner = ? WHERE id = ?", newOwner, id);
      }
    });
    this.out.send(who, kind === "kicked" ? { t: "room_gone", id, reason: kind, by } : { t: "room_gone", id, reason: kind }, { rooms: true });
    if (this.sql.exec("SELECT 1 FROM rooms WHERE id = ?", id).length > 0) {
      const note: Record<string, unknown> = { kind, by, users: [who] };
      if (newOwner) note.newOwner = newOwner;
      this.emitRoom(id, note);
    }
  }

  private dropRoom(id: string): void {
    this.sql.exec("DELETE FROM message_keys WHERE message_id IN (SELECT id FROM messages WHERE conv = ?)", id);
    this.sql.exec("DELETE FROM messages WHERE conv = ?", id);
    this.sql.exec("DELETE FROM room_members WHERE room_id = ?", id);
    this.sql.exec("DELETE FROM rooms WHERE id = ?", id);
  }

  private ownerRoom(username: string, m: Record<string, unknown>): string | null {
    const id = this.memberRoom(username, m);
    if (!id) return null;
    if (this.sql.exec<{ owner: string }>("SELECT owner FROM rooms WHERE id = ?", id)[0].owner !== username) {
      this.err(username, m.ref, "not_owner");
      return null;
    }
    return id;
  }

  private onRoomKick(username: string, m: Record<string, unknown>): void {
    const id = this.ownerRoom(username, m);
    if (!id) return;
    if (typeof m.user !== "string" || m.user === username) return this.err(username, m.ref, "bad_request");
    if (!this.isMember(id, m.user)) return this.err(username, m.ref, "not_member", m.user.slice(0, 20));
    this.removeMember(id, m.user, username, "kicked");
  }

  private onRoomLeave(username: string, m: Record<string, unknown>): void {
    const id = this.memberRoom(username, m);
    if (id) this.removeMember(id, username, username, "left");
  }

  private onRoomClose(username: string, m: Record<string, unknown>): void {
    const id = this.ownerRoom(username, m);
    if (!id) return;
    const members = this.members(id);
    this.sql.tx(() => this.dropRoom(id));
    for (const u of members) this.out.send(u, { t: "room_gone", id, reason: "closed", by: username }, { rooms: true });
  }

  private onRoomName(username: string, m: Record<string, unknown>): void {
    const id = this.ownerRoom(username, m);
    if (!id) return;
    const name = validRoomName(m.name);
    if (name === null) return this.err(username, m.ref, "invalid_room_name");
    this.sql.exec("UPDATE rooms SET name = ? WHERE id = ?", name, id);
    this.emitRoom(id, { kind: "renamed", by: username, name }, { user: username, ref: m.ref });
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
