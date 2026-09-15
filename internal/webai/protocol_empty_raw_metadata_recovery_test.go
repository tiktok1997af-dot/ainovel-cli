package webai

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/agentcore"
)

func emptyRawToolResponseWithMetadata(metadata string) string {
	return rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\n"
}

func TestRawToolCallRecoversEmptyRawRegionFromDeclaredMetadataString(t *testing.T) {
	const content = "Chương 1\n\nMưa rơi trên mái ngói."
	raw := emptyRawToolResponseWithMetadata(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":"Chương 1\n\nMưa rơi trên mái ngói."},"raw_string_field":"content"}`)

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
		t.Fatalf("decode recovered args: %v", err)
	}
	if args.Chapter != 1 || args.Mode != "write" || args.Content != content {
		t.Fatalf("recovered args = %+v", args)
	}
}

func TestRawToolCallEmptyRawMetadataRecoveryRemainsFailClosed(t *testing.T) {
	nonStringRawFieldTool := rawContentToolSpec()
	nonStringRawFieldTool.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chapter": map[string]any{"type": "integer"},
			"content": map[string]any{"type": "integer"},
			"mode":    map[string]any{"type": "string"},
		},
	}

	tests := []struct {
		name  string
		raw   string
		tools []agentcore.ToolSpec
	}{
		{
			name:  "metadata string is whitespace only",
			raw:   emptyRawToolResponseWithMetadata(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":"   \n\t"},"raw_string_field":"content"}`),
			tools: []agentcore.ToolSpec{rawContentToolSpec()},
		},
		{
			name:  "metadata raw value is not a JSON string",
			raw:   emptyRawToolResponseWithMetadata(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":42},"raw_string_field":"content"}`),
			tools: []agentcore.ToolSpec{rawContentToolSpec()},
		},
		{
			name:  "raw field schema is not string",
			raw:   emptyRawToolResponseWithMetadata(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":42},"raw_string_field":"content"}`),
			tools: []agentcore.ToolSpec{nonStringRawFieldTool},
		},
		{
			name: "nonempty raw region plus metadata raw field is ambiguous",
			raw: rawToolCallPrefix + "\n" +
				`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write","content":"metadata"},"raw_string_field":"content"}` + "\n" +
				rawValueStart + "\nraw-region",
			tools: []agentcore.ToolSpec{rawContentToolSpec()},
		},
		{
			name:  "raw field leaked beside arguments remains rejected",
			raw:   emptyRawToolResponseWithMetadata(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content","content":"shadow"}`),
			tools: []agentcore.ToolSpec{rawContentToolSpec()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseResponseWithRawText("request", tt.raw, tt.tools)
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("err = %v, want ErrProtocol", err)
			}
		})
	}
}
