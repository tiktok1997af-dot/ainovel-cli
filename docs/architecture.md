# ainovel-cli — Kiến trúc sản phẩm WEB-only hiện tại

Tài liệu này mô tả **kiến trúc production hiện tại** sau W5. Các tài liệu `chatgpt-web-bridge-*`, `w5a-*`, `w5b-*`, `w5c-*`, `w5d-*` là provenance của quá trình migration và có thể nhắc tới code/API đã bị loại bỏ.

## 1. Product invariant

Sản phẩm chỉ có **một AI execution path**:

```text
Host
  -> SessionManager
  -> GeminiWebTransport
  -> WebChatModel
  -> Agents / Arbiter / semantic functions
```

Ở tầng browser:

```text
GeminiWebTransport
  -> owned visible Google Chrome
  -> persistent browser profile
  -> user logs in manually
  -> Gemini Web
```

Không có production path thứ hai tới AI HTTP API, Ollama/local inference, provider fallback hay provider/model hot-swap.

## 2. Trust boundary

### Host/local side

Host sở hữu:

- project files và Store;
- checkpoints, recovery và session logs;
- import/export;
- local Tools;
- validation/schema/digest;
- Engine/Workers/Arbiter;
- browser lifecycle và readiness state.

### Website side

Gemini Web chỉ nhận nội dung hội thoại mà `WebChatModel` gửi qua browser transport và trả về nội dung website hiển thị.

Website **không** được cấp:

- quyền duyệt filesystem cục bộ;
- đường dẫn file để tự mở;
- Google password/cookie/session token từ ainovel-cli;
- API credential;
- cơ chế gọi local Tools trực tiếp.

Nếu một Agent cần dữ liệu từ file/project, local Tool/Store đọc dữ liệu trước, sau đó chỉ phần nội dung cần thiết mới được đưa vào browser conversation.

## 3. Browser session lifecycle

SessionManager quản lý một Chrome session thuộc ainovel-cli với persistent profile.

Các readiness state chính:

- `STARTING`
- `AUTH_REQUIRED`
- `READY`
- `BUSY`
- `DEGRADED`
- `FAILED`
- `STOPPED`

`AUTH_REQUIRED` có nghĩa người dùng phải đăng nhập trực tiếp trong Chrome. Runtime không trích xuất credential và không đổi sang API fallback.

Profile được giữ bền vững để session login có thể tồn tại qua lần khởi động sau.

## 4. WebChatModel

`WebChatModel` là adapter từ browser transport sang interface chat-model provider-neutral mà Engine/Agents đang sử dụng.

Trách nhiệm:

- serialize lượt hội thoại/tool protocol cho web transport;
- gửi một turn qua Gemini Web;
- thu response đã hoàn tất;
- map cancel/timeout/browser failure sang lỗi runtime;
- giữ identity runtime cố định là `web/gemini-web`.

Nó không:

- tạo API client;
- nhận API key/Base URL;
- switch provider/model;
- thực hiện failover sang AI khác;
- tuyên bố provider token/cost telemetry mà website không cung cấp.

## 5. Engine, Workers và Arbiter

Phần orchestration tiểu thuyết vẫn chạy cục bộ.

```text
User intent / project state
        |
        v
      Engine
        |
        +--> planning / routing / gates
        |
        +--> Workers / semantic functions
        |       |
        |       +--> local context assembly
        |       +--> WebChatModel when AI reasoning is needed
        |       +--> local Tools / Store for deterministic actions
        |
        +--> Arbiter / validation / chapter advance rules
```

Các role/worker khác nhau có thể có prompt/policy khác nhau, nhưng **không có provider identity riêng**. Chúng dùng cùng fixed browser-backed model path.

## 6. Tools và Store

Tools là capability cục bộ. Ví dụ:

- đọc context/trạng thái truyện;
- plan/draft/check/commit chapter;
- import/export;
- sync chỉnh sửa;
- diagnostics;
- checkpoint/recovery.

Tool result có thể được đưa vào một AI turn dưới dạng nội dung hội thoại khi cần, nhưng tool execution vẫn ở Host.

Store là source of truth cho trạng thái có cấu trúc. Browser conversation không được coi là persistent project memory.

## 7. Context management

Context management là cơ chế cục bộ gồm:

- recent-message tail;
- structured project memory;
- store-based compaction;
- summary/restore pack;
- local context estimates.

`context_window` trong config là local planning/compaction budget. Nó không phải declaration gửi tới Gemini API và không phải authoritative website limit.

Khi cần LLM summary, lượt summary cũng đi qua `WebChatModel -> Gemini Web`, không qua một API provider riêng.

Chi tiết: `docs/context-management.md`.

## 8. Semantic import

Import pipeline giữ invariant:

```text
local source file
  -> deterministic local ingest/snapshot
  -> semantic segmentation/analysis through WebChatModel
  -> local coverage/schema/digest validation
  -> local staged artifacts
  -> publish into Store only after gates pass
```

Website không tự đọc file nguồn. Host quyết định payload nào được gửi cho Gemini Web.

Chi tiết: `docs/import-pipeline.md`.

## 9. Setup và config

First-run Setup Wizard chỉ nhận:

- creative language;
- Chrome executable path tùy chọn;
- persistent profile name.

WEB config tối thiểu:

```json
{
  "web": {
    "enabled": true,
    "site": "gemini-web",
    "profile_name": "default"
  }
}
```

`/model` là read-only browser/Gemini status.

`/config` chỉ chỉnh Chrome path/profile/site browser settings.

Legacy provider/API-shaped config không tạo API runtime; nó bị từ chối với migration guidance.

## 10. Headless boundary

Headless chỉ thay entry UI, không thay AI transport.

```text
TUI -----------+
               +--> Host --> browser-backed WEB runtime
Headless ------+
```

First-run setup phải hoàn thành trong TUI trước. Headless không phải server/API mode.

## 11. Usage / telemetry boundary

Gemini Web transport hiện không cung cấp authoritative provider Usage/billing telemetry.

Do đó current product không dựng lại:

- provider token totals;
- prompt-cache hit/miss billing;
- cache savings;
- USD cost;
- API-dollar budget sentinel.

Các số liệu context nội bộ chỉ là local estimates phục vụ compaction/diagnostics. Chúng không phải hóa đơn hoặc usage do Gemini xác nhận.

## 12. Prompt cache boundary

ainovel-cli không điều khiển OpenAI/Anthropic/LiteLLM provider prompt-cache protocol trong WEB runtime.

Nếu Gemini Web có caching nội bộ phía dịch vụ thì đó là chi tiết của website/tài khoản và không được current product xem như một capability có thể cấu hình hay đo billing.

Thiết kế API-era `docs/prompt-cache-design.md` đã được loại khỏi current-product documentation ở W5D-D3.

## 13. Update / install / release boundary

Production updater và installer chỉ lấy release từ:

`tiktok1997af-dot/ainovel-cli`

Release notes được tạo deterministic từ Git history. Release automation không cần Gemini/OpenAI/Anthropic API secrets.

Docker runtime không được publish/advertise như WEB-only runtime cho tới khi có một contract riêng chứng minh container có thể sở hữu visible browser session đúng boundary.

## 14. Security invariants

Không thay đổi các invariant sau nếu chưa mở stage kiến trúc mới:

1. visible browser; không hidden browser mặc định;
2. user login thủ công;
3. không credential/cookie extraction;
4. không AI API key;
5. không API provider fallback;
6. không website-side local file execution;
7. local Tools/Store là nơi thực hiện deterministic side effects;
8. browser/profile change không được âm thầm thay active session giữa một run;
9. update/install chỉ từ fork được khóa.

## 15. Failure model

Các nhóm lỗi chính:

- Chrome executable không tìm thấy;
- browser launch/profile failure;
- `AUTH_REQUIRED`;
- website readiness degraded;
- submit/response timeout;
- cancel;
- response/tool protocol mismatch;
- local Tool/Store/validation failure.

Browser failure không kích hoạt API fallback. Recovery phải giữ nguyên WEB-only boundary.

## 16. Observability

Quan sát runtime qua:

- readiness/status UI;
- `/diag`;
- `diag-export.md`;
- session logs;
- Store artifacts/checkpoints;
- local context rewrite events.

Không dùng token-dollar telemetry API-era làm health signal.

Xem `docs/observability.md`.

## 17. Historical provenance

Các tài liệu sau có thể chứa từ khóa API/provider/Ollama vì chúng ghi lại quá trình chuyển đổi:

- `docs/chatgpt-web-bridge-audit.md`
- `docs/chatgpt-web-bridge-w1-verification.md`
- `docs/chatgpt-web-bridge-w2.md` ... `w5.md`
- `docs/w5a-*`
- `docs/w5b-*`
- `docs/w5c-*`
- `docs/w5d-*`

Chúng **không phải current user/product instructions**. Khi có xung đột, runtime/code + tài liệu current-product này là nguồn mô tả hành vi hiện tại.