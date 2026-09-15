package webai

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/agentcore"
)

func bareRawToolResponse(metadata, value string) string {
	return rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\n" + value
}

func TestRawToolCallRecoversDeclaredArgumentsWhenArgumentsWrapperIsOmitted(t *testing.T) {
	const content = "Chương 1\n\nMưa rơi trên mái ngói."
	raw := bareRawToolResponse(
		`{"name":"draft_chapter","chapter":1,"mode":"write","raw_string_field":"content"}`,
		content,
	)

	msg, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("parseResponseWithRawText: %v", err)
	}
	calls := msg.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "draft_chapter" {
		t.Fatalf("tool calls = %+v, want one draft_chapter", calls)
	}
	var args struct {
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(calls[0].Args, &args); err != nil {
		t.Fatalf("decode reconstructed args: %v", err)
	}
	if args.Chapter != 1 || args.Mode != "write" || args.Content != content {
		t.Fatalf("reconstructed args = %+v", args)
	}
}

func TestRawToolCallMissingArgumentsWrapperRecoveryRemainsFailClosed(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
	}{
		{
			name:     "undeclared top-level argument",
			metadata: `{"name":"draft_chapter","chapter":1,"mode":"write","chapters":1,"raw_string_field":"content"}`,
		},
		{
			name:     "raw field leaked into metadata",
			metadata: `{"name":"draft_chapter","chapter":1,"mode":"write","raw_string_field":"content","content":"shadow"}`,
		},
		{
			name:     "explicit null arguments",
			metadata: `{"name":"draft_chapter","arguments":null,"chapter":1,"mode":"write","raw_string_field":"content"}`,
		},
		{
			name:     "explicit array arguments",
			metadata: `{"name":"draft_chapter","arguments":[],"chapter":1,"mode":"write","raw_string_field":"content"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := bareRawToolResponse(tt.metadata, "x")
			_, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{rawContentToolSpec()})
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("err = %v, want ErrProtocol", err)
			}
		})
	}
}
