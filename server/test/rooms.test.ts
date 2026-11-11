import { describe, it, expect, beforeEach } from "vitest";
import { memorySql } from "./sqlite";
import { migrate } from "../src/schema";
import { Auth } from "../src/auth";
import { Core, type Outbox } from "../src/core";
import { validRoomName, parseConv } from "../src/validate";
import type { Sql } from "../src/sql";

const PK = Buffer.alloc(32, 7).toString("base64");
const NONCE = "n".repeat(32);
const W = (u: string) => (u + "w".repeat(96)).slice(0, 96);

class Rec implements Outbox {
  sent: { to: string; msg: any; rooms: boolean }[] = [];
  on = new Set<string>();
  send(to: string, msg: object, opts?: { rooms?: boolean }) { this.sent.push({ to, msg, rooms: !!opts?.rooms }); }
  online() { return [...this.on]; }
  to(u: string, t?: string) { return this.sent.filter((s) => s.to === u && (!t || s.msg.t === t)).map((s) => s.msg); }
  clear() { this.sent = []; }
}

let sql: Sql, t: number, out: Rec, auth: Auth, core: Core;
const USERS = ["alice", "bobby", "carol", "dave1"];

async function addUser(name: string) {
  await auth.register({ invite: auth.createInvite(), username: name, displayName: name.toUpperCase(), pin: "123456", publicKey: PK });
}
const msg = (from: string, o: Record<string, unknown>) => core.onMessage(from, JSON.stringify(o));
const keys = (...us: string[]) => Object.fromEntries(us.map((u) => [u, W(u)]));
function create(from: string, name: string, members: string[] = []): string {
  out.clear();
  msg(from, { t: "room_create", ref: "c", name, members });
  const ev = out.to(from, "room")[0];
  return ev?.room.id;
}

beforeEach(async () => {
  sql = memorySql(); migrate(sql); t = 1_000_000_000_000;
  out = new Rec(); auth = new Auth(sql, () => t); core = new Core(sql, auth, out, () => t);
  for (const u of USERS) await addUser(u);
  out.on = new Set(USERS);
});

describe("room name / conv validation", () => {
  it("accepts letters (incl. Vietnamese), digits, spaces and listed symbols", () => {
    for (const n of ["dev", "Phòng họp 1", "a-b_c", "!@#$%^&*()_-+=~`", "  trim  "]) expect(validRoomName(n)).not.toBeNull();
    expect(validRoomName("  trim  ")).toBe("trim");
  });
  it("rejects empty, long, emoji, control chars and other symbols", () => {
    for (const n of ["", "   ", "x".repeat(33), "hi😀", "a\nb", "a/b", "a<b>", "a.b", 5, null]) expect(validRoomName(n as any)).toBeNull();
  });
  it("parses room conv ids", () => {
    expect(parseConv("room:0123456789ab")).toEqual({ kind: "room", id: "room:0123456789ab" });
    expect(parseConv("room:xyz")).toBeNull();
  });
});

describe("create", () => {
  it("creates a room, owner is creator, members get the room event, name stored trimmed", () => {
    out.clear();
    msg("alice", { t: "room_create", ref: "r1", name: " Dev ", members: ["bobby"] });
    const a = out.to("alice", "room")[0], b = out.to("bobby", "room")[0];
    expect(a.room).toMatchObject({ name: "Dev", owner: "alice", members: ["alice", "bobby"] });
    expect(a.room.id).toMatch(/^room:[0-9a-f]{12}$/);
    expect(a.ref).toBe("r1");
    expect(b.room.id).toBe(a.room.id);
    expect(b.ref).toBeUndefined();
    expect(out.to("carol")).toHaveLength(0);
    expect(out.sent.every((s) => s.rooms)).toBe(true);
  });
  it("same name twice makes two rooms (names are not unique)", () => {
    expect(create("alice", "x")).not.toBe(create("bobby", "x"));
  });
  it("rejects bad name, unknown or keyless members, duplicates are folded", () => {
    create("alice", "bad/name");
    expect(out.to("alice", "error")[0].error).toBe("invalid_room_name");
    out.clear(); msg("alice", { t: "room_create", name: "ok", members: ["nobody"] });
    expect(out.to("alice", "error")[0]).toMatchObject({ error: "unknown_user", detail: "nobody" });
    out.clear(); msg("alice", { t: "room_create", name: "ok", members: ["bobby", "bobby", "alice"] });
    expect(out.to("alice", "room")[0].room.members).toEqual(["alice", "bobby"]);
  });
  it("enforces 20 members and 10 rooms per person", async () => {
    const many: string[] = [];
    for (let i = 0; i < 20; i++) { const n = `user${String(i).padStart(2, "0")}`; await addUser(n); many.push(n); }
    out.clear(); msg("alice", { t: "room_create", name: "big", members: many });
    expect(out.to("alice", "error")[0].error).toBe("room_full");
    for (let i = 0; i < 10; i++) expect(create("alice", `r${i}`)).toBeTruthy();
    out.clear(); msg("alice", { t: "room_create", name: "eleventh", members: [] });
    expect(out.to("alice", "error")[0].error).toBe("room_limit");
    out.clear(); msg("bobby", { t: "room_create", name: "z", members: ["alice"] });
    expect(out.to("bobby", "error")[0]).toMatchObject({ error: "user_room_limit", detail: "alice" });
  });
});

describe("visibility", () => {
  it("hello lists only my rooms, and only for rooms-capable clients", () => {
    const id = create("alice", "secret", ["bobby"]);
    expect((core.hello("bobby", true) as any).rooms).toEqual([{ id, name: "secret", owner: "alice", members: ["alice", "bobby"] }]);
    expect((core.hello("carol", true) as any).rooms).toEqual([]);
    expect((core.hello("bobby") as any).rooms).toBeUndefined();
  });
  it("hello hides room messages from old clients", () => {
    const id = create("alice", "secret", ["bobby"]);
    msg("alice", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby") });
    expect((core.hello("bobby", true) as any).messages).toHaveLength(1);
    expect((core.hello("bobby") as any).messages).toHaveLength(0);
    expect((core.hello("carol", true) as any).messages).toHaveLength(0);
  });
  it("non-members get not_found for every room operation, same as a missing room", () => {
    const id = create("alice", "secret", ["bobby"]);
    const ops = [
      { t: "room_invite", room: id, users: ["dave1"] }, { t: "room_kick", room: id, user: "bobby" },
      { t: "room_leave", room: id }, { t: "room_close", room: id }, { t: "room_name", room: id, name: "x" },
    ];
    for (const op of ops) {
      out.clear(); msg("carol", op);
      const real = out.to("carol", "error")[0].error;
      out.clear(); msg("carol", { ...op, room: "room:000000000000" });
      expect(out.to("carol", "error")[0].error).toBe(real);
      expect(real).toBe("not_found");
    }
    out.clear(); msg("carol", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby") });
    expect(out.to("carol", "error")[0].error).toBe("bad_conv");
    msg("carol", { t: "typing", conv: id });
    expect(out.to("alice")).toHaveLength(0);
  });
});

describe("messaging", () => {
  it("sends to members only, with recipients_mismatch for a stale member list", () => {
    const id = create("alice", "r", ["bobby"]);
    out.clear();
    msg("alice", { t: "send", ref: "s", conv: id, nonce: NONCE, ct: "c", keys: keys("alice") });
    expect(out.to("alice", "error")[0].error).toBe("recipients_mismatch");
    out.clear();
    msg("alice", { t: "send", ref: "s", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby") });
    expect(out.to("bobby", "msg")[0].message.key).toBe(W("bobby"));
    expect(out.to("carol")).toHaveLength(0);
    expect(out.sent.filter((s) => s.msg.t === "msg").every((s) => s.rooms)).toBe(true);
  });
  it("typing in a room reaches other members only", () => {
    const id = create("alice", "r", ["bobby"]);
    out.clear(); msg("alice", { t: "typing", conv: id });
    expect(out.to("bobby", "typing")).toHaveLength(1);
    expect(out.to("carol")).toHaveLength(0);
    expect(out.to("alice")).toHaveLength(0);
  });
});

describe("invite", () => {
  it("any member can invite; everyone gets the update with a note", () => {
    const id = create("alice", "r", ["bobby"]);
    out.clear(); msg("bobby", { t: "room_invite", ref: "i", room: id, users: ["carol"] });
    for (const u of ["alice", "bobby", "carol"]) expect(out.to(u, "room")[0].room.members).toEqual(["alice", "bobby", "carol"]);
    expect(out.to("carol", "room")[0].note).toEqual({ kind: "added", by: "bobby", users: ["carol"] });
    expect(out.to("bobby", "room")[0].ref).toBe("i");
  });
  it("new member cannot read earlier messages", () => {
    const id = create("alice", "r", ["bobby"]);
    msg("alice", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby") });
    msg("alice", { t: "room_invite", room: id, users: ["carol"] });
    expect((core.hello("carol", true) as any).messages).toHaveLength(0);
  });
  it("errors: already member, unknown user, full room, invitee over limit", async () => {
    const id = create("alice", "r", ["bobby"]);
    out.clear(); msg("alice", { t: "room_invite", room: id, users: ["bobby"] });
    expect(out.to("alice", "error")[0]).toMatchObject({ error: "already_member", detail: "bobby" });
    out.clear(); msg("alice", { t: "room_invite", room: id, users: ["ghost"] });
    expect(out.to("alice", "error")[0]).toMatchObject({ error: "unknown_user", detail: "ghost" });
    const many: string[] = [];
    for (let i = 0; i < 18; i++) { const n = `user${String(i).padStart(2, "0")}`; await addUser(n); many.push(n); }
    out.clear(); msg("alice", { t: "room_invite", room: id, users: [...many, "carol"] });
    expect(out.to("alice", "error")[0].error).toBe("room_full");
    for (let i = 0; i < 10; i++) create("dave1", `d${i}`);
    out.clear(); msg("alice", { t: "room_invite", room: id, users: ["dave1"] });
    expect(out.to("alice", "error")[0]).toMatchObject({ error: "user_room_limit", detail: "dave1" });
  });
});

describe("leave / kick / close / rename", () => {
  it("leave removes the member, drops their keys, tells the others", () => {
    const id = create("alice", "r", ["bobby", "carol"]);
    msg("alice", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby", "carol") });
    out.clear(); msg("carol", { t: "room_leave", room: id });
    expect(out.to("carol", "room_gone")[0]).toMatchObject({ id, reason: "left" });
    expect(out.to("alice", "room")[0].room.members).toEqual(["alice", "bobby"]);
    expect(out.to("alice", "room")[0].note).toEqual({ kind: "left", by: "carol", users: ["carol"] });
    expect((core.hello("carol", true) as any).messages).toHaveLength(0);
    expect((core.hello("alice", true) as any).messages).toHaveLength(1);
    out.clear(); msg("carol", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby", "carol") });
    expect(out.to("carol", "error")[0].error).toBe("bad_conv");
  });
  it("owner leaving passes ownership to the earliest joiner; last one out deletes the room", () => {
    const id = create("alice", "r", ["bobby"]);
    t += 1000; msg("bobby", { t: "room_invite", room: id, users: ["carol"] });
    out.clear(); msg("alice", { t: "room_leave", room: id });
    expect(out.to("bobby", "room")[0].room.owner).toBe("bobby");
    expect(out.to("bobby", "room")[0].note).toMatchObject({ kind: "left", newOwner: "bobby" });
    msg("bobby", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("bobby", "carol") });
    msg("bobby", { t: "room_leave", room: id }); msg("carol", { t: "room_leave", room: id });
    expect((core.hello("carol", true) as any).rooms).toEqual([]);
    expect(sql.exec("SELECT * FROM messages WHERE conv = ?", id)).toHaveLength(0);
    expect(sql.exec("SELECT * FROM message_keys")).toHaveLength(0);
    expect(sql.exec("SELECT * FROM rooms")).toHaveLength(0);
  });
  it("only the owner can kick; kicked user loses the room and history", () => {
    const id = create("alice", "r", ["bobby", "carol"]);
    msg("alice", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby", "carol") });
    out.clear(); msg("bobby", { t: "room_kick", room: id, user: "carol" });
    expect(out.to("bobby", "error")[0].error).toBe("not_owner");
    out.clear(); msg("alice", { t: "room_kick", room: id, user: "carol" });
    expect(out.to("carol", "room_gone")[0]).toMatchObject({ id, reason: "kicked", by: "alice" });
    expect(out.to("bobby", "room")[0].note).toEqual({ kind: "kicked", by: "alice", users: ["carol"] });
    expect((core.hello("carol", true) as any).messages).toHaveLength(0);
    out.clear(); msg("alice", { t: "room_kick", room: id, user: "carol" });
    expect(out.to("alice", "error")[0].error).toBe("not_member");
    out.clear(); msg("alice", { t: "room_kick", room: id, user: "alice" });
    expect(out.to("alice", "error")[0].error).toBe("bad_request");
  });
  it("only the owner can close; everything is deleted", () => {
    const id = create("alice", "r", ["bobby"]);
    msg("alice", { t: "send", conv: id, nonce: NONCE, ct: "c", keys: keys("alice", "bobby") });
    out.clear(); msg("bobby", { t: "room_close", room: id });
    expect(out.to("bobby", "error")[0].error).toBe("not_owner");
    out.clear(); msg("alice", { t: "room_close", room: id });
    for (const u of ["alice", "bobby"]) expect(out.to(u, "room_gone")[0]).toMatchObject({ id, reason: "closed" });
    expect(sql.exec("SELECT * FROM messages")).toHaveLength(0);
    expect(sql.exec("SELECT * FROM message_keys")).toHaveLength(0);
    expect(sql.exec("SELECT * FROM room_members")).toHaveLength(0);
  });
  it("only the owner can rename, with name validation", () => {
    const id = create("alice", "r", ["bobby"]);
    out.clear(); msg("bobby", { t: "room_name", room: id, name: "x" });
    expect(out.to("bobby", "error")[0].error).toBe("not_owner");
    out.clear(); msg("alice", { t: "room_name", room: id, name: "a/b" });
    expect(out.to("alice", "error")[0].error).toBe("invalid_room_name");
    out.clear(); msg("alice", { t: "room_name", room: id, name: "Mới" });
    expect(out.to("bobby", "room")[0]).toMatchObject({ room: { name: "Mới" }, note: { kind: "renamed", by: "alice", name: "Mới" } });
  });
});
