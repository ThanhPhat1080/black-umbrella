# Umbrella

[**English**](README.md) · [Tiếng Việt](README.vi.md)

End-to-end encrypted terminal chat for a small team: a Hell room, direct messages, threads,
a keep (anchor) button and 24-hour message expiry. A Cloudflare Worker + Durable Object stores
ciphertext only; a cross-platform Go TUI client (`umbrella`) does all the cryptography.

<img width="769" height="527" alt="Screenshot 2026-11-12 at 10 07 35" src="https://github.com/user-attachments/assets/a21fb4f9-ee6c-4fd4-a000-8bad00c47de8" />



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

## Install and update

Download the binary for your machine from the [latest release](https://github.com/ThanhPhat1080/black-umbrella/releases/latest)
(`umbrella-darwin-{arm64,amd64}`, `umbrella-linux-{amd64,arm64}`, `umbrella-windows-amd64.exe`).
Your key lives in `~/.umbrella/` (`%APPDATA%\umbrella\` on Windows), so updating only replaces the
program and never requires registering again.

From v1.2.0 on, just type `/update` in the chat (or run `umbrella update`). It downloads the latest release,
checks its SHA-256, and replaces the program in place; reopen Umbrella afterwards. If it can't write to the install
folder (e.g. `/usr/local/bin`), run `sudo umbrella update`. Versions before v1.2.0 have no updater, so download
once manually as below.

macOS (use `amd64` on Intel Macs; the same commands install it the first time):

```sh
sudo mkdir -p /usr/local/bin
sudo mv ~/Downloads/umbrella-darwin-arm64 /usr/local/bin/umbrella
sudo chmod +x /usr/local/bin/umbrella
sudo xattr -d com.apple.quarantine /usr/local/bin/umbrella   # binary is not notarised; ignore "No such xattr"
```

Linux (use `arm64` on ARM machines; the same command installs it the first time):

```sh
sudo install -m 755 ~/Downloads/umbrella-linux-amd64 /usr/local/bin/umbrella
```

Windows (PowerShell):

```powershell
# first install only
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\umbrella" | Out-Null
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";$env:LOCALAPPDATA\umbrella", "User")
# install or update
Move-Item -Force "$HOME\Downloads\umbrella-windows-amd64.exe" "$env:LOCALAPPDATA\umbrella\umbrella.exe"
```

On first install reopen PowerShell. If SmartScreen warns, choose **More info → Run anyway**. If Windows says
the file is in use, close every running `umbrella` window and retry. Windows Terminal is recommended.

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
| Tab | with a thread open: switch between the main textbox and the thread reply textbox; otherwise input ⇄ message list |
| Ctrl+N / Ctrl+P | next / previous conversation |
| Up / Down | select message (when the list is focused) |
| o | open the selected message's first link in the browser (clicking any link also opens it) |
| y | copy the selected message's first link |
| c | copy the selected message's full text |
| Mouse drag | select text inside a pane; it is copied to the clipboard on release |
| /h | show all shortcuts |
| Ctrl+X | toggle select mode: mouse capture off so you can drag-select text and copy it with your terminal |
| a | toggle keep (anchor) on your own message |
| r or Right | open the selected message's thread (Right also focuses the thread reply textbox) |
| Left or Esc | with a thread open: close it and return to the message list with the same message selected |
| Esc | message list without a thread / picker: return to the input |
| Ctrl+C | quit (asks to confirm: press Ctrl+C again, or y/Enter) |
| /quit | quit immediately |

Textbox editing: Ctrl+←/→ or Option+←/→ jump a word, Home/End (or Ctrl+E for the end; Ctrl+A for the start on macOS) go to the line start/end,
Ctrl+W deletes a word, Ctrl+U deletes to the line start, **Ctrl+L** (or Alt+A; also **Ctrl+A on Windows and Linux**) selects all text and copies it
(type to replace, Backspace to cut). Terminals never deliver the Cmd key, so on a Mac Cmd+A/Cmd+← are handled by the
terminal itself: in iTerm2 map them under Settings → Profiles → Keys → Key Mappings with the action
"Send Hex Code": Cmd+A → `0x0c`, Cmd+← → `0x01`, Cmd+→ → `0x05`, Cmd+Backspace → `0x15`.

Text selection: while the app has the mouse, most terminals still let you bypass it by holding Shift (Windows Terminal, GNOME Terminal, Konsole) or Option ⌥ (iTerm2) while dragging; or use Ctrl+X select mode above.

Commands: `/i` kaomoji picker (also the `[+]` button), `/whois @user`, `/name <display name>`, `/buzz` shake everyone's screen, `/dance` a surprise animal dances for everyone, `/room <name> [@user …]` create a private room, then `/invite @user`, `/members`, `/leave` (owner: `/roomname`, `/kick @user`, `/closeroom`), `/cow [text]` send an ASCII cow, `/update` install the latest version, `/e` hide/show the sidebar pet, `/quit`.
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
