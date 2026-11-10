First release of **Umbrella** ☂: an end-to-end encrypted terminal chat. Server: `https://umbrella.black-umbrella.workers.dev` (built into the binaries).

**Hướng dẫn tiếng Việt:** [README.vi.md](https://github.com/ThanhPhat1080/black-umbrella/blob/feat/phase1/README.vi.md)

| Máy / Platform | File |
|---|---|
| macOS Apple Silicon | `umbrella-darwin-arm64` |
| macOS Intel | `umbrella-darwin-amd64` |
| Linux x86-64 | `umbrella-linux-amd64` |
| Linux ARM64 | `umbrella-linux-arm64` |
| Windows | `umbrella-windows-amd64.exe` |

Get an invite code from your admin, then run `umbrella register`, then `umbrella`.

The binaries are unsigned:
- **macOS:** run `xattr -d com.apple.quarantine <file>`.
- **Windows:** choose SmartScreen → More info → Run anyway.

Verify downloads with `SHA256SUMS.txt`.

Built from [ThanhPhat1080/black-umbrella#1](https://github.com/ThanhPhat1080/black-umbrella/pull/1) (branch `feat/phase1`).
