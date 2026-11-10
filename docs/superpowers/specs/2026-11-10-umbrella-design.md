# Umbrella — Design Spec (Phase 1)

Internal, end-to-end encrypted terminal chat for ~10 people.
Runs natively on macOS, Linux and Windows (PowerShell / Windows Terminal).

## 1. Architecture

```
 Go TUI client (umbrella)  ──HTTPS──▶  Cloudflare Worker  ──▶  Durable Object "Hub"
   - all crypto lives here  ◀──WSS──     (router only)          - SQLite (ciphertext only)
   - nothing cached on disk                                      - WebSockets (hibernation)
                                                                 - alarm: 24h expiry sweep
```

- **Server:** TypeScript Cloudflare Worker forwarding every request to a single Durable
  Object named `hub`. The DO owns all state in its built-in SQLite, all WebSockets
  (Hibernation API) and an alarm for expiry. Fits the free plan.
- **Server logic** lives in a pure `Core` class over a small `Sql` interface so it is
  testable in Node (`node:sqlite`) without the Workers runtime; `HubDO` is a thin adapter.
- **Client:** Go + Bubble Tea, single static binary per OS. Server URL baked in at build
  time (`-ldflags -X`), overridable with `--server`.
- Repository: monorepo `server/` + `client/`.

## 2. Accounts & auth

| Rule | Value |
|---|---|
| Username | `^[a-z0-9_]{3,20}$`, immutable, unique |
| Display name | 1–32 runes after trim, any Unicode (Vietnamese, emoji); changeable |
| PIN | exactly 6 ASCII digits |
| Sign-up | requires a single-use invite code |
| PIN storage (server) | PBKDF2-SHA256, 100 000 iterations, 16-byte random salt |
| Lockout | 5 consecutive failures → lock 15 min; each further lock doubles (15, 30, 60 …), reset on successful login; admin can unlock |
| Session token | 32 random bytes (base64url), stored as SHA-256 hash, valid 7 days |

Admin is a CLI (`umbrella admin …`) authenticating with `ADMIN_TOKEN` (Worker secret):
- `admin invite` → prints a new invite code.
- `admin reset <username>` → deletes that user's PIN hash, public key and sessions, and
  prints an invite code bound to that username so they can re-register (new key).
- `admin unlock <username>` → clears failures/lock.

## 3. Encryption (client only)

- Each user has a static X25519 key pair generated on their device (NaCl `box`).
- Private key stored only on device: `~/.umbrella/key.json`
  (Windows `%APPDATA%\umbrella\key.json`), file mode 0600, encrypted with
  `secretbox(key = Argon2id(pin, salt16, time=3, mem=64 MiB, threads=1, len=32))`.
- Per message: random 32-byte content key `K`; `body = secretbox(json{"text"}, nonce24, K)`.
  For every recipient (including the sender) `wrapped = nonce24 || box.Seal(K, nonce24,
  recipientPub, senderPriv)` — authenticated to the sender's public key.
- Recipients: Hell room = every registered user at send time; DM = the two participants.
- The server is trusted to distribute public keys (no fingerprint verification, by decision).
- No forward secrecy. New members cannot read earlier messages. No plaintext on disk.
- `umbrella export-key <file>` copies the encrypted key file for backup.

## 4. Conversations & messages

- Conversations: `hell` (Hell room, always first) and `dm:<a>|<b>` (usernames sorted).
- Message: server id (time-sortable), conv, sender, optional `parentId` (thread reply;
  parent must be a root in the same conv), createdAt, kept flag, ciphertext, wrapped keys.
- Text max 4000 runes. No images/attachments. No edit, no delete.
- **Expiry:** every non-kept message is deleted 24 h after `createdAt`. Alarm sweeps every
  5 minutes and broadcasts `expired` ids.
- **⚓ Keep:** only the sender can toggle. Kept → never deleted. Unkeep → normal 24 h rule;
  if already older than 24 h it is deleted immediately. Applies to single messages; a
  reply kept under an expired root shows the root as `[message expired]`.
- **Threads:** Slack-style; roots show `💬 N replies`; opening a thread shows a right panel.
- **Mentions:** `@username` in Hell room → highlighted, terminal bell, unread badge.
  Unread counts are in-memory per client run.
- **Typing:** "<name> is typing…" for 4 s after a typing event (sent at most every 2 s).
- **Presence:** online if ≥1 socket open; `lastSeen` set on last disconnect.
- **Links:** `https?://\S+` detected; no preview; a `📋` marker next to the message copies it.

## 5. UI

- English UI; input accepts any Unicode (Vietnamese IME works; width via runewidth).
- Layout: sidebar | chat | thread panel (when open).
- Sidebar: Hell room on top; then users — online first, then alphabetical — each with a
  full 5×5 identicon (3 lines), online dot, unread badge.
- Identicon: SHA-256(username) → colour + 15 bits for a vertically symmetric 5×5 grid,
  rendered with half blocks (`▀▄█`), 10 cols × 3 lines. Messages show a coloured `■`.
- Profile (`/whois @user` or clicking a user's avatar): identicon, display name,
  @username, joined date, online / "last seen X ago".
- Commands: `/i` (kaomoji picker), `/whois @u`, `/name <new display name>`, `/quit`.
- `+` button under the input or `/i` opens a picker of 50 bundled kaomoji; choosing inserts
  it into the input and copies it to the clipboard.
- Mouse (click sidebar, `+`, `📋`, `⚓`, `💬`) and keyboard: `Tab` focus switch,
  `Ctrl+N/P` conversation switch, `↑/↓` select message, `y` copy link, `a` keep toggle,
  `r` open thread, `Esc` close panel/picker, `Enter` send, `Alt+Enter` newline.
- Clipboard: OS clipboard, OSC 52 fallback.

## 6. Login flow

`umbrella` → if no key file: prompt to `umbrella register`. Otherwise ask PIN once:
decrypt key file (wrong PIN fails locally first) → `POST /api/login` → open WebSocket.
