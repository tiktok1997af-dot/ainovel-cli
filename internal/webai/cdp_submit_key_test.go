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

func TestCDPPressEnterEmitsExactlyOneTrustedKeyDownUp(t *testing.T) {
	type observed struct {
		Method                string
		Type                  string
		Key                   string
		Code                  string
		WindowsVirtualKeyCode int
	}
	observedCh := make(chan []observed, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		got := make([]observed, 0, 2)
		for i := 0; i < 2; i++ {
			var request struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			var params struct {
				Type                  string `json:"type"`
				Key                   string `json:"key"`
				Code                  string `json:"code"`
				WindowsVirtualKeyCode int    `json:"windowsVirtualKeyCode"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return
			}
			got = append(got, observed{
				Method:                request.Method,
				Type:                  params.Type,
				Key:                   params.Key,
				Code:                  params.Code,
				WindowsVirtualKeyCode: params.WindowsVirtualKeyCode,
			})
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
	if err := evaluator.PressEnter(context.Background()); err != nil {
		t.Fatalf("PressEnter: %v", err)
	}

	got := <-observedCh
	if len(got) != 2 {
		t.Fatalf("events = %d, want keyDown+keyUp", len(got))
	}
	if got[0].Method != "Input.dispatchKeyEvent" || got[0].Type != "keyDown" || got[1].Method != "Input.dispatchKeyEvent" || got[1].Type != "keyUp" {
		t.Fatalf("unexpected CDP events: %+v", got)
	}
	for _, event := range got {
		if event.Key != "Enter" || event.Code != "Enter" || event.WindowsVirtualKeyCode != 13 {
			t.Fatalf("Enter key metadata drifted: %+v", got)
		}
	}
}
