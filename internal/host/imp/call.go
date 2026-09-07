package imp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	"github.com/voocel/ainovel-cli/internal/llmretry"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// callModel 是内核对模型的最小依赖，便于测试注入 mock。
type callModel interface {
	Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error)
}

// errTruncated 表示模型因长度停止（容量错误）。携带原始文本供调用方决定失败或前缀打捞（§9.5）。
type errTruncated struct {
	Raw string
}

func (e *errTruncated) Error() string { return "模型输出被长度截断（stop=length）" }

// errSemantic 表示无法通过重问修复的输出层失败，携带原始响应，
// 供 runner 统一落 failures/ 失败工件（§14.2），所有语义函数共用。
type errSemantic struct {
	Raw string
	Err error
}

func (e *errSemantic) Error() string { return e.Err.Error() }
func (e *errSemantic) Unwrap() error { return e.Err }

// callProfile 承载 thinking 与可观测性选项，由 Host 探测的 ModelRuntime 派生。
// 结构化协议由 callStructured 依据模型事实和静态 Contract 独立选择。
type callProfile struct {
	thinking agentcore.ThinkingLevel
	// notify 可选：把请求退避重试/校验重问回显给界面；nil 时静默（§14.1）。
	// retryAt 非零 = 下次重试的截止时刻，UI 据此渲染逐秒倒计时（事件只带截止点，剩余时间渲染时算）。
	notify func(msg string, retryAt time.Time)
	// progress 可选：回显长时阶段的内部推进（切分第 N/M 块、区间摘要 N/M）；nil 时静默。
	// 切分/综合在函数内部逐块/逐区间调用模型，单块可达数分钟，没有它面板整段静默像卡死（§14.1）。
	progress func(current, total int, msg string)
	// log 可选：导入专属日志（logs/import.log）；nil 回退默认 logger。
	log *slog.Logger
}

func (p callProfile) logger() *slog.Logger {
	if p.log != nil {
		return p.log
	}
	return slog.Default()
}

// step 回显一条普通进度（长时阶段的内部推进）。
func (p callProfile) step(current, total int, format string, args ...any) {
	if p.progress != nil {
		p.progress(current, total, fmt.Sprintf(format, args...))
	}
}

// say 回显一条长时调用状态。重试可能静默数分钟（指数退避累计 2 分钟以上），
// 不回显用户会误以为卡死。
func (p callProfile) say(format string, args ...any) {
	p.sayRetry(time.Time{}, format, args...)
}

// sayRetry 回显一条带重试截止时刻的状态，供 UI 倒计时。
func (p callProfile) sayRetry(retryAt time.Time, format string, args ...any) {
	if p.notify != nil {
		p.notify(fmt.Sprintf(format, args...), retryAt)
	}
}

// snippet 把多行文本压成单行短摘要供界面回显：合并空白、截到 max 个 rune。
func snippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// briefErr 把错误压成单行短文本供界面回显（完整错误链仍走日志与失败工件）。
// 适配器结构化事实放前面：截断时优先保住"哪类错、什么状态码"，网关 message 可牺牲。
func briefErr(err error) string {
	s := err.Error()
	if d := modelErrDetail(err); d != "" {
		s = d + "：" + s
	}
	return snippet(s, 100)
}

// modelErrDetail extracts browser-transport facts without depending on an API
// provider implementation. The web adapter preserves the actionable failure
// classes needed by import retry/error reporting.
func modelErrDetail(err error) string {
	var webErr *webai.Error
	if !errors.As(err, &webErr) {
		return ""
	}
	parts := make([]string, 0, 2)
	if webErr.Kind != "" {
		parts = append(parts, string(webErr.Kind))
	}
	if webErr.Op != "" {
		parts = append(parts, webErr.Op)
	}
	return strings.Join(parts, "，")
}

// callOptions 组装本次调用的 CallOption：始终带输出上限；按能力可选 thinking。
// thinking 仅在非 Auto 时发送——对不支持 thinking 的模型发任何等级（含 off）都是非法参数（与 arbiter 同策略）。
func (p callProfile) callOptions(maxTokens int) []agentcore.CallOption {
	opts := []agentcore.CallOption{agentcore.WithMaxTokens(maxTokens)}
	if p.thinking != agentcore.ThinkingAuto {
		opts = append(opts, agentcore.WithThinking(p.thinking))
	}
	return opts
}

// callStructured 为导入层适配统一结构化执行器，并把通用失败映射为导入工件语义。
func callStructured[T any](ctx context.Context, m callModel, contract llmcontract.Contract, systemPrompt, payload string, maxTokens int, prof callProfile, validate func(*T) error) (T, error) {
	out, err := llmcontract.Execute(ctx, m, llmcontract.Request[T]{
		Contract:     contract,
		SystemPrompt: systemPrompt,
		Payload:      payload,
		Options:      prof.callOptions(maxTokens),
		Validate:     validate,
		Agent:        "import",
		Hooks: llmcontract.Hooks{
			Resolved: func(res llmcontract.Resolution) {
				prof.logger().Debug("imp 结构化协议选择",
					"contract", contract.Name, "structured_mode", res.Mode,
					"capability_source", res.Source, "provider", res.Provider,
					"model", res.Model, "schema_fingerprint", contract.Fingerprint())
			},
			RequestRetry: func(ev llmretry.Event) {
				prof.sayRetry(time.Now().Add(ev.Delay), "模型请求失败（%s），进行第 %d 次重试", briefErr(ev.Err), ev.Attempt)
				prof.logger().Warn("imp 模型请求重试", "attempt", ev.Attempt, "delay", ev.Delay, "err", ev.Err)
			},
			Correction: func(ev llmcontract.Correction) {
				prof.say("输出校验未通过（%s），带错误反馈进行第 %d 次重问", briefErr(ev.Err), ev.Attempt+1)
				prof.logger().Warn("imp 结构化输出自愈", "attempt", ev.Attempt,
					"layer", ev.Layer, "structured_mode", ev.Mode, "err", ev.Err)
			},
		},
	})
	if err == nil {
		return out, nil
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	var failure *llmcontract.Failure
	if !errors.As(err, &failure) {
		return out, fmt.Errorf("imp: %w", err)
	}
	switch failure.Kind {
	case llmcontract.FailureLength:
		return out, &errTruncated{Raw: failure.Raw}
	case llmcontract.FailureSafety, llmcontract.FailureContract, llmcontract.FailureProtocol:
		if failure.Raw != "" {
			return out, &errSemantic{Raw: failure.Raw, Err: fmt.Errorf("imp: %w", failure)}
		}
	case llmcontract.FailureRequest:
		if detail := modelErrDetail(failure); detail != "" {
			return out, fmt.Errorf("imp: 模型调用失败（%s）：%w", detail, failure)
		}
	}
	return out, fmt.Errorf("imp: %w", failure)
}
