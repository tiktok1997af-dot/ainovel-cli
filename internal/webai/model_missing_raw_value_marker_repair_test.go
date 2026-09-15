package webai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func rawDraftMissingValueStartMarker(prose string) string {
	return "TOOL_CALL_RAW\n" +
		`{"name":"draft_chapter","arguments":{"chapter":2,"mode":"write"},"raw_string_field":"content"}` +
		"\n" + prose
}

func TestModelRepairsMissingRawValueStartMarkerWithTargetedPrompt(t *testing.T) {
	const prose = "Mưa gõ lên mái tôn.\nNgười giao thư mở phong bì cuối cùng."
	transport := &fakeTransport{responses: []string{
		rawDraftMissingValueStartMarker(prose),
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
		"required raw-value start delimiter",
		rawValueStart,
		"same intended tool call",
		"preserve the same intended raw string value",
		"exactly one line containing",
		"No local tool from that answer has been executed",
	} {
		if !strings.Contains(repair, required) {
			t.Fatalf("targeted missing-marker repair prompt missing %q: %q", required, repair)
		}
	}
}

func TestModelMissingRawValueStartMarkerRepairRemainsBounded(t *testing.T) {
	const prose = "chapter prose that must not execute without the delimiter"
	malformed := rawDraftMissingValueStartMarker(prose)
	transport := &fakeTransport{responses: []string{
		malformed,
		malformed,
		malformed,
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
		if !strings.Contains(prompt, "required raw-value start delimiter") ||
			!strings.Contains(prompt, rawValueStart) ||
			!strings.Contains(prompt, "same intended tool call") {
			t.Fatalf("repair prompt %d was not targeted: %q", i+1, prompt)
		}
	}
}
