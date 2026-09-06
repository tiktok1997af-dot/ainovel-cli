package imp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// flakyModel 前 fails 次返回可重试错误，之后按 mockModel 响应。
type flakyModel struct {
	mockModel
	fails int
}

func (f *flakyModel) Generate(ctx context.Context, msgs []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	if f.fails > 0 {
		f.fails--
		return nil, fastRetryErr{}
	}
	return f.mockModel.Generate(ctx, msgs, tools, opts...)
}

// fastRetryErr 可重试且退避极短（RetryAfter 命中 RetryHinter），保证测试快速。
type fastRetryErr struct{}

func (fastRetryErr) Error() string             { return "rate limited" }
func (fastRetryErr) Retryable() bool           { return true }
func (fastRetryErr) RetryAfter() time.Duration { return time.Millisecond }

// TestCallStructuredNotifiesRetries 守护重试可见性：请求退避与校验重问都必须回显，
// 否则指数退避可静默数分钟，用户会误以为导入卡死（截图问题：3 分钟无声后才报错）。
// 请求退避还必须携带非零 retryAt 截止时刻——UI 倒计时依赖它；校验重问即时发生，retryAt 为零。
func TestCallStructuredNotifiesRetries(t *testing.T) {
	m := &flakyModel{mockModel: mockModel{responses: []string{"不是 JSON", `{"boundaries":[]}`}}, fails: 2}
	var notes []string
	var retries, reasks int
	prof := callProfile{notify: func(s string, retryAt time.Time) {
		notes = append(notes, s)
		if !retryAt.IsZero() {
			retries++
		}
		if strings.Contains(s, "重问") {
			reasks++
		}
	}}
	if _, err := callStructured[boundaryBatch](context.Background(), m, segmentContract, "sys", "p", 100, prof, nil); err != nil {
		t.Fatalf("最终应成功：%v", err)
	}
	if retries != 2 || reasks != 1 {
		t.Fatalf("应回显 2 次带截止时刻的请求退避 + 1 次校验重问，得 %d/%d：%v", retries, reasks, notes)
	}
}

// TestBriefErrIncludesWebTransportFacts 守护 WEB-only 错误回显的可诊断性：
// importer 只依赖 browser transport taxonomy，不依赖 HTTP provider/LiteLLM 类型。
func TestBriefErrIncludesWebTransportFacts(t *testing.T) {
	webErr := &webai.Error{
		Kind:  webai.ErrorTransport,
		Op:    "gemini.submit",
		Cause: errors.New("connection refused"),
		Retry: true,
	}
	got := briefErr(fmt.Errorf("外层包装：%w", webErr))
	for _, want := range []string{"transport", "gemini.submit", "connection refused"} {
		if !strings.Contains(got, want) {
			t.Fatalf("回显应包含 %q，得 %q", want, got)
		}
	}
	if !strings.HasPrefix(got, "transport") {
		t.Fatalf("WEB transport facts 应在前，得 %q", got)
	}
	if got := briefErr(errors.New("普通错误")); got != "普通错误" {
		t.Fatalf("非适配器错误应保持原样，得 %q", got)
	}
}

// TestCallStructuredCancelIsNotSemanticFailure 守护取消语义：用户取消（Esc）不是语义失败，
// 不得包装成「N 次尝试」的 errSemantic——那会误导排查方向并多落一份误导性 failures/ 工件。
func TestCallStructuredCancelIsNotSemanticFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &mockModel{responses: []string{"垃圾输出"}}
	_, err := callStructured[boundaryBatch](ctx, m, segmentContract, "sys", "p", 100, callProfile{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled，得 %v", err)
	}
	var se *errSemantic
	if errors.As(err, &se) {
		t.Fatal("取消不应被包装成语义失败")
	}
}

// TestCallStructuredCarriesRawOnSemanticFailure 守护 §14.2：输出层契约违约时，
// 错误必须携带原始响应，供 runner 统一落 failures/ 失败工件。
func TestCallStructuredCarriesRawOnSemanticFailure(t *testing.T) {
	m := &nativeImportModel{mockModel: &mockModel{responses: []string{"垃圾输出 not json"}}}
	_, err := callStructured[boundaryBatch](context.Background(), m, segmentContract, "sys", "payload", 100, callProfile{}, nil)
	var se *errSemantic
	if !errors.As(err, &se) {
		t.Fatalf("应返回 errSemantic，得 %T：%v", err, err)
	}
	if se.Raw != "垃圾输出 not json" || !strings.Contains(se.Error(), "契约违约") {
		t.Fatalf("Raw 应携带最后一次原始响应，得 %q", se.Raw)
	}
}

func TestCallStructuredCarriesRawOnProtocolFailure(t *testing.T) {
	m := &nativeImportModel{mockModel: &mockModel{
		responses: []string{"upstream malformed output"},
		stops:     []agentcore.StopReason{agentcore.StopReasonError},
	}}
	_, err := callStructured[boundaryBatch](context.Background(), m, segmentContract, "sys", "payload", 100, callProfile{}, nil)
	var se *errSemantic
	if !errors.As(err, &se) || se.Raw != "upstream malformed output" || !strings.Contains(se.Error(), "stop_reason=error") {
		t.Fatalf("协议错误应携带原始响应，得 %T：%v", err, err)
	}
}
