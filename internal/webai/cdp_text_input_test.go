package webai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestCDPReplaceTextUsesTrustedFocusSelectClearInsertWithoutSubmit(t *testing.T) {
	type observed struct {
		Method string
		Type   string
		Key    string
		Text   string
	}
	observedCh := make(chan []observed, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		got := make([]observed, 0, 7)
		for i := 0; i < 7; i++ {
			var request struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			var params struct {
				Type string `json:"type"`
				Key  string `json:"key"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return
			}
			got = append(got, observed{Method: request.Method, Type: params.Type, Key: params.Key, Text: params.Text})
			if err := conn.WriteJSON(map[string]any{"id": request.ID, "result": map[string]any{}}); err != nil {
				return
			}
		}
		observedCh <- got
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	evaluator, err := newCDPEvaluator(context.Background(), nil, wsURL)
	if err != nil {
		t.Fatalf("newCDPEvaluator: %v", err)
	}
	defer evaluator.Close()
	prompt := "line one\nline two"
	if err := evaluator.ReplaceText(context.Background(), 120, 240, prompt); err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}

	got := <-observedCh
	if len(got) != 7 {
		t.Fatalf("events = %d, want 7", len(got))
	}
	wantMethods := []string{
		"Input.dispatchMouseEvent",
		"Input.dispatchMouseEvent",
		"Input.dispatchKeyEvent",
		"Input.dispatchKeyEvent",
		"Input.dispatchKeyEvent",
		"Input.dispatchKeyEvent",
		"Input.insertText",
	}
	for i, want := range wantMethods {
		if got[i].Method != want {
			t.Fatalf("event %d method = %q, want %q; all=%+v", i, got[i].Method, want, got)
		}
	}
	if got[2].Type != "rawKeyDown" || got[2].Key != "a" || got[3].Type != "keyUp" || got[3].Key != "a" {
		t.Fatalf("Ctrl+A sequence drifted: %+v", got[2:4])
	}
	if got[4].Type != "rawKeyDown" || got[4].Key != "Backspace" || got[5].Type != "keyUp" || got[5].Key != "Backspace" {
		t.Fatalf("Backspace sequence drifted: %+v", got[4:6])
	}
	if got[6].Text != prompt {
		t.Fatalf("inserted text mismatch: %q", got[6].Text)
	}
	for _, event := range got {
		if event.Key == "Enter" {
			t.Fatal("ReplaceText must never submit by pressing Enter")
		}
	}
}
