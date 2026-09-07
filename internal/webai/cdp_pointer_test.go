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

func TestCDPPointerClickEmitsExactlyOneTrustedPressRelease(t *testing.T) {
	type observed struct {
		Method string
		Type   string
		X      float64
		Y      float64
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
				Type string  `json:"type"`
				X    float64 `json:"x"`
				Y    float64 `json:"y"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return
			}
			got = append(got, observed{Method: request.Method, Type: params.Type, X: params.X, Y: params.Y})
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
	if err := evaluator.Click(context.Background(), 123.5, 456.25); err != nil {
		t.Fatalf("Click: %v", err)
	}

	got := <-observedCh
	if len(got) != 2 {
		t.Fatalf("events = %d, want press+release", len(got))
	}
	if got[0].Method != "Input.dispatchMouseEvent" || got[0].Type != "mousePressed" || got[1].Method != "Input.dispatchMouseEvent" || got[1].Type != "mouseReleased" {
		t.Fatalf("unexpected CDP events: %+v", got)
	}
	for _, event := range got {
		if event.X != 123.5 || event.Y != 456.25 {
			t.Fatalf("coordinates drifted: %+v", got)
		}
	}
}
