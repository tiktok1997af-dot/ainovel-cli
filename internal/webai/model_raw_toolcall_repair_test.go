package webai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func rawDraftToolSpec() agentcore.ToolSpec {
	strict := true
	return agentcore.ToolSpec{
		Name:        "draft_chapter",
		Description: "persist a chapter draft",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapter": map[string]any{"type": "integer"},
				"content": map[string]any{"type": "string"},
				"mode": map[string]any{"type": "string", "enum": []string{"write", "append"}},
			},
			"required":             []string{"chapter", "content", "mode"},
			"additionalProperties": false,
		},
		Strict: &strict,
	}
}

func malformedRawDraft(field, prose string) string {
	return "TOOL_CALL_RAW\n" +
		`{"name":"draft_chapter","arguments":{"chapter":2,"mode":"write"},"raw_string_field":` +
		strconvQuote(field) + "}\n" + rawValueStart + "\n" + prose
}

func validRawDraft(prose string) string {
	return "TOOL_CALL_RAW\n" +
		`{"name":"draft_chapter","arguments":{"chapter":2,"mode":"write"},"raw_string_field":"content"}` +
		"\n" + rawValueStart + "\n" + prose
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestModelRepairsInvalidRawStringFieldWithTargetedPrompt(t *testing.T) {
	const prose = "Mưa gõ lên mái tôn.\nNgười giao thư mở phong bì cuối cùng."
	transport := &fakeTransport{responses: []string{
		malformedRawDraft(" content ", prose),
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
		"raw_string_field",
		"invalid",
		"leading or trailing whitespace",
		"metadata arguments must omit",
		"same intended tool call",
		"same raw string value",
	} {
		if !strings.Contains(repair, required) {
			t.Fatalf("targeted raw repair prompt missing %q: %q", required, repair)
		}
	}
	if strings.Contains(repair, `raw_string_field":"content"`) {
		t.Fatal("repair prompt must stay schema-generic; it must not hard-code draft_chapter/content")
	}
}

func TestModelInvalidRawStringFieldRepairRemainsBounded(t *testing.T) {
	const prose = "chapter prose"
	transport := &fakeTransport{responses: []string{
		malformedRawDraft(" content ", prose),
		malformedRawDraft("\tcontent", prose),
		malformedRawDraft("content\n", prose),
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
		if !strings.Contains(prompt, "raw_string_field") || !strings.Contains(prompt, "leading or trailing whitespace") {
			t.Fatalf("repair prompt %d was not targeted: %q", i+1, prompt)
		}
	}
}
