# Semantic Import Pipeline — WEB-only Runtime

Tài liệu này mô tả pipeline import tiểu thuyết ngoài trong **current product**. Những revision cũ từng mô tả role-specific provider/model configuration không còn là runtime contract.

## 1. Mục tiêu

Import không phải “đưa cả file cho website tự đọc”. Đây là một pipeline cục bộ có các semantic step được hỗ trợ bởi fixed browser-backed model:

```text
Local source file
  -> deterministic ingest / normalize / snapshot
  -> semantic segmentation via WebChatModel
  -> local coverage validation
  -> user confirmation when required
  -> semantic chapter analysis via WebChatModel
  -> semantic synthesis via WebChatModel
  -> local schema/digest validation
  -> staged artifacts
  -> publish into Store only after gates pass
```

AI identity trong toàn bộ pipeline là:

`web/gemini-web`

Không có provider/model selection riêng cho import.

## 2. Security boundary

Source file được mở bởi **Host/local code**.

Gemini Web không được cấp:

- đường dẫn file cục bộ để tự mở;
- quyền filesystem;
- API key;
- browser credential/cookie;
- local tool execution.

Khi một bước semantic cần nội dung, Host chọn và serialize phần text cần thiết thành browser conversation. Response được trả lại cho Host để code kiểm tra/parse/validate.

## 3. Supported source contract

Current import flow tập trung vào text-based source như TXT/Markdown theo implementation hiện có.

Ingest layer chịu trách nhiệm:

- đọc bytes;
- detect/decode encoding theo capability hiện tại;
- normalize line endings/text;
- tạo local snapshot;
- tính digest;
- giữ source mapping đủ để chứng minh coverage.

Không để một semantic model tự quyết định byte ranges hoặc filesystem state.

## 4. Workspace

Import dùng staging workspace cục bộ, ví dụ:

```text
meta/import/
├── manifest.json
├── intent.json
├── source.txt
├── guidance.txt
├── segmentation.json
├── confirmation.json
├── analyses/
├── range-digests/
├── synthesis.json
├── story-resolution.json
└── failures/
    ├── last.json
    └── last-response.txt
```

Mục đích:

- crash recovery;
- resumability;
- audit;
- deterministic revalidation;
- tránh publish trạng thái nửa vời vào Store chính.

## 5. Manifest và digest

Manifest lưu các fact local như:

- source name;
- raw digest;
- normalized digest;
- encoding;
- size;
- schema/version metadata.

Absolute source path không cần trở thành semantic memory và không được gửi lên website chỉ để phục hồi import.

Sau khi snapshot đã tồn tại, recovery ưu tiên snapshot + digest thay vì giả định file gốc vẫn ở cùng path.

## 6. Intent

Intent lưu các quyết định của người dùng cần tồn tại qua restart, ví dụ:

- auto-confirm segmentation có được phép hay không;
- story resolution đã được chọn nếu flow yêu cầu;
- có tiếp tục workflow sau import hay dừng ở gate.

Runner không được “đoán lại” intent từ output semantic.

## 7. Segmentation

### 7.1 Semantic responsibility

Gemini Web có thể được dùng để hiểu các boundary mở như:

- chapter title;
- volume title;
- preface/appendix/extra text;
- custom formatting không thể đóng bằng regex đơn giản.

### 7.2 Local responsibility

Code cục bộ phải xác minh:

- source ranges tăng dần;
- không overlap;
- coverage đầy đủ theo contract;
- stable anchors/digests khớp source snapshot;
- schema/type hợp lệ.

Semantic response không tự có quyền publish hoặc bỏ text.

### 7.3 Confirmation

Nếu pipeline yêu cầu người dùng xác nhận boundary, state phải dừng ở confirmation gate cho tới khi có intent hợp lệ.

## 8. Analyze

Chapter analysis được thực hiện theo batch/range có giới hạn thay vì gửi toàn bộ sách thành một prompt khổng lồ.

Mỗi semantic turn đi qua:

`Import Runner -> WebChatModel -> Gemini Web`

Output có thể chứa structured facts như:

- events;
- character state;
- world facts;
- chapter purpose;
- hooks/open threads;
- continuity information.

Code cục bộ kiểm tra schema, chapter reference và digest trước khi ghi staging artifact.

## 9. Synthesize

Sau khi chapter/range facts đủ, pipeline tổng hợp dần:

- premise;
- character model;
- world rules;
- arcs/volumes;
- story resolution;
- planning/continuation facts.

Synthesis phải dựa trên validated staged facts thay vì phụ thuộc vào một browser conversation vô hạn.

## 10. Publish

Không publish chính thức cho tới khi các artifact cần thiết đã pass local validation.

Publish phải:

- idempotent theo digest/state contract;
- dùng Store/commit semantics hiện có;
- không để crash giữa semantic analysis tạo ra project chính nửa hoàn chỉnh;
- giữ checkpoint/recovery evidence.

## 11. Recovery

Next action nên được suy ra từ artifact facts thay vì chỉ một mutable `stage` flag.

Ví dụ:

```text
missing snapshot/manifest    -> ingest
missing valid segmentation   -> segment
missing confirmation         -> wait/confirm
missing analysis             -> analyze first missing range
missing synthesis            -> synthesize
publish mismatch             -> validate/publish
all identities match         -> done
```

Nếu input digest thay đổi, artifact cũ không được tái sử dụng chỉ vì tên file giống nhau.

## 12. Guidance

Nếu import hỗ trợ natural-language guidance, guidance là một input semantic có version/digest và phải được tính vào reuse decision.

Thay đổi guidance có thể invalidate segmentation hoặc các artifact phụ thuộc theo contract tương ứng.

## 13. Browser/runtime identity

Current product không có:

- `import_segment` provider/model selector;
- `import_analyze` provider/model selector;
- `import_synthesize` provider/model selector;
- OpenAI/Anthropic/Gemini API configuration cho import;
- Ollama fallback;
- provider failover.

Mọi semantic stage dùng fixed browser-backed identity `web/gemini-web`.

Prompt/policy có thể khác nhau giữa segmentation/analyze/synthesize, nhưng transport/model runtime vẫn cố định.

## 14. Usage và cost semantics

Import có thể ghi local operational metrics như:

- số chapter/range đã xử lý;
- local payload/context estimate;
- stage duration;
- retry/failure count;
- browser readiness/failure category.

Không được diễn giải chúng thành:

- authoritative Gemini token usage;
- provider prompt-cache hit rate;
- USD API cost;
- API budget protection.

Gemini Web bridge không cung cấp billing telemetry cần thiết để đưa ra các khẳng định đó.

## 15. Failure capture

Khi semantic step fail, staging workspace có thể lưu response/error phục vụ chẩn đoán theo privacy policy hiện có.

Failure không được:

- kích hoạt API fallback;
- tự chuyển sang provider khác;
- silently publish partial semantic state;
- bỏ qua digest/schema mismatch.

## 16. Headless

Import có thể được điều khiển từ các entrypoint được code hỗ trợ, nhưng headless **không** thay semantic transport.

Nếu chạy không có TUI, setup browser/profile vẫn phải được hoàn thành trước. Gemini Web/browser-backed runtime vẫn là đường AI duy nhất.

## 17. Local-vs-web responsibility matrix

| Việc | Local Host/Code | Gemini Web |
|---|---:|---:|
| Đọc file nguồn | Có | Không |
| Decode/normalize | Có | Không |
| Tính SHA/digest | Có | Không |
| Xác minh coverage/range | Có | Không |
| Hiểu boundary mở | Kiểm tra | Có thể suy luận |
| Phân tích nội dung chương | Điều phối/validate | Có thể suy luận |
| Tổng hợp semantic | Điều phối/validate | Có thể suy luận |
| Ghi staging artifacts | Có | Không |
| Ghi Store chính | Có | Không |
| Thực hiện local tool | Có | Không |
| Đăng nhập Google | Người dùng trong browser | Website xử lý login UI |

## 18. Observability

Các điểm nên kiểm tra khi import lỗi:

- `meta/import/manifest.json`;
- source/guidance digest;
- segmentation coverage;
- confirmation/intent;
- first missing analysis range;
- synthesis identity;
- `failures/`;
- browser readiness;
- `/diag` và session logs khi phù hợp.

## 19. Maintainer invariants

Mọi thay đổi import phải giữ:

1. local source access only;
2. full coverage/no silent data loss theo contract;
3. semantic AI turn chỉ qua WebChatModel;
4. fixed `web/gemini-web` runtime identity;
5. no API credential/provider fallback;
6. validate before publish;
7. digest-based reuse/recovery;
8. no authoritative billing claims without provider evidence;
9. website không thực hiện filesystem side effect.

## 20. Historical note

Các revision cũ của import RFC có thể nhắc tới role-based model tiers, provider usage hoặc API-era assumptions. Những phần đó là lịch sử thiết kế. Current product contract được mô tả trong tài liệu này và bởi runtime/code hiện tại.