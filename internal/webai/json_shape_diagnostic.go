package webai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func annotateJSONSyntaxShape(payload string, err error) error {
	var webErr *Error
	if !errors.As(err, &webErr) || webErr == nil || webErr.Kind != ErrorProtocol || webErr.Op != "decode response" {
		return err
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(webErr.Cause, &syntaxErr) || syntaxErr == nil {
		return err
	}
	return protocolError("decode response", fmt.Errorf("%s: %w", jsonSyntaxShapeFingerprint(payload, syntaxErr.Offset), syntaxErr))
}

func jsonSyntaxShapeFingerprint(payload string, offset int64) string {
	idx := int(offset) - 1
	if idx < 0 {
		idx = 0
	}
	if idx > len(payload) {
		idx = len(payload)
	}

	stack := jsonStructuralStack(payload[:idx])
	expected := "none"
	if len(stack) > 0 {
		switch stack[len(stack)-1] {
		case '{':
			expected = "object_close"
		case '[':
			expected = "array_close"
		}
	}

	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf(
		"json_shape length=%d offset=%d prev=%s at=%s next=%s expected=%s stack=%s sha256=%s",
		len(payload), offset,
		jsonByteClassAt(payload, idx-1),
		jsonByteClassAt(payload, idx),
		jsonByteClassAt(payload, idx+1),
		expected,
		jsonStackName(stack),
		hex.EncodeToString(sum[:6]),
	)
}

func jsonStructuralStack(prefix string) []byte {
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	for i := 0; i < len(prefix); i++ {
		b := prefix[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch b {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if b == '"' {
			inString = true
			continue
		}
		switch b {
		case '{', '[':
			stack = append(stack, b)
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return stack
}

func jsonStackName(stack []byte) string {
	if len(stack) == 0 {
		return "empty"
	}
	parts := make([]string, 0, len(stack))
	for _, b := range stack {
		switch b {
		case '{':
			parts = append(parts, "object")
		case '[':
			parts = append(parts, "array")
		}
	}
	return strings.Join(parts, ">")
}

func jsonByteClassAt(payload string, idx int) string {
	if idx < 0 {
		return "bof"
	}
	if idx >= len(payload) {
		return "eof"
	}
	b := payload[idx]
	switch b {
	case '{':
		return "object_open"
	case '}':
		return "object_close"
	case '[':
		return "array_open"
	case ']':
		return "array_close"
	case ':':
		return "colon"
	case ',':
		return "comma"
	case '"':
		return "quote"
	case ' ', '\t', '\r', '\n':
		return "whitespace"
	case '-':
		return "minus"
	case '.':
		return "dot"
	}
	if b >= '0' && b <= '9' {
		return "digit"
	}
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
		return "alpha"
	}
	if b >= 0x80 {
		return "non_ascii"
	}
	return "other"
}
