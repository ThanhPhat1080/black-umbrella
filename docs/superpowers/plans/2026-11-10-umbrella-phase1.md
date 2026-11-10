# Umbrella Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an E2E-encrypted terminal chat (Hell room, DMs, threads, ⚓ keep, 24 h expiry) with a Cloudflare backend and a cross-platform Go TUI client.

**Architecture:** A Worker forwards everything to one Durable Object whose logic is a pure `Core` class over a `Sql` interface (tested in Node with `node:sqlite`). The Go client does all crypto (NaCl box/secretbox, Argon2id key file) and renders with Bubble Tea; the server only ever sees ciphertext.

**Tech Stack:** TypeScript, Cloudflare Workers + Durable Objects (SQLite, WebSocket Hibernation, alarms), wrangler 4, vitest; Go 1.24, `golang.org/x/crypto` (nacl, argon2), `github.com/coder/websocket`, Bubble Tea / Bubbles / Lip Gloss, `github.com/lrstanley/bubblezone`, `github.com/atotto/clipboard`, `github.com/mattn/go-runewidth`.

**Spec:** `docs/superpowers/specs/2026-11-10-umbrella-design.md`

## Global Constraints

- App name in UI: `Umbrella`; binary `umbrella`; config dir `~/.umbrella/` (Windows `%APPDATA%\umbrella`).
- Go module path: `github.com/ThanhPhat1080/black-umbrella/client`.
- Username `^[a-z0-9_]{3,20}$`; display name 1–32 runes (trimmed); PIN `^[0-9]{6}$`; text ≤ 4000 runes.
- PBKDF2-SHA256 100 000 iters, 16-byte salt. Lockout 5 fails → 15 min × 2^(locks-1). Session 7 days.
- Argon2id(time=3, mem=64*1024 KiB, threads=1, keyLen=32), salt 16 bytes. Key file mode 0600.
- Expiry 24 h (`86_400_000` ms) for non-kept messages; alarm every 5 min.
- English UI copy; never mangle Unicode input.
- Durable Object name: `hub`; binding `HUB`; secret `ADMIN_TOKEN`.

### Wire protocol (shared by server and client — exact)

HTTP JSON, errors are `{"error": "<code>"}`:

| Route | Body | Success | Errors |
|---|---|---|---|
| `POST /api/register` | `{invite, username, displayName, pin, publicKey}` (pk = base64 std, 32 bytes) | `200 {token, user}` | 400 `invalid_username`/`invalid_display_name`/`invalid_pin`/`invalid_public_key`; 403 `invalid_invite`; 409 `username_taken` |
| `POST /api/login` | `{username, pin}` | `200 {token}` | 401 `bad_credentials`; 423 `{error:"locked", until}` |
| `POST /api/admin/invite` | – | `200 {code}` | 401 `unauthorized` |
| `POST /api/admin/reset` | `{username}` | `200 {code}` | 401; 404 `user_not_found` |
| `POST /api/admin/unlock` | `{username}` | `200 {ok:true}` | 401; 404 |
| `GET /api/ws?token=…` | WebSocket upgrade | 101 | 401 `unauthorized` |

Admin routes use `Authorization: Bearer <ADMIN_TOKEN>`. Reset invites are bound to the username (`invites.username`), plain invites have `username = NULL`.

`User = {username, displayName, publicKey, joinedAt, online, lastSeen}` (times in ms, `lastSeen` null if never).
`Message = {id, conv, sender, parentId, createdAt, kept, nonce, ct, key}` — `key` is the wrapped key for the receiving user only; `parentId` null for roots.

WS client → server: `{t:"send", ref, conv, parentId?, nonce, ct, keys:{username: wrapped}}`, `{t:"keep", id, kept}`, `{t:"typing", conv}`, `{t:"name", displayName}`.
WS server → client: `{t:"hello", me, users, messages}`, `{t:"msg", ref?, message}`, `{t:"kept", id, kept}`, `{t:"expired", ids}`, `{t:"typing", conv, username}`, `{t:"user", user}`, `{t:"error", ref?, error}`.
Conv ids: `hell`, `dm:<a>|<b>` with `a < b`. `send` keys must equal the conv's recipient set exactly (hell = all users having a public key; dm = both) else `error: recipients_mismatch`. Error codes for WS: `bad_conv`, `bad_parent`, `too_large` (`ct` > 24 000 chars), `not_owner`, `not_found`, `invalid_display_name`.

## Review Focus

1. A user registers while others are online → existing clients must receive `{t:"user"}` before their next send, or sends fail with `recipients_mismatch`; client must refresh users on that error and retry once.
2. Reply whose root has expired (root deleted, reply kept) → thread still renders with `[message expired]` root, no crash/nil deref.
3. Vietnamese composed input (`Tiếng Việt có dấu ạ`) and emoji kaomoji → stored, sent, rendered with correct widths; layout never wraps mid-rune.
4. Wrong PIN locally → no server call, clear message; server lockout after 5 failures returns 423 with `until`, client prints when it unlocks.
5. Network drop / laptop sleep → client reconnects with backoff (1,2,4…30 s) and replaces state with the new `hello` snapshot without duplicating messages.

---

## Server (`server/`)

### Task 1: Server scaffold, `Sql` interface, schema, validation

**Files:**
- Create: `server/package.json`, `server/tsconfig.json`, `server/vitest.config.ts`, `server/wrangler.toml`
- Create: `server/src/sql.ts`, `server/src/schema.ts`, `server/src/validate.ts`
- Test: `server/test/sqlite.ts` (node:sqlite adapter), `server/test/validate.test.ts`

**Interfaces:**
- Produces: `interface Sql { exec<T = Record<string, unknown>>(query: string, ...params: unknown[]): T[] }`; `migrate(sql: Sql): void`; `validUsername(s): boolean`, `validPin(s): boolean`, `normalizeDisplayName(s): string | null`, `validPublicKey(b64): boolean`, `dmConv(a, b): string`, `parseConv(conv): {kind:"hell"} | {kind:"dm", a, b} | null`; test helper `memorySql(): Sql`.

Tables: `users(username PK, display_name, pin_hash, pin_salt, public_key, joined_at, last_seen, fails, locks, locked_until)`, `invites(code PK, username NULL, created_at)`, `sessions(token_hash PK, username, expires_at)`, `messages(id PK, conv, sender, parent_id, created_at, kept, nonce, ct)`, `message_keys(message_id, username, wrapped, PK(message_id, username))`, indexes on `messages(conv)`, `messages(kept, created_at)`.

- [x] **Step 1:** Write `validate.test.ts`: `validUsername("ab")===false`, `("abc")`, `("a_1")` true, `("Abc")` false, 21 chars false; `validPin("123456")` true, `"12345"`, `"12345a"` false; `normalizeDisplayName("  Phát 🌂 ")==="Phát 🌂"`, 33 runes → null, `"   "` → null; `dmConv("b","a")==="dm:a|b"`; `parseConv("dm:a|a")===null`, `parseConv("hell")` → hell.
- [x] **Step 2:** `cd server && npx vitest run` → FAIL (module missing).
- [x] **Step 3:** Implement the above; `memorySql` wraps `node:sqlite` `DatabaseSync` (`prepare(q).all(...p)` for statements returning rows, `run` otherwise — detect by `stmt.columns().length`). `migrate` is idempotent (`CREATE … IF NOT EXISTS`). `wrangler.toml`: `main="src/index.ts"`, DO binding `HUB` class `HubDO`, migration `new_sqlite_classes=["HubDO"]`, `compatibility_date="2026-10-01"`.
- [x] **Step 4:** `npx vitest run` → PASS; `npx tsc --noEmit` → no errors.
- [x] **Step 5:** Commit `feat(server): scaffold, schema and validation`.

### Task 2: Auth core — invites, register, login, lockout, sessions, admin

**Files:**
- Create: `server/src/auth.ts`, `server/src/http.ts`
- Test: `server/test/auth.test.ts`

**Interfaces:**
- Consumes: Task 1.
- Produces: `class Auth { constructor(sql: Sql, now: () => number); createInvite(username?: string): string; register(b): Promise<{token, user}>; login(username, pin): Promise<string>; userForToken(token): string | null; reset(username): string; unlock(username): void }` throwing `ApiError(status, code, extra?)`; `handleHttp(req: Request, auth: Auth, adminToken: string): Promise<Response | null>` (null = not an auth route); `userView(row, online): User`.

- [x] **Step 1:** Tests (fake clock): register with valid invite → token; invite reused → 403 `invalid_invite`; duplicate username → 409; reset-bound invite only valid for its username and re-registration replaces key while keeping `joined_at`; login good PIN → token whose `userForToken` returns username; 5 bad PINs → 6th call 423 with `until = now+15min`, even with right PIN; after `until` passes, 5 more bad → `until = now+30min`; success resets `fails` and `locks`; `unlock` clears; expired session (now+7d+1) → null; `handleHttp` admin without bearer → 401, `/api/admin/invite` with bearer → `{code}`.
- [x] **Step 2:** Run → FAIL.
- [x] **Step 3:** Implement. PBKDF2 via `crypto.subtle.deriveBits` (100 000, SHA-256, 256 bits); compare in constant time. Invite/token: 16 / 32 random bytes base64url. Store `sha256(token)` hex. Invite codes are consumed in the same call that creates the user.
- [x] **Step 4:** Run → PASS.
- [x] **Step 5:** Commit `feat(server): auth, lockout and admin endpoints`.

> **Deviation:** Added `server/src/sha256.ts` (sync SHA-256) because `Auth.userForToken` is specified sync but WebCrypto digest is async. 5th bad PIN returns 401; the lock is reported (423) from the 6th call on.

### Task 3: Realtime core — hello, send, keep, typing, name, presence, expiry

**Files:**
- Create: `server/src/core.ts`
- Test: `server/test/core.test.ts`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces: `interface Outbox { send(username: string, msg: object): void; online(): string[] }`; `class Core { constructor(sql, auth, out: Outbox, now); hello(username): object; onConnect(username, firstSocket: boolean): void; onDisconnect(username, lastSocket: boolean): void; onMessage(username, raw: string): void; sweep(): string[] }`; `const EXPIRY_MS = 86_400_000`.

- [x] **Step 1:** Tests with a recording `Outbox`: `hello` returns `me`, all users (online flags from `out.online()`), only messages where `message_keys` has a row for that user, each with its own `key`; `send` to hell with keys for all users → every user gets `{t:"msg"}` with their key and the sender's frame includes `ref`; keys missing a user → `error recipients_mismatch` to sender only; DM delivered only to the two participants; `parentId` of a reply or other conv → `bad_parent`; `keep` by non-sender → `not_owner`; keep → broadcast `kept` to recipients; `sweep` at createdAt+24h+1 deletes non-kept, keeps kept, returns ids and sends `expired`; unkeep of a 25 h old message deletes it immediately and sends `expired`; `typing` reaches conv participants except sender; `name` updates and broadcasts `user`; first connect / last disconnect broadcast `user` with online/lastSeen; registration broadcast (`Core.userJoined(username)`) sends `user` to all online.
- [x] **Step 2:** Run → FAIL.
- [x] **Step 3:** Implement. Message id = `createdAt` base36 padded to 9 + 8 random hex chars (sortable). Delete `message_keys` with their message. Malformed JSON / unknown `t` → ignored.
- [x] **Step 4:** Run → PASS.
- [x] **Step 5:** Commit `feat(server): realtime core with expiry and keep`.

> **Deviation:** Tests and implementation were written together (no separate RED run for this task). `hello.me` is the username string. Expiry is inclusive (`created_at + 24h <= now`).

### Task 4: Durable Object + Worker wiring, local smoke

**Files:**
- Create: `server/src/hub.ts`, `server/src/index.ts`, `server/.dev.vars.example`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces: `export class HubDO extends DurableObject`, `export default { fetch }`.

- [x] **Step 1:** `HubDO`: `sql` adapter over `ctx.storage.sql.exec(q, ...p).toArray()`; `migrate` in constructor via `blockConcurrencyWhile`; `/api/ws` validates token, `ctx.acceptWebSocket(server, [username])`, sends `hello`; `webSocketMessage/Close/Error` route to `Core` using `ctx.getTags(ws)[0]`; `Outbox.send` iterates `ctx.getWebSockets(username)`; alarm runs `sweep()` and reschedules `+5 min` (set on first request if none). After successful register, call `core.userJoined`.
- [x] **Step 2:** Worker `fetch` forwards to `env.HUB.get(env.HUB.idFromName("hub"))`.
- [x] **Step 3:** Verify: `npx wrangler dev --port 8787` with `.dev.vars` `ADMIN_TOKEN=dev`; `curl -XPOST -H 'Authorization: Bearer dev' localhost:8787/api/admin/invite` → `{"code":…}`; `npx tsc --noEmit` clean.
- [x] **Step 4:** Commit `feat(server): durable object hub and worker`.

> **Deviation:** Used `--local --persist-to` in verification; upgrade helper named `upgrade` (DurableObject base class already defines `connect`).

## Client (`client/`)

### Task 5: Go module + crypto envelope

**Files:**
- Create: `client/go.mod`, `client/internal/e2e/e2e.go`
- Test: `client/internal/e2e/e2e_test.go`

**Interfaces:**
- Produces: `type KeyPair struct{ Public, Private [32]byte }`; `GenerateKeyPair() (KeyPair, error)`; `type Sealed struct{ Nonce, CT string; Keys map[string]string }` (base64 std); `Seal(text string, sender KeyPair, recipients map[string][32]byte) (Sealed, error)`; `Open(nonce, ct, wrappedKey string, me KeyPair, senderPub [32]byte) (string, error)`; `ErrDecrypt`.

- [x] **Step 1:** Tests: round-trip for 3 recipients each opening their own key; `Open` with wrong sender pub → `ErrDecrypt`; tampered ct → `ErrDecrypt`; Vietnamese text `"Tiếng Việt có dấu ạ ( ˘ ³˘)♥"` round-trips byte-exact.
- [x] **Step 2:** `cd client && go test ./internal/e2e/` → FAIL.
- [x] **Step 3:** Implement per spec §3 (payload JSON `{"text":…}`).
- [x] **Step 4:** PASS.
- [x] **Step 5:** Commit `feat(client): e2e envelope`.

> **Deviation:** go.mod pinned to go 1.24 and golang.org/x/crypto v0.41.0 (latest x/crypto requires Go 1.26; use GOTOOLCHAIN=local).

### Task 6: Key store + config paths

**Files:**
- Create: `client/internal/store/store.go`
- Test: `client/internal/store/store_test.go`

**Interfaces:**
- Produces: `Dir() (string, error)` (honours `UMBRELLA_HOME` env for tests); `type KeyFile struct{ Username, Server, PublicKey, Salt, Nonce, Box string }`; `SaveKey(path, username, server, pin string, kp e2e.KeyPair) error`; `LoadKey(path, pin string) (KeyFile, e2e.KeyPair, error)`; `ErrWrongPIN`; `ExportKey(src, dst string) error`.

- [x] **Step 1:** Tests: save/load round trip; wrong PIN → `ErrWrongPIN`; file mode `0600` (skip on windows); export copies bytes exactly. Use Argon2 params overridable via package var for test speed but assert defaults equal the Global Constraints.
- [x] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS.
- [x] **Step 5:** Commit `feat(client): pin-encrypted key store`.

### Task 7: API client (HTTP + WebSocket) and protocol types

**Files:**
- Create: `client/internal/proto/proto.go`, `client/internal/api/http.go`, `client/internal/api/ws.go`
- Test: `client/internal/api/http_test.go` (httptest server)

**Interfaces:**
- Produces: `proto.User`, `proto.Message`, `proto.Event{T string; Me string; Users []User; Messages []Message; Message *Message; Ref, ID, Conv, Username, Error string; Kept bool; IDs []string; User *User; Until int64}`, outbound structs `Send`, `Keep`, `Typing`, `Name`; `type Client struct{ BaseURL string; HTTP *http.Client }`; `Register(ctx, RegisterReq) (token string, err error)`; `Login(ctx, user, pin) (string, error)`; `AdminInvite/AdminReset/AdminUnlock(ctx, adminToken, …)`; `type APIError struct{ Status int; Code string; Until int64 }`; `Dial(ctx, baseURL, token) (*Conn, error)` with `Conn.Events() <-chan proto.Event`, `Conn.Send(v any) error`, `Conn.Close()`.

- [ ] **Step 1:** Tests: Login 423 → `*APIError{Status:423, Code:"locked", Until:…}`; Register posts exact JSON field names from the wire table; ws URL derived `https→wss`, `http→ws`.
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(client): api client`.

### Task 8: Chat state model (pure)

**Files:**
- Create: `client/internal/chat/state.go`
- Test: `client/internal/chat/state_test.go`

**Interfaces:**
- Consumes: `proto`, `e2e`.
- Produces: `type Msg struct{ ID, Conv, Sender, ParentID, Text string; CreatedAt time.Time; Kept, Undecryptable bool; Replies int }`; `type State struct{…}`; `New(me string, kp e2e.KeyPair) *State`; `(*State) Apply(ev proto.Event) (Effects)` where `Effects{ Bell bool }`; `Timeline(conv) []Msg` (roots only, oldest first); `Thread(rootID) (root *Msg, replies []Msg)` (root nil-safe → placeholder `Msg{Text:"[message expired]"}`); `Sidebar() []Entry{Conv, Label, Username string; Online bool; Unread int}` (hell first, then online, then alpha); `SetActive(conv)` clears unread; `Recipients(conv) map[string][32]byte`; `Typing(conv, now) []string`; `Users() map[string]proto.User`; `Mentions(text, me) bool`.

- [ ] **Step 1:** Tests: `hello` replaces state (applying twice → no duplicates); `msg` in inactive conv increments unread; `@me` in hell → `Bell` and unread; `expired` removes messages and decrements root reply counts; reply whose root missing → `Thread` returns placeholder root; `kept` toggles; sidebar ordering (`hell`, online `bob`, online `zed`, offline `amy`); typing entry disappears after 4 s; `Recipients("hell")` = all users with keys; DM conv for me+bob is `dm:bob|me` sorted.
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(client): chat state model`.

### Task 9: Avatar, links, kaomoji, clipboard

**Files:**
- Create: `client/internal/avatar/avatar.go`, `client/internal/links/links.go`, `client/internal/kaomoji/kaomoji.go`, `client/internal/clip/clip.go`
- Test: `avatar_test.go`, `links_test.go`, `kaomoji_test.go` alongside

**Interfaces:**
- Produces: `avatar.Grid(username) (grid [5][5]bool, color string)` (hex `#rrggbb`); `avatar.Render(username) []string` (3 lines, each 10 cells wide); `avatar.Dot(username) string` (coloured `■`); `links.Find(text) []string`; `kaomoji.All []string` (exactly 50, unique); `clip.Copy(s string) error` (atotto, fallback OSC 52 to stdout).

- [ ] **Step 1:** Tests: grid deterministic and column-symmetric (`g[r][c]==g[r][4-c]`); `Render` returns 3 lines of display width 10; `links.Find("xem https://a.b/c?d=1, và http://x.y")` → both URLs without trailing comma; `len(kaomoji.All)==50` and unique.
- [ ] **Step 2:** FAIL. **Step 3:** implement (half-block rows: pair rows (0,1),(2,3),(4,—)). **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(client): avatar, links, kaomoji, clipboard`.

### Task 10: TUI

**Files:**
- Create: `client/internal/tui/model.go`, `client/internal/tui/view.go`, `client/internal/tui/keys.go`
- Test: `client/internal/tui/view_test.go`

**Interfaces:**
- Consumes: Tasks 7–9.
- Produces: `tui.Run(ctx, conn *api.Conn, st *chat.State, kp e2e.KeyPair, reconnect func(context.Context) (*api.Conn, error)) error`.

Behaviour (spec §4–5): layout sidebar(24) | chat | thread(40% when open); bubbles `textarea` (Enter sends, Alt+Enter newline, 4000-rune limit shown as counter); `/i` or click `+` → kaomoji picker overlay (arrows + Enter → insert at cursor + `clip.Copy`); `/whois @u`, clicking a user's avatar → profile overlay; `/name x` → `Name`; message line `■ Name  hh:mm  text  [📋][⚓][💬 N]` with zones via bubblezone; keybindings per spec; typing events throttled 2 s; on `recipients_mismatch` resend once after next `user` event; on connection loss → status bar `Reconnecting…` and `reconnect` with backoff 1,2,4…30 s; terminal bell via `\a` when `Effects.Bell`.

- [ ] **Step 1:** Tests on pure render helpers: `renderMessage` includes `📋` iff message has a link and `⚓` styled when kept; long Vietnamese line wraps at width without splitting runes (`runewidth.StringWidth` of each line ≤ width); thread view with nil root shows `[message expired]`.
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS; `go vet ./...` clean.
- [ ] **Step 5:** Commit `feat(client): terminal UI`.

### Task 11: CLI commands

**Files:**
- Create: `client/cmd/umbrella/main.go`

**Interfaces:**
- Consumes: everything above. `var DefaultServer = "http://localhost:8787"` (set via `-ldflags "-X main.DefaultServer=…"`).

Commands: `umbrella` (chat; PIN prompt hidden via `golang.org/x/term`), `umbrella register` (prompts invite, username, display name, PIN twice; generates key; saves key file; refuses to overwrite an existing key file without `--force`), `umbrella export-key <file>`, `umbrella admin invite|reset <user>|unlock <user>` (admin token from `UMBRELLA_ADMIN_TOKEN` or prompt). Global flag `--server`. Wrong local PIN → `Wrong PIN.` exit 1 without network; 423 → `Account locked until 15:04.`

- [ ] **Step 1:** `go build ./... && go vet ./...` clean; `./umbrella --help` lists the commands.
- [ ] **Step 2:** Commit `feat(client): cli commands`.

### Task 12: End-to-end test, build script, README

**Files:**
- Create: `client/e2e_test/e2e_test.go` (build tag `e2e`), `scripts/e2e.sh`, `scripts/build.sh`, `README.md`, `.gitignore`

- [ ] **Step 1:** `e2e_test.go` (env `UMBRELLA_E2E_SERVER`): admin invite ×2 → register alice & bob → both dial → alice sends to hell → bob decrypts the text; bob replies in thread; alice keeps; DM alice→bob not visible to a third user carol registered after.
- [ ] **Step 2:** `scripts/e2e.sh` starts `wrangler dev` with a temp persist dir, waits for port, runs `go test -tags e2e ./e2e_test/`, kills server. Run → PASS.
- [ ] **Step 3:** `scripts/build.sh <server-url>` → `dist/umbrella-{darwin,linux}-{amd64,arm64}`, `dist/umbrella-windows-amd64.exe`.
- [ ] **Step 4:** README: deploy (`wrangler login`, `wrangler secret put ADMIN_TOKEN`, `wrangler deploy`), build, onboarding (admin invite → register), key bindings, security model & its limits (trusted key distribution, no forward secrecy, 6-digit PIN key file is brute-forceable if stolen).
- [ ] **Step 5:** Commit `chore: e2e test, build script, readme`; push branch.
