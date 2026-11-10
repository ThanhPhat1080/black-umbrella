# Umbrella

[**English**](README.md) · [Tiếng Việt](README.vi.md)

End-to-end encrypted terminal chat for a small team: a Hell room, direct messages, threads,
a keep (anchor) button and 24-hour message expiry. A Cloudflare Worker + Durable Object stores
ciphertext only; a cross-platform Go TUI client (`umbrella`) does all the cryptography.

```
client/   Go TUI (Bubble Tea), NaCl box/secretbox, Argon2id key file
server/   Cloudflare Worker + Durable Object (SQLite), tested in Node via node:sqlite
scripts/  build.sh, e2e.sh
docs/     design spec and implementation plan
```

## Deploy the server

Requires Node 22 and a Cloudflare account (the free plan is enough).

```sh
cd server
npm ci
npx wrangler login
npx wrangler secret put ADMIN_TOKEN     # choose a long random value
npx wrangler deploy
```

Local development: `cp .dev.vars.example .dev.vars && npx wrangler dev --port 8787`
(the default admin token in `.dev.vars.example` is `dev`).

## Build the client

Needs Go 1.24+.

```sh
scripts/build.sh https://umbrella.black-umbrella.workers.dev
# -> dist/umbrella-{darwin,linux}-{amd64,arm64}, dist/umbrella-windows-amd64.exe
```

The server URL is baked in with `-ldflags -X main.DefaultServer=…` and can be overridden with
`--server URL`.

## Onboarding

1. Admin creates an invite: `UMBRELLA_ADMIN_TOKEN=… umbrella admin invite` (prints a single-use code).
2. The new member runs `umbrella register`, entering the invite code, a username
   (`a-z 0-9 _`, 3-20), a display name (any Unicode, up to 32 characters) and a 6-digit PIN.
   A key pair is generated on their machine and saved to `~/.umbrella/key.json`
   (`%APPDATA%\umbrella\key.json` on Windows, mode 0600, encrypted with the PIN).
3. They run `umbrella`, enter their PIN and chat. Back the key up with `umbrella export-key <file>`.

Admin commands: `umbrella admin invite`, `admin reset <user>` (the user re-registers with a new
key using the printed code; old messages become unreadable for them), `admin unlock <user>`.
Five wrong PINs lock the account for 15 minutes, doubling on each further lock.

Note: the WebSocket session token travels in the `?token=` query string (simple for Workers and
the Go client). It is only sent over HTTPS in production, but avoid logging full request URLs
on any proxy you put in front of the Worker.

## Key bindings

| Key | Action |
|---|---|
| Enter / Alt+Enter | send / newline |
| Tab | cycle focus: input, messages, thread |
| Ctrl+N / Ctrl+P | next / previous conversation |
| Up / Down | select message (when the list is focused) |
| y | copy the selected message's first link |
| Ctrl+T | with a thread open: switch the input between replying in the thread and the main chat |
| a | toggle keep (anchor) on your own message |
| r or Right | open the selected message's thread (Right also moves focus to the thread panel) |
| Left or Esc | with a thread open: close it and return to the message list with the same message selected |
| Esc | message list without a thread / picker: return to the input |
| Ctrl+C or /quit | quit |

Commands: `/i` kaomoji picker (also the `[+]` button), `/whois @user`, `/name <display name>`, `/quit`.
Mouse: click conversations, avatars, `[+]`, and the `[📋]` `[⚓]` `[💬]` buttons on messages.

## Tests

```sh
cd server && npx vitest run && npx tsc --noEmit
cd client && go test ./...
scripts/e2e.sh        # starts wrangler dev locally, runs the end-to-end Go test
```

## Security model and its limits

- The server stores and relays ciphertext, usernames, display names, public keys and timestamps.
  It never sees message text. Each message uses a random content key, wrapped per recipient with
  NaCl `box` (X25519), authenticated to the sender's public key.
- **Trusted key distribution:** public keys come from the server and are not verified out of band.
  A malicious or compromised server could substitute keys for new messages.
- **No forward secrecy:** keys are long-lived; compromise of a private key exposes all messages
  addressed to it that someone retained. New members cannot read earlier messages.
- **Weak key-file protection:** the private key is encrypted with Argon2id from a 6-digit PIN
  (only 10^6 possibilities). If the key file is stolen it can be brute-forced offline; guard it
  like a password file. The PIN also protects the login (PBKDF2, rate-limited server-side).
- Expiry and "keep" are enforced by the server; a recipient can always retain what they received.
- Metadata (who talks to whom, when, message sizes) is visible to the server operator.
