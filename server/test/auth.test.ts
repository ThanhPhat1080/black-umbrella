import { describe, it, expect, beforeEach } from "vitest";
import { memorySql } from "./sqlite";
import { migrate } from "../src/schema";
import { Auth, ApiError } from "../src/auth";
import { handleHttp } from "../src/http";
import type { Sql } from "../src/sql";

const PK = Buffer.alloc(32, 7).toString("base64");
const PK2 = Buffer.alloc(32, 9).toString("base64");
const MIN = 60_000;
const DAY = 86_400_000;

let sql: Sql;
let t: number;
let auth: Auth;

beforeEach(() => {
  sql = memorySql();
  migrate(sql);
  t = 1_000_000;
  auth = new Auth(sql, () => t);
});

const reg = (invite: string, username = "alice", pin = "123456", publicKey = PK) =>
  auth.register({ invite, username, displayName: "Alice", pin, publicKey });

async function code(fn: () => Promise<unknown> | unknown): Promise<ApiError> {
  try {
    await fn();
  } catch (e) {
    if (e instanceof ApiError) return e;
    throw e;
  }
  throw new Error("expected ApiError");
}

describe("register", () => {
  it("valid invite gives a token that resolves to the user", async () => {
    const r = await reg(auth.createInvite());
    expect(r.user.username).toBe("alice");
    expect(r.user.publicKey).toBe(PK);
    expect(auth.userForToken(r.token)).toBe("alice");
  });
  it("invite is single-use", async () => {
    const inv = auth.createInvite();
    await reg(inv);
    const e = await code(() => reg(inv, "bobby"));
    expect([e.status, e.code]).toEqual([403, "invalid_invite"]);
  });
  it("bad input codes", async () => {
    const inv = auth.createInvite();
    expect((await code(() => reg(inv, "A"))).code).toBe("invalid_username");
    expect((await code(() => reg(inv, "alice", "12"))).code).toBe("invalid_pin");
    expect((await code(() => reg(inv, "alice", "123456", "xx"))).code).toBe("invalid_public_key");
    expect((await code(() => auth.register({ invite: inv, username: "alice", displayName: " ", pin: "123456", publicKey: PK }))).code).toBe("invalid_display_name");
    // invalid attempts must not burn the invite
    await reg(inv);
  });
  it("duplicate username is 409 and keeps the invite", async () => {
    await reg(auth.createInvite());
    const inv2 = auth.createInvite();
    const e = await code(() => reg(inv2));
    expect([e.status, e.code]).toEqual([409, "username_taken"]);
    await reg(inv2, "bobby");
  });
  it("reset invite is bound to its user and replaces key, keeping joined_at", async () => {
    const r = await reg(auth.createInvite());
    const joined = r.user.joinedAt;
    t += 5000;
    const rc = auth.reset("alice");
    expect(auth.userForToken(r.token)).toBeNull();
    expect(sql.exec("SELECT public_key, pin_hash FROM users WHERE username='alice'")).toEqual([{ public_key: null, pin_hash: null }]);
    expect((await code(() => reg(rc, "bobby"))).code).toBe("invalid_invite");
    const r2 = await reg(rc, "alice", "654321", PK2);
    expect(r2.user.publicKey).toBe(PK2);
    expect(r2.user.joinedAt).toBe(joined);
    expect(await auth.login("alice", "654321")).toBeTruthy();
  });
  it("reset of unknown user is 404", async () => {
    const e = await code(() => auth.reset("nobody"));
    expect([e.status, e.code]).toEqual([404, "user_not_found"]);
  });
});

describe("login", () => {
  beforeEach(async () => {
    await reg(auth.createInvite());
  });
  it("good pin returns a token", async () => {
    const tok = await auth.login("alice", "123456");
    expect(auth.userForToken(tok)).toBe("alice");
  });
  it("unknown user and bad pin are 401", async () => {
    expect((await code(() => auth.login("zed", "123456"))).status).toBe(401);
    const e = await code(() => auth.login("alice", "000000"));
    expect([e.status, e.code]).toEqual([401, "bad_credentials"]);
  });
  it("5 failures lock for 15 min even for the right pin; then 30 min", async () => {
    for (let i = 0; i < 5; i++) expect((await code(() => auth.login("alice", "000000"))).status).toBe(401);
    const e = await code(() => auth.login("alice", "123456"));
    expect([e.status, e.code, e.extra?.until]).toEqual([423, "locked", t + 15 * MIN]);
    t += 15 * MIN + 1;
    for (let i = 0; i < 5; i++) await code(() => auth.login("alice", "000000"));
    const e2 = await code(() => auth.login("alice", "123456"));
    expect(e2.extra?.until).toBe(t + 30 * MIN);
  });
  it("success resets fails and locks", async () => {
    for (let i = 0; i < 4; i++) await code(() => auth.login("alice", "000000"));
    await auth.login("alice", "123456");
    expect(sql.exec("SELECT fails, locks FROM users")).toEqual([{ fails: 0, locks: 0 }]);
    for (let i = 0; i < 4; i++) await code(() => auth.login("alice", "000000"));
    await auth.login("alice", "123456"); // still not locked
  });
  it("unlock clears the lock", async () => {
    for (let i = 0; i < 5; i++) await code(() => auth.login("alice", "000000"));
    auth.unlock("alice");
    expect(await auth.login("alice", "123456")).toBeTruthy();
    expect((await code(() => auth.unlock("nobody"))).status).toBe(404);
  });
  it("sessions expire after 7 days", async () => {
    const tok = await auth.login("alice", "123456");
    t += 7 * DAY - 1;
    expect(auth.userForToken(tok)).toBe("alice");
    t += 2;
    expect(auth.userForToken(tok)).toBeNull();
    expect(auth.userForToken("garbage")).toBeNull();
  });
});

describe("http", () => {
  const call = (path: string, body: unknown, bearer?: string) =>
    handleHttp(
      new Request("http://x" + path, {
        method: "POST",
        headers: bearer ? { authorization: `Bearer ${bearer}` } : {},
        body: JSON.stringify(body),
      }),
      auth,
      "secret",
    );
  it("non-auth route returns null", async () => {
    expect(await handleHttp(new Request("http://x/api/ws"), auth, "secret")).toBeNull();
  });
  it("admin requires bearer", async () => {
    expect((await call("/api/admin/invite", {}))!.status).toBe(401);
    expect((await call("/api/admin/invite", {}, "wrong"))!.status).toBe(401);
    const r = (await call("/api/admin/invite", {}, "secret"))!;
    expect(r.status).toBe(200);
    expect(typeof (await r.json() as { code: string }).code).toBe("string");
  });
  it("register + login over http, 423 carries until", async () => {
    const inv = ((await (await call("/api/admin/invite", {}, "secret"))!.json()) as { code: string }).code;
    const rr = (await call("/api/register", { invite: inv, username: "alice", displayName: "A", pin: "123456", publicKey: PK }))!;
    expect(rr.status).toBe(200);
    for (let i = 0; i < 5; i++) await call("/api/login", { username: "alice", pin: "000000" });
    const l = (await call("/api/login", { username: "alice", pin: "123456" }))!;
    expect(l.status).toBe(423);
    expect(await l.json()).toEqual({ error: "locked", until: t + 15 * MIN });
    expect((await call("/api/admin/unlock", { username: "alice" }, "secret"))!.status).toBe(200);
    expect((await call("/api/admin/reset", { username: "nobody" }, "secret"))!.status).toBe(404);
  });
});
