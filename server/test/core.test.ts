import { describe, it, expect, beforeEach } from "vitest";
import { memorySql } from "./sqlite";
import { migrate } from "../src/schema";
import { Auth } from "../src/auth";
import { Core, EXPIRY_MS, type Outbox } from "../src/core";
import type { Sql } from "../src/sql";

const PK = Buffer.alloc(32, 7).toString("base64");
const HOUR = 3_600_000;

class Rec implements Outbox {
  sent: { to: string; msg: any }[] = [];
  on = new Set<string>();
  send(to: string, msg: object) { this.sent.push({ to, msg }); }
  online() { return [...this.on]; }
  to(u: string, t?: string) { return this.sent.filter((s) => s.to === u && (!t || s.msg.t === t)).map((s) => s.msg); }
  clear() { this.sent = []; }
}

let sql: Sql, t: number, out: Rec, auth: Auth, core: Core;

async function addUser(name: string) {
  await auth.register({ invite: auth.createInvite(), username: name, displayName: name.toUpperCase(), pin: "123456", publicKey: PK });
}
const send = (from: string, o: Record<string, unknown>) => core.onMessage(from, JSON.stringify({ t: "send", ...o }));
const hellKeys = (...us: string[]) => Object.fromEntries(us.map((u) => [u, "k-" + u]));

beforeEach(async () => {
  sql = memorySql(); migrate(sql); t = 1_000_000_000_000;
  out = new Rec(); auth = new Auth(sql, () => t); core = new Core(sql, auth, out, () => t);
  for (const u of ["alice", "bobby", "carol"]) await addUser(u);
  out.on = new Set(["alice", "bobby", "carol"]);
});

describe("hello", () => {
  it("returns me, users with online flags, and only own messages with own key", () => {
    out.on = new Set(["bobby"]);
    send("alice", { ref: "r1", conv: "hell", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby", "carol") });
    send("alice", { conv: "dm:alice|bobby", nonce: "n", ct: "c2", keys: hellKeys("alice", "bobby") });
    const h: any = core.hello("carol");
    expect(h.t).toBe("hello");
    expect(h.me).toBe("carol");
    expect(h.users.map((u: any) => [u.username, u.online])).toEqual([["alice", false], ["bobby", true], ["carol", true]]);
    expect(h.messages).toHaveLength(1);
    expect(h.messages[0]).toMatchObject({ conv: "hell", key: "k-carol", kept: false, parentId: null, sender: "alice" });
    expect((core.hello("bobby") as any).messages).toHaveLength(2);
  });
});

describe("send", () => {
  it("hell reaches everyone with their own key; ref only to sender", () => {
    send("alice", { ref: "r1", conv: "hell", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby", "carol") });
    for (const u of ["alice", "bobby", "carol"]) {
      const [m] = out.to(u, "msg");
      expect(m.message.key).toBe("k-" + u);
      expect(m.ref).toBe(u === "alice" ? "r1" : undefined);
    }
  });
  it("missing or extra recipients -> recipients_mismatch to sender only", () => {
    send("alice", { ref: "r", conv: "hell", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby") });
    send("alice", { ref: "r2", conv: "hell", nonce: "n", ct: "c", keys: { ...hellKeys("alice", "bobby", "carol"), zed: "x" } });
    expect(out.sent.map((s) => [s.to, s.msg.error, s.msg.ref])).toEqual([["alice", "recipients_mismatch", "r"], ["alice", "recipients_mismatch", "r2"]]);
  });
  it("users without a public key are not recipients", async () => {
    auth.reset("carol");
    send("alice", { conv: "hell", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby") });
    expect(out.to("alice", "msg")).toHaveLength(1);
    expect(out.to("carol")).toHaveLength(0);
  });
  it("dm only to participants; strangers cannot post", () => {
    send("alice", { conv: "dm:alice|bobby", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby") });
    expect(out.to("carol")).toHaveLength(0);
    expect(out.to("bobby", "msg")).toHaveLength(1);
    send("carol", { conv: "dm:alice|bobby", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby") });
    expect(out.to("carol", "error")[0].error).toBe("bad_conv");
    send("alice", { conv: "dm:bobby|alice", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby") });
    expect(out.to("alice", "error")[0].error).toBe("bad_conv");
  });
  it("too_large", () => {
    send("alice", { conv: "hell", nonce: "n", ct: "x".repeat(24_001), keys: hellKeys("alice", "bobby", "carol") });
    expect(out.to("alice", "error")[0].error).toBe("too_large");
    out.clear();
    send("alice", { conv: "hell", nonce: "n", ct: "x".repeat(24_000), keys: hellKeys("alice", "bobby", "carol") });
    expect(out.to("alice", "msg")).toHaveLength(1);
  });
  it("threads: parent must be a root of the same conv", () => {
    const k = hellKeys("alice", "bobby", "carol");
    send("alice", { conv: "hell", nonce: "n", ct: "c", keys: k });
    const root = out.to("alice", "msg")[0].message.id;
    send("bobby", { conv: "hell", parentId: root, nonce: "n", ct: "c", keys: k });
    const reply = out.to("alice", "msg")[1].message;
    expect(reply.parentId).toBe(root);
    out.clear();
    send("bobby", { ref: "a", conv: "hell", parentId: reply.id, nonce: "n", ct: "c", keys: k });
    send("bobby", { ref: "b", conv: "dm:alice|bobby", parentId: root, nonce: "n", ct: "c", keys: hellKeys("alice", "bobby") });
    send("bobby", { ref: "c", conv: "hell", parentId: "nope", nonce: "n", ct: "c", keys: k });
    expect(out.sent.map((s) => s.msg.error)).toEqual(["bad_parent", "bad_parent", "bad_parent"]);
  });
  it("ignores malformed input", () => {
    core.onMessage("alice", "{not json");
    core.onMessage("alice", JSON.stringify({ t: "wat" }));
    core.onMessage("alice", "null");
    expect(out.sent).toHaveLength(0);
  });
});

describe("keep & expiry", () => {
  let id: string;
  beforeEach(() => {
    send("alice", { conv: "hell", nonce: "n", ct: "c", keys: hellKeys("alice", "bobby", "carol") });
    id = out.to("alice", "msg")[0].message.id;
    out.clear();
  });
  it("only sender can keep; broadcast to recipients", () => {
    core.onMessage("bobby", JSON.stringify({ t: "keep", id, kept: true }));
    expect(out.to("bobby", "error")[0].error).toBe("not_owner");
    core.onMessage("alice", JSON.stringify({ t: "keep", id: "zzz", kept: true }));
    expect(out.to("alice", "error")[0].error).toBe("not_found");
    out.clear();
    core.onMessage("alice", JSON.stringify({ t: "keep", id, kept: true }));
    for (const u of ["alice", "bobby", "carol"]) expect(out.to(u, "kept")).toEqual([{ t: "kept", id, kept: true }]);
  });
  it("sweep deletes non-kept after 24h, keeps kept, notifies", () => {
    send("alice", { conv: "hell", nonce: "n", ct: "c2", keys: hellKeys("alice", "bobby", "carol") });
    const id2 = out.to("alice", "msg")[0].message.id;
    core.onMessage("alice", JSON.stringify({ t: "keep", id: id2, kept: true }));
    out.clear();
    t += EXPIRY_MS - 1;
    expect(core.sweep()).toEqual([]);
    t += 2;
    expect(core.sweep()).toEqual([id]);
    expect(out.to("bobby", "expired")).toEqual([{ t: "expired", ids: [id] }]);
    expect(sql.exec("SELECT id FROM messages")).toEqual([{ id: id2 }]);
    expect(sql.exec("SELECT message_id FROM message_keys WHERE message_id = ?", id)).toEqual([]);
  });
  it("unkeeping an old message deletes it immediately", () => {
    core.onMessage("alice", JSON.stringify({ t: "keep", id, kept: true }));
    t += 25 * HOUR;
    expect(core.sweep()).toEqual([]);
    out.clear();
    core.onMessage("alice", JSON.stringify({ t: "keep", id, kept: false }));
    expect(out.to("bobby", "expired")).toEqual([{ t: "expired", ids: [id] }]);
    expect(sql.exec("SELECT id FROM messages")).toEqual([]);
  });
  it("unkeeping a fresh message just toggles", () => {
    core.onMessage("alice", JSON.stringify({ t: "keep", id, kept: true }));
    core.onMessage("alice", JSON.stringify({ t: "keep", id, kept: false }));
    expect(out.to("bobby", "kept").map((k) => k.kept)).toEqual([true, false]);
  });
  it("kept reply survives its expired root", () => {
    send("bobby", { conv: "hell", parentId: id, nonce: "n", ct: "r", keys: hellKeys("alice", "bobby", "carol") });
    const rid = out.to("bobby", "msg").at(-1).message.id;
    core.onMessage("bobby", JSON.stringify({ t: "keep", id: rid, kept: true }));
    t += EXPIRY_MS + 1;
    core.sweep();
    const h: any = core.hello("carol");
    expect(h.messages.map((m: any) => [m.id, m.parentId])).toEqual([[rid, id]]);
  });
});

describe("typing, name, presence", () => {
  it("typing reaches participants except sender", () => {
    core.onMessage("alice", JSON.stringify({ t: "typing", conv: "hell" }));
    expect(out.sent.map((s) => s.to).sort()).toEqual(["bobby", "carol"]);
    out.clear();
    core.onMessage("alice", JSON.stringify({ t: "typing", conv: "dm:alice|bobby" }));
    expect(out.sent).toEqual([{ to: "bobby", msg: { t: "typing", conv: "dm:alice|bobby", username: "alice" } }]);
    out.clear();
    core.onMessage("carol", JSON.stringify({ t: "typing", conv: "dm:alice|bobby" }));
    expect(out.sent).toHaveLength(0);
  });
  it("name updates and broadcasts user; invalid name errors", () => {
    core.onMessage("alice", JSON.stringify({ t: "name", displayName: "  Phát 🌂 " }));
    expect(out.to("bobby", "user")[0].user).toMatchObject({ username: "alice", displayName: "Phát 🌂", online: true });
    core.onMessage("alice", JSON.stringify({ t: "name", displayName: "   " }));
    expect(out.to("alice", "error")[0].error).toBe("invalid_display_name");
  });
  it("presence broadcast on first connect / last disconnect", () => {
    out.on = new Set(["bobby"]);
    core.onConnect("alice", false);
    expect(out.sent).toHaveLength(0);
    out.on.add("alice");
    core.onConnect("alice", true);
    expect(out.to("bobby", "user")[0].user).toMatchObject({ username: "alice", online: true });
    out.clear();
    t += 1000;
    core.onDisconnect("alice", true);
    expect(out.to("bobby", "user")[0].user).toMatchObject({ username: "alice", online: false, lastSeen: t });
  });
  it("userJoined notifies all online", async () => {
    await addUser("dave");
    out.clear();
    core.userJoined("dave");
    expect(out.sent.map((s) => s.to).sort()).toEqual(["alice", "bobby", "carol"]);
    expect(out.to("alice", "user")[0].user.username).toBe("dave");
  });
});
