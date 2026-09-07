package webai

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestDOMDelimitedTextBodyParsesWithoutOuterWrapper(t *testing.T) {
	msg, err := parseResponseWithRawText("request", "TEXT\nhello from Gemini Web", nil)
	if err != nil {
		t.Fatalf("parseResponseWithRawText: %v", err)
	}
	if got := msg.TextContent(); got != "hello from Gemini Web" {
		t.Fatalf("text = %q", got)
	}
	if msg.StopReason != agentcore.StopReasonStop {
		t.Fatalf("stop reason = %q", msg.StopReason)
	}
}

func TestDOMDelimitedSmallToolBodyUsesLockedToolValidation(t *testing.T) {
	raw := `{"kind":"tool_calls","tool_calls":[{"name":"draft_chapter","arguments":{"chapter":1,"content":"short","mode":"write"}}]}`
	msg, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("parseResponseWithRawText: %v", err)
	}
	calls := msg.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "draft_chapter" {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].Args, &args); err != nil {
		t.Fatalf("decode args: %v", err)
	}
	if args["content"] != "short" || args["mode"] != "write" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestDOMDelimitedRawToolUsesAssistantMessageEndAsValueBoundary(t *testing.T) {
	metadata := `{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content"}`
	content := "Chương 1\n\nMưa rơi trên mái tôn.\nNgười đưa thư dừng trước cửa."
	raw := rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\n" + content
	msg, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("parseResponseWithRawText: %v", err)
	}
	calls := msg.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d", len(calls))
	}
	var args struct {
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(calls[0].Args, &args); err != nil {
		t.Fatalf("decode args: %v", err)
	}
	if args.Chapter != 1 || args.Mode != "write" || args.Content != content {
		t.Fatalf("reconstructed args mismatch: %+v", args)
	}
}

func TestDOMDelimitedParserRetainsStrictLegacyCompatibility(t *testing.T) {
	legacy := wrappedResponse("TEXT\nlegacy response")
	msg, err := parseResponseWithRawText("request", legacy, nil)
	if err != nil {
		t.Fatalf("legacy parse: %v", err)
	}
	if got := msg.TextContent(); got != "legacy response" {
		t.Fatalf("legacy text = %q", got)
	}

	content := "legacy raw content"
	legacyRaw := rawToolResponse(`{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content"}`, content)
	msg, err = parseResponseWithRawText("request", legacyRaw, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("legacy raw parse: %v", err)
	}
	if len(msg.ToolCalls()) != 1 {
		t.Fatalf("legacy raw tool calls = %d", len(msg.ToolCalls()))
	}
}

func TestDOMDelimitedParserNeverReinterpretsPartialLegacyWrappers(t *testing.T) {
	cases := map[string]string{
		"missing end": responseStart + "\nTEXT\npartial",
		"stray end":   "TEXT\npartial\n" + responseEnd,
		"commentary":  "prefix\nTEXT\nhello",
		"markdown":    "```\nTEXT\nhello\n```",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseResponseWithRawText("request", raw, nil); !errors.Is(err, ErrProtocol) {
				t.Fatalf("err = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestDOMDelimitedParserRejectsUnknownOrAmbiguousBodies(t *testing.T) {
	badJSON := `{"choice":"continue"}`
	if _, err := parseResponseWithRawText("request", badJSON, nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("arbitrary JSON err = %v, want ErrProtocol", err)
	}

	unknownTool := `{"kind":"tool_calls","tool_calls":[{"name":"shell","arguments":{}}]}`
	if _, err := parseResponseWithRawText("request", unknownTool, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("unknown tool err = %v, want ErrProtocol", err)
	}

	if _, err := parseResponseWithRawText("request", "TEXT", nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("empty TEXT err = %v, want ErrProtocol", err)
	}
}

func TestDOMDelimitedRawToolRejectsNestedOrTrailingLegacyMarkers(t *testing.T) {
	metadata := `{"name":"draft_chapter","arguments":{"chapter":1,"mode":"write"},"raw_string_field":"content"}`
	nestedStart := rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\nbefore\n" + rawValueStart + "\nafter"
	if _, err := parseResponseWithRawText("request", nestedStart, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("nested raw start err = %v, want ErrProtocol", err)
	}

	trailing := rawToolCallPrefix + "\n" + metadata + "\n" + rawValueStart + "\nvalue\n" + rawValueEnd + "\ntrailing"
	if _, err := parseResponseWithRawText("request", trailing, []agentcore.ToolSpec{rawContentToolSpec()}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("trailing raw end err = %v, want ErrProtocol", err)
	}
}

func TestDOMDelimitedTextPreservesJSONAndMarkdownAsTextAfterTEXTPrefix(t *testing.T) {
	body := "TEXT\n{\"not\":\"a tool\"}\n\n# Markdown\n- item"
	msg, err := parseResponseWithRawText("request", body, nil)
	if err != nil {
		t.Fatalf("parseResponseWithRawText: %v", err)
	}
	got := msg.TextContent()
	if !strings.Contains(got, `{"not":"a tool"}`) || !strings.Contains(got, "# Markdown") {
		t.Fatalf("text content was altered: %q", got)
	}
}
