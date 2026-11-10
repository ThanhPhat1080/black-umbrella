# Chạy local và debug Umbrella

[← Về README tiếng Việt](README.vi.md)

Có 2 cách chạy Umbrella từ source code:

| Cách | Dùng khi | Server |
|---|---|---|
| [Cách 1: chỉ chạy client](#cách-1--chỉ-chạy-client-từ-source-kết-nối-server-thật) | sửa giao diện / client, muốn chat với mọi người như thường | server thật trên Cloudflare |
| [Cách 2: chạy cả server lẫn client](#cách-2--chạy-cả-server-lẫn-client-trên-máy) | sửa server, thử tính năng mới, test không ảnh hưởng ai | `wrangler dev` trên máy bạn |

## Chuẩn bị

- **Go 1.24 trở lên** và **Node 22**. Trên Mac:

  ```bash
  brew install go node@22
  ```

- Clone repo. Code nằm ở branch `feat/phase1` (hiện là nhánh mặc định):

  ```bash
  git clone https://github.com/ThanhPhat1080/black-umbrella.git
  ```
  ```bash
  cd black-umbrella
  ```

---

## Cách 1 — Chỉ chạy client từ source, kết nối server thật

Dùng chung tài khoản và file key bạn đang có (`~/.umbrella/key.json`). Không cần Node, không cần Cloudflare.

```bash
cd client && go run ./cmd/umbrella
```

Client mặc định kết nối `https://umbrella.black-umbrella.workers.dev`.

> ⚠️ Bạn đang chat **thật** với mọi người. Muốn thử nghiệm thoải mái, dùng Cách 2.

---

## Cách 2 — Chạy cả server lẫn client trên máy

### Terminal 1: chạy server local

```bash
cd server
```
```bash
npm ci
```
```bash
cp .dev.vars.example .dev.vars
```
```bash
npx wrangler dev --port 8787
```

- Server chạy ở `http://localhost:8787`.
- `ADMIN_TOKEN` khi chạy local là `dev` (đặt trong `server/.dev.vars`, file này không được commit).
- **Không cần đăng nhập Cloudflare.** Dữ liệu lưu ở `server/.wrangler/state`.
- Sửa code trong `server/src/` thì wrangler tự reload.

### Terminal 2: tạo tài khoản test và chat

Chạy trong thư mục `client/`.

> ⚠️ **Luôn đặt `UMBRELLA_HOME` sang thư mục riêng**, nếu không app sẽ dùng chung `~/.umbrella/key.json` với tài khoản thật của bạn.

```bash
cd client
```
```bash
export UMBRELLA_HOME=~/.umbrella-dev
```

Tạo mã mời:

```bash
UMBRELLA_ADMIN_TOKEN=dev go run ./cmd/umbrella admin invite --server http://localhost:8787
```

Đăng ký (nhập mã mời vừa in ra, username, display name, PIN):

```bash
go run ./cmd/umbrella register --server http://localhost:8787
```

Mở chat:

```bash
go run ./cmd/umbrella --server http://localhost:8787
```

### Thử chat 2 người

Mở **terminal 3**, dùng một thư mục `UMBRELLA_HOME` khác rồi lặp lại 3 lệnh ở trên với một mã mời mới:

```bash
export UMBRELLA_HOME=~/.umbrella-dev2
```

### Các lệnh admin khi chạy local

```bash
UMBRELLA_ADMIN_TOKEN=dev go run ./cmd/umbrella admin unlock <username> --server http://localhost:8787
```
```bash
UMBRELLA_ADMIN_TOKEN=dev go run ./cmd/umbrella admin reset <username> --server http://localhost:8787
```

---

## Chạy test

Server (unit test, chạy bằng Node, không cần wrangler):

```bash
cd server && npx vitest run
```

Kiểm tra kiểu TypeScript:

```bash
cd server && npx tsc --noEmit
```

Client:

```bash
cd client && go test ./...
```

End-to-end (tự bật server local trên cổng 8791, chạy luồng đăng ký → chat → thread → keep → reset, rồi tự tắt):

```bash
scripts/e2e.sh
```

---

## Build file chạy

Build giống bản release, trỏ vào server local:

```bash
scripts/build.sh http://localhost:8787
```

Kết quả nằm trong `dist/`. Build cho server thật thì thay bằng `https://umbrella.black-umbrella.workers.dev`.

---

## Mẹo debug

- **Xóa sạch dữ liệu local:** tắt server (`Ctrl+C` ở terminal 1) rồi xóa thư mục:

  ```bash
  rm -rf server/.wrangler/state
  ```

  Nhớ xóa luôn các thư mục `~/.umbrella-dev*` vì tài khoản cũ không còn trên server.

- **`go` đòi tải phiên bản Go khác:** thêm `GOTOOLCHAIN=local` trước lệnh, ví dụ `GOTOOLCHAIN=local go test ./...`.

- **Xem log server:** mọi request và lỗi hiện ở terminal 1 (`wrangler dev`). Thêm `console.log(...)` trong `server/src/` để in thêm.

- **Xem log server thật trên Cloudflare** (cần `npx wrangler login`):

  ```bash
  cd server && npx wrangler tail
  ```

- **Client không in log ra màn hình** vì giao diện chiếm toàn bộ terminal. Muốn debug, ghi ra file rồi xem ở terminal khác:

  ```bash
  tail -f /tmp/umbrella-debug.log
  ```

  Trong code Go, ghi log bằng `tea.LogToFile("/tmp/umbrella-debug.log", "debug")` ở đầu `main`, rồi dùng `log.Printf(...)`.

- **Giao diện bị vỡ / lệch:** thử terminal khác (iTerm2, Windows Terminal), hoặc chạy trong `tmux` cỡ cố định để so sánh:

  ```bash
  tmux new -s dev -x 120 -y 35
  ```

- **Đang chạy server local mà client vẫn vào server thật:** kiểm tra lại bạn có quên `--server http://localhost:8787` không. Khi không có cờ này, client dùng server lưu trong file key, rồi mới đến server mặc định.
