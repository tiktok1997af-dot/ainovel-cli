package webai

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestJSONSyntaxShapeFingerprintClassifiesMismatchedArrayCloserWithoutContentLeak(t *testing.T) {
	payload := `{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":{"chapter":1}}}`
	var target any
	err := json.Unmarshal([]byte(payload), &target)
	if err == nil {
		t.Fatal("expected malformed JSON")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected *json.SyntaxError, got %T: %v", err, err)
	}

	fingerprint := jsonSyntaxShapeFingerprint(payload, syntaxErr.Offset)
	for _, required := range []string{
		"json_shape ",
		"at=object_close",
		"expected=array_close",
		"stack=object>array",
		"sha256=",
	} {
		if !strings.Contains(fingerprint, required) {
			t.Fatalf("fingerprint %q missing %q", fingerprint, required)
		}
	}
	for _, forbidden := range []string{"save_chapter", "tool_calls", "chapter"} {
		if strings.Contains(fingerprint, forbidden) {
			t.Fatalf("fingerprint leaked payload content %q: %s", forbidden, fingerprint)
		}
	}
}

func TestAnnotateJSONSyntaxShapePreservesTypedSyntaxError(t *testing.T) {
	payload := `{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":{"chapter":1}}}`
	var target any
	syntaxErr := json.Unmarshal([]byte(payload), &target)
	wrapped := protocolError("decode response", syntaxErr)
	annotated := annotateJSONSyntaxShape(payload, wrapped)

	if !errors.Is(annotated, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", annotated)
	}
	var typed *json.SyntaxError
	if !errors.As(annotated, &typed) {
		t.Fatalf("typed *json.SyntaxError must survive annotation: %v", annotated)
	}
	text := annotated.Error()
	if !strings.Contains(text, "json_shape ") || !strings.Contains(text, "expected=array_close") {
		t.Fatalf("shape diagnostic missing from error: %s", text)
	}
	if strings.Contains(text, "save_chapter") {
		t.Fatalf("annotated error leaked payload content: %s", text)
	}
}
