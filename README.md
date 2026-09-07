# ainovel-cli — WEB-only Gemini Novel Studio

`ainovel-cli` là công cụ dòng lệnh/TUI hỗ trợ sáng tác tiểu thuyết dài kỳ bằng kiến trúc đa tác nhân. **Bản hiện tại chỉ có một đường AI duy nhất: Gemini Web trong cửa sổ Google Chrome nhìn thấy được và do người dùng tự đăng nhập.**

> **Không dùng AI API.** Không nhập hoặc lưu Gemini/OpenAI/Anthropic/OpenRouter/DeepSeek API key, không cấu hình Base URL, không chạy Ollama/local inference, không có provider fallback và không dùng Docker như một runtime được hỗ trợ.

## 1. Mô hình hoạt động hiện tại

```text
Engine / Workers
  -> WebChatModel
  -> browser session manager
  -> Chrome hiển thị + profile bền vững
  -> người dùng đăng nhập Gemini Web thủ công
  -> Gemini Web
  -> response/tool protocol
  -> local Tools / Store trên máy người dùng
```

AI website chỉ là kênh hội thoại. Việc đọc/ghi file dự án, checkpoint, import/export và các tool nội bộ vẫn chạy **cục bộ trong ainovel-cli**; website không được cấp đường dẫn máy hay quyền tự đọc file cục bộ.

## 2. Yêu cầu hệ thống

- Google Chrome có thể mở thành cửa sổ hiển thị trên desktop.
- Có tài khoản có thể đăng nhập và sử dụng Gemini Web.
- Binary `ainovel-cli` phù hợp hệ điều hành/kiến trúc, hoặc Go nếu tự build từ source.
- Kết nối mạng tới Gemini Web và GitHub Releases khi cài/cập nhật.

Docker không phải runtime được hỗ trợ cho bản WEB-only vì container không sở hữu được phiên Chrome hiển thị theo contract hiện tại.

## 3. Cài đặt

### Từ GitHub Releases

Chỉ sử dụng release của repository này:

`https://github.com/tiktok1997af-dot/ainovel-cli/releases`

Windows: tải archive phù hợp từ Releases, giải nén và đặt `ainovel-cli.exe` vào thư mục thuộc `PATH` hoặc chạy trực tiếp.

Linux/macOS có thể dùng installer của repository:

```bash
curl -fsSL https://raw.githubusercontent.com/tiktok1997af-dot/ainovel-cli/main/scripts/install.sh | sh
```

Installer tải artifact + checksum từ **chính fork này**, chỉ qua HTTPS và xác minh SHA-256 trước khi cài.

### Build từ source

```bash
git clone https://github.com/tiktok1997af-dot/ainovel-cli.git
cd ainovel-cli
go build -o ainovel-cli ./cmd/ainovel-cli
```

## 4. Lần chạy đầu tiên

Chạy:

```bash
ainovel-cli
```

Setup Wizard sẽ chỉ hỏi các thông tin thuộc WEB runtime:

1. Ngôn ngữ sáng tác (`vi` mặc định hoặc `zh`).
2. Đường dẫn Chrome tùy chọn — để trống để tự dò.
3. Tên browser profile bền vững — mặc định `default`.

Wizard **không** hỏi provider, model API, API key hay Base URL.

Sau khi cấu hình, ainovel-cli mở Chrome bằng profile đã chọn. Nếu Gemini chưa đăng nhập, hãy đăng nhập **trực tiếp trong cửa sổ Chrome đó**. ainovel-cli không yêu cầu mật khẩu Google, không đọc cookie để trích xuất credential và không chuyển credential vào file truyện.

Khi phiên web sẵn sàng, trạng thái browser chuyển sang `READY` và luồng sáng tác có thể bắt đầu.

## 5. Cấu hình WEB-only

Ví dụ tối thiểu:

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

Có thể thêm `web.browser_path` nếu Chrome không được tự dò thấy.

`context_window` là giới hạn/ước lượng **cục bộ cho quản lý context của ainovel-cli**, không phải khai báo context-window của một AI API.

Không thêm các trường `providers`, `api_key`, `base_url`, provider fallback, Ollama hoặc API model list. Config API-era cũ không còn là runtime hợp lệ.

Xem mẫu đầy đủ tại `config.example.jsonc`.

## 6. `/model` và `/config`

### `/model`

Trong bản WEB-only, `/model` là **màn hình trạng thái chỉ đọc**, không phải model/provider switcher. Nó có thể hiển thị:

- transport: WEB-only;
- site/model label: Gemini Web;
- readiness: `STARTING`, `AUTH_REQUIRED`, `READY`, `BUSY`, `DEGRADED`, `FAILED`, `STOPPED`;
- browser PID/profile khi phù hợp;
- hướng dẫn đăng nhập thủ công khi `AUTH_REQUIRED`.

Không thể dùng `/model` để chọn OpenAI/Claude/DeepSeek/Ollama hoặc cấu hình fallback.

### `/config`

Trong bản WEB-only, `/config` chỉ chỉnh cấu hình browser:

- đường dẫn Chrome;
- tên persistent profile;
- site hiện cố định là `gemini-web`.

Thay đổi browser config có hiệu lực an toàn ở lần khởi động tiếp theo; phiên Chrome đang dùng không bị thay thế âm thầm giữa một lượt viết.

`/config` không nhận API key, Base URL, cookie, Google credential hay provider definition.

## 7. Bắt đầu sáng tác trong TUI

Sau khi browser ở trạng thái `READY`:

- nhập ý tưởng hoặc chỉ dẫn vào ô chat;
- dùng chế độ bắt đầu nhanh hoặc đồng sáng tác theo TUI;
- dùng `/` để mở danh sách slash command;
- `Ctrl+C` hai lần để lưu an toàn và thoát.

Các lệnh dự án hiện có như `/diag`, `/review`, `/next`, `/start`, `/import`, `/reopen`, `/cocreate`, `/simulate`, `/importsim`, `/sync`, `/export` tiếp tục thao tác trên Engine/Tools/Store cục bộ. Mọi tác vụ cần suy luận AI đều đi qua cùng `WebChatModel -> Gemini Web`.

## 8. Headless

`--headless` chỉ bỏ giao diện TUI; **nó không biến ainovel-cli thành API/server runtime và không thay thế browser bằng HTTP model API**.

Phải chạy TUI ít nhất một lần để hoàn tất setup WEB-only và đăng nhập browser profile trước khi dùng headless.

```bash
ainovel-cli --headless --prompt "Ý tưởng truyện..."
ainovel-cli --headless --prompt-file premise.txt
```

Browser-backed Gemini Web vẫn là AI execution path của headless.

## 9. Import, dữ liệu cục bộ và quyền riêng tư

- Import hiện xử lý file nguồn ở Host/local Tools; nội dung cần suy luận được đóng gói thành lượt hội thoại qua Gemini Web.
- Gemini Web không được trao quyền trực tiếp duyệt filesystem của máy.
- Profile đăng nhập Chrome được giữ trong vùng profile của browser; không sao chép Google credential/cookie vào project artifacts.
- Novel data, checkpoints, summaries, diagnostics và exports nằm trong workspace cục bộ của ainovel-cli.

## 10. Usage, token và chi phí

Gemini Web bridge hiện **không cung cấp authoritative provider Usage/billing telemetry** cho ainovel-cli. Vì vậy sản phẩm không tuyên bố:

- số token provider chính xác;
- prompt-cache billing/cache-hit rate của provider;
- USD cost/savings chính xác;
- API-dollar budget enforcement.

Các con số context nội bộ nếu xuất hiện chỉ phục vụ quản lý bộ nhớ cục bộ; chúng không phải hóa đơn hay số liệu usage do Gemini cung cấp.

## 11. Cập nhật

Kiểm tra/cập nhật binary:

```bash
ainovel-cli update
```

Hoặc chỉ định version:

```bash
ainovel-cli update vX.Y.Z
```

Updater production chỉ nhận release từ `tiktok1997af-dot/ainovel-cli` và xác minh artifact trước khi thay binary.

Xem version:

```bash
ainovel-cli --version
```

## 12. Troubleshooting nhanh

- **Chrome không tìm thấy:** mở `/config` hoặc sửa `web.browser_path` tới executable Chrome hợp lệ rồi khởi động lại.
- **`AUTH_REQUIRED`:** đăng nhập Gemini trực tiếp trong cửa sổ Chrome do ainovel-cli mở; không dán credential vào TUI/config.
- **`DEGRADED`/`FAILED`:** giữ nguyên profile, kiểm tra Chrome/Gemini Web/network rồi khởi động lại. Không chuyển sang API fallback — sản phẩm không có fallback đó.
- **Config cũ có provider/API key:** chuyển sang schema `web` theo `config.example.jsonc`; không cố tái kích hoạt runtime API cũ.

## 13. Kiến trúc và tài liệu

- `docs/architecture.md` — kiến trúc sản phẩm WEB-only hiện tại.
- `docs/context-management.md` — quản lý context cục bộ trong browser-backed runtime.
- `docs/import-pipeline.md` — semantic import qua fixed `web/gemini-web` identity.
- `docs/observability.md` — chẩn đoán local Store/Engine.
- `docs/chatgpt-web-bridge-*`, `docs/w5a-*`, `docs/w5b-*`, `docs/w5c-*`, `docs/w5d-*` — **historical migration/audit provenance**; các từ API/provider trong đó mô tả trạng thái lịch sử, không phải hướng dẫn runtime hiện tại.

## 14. Những đường chạy không được hỗ trợ

Bản hiện tại **không hỗ trợ**:

- OpenAI / Anthropic / Gemini API / OpenRouter / DeepSeek API;
- API key hoặc custom Base URL;
- provider/model hot-swap hoặc fallback;
- Ollama/local LLM execution;
- Docker runtime;
- hidden-browser automation;
- credential/cookie extraction để đăng nhập thay người dùng.

## License

Xem `LICENSE`.