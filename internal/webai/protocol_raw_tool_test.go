package webai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func rawContentToolSpec() agentcore.ToolSpec {
	strict := true
	return agentcore.ToolSpec{
		Name:        "draft_chapter",
		Description: "save a full chapter draft",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapter": map[string]any{"type": "integer"},
				"content": map[string]any{"type": "string"},
				"mode":    map[string]any{"type": "string", "enum": []string{"write", "append"}},
			},
			"required":             []string{"chapter", "content", "mode"},
			"additionalProperties": false,
		},
		Strict: &strict,
	}
}

func rawToolResponse(metadata, value string) string {
	return wrappedResponse(rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\n" + value + "\n" + rawValueEnd)
}

func TestRawToolCallReconstructsLongStringArgumentVerbatim(t *testing.T) {
	content := "Chương 1\n\nLan nói: \"Đừng mở thư.\"\nMưa gõ lên mái tôn."
	raw := rawToolResponse(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content"}`, content)
	msg, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("parseResponseWithRawText: %v", err)
	}
	calls := msg.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "draft_chapter" {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
	var args struct {
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(calls[0].Args, &args); err != nil {
		t.Fatalf("unmarshal reconstructed args: %v", err)
	}
	if args.Chapter != 1 || args.Mode != "write" || args.Content != content {
		t.Fatalf("reconstructed args mismatch: %+v", args)
	}
	if msg.StopReason != agentcore.StopReasonToolUse {
		t.Fatalf("stop reason = %q", msg.StopReason)
	}
}

func TestRawToolCallRejectsUnknownToolAndDuplicateRawField(t *testing.T) {
	unknown := rawToolResponse(`{"name":"shell","arguments":{},"raw_string_field":"content"}`, "x")
	if _, err := parseResponseWithRawText("request", unknown, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("unknown tool err = %v, want ErrProtocol", err)
	}

	duplicate := rawToolResponse(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":"already here"},"raw_string_field":"content"}`, "x")
	if _, err := parseResponseWithRawText("request", duplicate, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("duplicate raw field err = %v, want ErrProtocol", err)
	}
}

func TestRawToolCallRejectsTrailingOrNestedRawMarkers(t *testing.T) {
	metadata := `{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content"}`
	trailing := wrappedResponse(rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\nx\n" + rawValueEnd + "\nextra")
	if _, err := parseResponseWithRawText("request", trailing, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("trailing content err = %v, want ErrProtocol", err)
	}

	nested := rawToolResponse(metadata, "before\n"+rawValueStart+"\nafter")
	if _, err := parseResponseWithRawText("request", nested, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("nested marker err = %v, want ErrProtocol", err)
	}
}

func TestModelRepairsMalformedLongToolCallIntoRawToolForm(t *testing.T) {
	content := "Một chương dài có dấu \"ngoặc kép\" và nhiều dòng.\nDòng hai."
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":"Một chương "lỗi""}}]}`),
		rawToolResponse(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content"}`, content),
	}}
	model := mustModel(t, transport)
	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("write chapter 1")}, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	calls := resp.Message.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d", len(calls))
	}
	if !strings.Contains(string(calls[0].Args), `"content"`) {
		t.Fatalf("reconstructed args missing content: %s", calls[0].Args)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 2 {
		t.Fatalf("round trips = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[0], rawToolCallPrefix) || !strings.Contains(prompts[0], rawValueStart) {
		t.Fatal("initial request did not advertise raw tool transport")
	}
	if !strings.Contains(prompts[1], rawToolCallPrefix) || !strings.Contains(prompts[1], "long top-level string") {
		t.Fatal("repair prompt did not advertise raw tool transport")
	}
	if strings.Contains(prompts[1], responseStart) || strings.Contains(prompts[1], responseEnd) {
		t.Fatal("repair prompt must not echo literal outer response markers")
	}
}
