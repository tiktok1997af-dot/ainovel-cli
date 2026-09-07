# Quản lý Context trong runtime WEB-only

Tài liệu này mô tả context management hiện tại của `ainovel-cli` sau migration sang Gemini Web.

## 1. Mục tiêu

Sáng tác tiểu thuyết dài kỳ không thể dựa vào việc browser conversation “nhớ” toàn bộ lịch sử. Runtime phải duy trì bộ nhớ có cấu trúc ở local Store và chỉ gửi phần cần thiết vào từng lượt Gemini Web.

Các mục tiêu chính:

1. giữ continuity dài hạn;
2. tránh context phình vô hạn;
3. phục hồi được sau restart/crash;
4. không coi browser transcript là source of truth;
5. không phụ thuộc provider API usage/billing telemetry.

## 2. Các lớp bộ nhớ

Context được tổ chức theo nhiều lớp:

- **Recent messages** — phần hội thoại gần nhất còn hữu ích;
- **Structured project memory** — chapter summaries, character state, world rules, foreshadowing, timeline, review notes trong Store;
- **Compacted context** — kết quả thay thế các message cũ bằng bản tóm tắt có cấu trúc;
- **Restore / handoff pack** — gói phục hồi sau compaction hoặc khi worker được dựng lại.

## 3. Data flow

Luồng bình thường:

```text
Store / project artifacts
  -> novel_context / local builders
  -> prompt context
  -> WebChatModel
  -> Gemini Web turn
```

Khi context quá lớn:

```text
messages + local estimates
  -> ToolResultMicrocompact
  -> LightTrim
  -> StoreSummaryCompact
  -> FullSummary nếu vẫn cần
  -> restore pack
  -> tiếp tục qua WebChatModel
```

Mọi bước đọc Store/build context/trim/validate đều chạy cục bộ. Nếu `FullSummary` cần suy luận AI thì lượt summary cũng đi qua fixed `WebChatModel -> Gemini Web`.

## 4. `context_window` là local budget

`context_window` trong config là tham số phục vụ planning/compaction của ainovel-cli.

Nó **không** có nghĩa:

- ainovel-cli đang cấu hình context window của Gemini API;
- website xác nhận chính xác giới hạn token bằng giá trị đó;
- local estimate bằng authoritative provider token usage.

Runtime dùng local token/context estimates để quyết định khi nào nên compact. Đây là heuristic vận hành, không phải billing telemetry.

## 5. Strategy pipeline

### 5.1 ToolResultMicrocompact

Mục tiêu: giảm các tool result cũ có khối lượng lớn nhưng không còn cần toàn văn.

Giữ lại đủ cấu trúc để biết tool đã chạy và kết quả nào cần tham chiếu; phần dữ liệu bền vững phải tồn tại ở Store/artifacts thay vì phụ thuộc message cũ.

### 5.2 LightTrim

Mục tiêu: cắt bớt text block quá dài trong một message mà vẫn giữ đầu/cuối hoặc thông tin định vị cần thiết.

Phù hợp với chapter text/tool output lớn khi chưa cần summary toàn bộ history.

### 5.3 StoreSummaryCompact

Đây là chiến lược ưu tiên của Writer khi có đủ structured memory trong Store.

Thay vì hỏi AI “tóm tắt mọi thứ đã nói”, runtime dựng lại context từ các fact đã persist như:

- tiến độ hiện tại;
- chapter summaries;
- layered outline/plan;
- character state;
- world rules;
- foreshadowing/open threads;
- timeline;
- review/repair notes;
- recent cast và các continuity artifact khác.

Ưu điểm chính là deterministic hơn và không cần thêm một Gemini Web turn.

### 5.4 FullSummary

Chỉ dùng khi các chiến lược rẻ hơn không đủ.

FullSummary tạo một summary mới qua **cùng browser-backed chat model**. Sau đó runtime gắn restore pack để worker tiếp tục có structured continuity.

Không mô tả bước này là “một API call”, không suy ra chi phí USD và không dùng provider prompt-cache assumption.

## 6. Restore pack

Sau compaction, worker không được giả định model còn nhớ lịch sử cũ.

Restore pack phải ưu tiên các fact cần thiết để tiếp tục công việc:

- current chapter/phase;
- intent và gate đang hoạt động;
- chapter plan;
- continuity constraints;
- unresolved review issues;
- relevant character/world/timeline state;
- các artifact path/identity cần cho local tools.

Restore pack là local runtime message được dựng từ state đã persist.

## 7. Structured context builder

`novel_context` và các builder liên quan chịu trách nhiệm chọn dữ liệu cần thiết cho một lượt làm việc.

Nguyên tắc:

1. ưu tiên fact có cấu trúc;
2. chỉ lấy chapter/history có liên quan;
3. không gửi toàn bộ workspace nếu không cần;
4. không đưa credential/browser cookie vào context;
5. local file access phải qua Host/Tools;
6. website chỉ thấy content mà runtime đã chọn để gửi.

## 8. Worker lifecycle

Khi một worker được spawn/restart, ContextManager được dựng lại từ config + project state hiện tại.

Không cần và không được dựa vào một API provider session bên ngoài để phục hồi. Persistent source of truth nằm ở Store/artifacts.

## 9. Handoff và recovery

Trong các giai đoạn dài như rewrite/review/import/repair, handoff package giúp một worker mới tiếp tục từ state đã xác minh.

Recovery ưu tiên:

1. checkpoint/project state;
2. structured handoff;
3. Store-backed context;
4. browser conversation chỉ như transient transport history nếu còn phù hợp.

## 10. Observability

Runtime có thể ghi nhận các sự kiện như:

- strategy nào vừa chạy;
- local context estimate trước/sau;
- message count trước/sau;
- projected/compacted state;
- summary/restore event;
- lỗi local builder hoặc browser turn.

Các con số này là **local operational metrics**.

Không được gắn nhãn chúng thành:

- Gemini token usage chính thức;
- provider cache-hit rate;
- USD cost/savings;
- API billing protection.

## 11. Prompt cache

Current WEB-only runtime không điều khiển provider prompt-cache protocol.

Do đó context strategy không được thiết kế quanh:

- OpenAI cached tokens;
- Anthropic cache write/read pricing;
- LiteLLM cache fields;
- API cache-savings estimates.

Nếu website/provider tự caching ở phía dịch vụ, đó không phải capability mà ainovel-cli có thể đo/điều khiển theo contract hiện tại.

## 12. Failure handling

### Browser turn fail trong FullSummary

- không chuyển sang API provider khác;
- giữ Store/checkpoint hiện tại;
- báo lỗi theo browser runtime contract;
- cho phép retry/restart theo flow hiện có.

### Local context builder fail

- không gửi payload nửa vời nếu invariant bị phá;
- giữ artifact phục vụ chẩn đoán;
- fail rõ ràng thay vì đoán.

### Context estimate lệch thực tế website

Đây là giới hạn dự kiến vì website không cung cấp authoritative token usage. Runtime phải xử lý bằng local safety margin/compaction, không giả vờ có số liệu provider chính xác.

## 13. Quy tắc dành cho maintainer

Khi thêm context feature mới:

1. xác định fact nào phải persist;
2. ưu tiên Store-backed reconstruction;
3. chỉ dùng Gemini Web turn khi thực sự cần semantic compression;
4. không thêm provider-specific API usage/cache/billing dependency;
5. không thêm credential vào messages/artifacts;
6. thêm diagnostic signal provider-neutral;
7. kiểm tra recovery sau restart.

## 14. Các file/code area liên quan

Các implementation area chính có thể gồm:

- `agentcore/context/*`;
- `internal/tools/novel_context*`;
- orchestrator/store-summary builders;
- writer restore/handoff/recovery code;
- Store/checkpoint artifacts;
- TUI/diag surfaces hiển thị context state.

Tên file cụ thể có thể thay đổi qua refactor; các invariant trong tài liệu này mới là contract current-product.

## 15. Historical note

Nếu gặp tài liệu cũ mô tả API-call waste, provider token Usage, prompt-cache pricing hoặc model-specific context billing, hãy coi đó là historical design/provenance. Current runtime chỉ sử dụng local context estimates + Gemini Web browser turns.