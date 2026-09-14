package webai

import (
	"encoding/json"
	"testing"

	"github.com/voocel/agentcore"
)

type typedToolSchemaCompat struct {
	Type       string                         `json:"type"`
	Properties map[string]typedPropertyCompat `json:"properties"`
}

type typedPropertyCompat struct {
	Type string `json:"type"`
}

func TestRawToolCallMetadataRecoversDeclaredChapterFromTypedSchema(t *testing.T) {
	tool := agentcore.ToolSpec{
		Name: "draft_chapter",
		Parameters: typedToolSchemaCompat{
			Type: "object",
			Properties: map[string]typedPropertyCompat{
				"chapter": {Type: "integer"},
				"content": {Type: "string"},
				"mode":    {Type: "string"},
			},
		},
	}
	metadata, err := decodeRawToolCallMetadata(
		`{"name":"draft_chapter","arguments":{"mode":"write"},"raw_string_field":"content","chapter":21}`,
		[]agentcore.ToolSpec{tool},
	)
	if err != nil {
		t.Fatalf("decodeRawToolCallMetadata: %v", err)
	}
	if got := string(metadata.Arguments["chapter"]); got != "21" {
		t.Fatalf("chapter = %s, want 21", got)
	}
	if got := string(metadata.Arguments["mode"]); got != `"write"` {
		t.Fatalf("mode = %s, want write", got)
	}
}

func TestRawToolCallMetadataRecoversDeclaredScaleFromRawJSONSchema(t *testing.T) {
	tool := agentcore.ToolSpec{
		Name: "architect_short",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"scale":{"type":"string"},
				"content":{"type":"string"}
			}
		}`),
	}
	metadata, err := decodeRawToolCallMetadata(
		`{"name":"architect_short","arguments":{},"raw_string_field":"content","scale":"short"}`,
		[]agentcore.ToolSpec{tool},
	)
	if err != nil {
		t.Fatalf("decodeRawToolCallMetadata: %v", err)
	}
	if got := string(metadata.Arguments["scale"]); got != `"short"` {
		t.Fatalf("scale = %s, want short", got)
	}
}

func TestRawToolCallMetadataJSONNormalizedSchemaStillRejectsUndeclaredKey(t *testing.T) {
	tool := agentcore.ToolSpec{
		Name:       "draft_chapter",
		Parameters: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer"},"content":{"type":"string"}}}`),
	}
	if _, err := decodeRawToolCallMetadata(
		`{"name":"draft_chapter","arguments":{},"raw_string_field":"content","chapters":21}`,
		[]agentcore.ToolSpec{tool},
	); err == nil {
		t.Fatal("undeclared metadata key must fail closed")
	}
}
