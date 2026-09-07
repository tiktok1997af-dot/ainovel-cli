# Hướng Dẫn Sử Dụng ainovel-cli — WEB-only Gemini

Tài liệu này mô tả **runtime hiện tại** của `ainovel-cli`.

> `ainovel-cli` chỉ dùng **Gemini Web trong cửa sổ Google Chrome hiển thị**. Người dùng tự đăng nhập vào Gemini trong browser profile bền vững. Không có AI API key, Base URL, Ollama, provider switching hay Docker runtime.

## 1. Chuẩn bị

Bạn cần:

- Google Chrome cài trên máy desktop;
- tài khoản có thể đăng nhập và sử dụng Gemini Web;
- binary `ainovel-cli` phù hợp hệ điều hành;
- kết nối mạng tới Gemini Web.

Không cần và không nên chuẩn bị Gemini API key, OpenAI key, Anthropic key, OpenRouter key hay DeepSeek key.

## 2. Cài đặt

Release chính thức của fork:

`https://github.com/tiktok1997af-dot/ainovel-cli/releases`

### Linux/macOS

```bash
curl -fsSL https://raw.githubusercontent.com/tiktok1997af-dot/ainovel-cli/main/scripts/install.sh | sh
```

Installer chỉ tải artifact/checksum từ repository này, dùng HTTPS và xác minh SHA-256.

### Windows

Tải archive phù hợp từ GitHub Releases, giải nén `ainovel-cli.exe` và chạy trực tiếp hoặc thêm thư mục chứa binary vào `PATH`.

### Build từ source

```bash
git clone https://github.com/tiktok1997af-dot/ainovel-cli.git
cd ainovel-cli
go build -o ainovel-cli ./cmd/ainovel-cli
```

## 3. Thiết lập lần đầu

Chạy:

```bash
ainovel-cli
```

Nếu chưa có cấu hình, Setup Wizard sẽ chạy trước TUI.

Wizard hiện chỉ cấu hình WEB runtime:

1. **Ngôn ngữ sáng tác**: `vi` hoặc `zh`.
2. **Chrome executable**: có thể để trống để tự dò.
3. **Persistent profile name**: mặc định `default`.

Wizard không hỏi:

- provider AI;
- API key;
- Base URL;
- model ID kiểu API;
- Ollama;
- fallback provider.

Sau đó ainovel-cli mở cửa sổ Chrome do nó quản lý.

### Nếu hiện `AUTH_REQUIRED`

1. Chuyển sang cửa sổ Chrome vừa được mở.
2. Đăng nhập tài khoản Google/Gemini **trực tiếp trên website**.
3. Hoàn tất mọi bước xác minh mà Google yêu cầu.
4. Giữ nguyên browser profile đó.
5. Quay lại ainovel-cli và chờ readiness chuyển sang `READY`.

Không nhập mật khẩu Google, cookie hay session token vào TUI/config.

## 4. Cấu hình mẫu

```json
{
  "web": {
    "enabled": true,
    "site": "gemini-web",
    "profile_name": "default"
  },
  "language": "vi",
  "reasoning_effort": "medium",
  "style": "default",
  "context_window": 200000
}
```

Nếu Chrome không tự dò được, thêm:

```json
{
  "web": {
    "enabled": true,
    "site": "gemini-web",
    "browser_path": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
    "profile_name": "default"
  }
}
```

`context_window` là ngân sách context **cục bộ** của ainovel-cli. Nó không phải một tham số API và không chứng minh context limit thực tế của tài khoản Gemini Web.

Không thêm `providers`, `api_key`, `base_url` hoặc cấu hình Ollama.

## 5. Trạng thái browser AI

Runtime có thể hiển thị các trạng thái:

- `STARTING` — đang khởi tạo browser/session;
- `AUTH_REQUIRED` — cần người dùng đăng nhập thủ công;
- `READY` — Gemini Web sẵn sàng;
- `BUSY` — đang có lượt AI hoạt động;
- `DEGRADED` — phiên còn tồn tại nhưng readiness không bảo đảm;
- `FAILED` — browser/session không thể tiếp tục;
- `STOPPED` — session đã dừng.

Không có nhánh tự động chuyển sang provider API khi browser lỗi.

## 6. `/model`

Trong runtime hiện tại, `/model` là **read-only status panel**.

Nó dùng để xem:

- transport WEB-only;
- site/model label `Gemini Web`;
- readiness state;
- browser PID/profile khi có;
- hướng dẫn đăng nhập khi cần.

`/model` không dùng để chọn Claude/OpenAI/DeepSeek/Ollama hay đổi API model.

## 7. `/config`

`/config` chỉ dành cho browser settings:

- Chrome executable path;
- persistent profile name;
- site hiện cố định `gemini-web`.

Thay đổi được lưu và áp dụng an toàn ở lần khởi động tiếp theo.

`/config` không chấp nhận API key, Base URL, cookie, Google credential hoặc provider registry.

## 8. Bắt đầu sáng tác

Khi Gemini Web ở `READY`, làm việc trong TUI như bình thường:

- nhập ý tưởng/chỉ dẫn vào ô chat;
- dùng chế độ bắt đầu nhanh hoặc đồng sáng tác;
- nhấn `/` để xem slash commands;
- `Ctrl+C` hai lần để lưu và thoát an toàn.

Các tác nhân sáng tác vẫn được điều phối cục bộ; các lượt cần AI đều đi qua cùng `WebChatModel -> Gemini Web`.

## 9. Các slash command chính

| Lệnh | Công dụng hiện tại |
|---|---|
| `/help` | Xem trợ giúp |
| `/model` | Xem trạng thái Gemini Web/browser — chỉ đọc |
| `/config` | Chỉnh Chrome path/profile |
| `/diag` | Chẩn đoán sức khỏe project/runtime |
| `/review` | Bật/tắt nghiệm thu từng chương |
| `/next` | Cho phép tiến chương khi gate yêu cầu |
| `/start <tệp>` | Bắt đầu từ file ý tưởng/dàn ý cục bộ |
| `/import <tệp>` | Import truyện từ file cục bộ |
| `/reopen <hướng>` | Mở tiếp tuyến truyện sau khi đã kết thúc |
| `/cocreate` | Vào chế độ đồng sáng tác |
| `/simulate` | Phân tích văn mẫu cục bộ theo workflow hiện có |
| `/importsim <file>` | Nhập hồ sơ mô phỏng văn phong |
| `/sync` | Đồng bộ chỉnh sửa thủ công trong project |
| `/export` | Xuất tác phẩm |

Các file được đọc/ghi bởi local Host/Tools. Gemini Web không tự duyệt filesystem.

## 10. Headless

Headless là chế độ không dùng TUI, **không phải API mode**.

Lần chạy đầu tiên vẫn phải dùng TUI để setup browser và đăng nhập profile.

Sau đó có thể chạy:

```bash
ainovel-cli --headless --prompt "Tiểu thuyết tu tiên..."
```

Hoặc:

```bash
ainovel-cli --headless --prompt-file premise.txt
```

Có thể đọc prompt từ stdin bằng `--prompt-file -` nếu workflow của bạn cần.

Dù headless, AI execution path vẫn là Gemini Web/browser-backed runtime.

## 11. Import truyện ngoài

`/import <tệp>` sử dụng semantic import pipeline theo nguyên tắc:

1. Host đọc và chuẩn hóa file cục bộ.
2. Tạo snapshot/workspace import cục bộ.
3. Các phần cần hiểu ngữ nghĩa được gửi thành lượt Gemini Web qua `WebChatModel`.
4. Code cục bộ kiểm tra coverage, digest, schema và thứ tự.
5. Chỉ publish vào Store chính khi các gate cần thiết đã đạt.

Website không được trao đường dẫn hoặc quyền truy cập file gốc trên máy.

## 12. Quản lý dữ liệu dự án

Dữ liệu truyện/checkpoint/summary/diagnostic/export nằm trong workspace của ainovel-cli.

Các artifact thường gặp gồm:

```text
output/<novel>/
├── novel/
├── meta/
│   ├── progress.json
│   ├── checkpoints.jsonl
│   ├── decisions.jsonl
│   └── sessions/
└── ...
```

Chi tiết quan sát runtime xem `docs/observability.md`.

## 13. Token, context và chi phí

WEB bridge không nhận authoritative provider usage/billing data từ Gemini Web.

Vì vậy:

- không xem các local token estimate là token usage chính thức của Gemini;
- không có prompt-cache billing/cache hit telemetry chính thức;
- không có USD cost/savings chính xác từ provider;
- không có API-dollar budget sentinel bảo vệ chi tiêu tài khoản Gemini.

Nếu cần kiểm soát quyền lợi/giới hạn tài khoản Gemini, hãy xem thông tin trực tiếp trong tài khoản/website Gemini; ainovel-cli không giả lập số liệu đó.

## 14. Cập nhật

Cập nhật lên release mới nhất:

```bash
ainovel-cli update
```

Cập nhật tới version cụ thể:

```bash
ainovel-cli update vX.Y.Z
```

Xem version:

```bash
ainovel-cli --version
```

Updater chỉ dùng release từ `tiktok1997af-dot/ainovel-cli`.

## 15. Xử lý lỗi thường gặp

### Chrome không được tìm thấy

- xác minh Chrome đã được cài;
- dùng `/config` hoặc `web.browser_path`;
- khởi động lại ainovel-cli.

### `AUTH_REQUIRED` không chuyển sang `READY`

- kiểm tra đúng cửa sổ/profile do ainovel-cli mở;
- hoàn tất login/consent trực tiếp trên Gemini Web;
- kiểm tra network;
- không tạo API key để “thay thế” — runtime không dùng API fallback.

### `DEGRADED` hoặc `FAILED`

- không xóa profile ngay nếu muốn giữ login;
- kiểm tra Chrome có còn chạy và Gemini Web có truy cập được không;
- khởi động lại ainovel-cli nếu cần;
- nếu lỗi lặp lại, dùng `/diag` và `diag-export.md` để thu thập bằng chứng không chứa prose/prompt nhạy cảm.

### Config cũ báo migration error

Config provider/API-era không còn hợp lệ. Hãy chuyển về schema `web` theo `config.example.jsonc`.

## 16. Các runtime không được hỗ trợ

Không dùng các hướng dẫn cũ về:

- OpenAI/Anthropic/Gemini/OpenRouter/DeepSeek API;
- API key/Base URL;
- Ollama hoặc local LLM;
- provider/model hot-swap/fallback;
- Docker Compose runtime;
- hidden browser;
- cookie/credential extraction.

Nếu gặp các nội dung đó trong `docs/chatgpt-web-bridge-*` hoặc `docs/w5*`, hãy hiểu chúng là **historical migration/audit provenance**, không phải hướng dẫn sử dụng hiện tại.