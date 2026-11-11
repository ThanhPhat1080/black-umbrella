# Umbrella ☂

[English](README.md) · **Tiếng Việt**

Umbrella là app chat nội bộ chạy trong terminal, **mã hóa đầu-cuối (E2E)**: server chỉ lưu dữ liệu đã mã hóa, không ai — kể cả admin — đọc được nội dung tin nhắn.

- Phòng chung **Hell room** + nhắn tin riêng **1-1**
- **Thread** trả lời giống Slack
- Tin nhắn **tự xóa sau 24 giờ**, bấm **⚓ keep** để giữ lại
- `@mention` để ping, chỉ báo "đang gõ…", trạng thái online
- **50 kaomoji** dễ thương có sẵn `( ˘ ³˘)♥`
- Avatar hình học tự sinh theo username
- Chạy trên **macOS, Linux, Windows (PowerShell / Windows Terminal)**

Server: `https://umbrella.black-umbrella.workers.dev`

---

## Mục lục

- [Phần 1 — Dành cho người dùng](#phần-1--dành-cho-người-dùng)
  - [Cài đặt](#1-cài-đặt)
  - [Đăng ký tài khoản](#2-đăng-ký-tài-khoản)
  - [Đăng nhập và chat](#3-đăng-nhập-và-chat)
  - [Giao diện](#4-giao-diện)
  - [Phím tắt và lệnh](#5-phím-tắt-và-lệnh)
  - [Các tính năng](#6-các-tính-năng)
  - [Sao lưu key, quên PIN, đổi máy](#7-sao-lưu-key-quên-pin-đổi-máy)
  - [Câu hỏi thường gặp](#8-câu-hỏi-thường-gặp)
- [Phần 2 — Dành cho Admin](#phần-2--dành-cho-admin)
- [Bảo mật: làm được gì và không làm được gì](#bảo-mật-làm-được-gì-và-không-làm-được-gì)

---

# Phần 1 — Dành cho người dùng

## 1. Cài đặt

Tải file phù hợp với máy bạn ở trang **[Releases](https://github.com/ThanhPhat1080/black-umbrella/releases)**:

| Máy | File |
|---|---|
| Mac chip Apple (M1/M2/M3/M4) | `umbrella-darwin-arm64` |
| Mac chip Intel | `umbrella-darwin-amd64` |
| Linux (đa số máy) | `umbrella-linux-amd64` |
| Linux ARM (Raspberry Pi…) | `umbrella-linux-arm64` |
| Windows | `umbrella-windows-amd64.exe` |

### macOS

Mac Intel thì thay `arm64` bằng `amd64`. `sudo` sẽ hỏi mật khẩu máy Mac (gõ không hiện ký tự, cứ gõ rồi Enter).

```bash
sudo mkdir -p /usr/local/bin
```
```bash
sudo mv ~/Downloads/umbrella-darwin-arm64 /usr/local/bin/umbrella
```
```bash
sudo chmod +x /usr/local/bin/umbrella
```
```bash
sudo xattr -d com.apple.quarantine /usr/local/bin/umbrella
```

> - Cần `sudo` vì `/usr/local/bin` thuộc quyền hệ thống (thiếu `sudo` sẽ gặp lỗi `Permission denied`). Trên Mac M1 trở lên thư mục này có thể chưa tồn tại, nên có lệnh `mkdir -p`.
> - Lệnh `xattr` cần thiết vì file chưa được Apple ký; không có nó macOS sẽ báo "cannot be opened because the developer cannot be verified". Nếu nó báo `No such xattr` thì bỏ qua.

### Linux

```bash
sudo install -m 755 ~/Downloads/umbrella-linux-amd64 /usr/local/bin/umbrella
```

### Linux: bản app có cửa sổ (không cần terminal)

Ngoài file `umbrella-linux-amd64` chạy trong terminal, có thêm **app desktop** (Electron): cùng một client, mở bằng icon trong menu ứng dụng. Mọi lệnh và phím giống hệt bản terminal; khác ở chỗ thao tác như web app:

- Mở app lần đầu, nếu chưa có key thì cửa sổ tự chạy bước đăng ký (mã mời, username, tên hiển thị, PIN). Sau đó mỗi lần mở chỉ cần nhập PIN. Key vẫn ở `~/.umbrella/`, dùng chung với bản terminal.
- **Ctrl+C** copy phần đang chọn, **Ctrl+X** cắt, **Ctrl+V** dán, **Ctrl+A** tô đen hết ô nhập. Kéo chuột để chọn chữ trong khung chat. Ctrl+C không thoát app (gõ `/quit` hoặc đóng cửa sổ). Không có menu chuột phải.
- Có thông báo desktop khi bạn nhận tin nhắn riêng, bị @nhắc hoặc bị buzz lúc cửa sổ đang ở nền; số tin chưa đọc hiện trên tiêu đề cửa sổ; `/buzz` làm rung cả cửa sổ. **Ctrl+=**, **Ctrl+-**, **Ctrl+0** đổi cỡ chữ.
- App tự kiểm tra bản mới khi mở và báo bằng thông báo (không có `/update` trong app).

Cài từ trang Releases:

```bash
sudo apt install ./umbrella-desktop-<phiên-bản>-amd64.deb
```

Hoặc dùng AppImage (Ubuntu 24.04 trở lên cần cài thêm thư viện FUSE 2 một lần):

```bash
sudo apt install libfuse2t64
```
```bash
chmod +x umbrella-desktop-<phiên-bản>-x86_64.AppImage && ./umbrella-desktop-<phiên-bản>-x86_64.AppImage
```

Cập nhật: tải bản mới và cài đè như lần đầu.

### Windows (PowerShell)

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\umbrella" | Out-Null
Move-Item "$HOME\Downloads\umbrella-windows-amd64.exe" "$env:LOCALAPPDATA\umbrella\umbrella.exe"
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";$env:LOCALAPPDATA\umbrella", "User")
```

Đóng PowerShell và mở lại để lệnh `umbrella` có hiệu lực. Nếu Windows SmartScreen cảnh báo, chọn **More info → Run anyway**.

> Nên dùng **Windows Terminal** (có sẵn trên Windows 11) để hiển thị màu, emoji và click chuột tốt nhất. Git Bash cũng chạy được.

Kiểm tra:

```bash
umbrella --help
```

### Cập nhật lên bản mới

Key và dữ liệu đăng nhập nằm riêng trong `~/.umbrella/` (Windows: `%APPDATA%\umbrella\`), nên cập nhật chỉ là thay file chương trình, **không cần đăng ký lại**. Thoát Umbrella (`Ctrl+C`) trước khi cập nhật. Tải bản mới nhất ở trang **[Releases](https://github.com/ThanhPhat1080/black-umbrella/releases/latest)**.

**macOS** (Mac Intel thì thay `arm64` bằng `amd64`):

```bash
sudo mv ~/Downloads/umbrella-darwin-arm64 /usr/local/bin/umbrella
```
```bash
sudo chmod +x /usr/local/bin/umbrella
```
```bash
sudo xattr -d com.apple.quarantine /usr/local/bin/umbrella
```

**Linux** (máy ARM thì thay `amd64` bằng `arm64`):

```bash
sudo install -m 755 ~/Downloads/umbrella-linux-amd64 /usr/local/bin/umbrella
```

**Windows (PowerShell):**

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\umbrella" | Out-Null
Move-Item -Force "$HOME\Downloads\umbrella-windows-amd64.exe" "$env:LOCALAPPDATA\umbrella\umbrella.exe"
```

Nếu Windows báo file đang được sử dụng, hãy đóng mọi cửa sổ đang chạy `umbrella` rồi chạy lại lệnh. Không cần chạy lại bước thêm `Path`.


## 2. Đăng ký tài khoản

Bạn cần một **mã mời (invite code)** do Admin gửi. Mỗi mã chỉ dùng được 1 lần.

```bash
umbrella register
```

App sẽ hỏi lần lượt:

| Câu hỏi | Quy tắc |
|---|---|
| `Invite code` | mã Admin gửi bạn |
| `Username` | chữ thường `a-z`, số `0-9`, dấu `_`; 3–20 ký tự; **không đổi được** |
| `Display name` | tên hiển thị, tối đa 32 ký tự, gõ tiếng Việt có dấu và emoji thoải mái; đổi được sau |
| `PIN` | đúng **6 chữ số**, nhập 2 lần (không hiện trên màn hình) |

Sau khi đăng ký, app tạo **cặp khóa mã hóa ngay trên máy bạn** và lưu ở:

- macOS / Linux: `~/.umbrella/key.json`
- Windows: `%APPDATA%\umbrella\key.json`

File này được mã hóa bằng PIN. **Khóa bí mật không bao giờ rời khỏi máy bạn.**

## 3. Đăng nhập và chat

```bash
umbrella
```

Nhập PIN 6 số là vào. Username được nhớ sẵn trong file key.

- Nhập **sai PIN**: app báo `Wrong PIN.` ngay trên máy.
- Server khóa tài khoản **15 phút** sau 5 lần đăng nhập sai liên tiếp (lần khóa sau gấp đôi: 30, 60 phút…). Liên hệ Admin để mở khóa sớm.

Thoát: gõ `/quit` hoặc bấm `Ctrl+C`.

## 4. Giao diện

```
╭──────────────────────╮╭────────────────────────────────────╮╭──────────────────────╮
│# Hell room        3  ││# Hell room                         ││Thread  (Esc to close)│
│──────────────────────││ ■ Alice Nguyễn  09:15              ││ ■ Alice  09:15       │
│▙▄▟ ● Alice        2  ││   Chào @bob, xem link này nha      ││   Chào @bob…         │
│▜▀▛ ○ Carol           ││   https://example.com [📋][💬 2]   ││ ──────────────────── │
│ ...                  ││ ■ Bob 🌂  09:16                    ││ ■ Bob  09:17         │
│                      ││   ok (＾▽＾)                       ││   đã xem ✅          │
│                      ││╭──────────────────────────────────╮││                      │
│                      │││ Message…                         │││                      │
│                      ││╰──────────────────────────────────╯││                      │
│                      ││[+] kaomoji  0/4000                 ││                      │
╰──────────────────────╯╰────────────────────────────────────╯╰──────────────────────╯
 Umbrella @bob · Enter send · Alt+Enter newline · …
```

- **Sidebar trái**: `# Hell room` luôn ở trên cùng, rồi một đường kẻ phân cách; bên dưới mỗi người **một dòng**: avatar mini 3 ô (cùng màu với tên và identicon), chấm online (`●`/`○`), tên (màu riêng của từng người) và số tin chưa đọc bên phải. Ai online lên trước, rồi theo bảng chữ cái. Hàng đang mở được tô nền; click avatar mini để xem profile (identicon đầy đủ), click phần còn lại để mở tin nhắn riêng. Tin đang chọn trong khung chat cũng được tô nền.
- **Khung giữa**: tin nhắn của cuộc trò chuyện đang mở, ô nhập ở dưới.
- **Panel phải**: thread, chỉ hiện khi bạn mở một thread.
- **Thanh dưới cùng**: trạng thái và gợi ý phím tắt.

## 5. Phím tắt và lệnh

### Phím tắt

| Phím | Tác dụng |
|---|---|
| `Enter` | gửi tin |
| `Alt+Enter` | xuống dòng trong tin nhắn |
| `Tab` | chuyển giữa ô chat chính và ô trả lời thread (khi thread đang mở); không có thread thì chuyển ô nhập ⇄ danh sách tin |
| `Ctrl+N` / `Ctrl+P` | sang cuộc trò chuyện kế tiếp / trước đó |
| `↑` (khi ô nhập trống) | chọn tin nhắn mới nhất, rồi `↑` / `↓` để di chuyển |
| `o` | mở link trong tin đang chọn bằng trình duyệt (click thẳng vào link cũng mở) |
| `y` | copy link trong tin đang chọn |
| `c` | copy nội dung tin đang chọn |
| Kéo chuột | bôi đen text trong một khung, thả chuột là tự copy vào clipboard |
| `/h` | hiện toàn bộ phím tắt |
| `Ctrl+X` | bật/tắt chế độ bôi đen để copy (tắt chuột của app, dùng bôi đen của terminal) |
| `a` | bật/tắt ⚓ keep (chỉ với tin của bạn) |
| `r` | mở thread của tin đang chọn |
| `→` | mở thread của tin đang chọn và chuyển focus sang panel thread |
| `←` hoặc `Esc` | đóng thread, quay về danh sách tin với tin đó vẫn đang được chọn |
| `Esc` | (khi không mở thread) đóng bảng kaomoji / profile, quay về ô nhập |
| `Ctrl+C` | thoát (hỏi xác nhận: bấm `Ctrl+C` lần nữa, hoặc `y`/`Enter`) |

Soạn tin trong ô nhập: `Ctrl+←/→` hoặc `Option+←/→` nhảy theo từ, `Home`/`End` (hoặc `Ctrl+E` về cuối dòng; trên macOS `Ctrl+A` về đầu dòng) về đầu/cuối dòng, `Ctrl+W` xoá một từ, `Ctrl+U` xoá đến đầu dòng, **`Ctrl+L`** (hoặc `Alt+A`; trên **Windows và Linux dùng được cả `Ctrl+A`**) chọn tất cả và copy luôn (gõ chữ để thay thế, `Backspace` để cắt). Trên Windows/Linux không cần cấu hình gì thêm. Trên Mac, terminal không bao giờ gửi phím Cmd cho ứng dụng, nên `Cmd+A`, `Cmd+←` do terminal xử lý. Trên iTerm2, vào Settings → Profiles → Keys → Key Mappings, chọn "Send Hex Code" rồi gán: `Cmd+A` → `0x0c`, `Cmd+←` → `0x01`, `Cmd+→` → `0x05`, `Cmd+Backspace` → `0x15`.

Mẹo bôi đen: hầu hết terminal cho phép bỏ qua chuột của app bằng cách giữ `Shift` (Windows Terminal, GNOME Terminal, Konsole) hoặc `Option ⌥` (iTerm2) khi kéo chuột; hoặc dùng `Ctrl+X`.

Khi đang ở danh sách tin, cứ gõ chữ bình thường — app tự đưa bạn về ô nhập.

### Lệnh gõ trong ô chat

| Lệnh | Tác dụng |
|---|---|
| `/i` | mở bảng chọn kaomoji |
| `/whois @username` | xem profile của một người |
| `/dance` | một con vật bất ngờ nhảy múa cho mọi người xem |
| `/room <tên> [@user …]` | tạo room riêng, thêm luôn những người bạn liệt kê |
| `/invite @user` | thêm người vào room đang mở (thành viên nào cũng thêm được) |
| `/members` | xem ai đang trong room |
| `/leave` | rời room đang mở |
| `/roomname <tên>`, `/kick @user`, `/closeroom` | chỉ chủ room: đổi tên, đuổi người, xoá room cho mọi người |
| `/cow [chữ]` | gửi hình con bò ASCII (nói câu bạn gõ) |
| `/e` | ẩn / hiện con mèo ở sidebar |
| `/name Tên mới` | đổi display name của bạn |
| `/quit` | thoát |

### Chuột

Click được vào: tên cuộc trò chuyện ở sidebar, avatar (mở profile), nút `[+]`, và các nút `[📋]` `[⚓]` `[💬]` trên từng tin nhắn.

> Một số môi trường (tmux, SSH, terminal cũ) không hỗ trợ click — mọi thao tác đều có phím tắt tương ứng.

## 6. Các tính năng

### Hell room và chat 1-1
- **Hell room**: phòng chung của mọi người.
- **Chat 1-1**: chọn tên một người ở sidebar (click hoặc `Ctrl+N`/`Ctrl+P`). Chỉ hai người đọc được.

### Ping
Gõ `@username` trong Hell room — người đó thấy tên được tô sáng, terminal kêu "bíp" và sidebar hiện số tin chưa đọc. Người đang offline sẽ thấy khi mở app lần sau.

### Thread
Chọn một tin (`↑`) rồi bấm `r`, hoặc click `[💬]`. Panel thread mở bên phải và có **ô trả lời riêng** ở dưới cùng (viền sáng khi đang focus); ô chat chính luôn gửi vào cuộc trò chuyện chính. `Tab` chuyển qua lại giữa hai ô, nháp ở mỗi ô được giữ nguyên; `→` hoặc click `[💬]` mở thread và focus thẳng vào ô trả lời; `←` (khi ô trả lời trống) hoặc `Esc` đóng thread. Tin gốc hiện số trả lời `💬 N`.

### Tin nhắn tự xóa sau 24 giờ và ⚓ keep
- Mọi tin bị **xóa khỏi server 24 giờ** sau khi gửi.
- Muốn giữ tin của bạn: chọn tin rồi bấm `a`, hoặc click `[⚓]`. Tin có ⚓ **không bao giờ bị xóa**.
- Chỉ người gửi mới keep được tin của mình. Bỏ keep thì tin quay lại luật 24 giờ (nếu đã quá 24 giờ sẽ bị xóa ngay).
- Mỗi người giữ tối đa 200 tin.
- Nếu tin gốc của thread đã hết hạn nhưng có reply được keep, tin gốc hiện `[message expired]`.

### Link
App không tạo preview cho link. Tin có link sẽ có nút `[📋]` — click hoặc chọn tin rồi bấm `y` để copy link.

### Kaomoji
Click `[+]` dưới ô chat hoặc gõ `/i` → dùng phím mũi tên chọn → `Enter`. Kaomoji được **chèn thẳng vào ô chat** và **copy vào clipboard** luôn.

### Profile
`/whois @username` hoặc click avatar để xem: avatar, display name, @username, ngày tham gia, online hay "last seen" bao lâu trước.

### Khác
- Tin nhắn tối đa **4000 ký tự**; tiếng Việt có dấu, emoji thoải mái.
- **Không gửi được hình ảnh/file** (chủ ý).
- Không sửa/xóa được tin đã gửi.
- Mất mạng: app tự kết nối lại, tin không bị trùng.

## 7. Sao lưu key, quên PIN, đổi máy

**Hãy sao lưu file key ngay sau khi đăng ký:**

```bash
umbrella export-key ~/umbrella-key-backup.json
```

Cất file backup ở nơi an toàn (USB, password manager). File được mã hóa bằng PIN, nhưng PIN chỉ có 6 số nên **đừng để lộ file này**.

**Đổi máy, còn file backup:** chép file backup vào `~/.umbrella/key.json` (Windows: `%APPDATA%\umbrella\key.json`) trên máy mới rồi chạy `umbrella`. Mọi thứ như cũ.

**Quên PIN hoặc mất máy, không có backup:**
1. Báo Admin để reset tài khoản. Admin gửi bạn một mã mời mới dành riêng cho username của bạn.
2. Chạy `umbrella register --force`, nhập mã mới, **cùng username cũ**, PIN mới.
3. Bạn có khóa mới. Tin nhắn cũ gửi cho bạn (kể cả tin ⚓) **không đọc lại được** — đây là cái giá của "không có backdoor".

## 8. Câu hỏi thường gặp

**Admin có đọc được tin nhắn không?** Không. Server chỉ thấy dữ liệu đã mã hóa, ai gửi cho ai, lúc nào. Xem thêm phần [Bảo mật](#bảo-mật-làm-được-gì-và-không-làm-được-gì).

**Người mới vào có đọc được tin cũ trong Hell room không?** Không — tin chỉ được mã hóa cho những ai đã có tài khoản lúc gửi.

**Gõ tiếng Việt bị lỗi?** Dùng Unikey/EVKey/bộ gõ macOS bình thường. Nếu chữ hiển thị lệch, thử Windows Terminal / iTerm2 / terminal mặc định thay cho terminal cũ.

**Màn hình bị lệch sau khi đổi kích thước cửa sổ?** Kéo lại kích thước cửa sổ, app sẽ vẽ lại.

**Báo `Session revoked by an admin`?** Admin đã reset tài khoản của bạn — xem mục 7.

**Báo `Account locked until HH:MM`?** Đợi đến giờ đó hoặc nhờ Admin mở khóa.

---

# Phần 2 — Dành cho Admin

Admin quản lý bằng chính lệnh `umbrella`, xác thực bằng **ADMIN_TOKEN** (secret đặt trên Cloudflare). Không có trang quản trị web.

## 1. Chuẩn bị

Đặt token trong biến môi trường để khỏi phải nhập mỗi lần (nếu không đặt, app sẽ hỏi):

```bash
export UMBRELLA_ADMIN_TOKEN='<token của bạn>'
```

PowerShell:

```powershell
$env:UMBRELLA_ADMIN_TOKEN = '<token của bạn>'
```

> Giữ ADMIN_TOKEN như mật khẩu. Ai có token có thể tạo invite và reset tài khoản (nhưng **vẫn không đọc được tin nhắn**).

## 2. Mời người mới

```bash
umbrella admin invite
```

App in ra một mã mời dùng 1 lần. Gửi mã đó cho người mới (qua kênh riêng tư) kèm link trang Releases và hướng dẫn [Phần 1](#phần-1--dành-cho-người-dùng).

## 3. Mở khóa tài khoản bị khóa do sai PIN

```bash
umbrella admin unlock <username>
```

## 4. Reset tài khoản (quên PIN, mất máy, nghi bị lộ key)

```bash
umbrella admin reset <username>
```

Lệnh này:
- Xóa PIN và khóa công khai của người đó trên server, **ngắt ngay** mọi phiên đang mở của họ.
- In ra **mã mời dành riêng cho username đó** — gửi cho họ để chạy `umbrella register --force`.
- Tin cũ người khác gửi cho họ không đọc lại được; tin cũ họ đã gửi vẫn hiển thị bình thường với người khác.

## 5. Deploy / cập nhật server

Cần Node 22 và tài khoản Cloudflare (gói Free là đủ).

```bash
cd server
```
```bash
npm ci
```
```bash
npx wrangler login
```
```bash
npx wrangler deploy
```

Lần đầu deploy cần đặt ADMIN_TOKEN (chọn chuỗi ngẫu nhiên dài, ví dụ `openssl rand -base64 32`):

```bash
npx wrangler secret put ADMIN_TOKEN
```

**Đổi ADMIN_TOKEN** (khi nghi bị lộ): chạy lại lệnh trên với giá trị mới. Token cũ hết hiệu lực ngay, người dùng không bị ảnh hưởng.

Toàn bộ dữ liệu (tài khoản, tin đã mã hóa) nằm trong một Durable Object có SQLite tích hợp. Server tự quét và xóa tin hết hạn mỗi 5 phút.

## 6. Build bản cài cho người dùng

Cần Go 1.24+:

```bash
scripts/build.sh https://umbrella.black-umbrella.workers.dev
```

Kết quả nằm trong `dist/`. Đăng lên trang Releases của GitHub để mọi người tải.

Bản app desktop cho Linux (cần thêm Node 22): `scripts/build-desktop.sh https://umbrella.black-umbrella.workers.dev v1.7.0` tạo `dist/umbrella-desktop-*.deb` và `.AppImage`. Workflow `release.yml` tự build cùng lúc khi đẩy tag `v*`. Chạy thử khi phát triển: `cd desktop && npm ci && npm start` (đặt `UMBRELLA_BIN` trỏ tới binary client, hoặc để ở `desktop/bin/umbrella`).

## 7. Chạy thử ở máy local

> Hướng dẫn đầy đủ (chạy client với server thật, chạy cả server lẫn client trên máy, test, mẹo debug): **[how-to-debug.vi.md](how-to-debug.vi.md)**.

```bash
cd server && cp .dev.vars.example .dev.vars && npx wrangler dev --port 8787
```

ADMIN_TOKEN khi chạy local là `dev`. Client trỏ vào bằng `--server`:

```bash
umbrella --server http://localhost:8787
```

Chạy test:

```bash
cd server && npx vitest run
```
```bash
cd client && go test ./...
```
```bash
scripts/e2e.sh
```

---

# Bảo mật: làm được gì và không làm được gì

**Làm được**
- Nội dung tin nhắn được mã hóa trên máy người gửi (X25519 + XSalsa20-Poly1305 / NaCl). Server, Cloudflare và Admin **không đọc được**.
- Mỗi tin dùng một khóa ngẫu nhiên riêng, bọc riêng cho từng người nhận; người nhận xác minh tin đúng là do người gửi tạo.
- Khóa bí mật chỉ nằm trên máy người dùng, không lưu tin nhắn ra ổ đĩa.
- Tin hết hạn bị xóa khỏi server sau 24 giờ.

**Không làm được (cần biết)**
- **Server được tin tưởng khi phân phát khóa công khai.** Một server bị chiếm quyền có thể đưa khóa giả để đọc lén các tin *mới*. App chưa có tính năng so mã fingerprint.
- **Không có forward secrecy**: lộ khóa bí mật thì lộ mọi tin gửi tới khóa đó mà ai đó còn giữ.
- **PIN chỉ có 6 số**: nếu file `key.json` bị đánh cắp, kẻ gian có thể dò PIN offline. Bảo vệ file này như mật khẩu.
- Server biết *siêu dữ liệu*: ai nhắn cho ai, lúc nào, tin dài bao nhiêu.
- Người nhận luôn có thể chụp màn hình/copy lại tin — "tự xóa" không chống được điều đó.
- Token phiên đăng nhập đi trong URL WebSocket (qua HTTPS); đừng bật log URL đầy đủ nếu đặt proxy trước server.
