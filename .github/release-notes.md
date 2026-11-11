**Umbrella** ☂: an end-to-end encrypted terminal chat. See [CHANGELOG.md](https://github.com/ThanhPhat1080/black-umbrella/blob/main/CHANGELOG.md) for what changed. Server: `https://umbrella.black-umbrella.workers.dev` (built into the binaries).

**Hướng dẫn tiếng Việt:** [README.vi.md](https://github.com/ThanhPhat1080/black-umbrella/blob/feat/phase1/README.vi.md)

| Máy / Platform | File |
|---|---|
| macOS Apple Silicon | `umbrella-darwin-arm64` |
| macOS Intel | `umbrella-darwin-amd64` |
| Linux x86-64 | `umbrella-linux-amd64` |
| Linux ARM64 | `umbrella-linux-arm64` |
| Windows | `umbrella-windows-amd64.exe` |
| Linux desktop app (window, x86-64) | `umbrella-desktop-<version>-amd64.deb` or `umbrella-desktop-<version>-x86_64.AppImage` |

**From v1.2.0:** just type `/update` in the chat (or `umbrella update`).

**Quick update** (your key stays in `~/.umbrella/`, no need to register again; quit Umbrella first):
- **macOS:** `sudo mv ~/Downloads/umbrella-darwin-arm64 /usr/local/bin/umbrella && sudo chmod +x /usr/local/bin/umbrella && sudo xattr -d com.apple.quarantine /usr/local/bin/umbrella` (use `amd64` on Intel)
- **Linux:** `sudo install -m 755 ~/Downloads/umbrella-linux-amd64 /usr/local/bin/umbrella`
- **Windows (PowerShell):** `New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\umbrella" | Out-Null; Move-Item -Force "$HOME\Downloads\umbrella-windows-amd64.exe" "$env:LOCALAPPDATA\umbrella\umbrella.exe"`

Get an invite code from your admin, then run `umbrella register`, then `umbrella`.

The binaries are unsigned:
- **macOS:** `sudo mkdir -p /usr/local/bin && sudo mv umbrella-darwin-arm64 /usr/local/bin/umbrella && sudo chmod +x /usr/local/bin/umbrella && sudo xattr -d com.apple.quarantine /usr/local/bin/umbrella`
- **Windows:** choose SmartScreen → More info → Run anyway.

**Desktop app (Linux):** `sudo apt install ./umbrella-desktop-<version>-amd64.deb` (recommended), or `chmod +x` the AppImage and run it (needs `libfuse2`). The same chat in a window: normal Ctrl+C / Ctrl+V, notifications, no terminal needed. See the README.

Verify downloads with `SHA256SUMS.txt`.
