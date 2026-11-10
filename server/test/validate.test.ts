import { describe, it, expect } from "vitest";
import { validUsername, validPin, normalizeDisplayName, validPublicKey, dmConv, parseConv } from "../src/validate";
import { memorySql } from "./sqlite";
import { migrate } from "../src/schema";

describe("validate", () => {
  it("usernames", () => {
    expect(validUsername("ab")).toBe(false);
    expect(validUsername("abc")).toBe(true);
    expect(validUsername("a_1")).toBe(true);
    expect(validUsername("Abc")).toBe(false);
    expect(validUsername("a".repeat(21))).toBe(false);
    expect(validUsername("a".repeat(20))).toBe(true);
  });
  it("pins", () => {
    expect(validPin("123456")).toBe(true);
    expect(validPin("12345")).toBe(false);
    expect(validPin("12345a")).toBe(false);
  });
  it("display names", () => {
    expect(normalizeDisplayName("  Phát 🌂 ")).toBe("Phát 🌂");
    expect(normalizeDisplayName("a".repeat(33))).toBeNull();
    expect(normalizeDisplayName("a".repeat(32))).toBe("a".repeat(32));
    expect(normalizeDisplayName("   ")).toBeNull();
  });
  it("public key", () => {
    expect(validPublicKey(Buffer.alloc(32, 1).toString("base64"))).toBe(true);
    expect(validPublicKey(Buffer.alloc(31, 1).toString("base64"))).toBe(false);
    expect(validPublicKey("!!")).toBe(false);
  });
  it("convs", () => {
    expect(dmConv("b", "a")).toBe("dm:a|b");
    expect(parseConv("dm:a|a")).toBeNull();
    expect(parseConv("hell")).toEqual({ kind: "hell" });
    expect(parseConv("dm:abc|bcd")).toEqual({ kind: "dm", a: "abc", b: "bcd" });
    expect(parseConv("dm:bcd|abc")).toBeNull();
    expect(parseConv("x")).toBeNull();
  });
});

describe("schema", () => {
  it("migrate is idempotent", () => {
    const s = memorySql();
    migrate(s);
    migrate(s);
    s.exec("INSERT INTO invites(code, username, created_at) VALUES (?,?,?)", "c", null, 1);
    expect(s.exec("SELECT code FROM invites")).toEqual([{ code: "c" }]);
  });
});

import { sha256Hex } from "../src/sha256";
import { createHash } from "node:crypto";
describe("sha256", () => {
  it("matches node", () => {
    for (const s of ["", "abc", "Tiếng Việt 🌂", "x".repeat(55), "x".repeat(56), "x".repeat(64), "x".repeat(1000)])
      expect(sha256Hex(s)).toBe(createHash("sha256").update(s).digest("hex"));
  });
});

describe("display name control chars", () => {
  it("rejects control characters and newlines", () => {
    for (const s of ["a\nb", "a\x1bb", "a\x07", "a\u0085b", "a b", "a\x7fb"])
      expect(normalizeDisplayName(s), JSON.stringify(s)).toBeNull();
    expect(normalizeDisplayName("Phát 🌂")).toBe("Phát 🌂");
  });
});

describe("schema sender_key migration", () => {
  it("adds sender_key to an old messages table", () => {
    const s = memorySql();
    s.exec("CREATE TABLE messages(id TEXT PRIMARY KEY, conv TEXT NOT NULL, sender TEXT NOT NULL, parent_id TEXT, created_at INTEGER NOT NULL, kept INTEGER NOT NULL DEFAULT 0, nonce TEXT NOT NULL, ct TEXT NOT NULL)");
    migrate(s);
    migrate(s);
    expect(s.exec<{ name: string }>("PRAGMA table_info(messages)").map((c) => c.name)).toContain("sender_key");
  });
});
