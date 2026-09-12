package webai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func emptyRawDraftForRepair() string {
	return validRawDraft("")
}

func TestModelRepairsEmptyRawStringValueWithTargetedPrompt(t *testing.T) {
	const prose = "Mưa gõ lên mái tôn.\nNgười giao thư mở bức thư cuối cùng."
	transport := &fakeTransport{responses: []string{
		emptyRawDraftForRepair(),
		validRawDraft(prose),
	}}
	model := mustModel(t, transport)

	resp, err := model.Generate(
		context.Background(),
		[]agentcore.Message{agentcore.UserMsg("write chapter 2")},
		[]agentcore.ToolSpec{rawDraftToolSpec()},
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	calls := resp.Message.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "draft_chapter" {
		t.Fatalf("tool calls = %+v, want one draft_chapter", calls)
	}
	var args struct {
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(calls[0].Args, &args); err != nil {
		t.Fatalf("decode repaired args: %v", err)
	}
	if args.Chapter != 2 || args.Mode != "write" || args.Content != prose {
		t.Fatalf("repaired args = %+v; prose preserved=%t", args, args.Content == prose)
	}

	prompts := transport.promptSnapshot()
	if len(prompts) != 2 {
		t.Fatalf("round trips = %d, want 2", len(prompts))
	}
	repair := prompts[1]
	for _, required := range []string{
		"raw string value was empty or whitespace",
		"same intended tool call",
		"complete non-empty intended raw string value",
		"do not stop immediately after the delimiter",
		"assistant message may end only after the complete raw value",
	} {
		if !strings.Contains(repair, required) {
			t.Fatalf("targeted empty-raw repair prompt missing %q: %q", required, repair)
		}
	}
}

func TestModelEmptyRawStringValueRepairRemainsBounded(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		emptyRawDraftForRepair(),
		emptyRawDraftForRepair(),
		emptyRawDraftForRepair(),
	}}
	model := mustModel(t, transport)

	_, err := model.Generate(
		context.Background(),
		[]agentcore.Message{agentcore.UserMsg("write chapter 2")},
		[]agentcore.ToolSpec{rawDraftToolSpec()},
	)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
	prompts := transport.promptSnapshot()
	if got, want := len(prompts), 1+maxProtocolFormatRepairs; got != want {
		t.Fatalf("round trips = %d, want %d", got, want)
	}
	for i, prompt := range prompts[1:] {
		if !strings.Contains(prompt, "raw string value was empty or whitespace") ||
			!strings.Contains(prompt, "complete non-empty intended raw string value") {
			t.Fatalf("repair prompt %d was not targeted: %q", i+1, prompt)
		}
	}
}
