import type { Sql } from "./sql";

const STATEMENTS = [
  `CREATE TABLE IF NOT EXISTS users(
    username TEXT PRIMARY KEY, display_name TEXT NOT NULL, pin_hash TEXT, pin_salt TEXT,
    public_key TEXT, joined_at INTEGER NOT NULL, last_seen INTEGER,
    fails INTEGER NOT NULL DEFAULT 0, locks INTEGER NOT NULL DEFAULT 0, locked_until INTEGER NOT NULL DEFAULT 0)`,
  `CREATE TABLE IF NOT EXISTS invites(code TEXT PRIMARY KEY, username TEXT, created_at INTEGER NOT NULL)`,
  `CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY, username TEXT NOT NULL, expires_at INTEGER NOT NULL)`,
  `CREATE TABLE IF NOT EXISTS messages(
    id TEXT PRIMARY KEY, conv TEXT NOT NULL, sender TEXT NOT NULL, parent_id TEXT,
    created_at INTEGER NOT NULL, kept INTEGER NOT NULL DEFAULT 0, nonce TEXT NOT NULL, ct TEXT NOT NULL, sender_key TEXT)`,
  `CREATE TABLE IF NOT EXISTS message_keys(
    message_id TEXT NOT NULL, username TEXT NOT NULL, wrapped TEXT NOT NULL, PRIMARY KEY(message_id, username))`,
  `CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(conv)`,
  `CREATE INDEX IF NOT EXISTS idx_messages_kept ON messages(kept, created_at)`,
];

export function migrate(sql: Sql): void {
  for (const s of STATEMENTS) sql.exec(s);
  const cols = sql.exec<{ name: string }>("PRAGMA table_info(messages)").map((c) => c.name);
  if (!cols.includes("sender_key")) sql.exec("ALTER TABLE messages ADD COLUMN sender_key TEXT");
}
