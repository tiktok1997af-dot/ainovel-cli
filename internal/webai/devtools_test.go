package webai

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

func fakeDevToolsProfile(t *testing.T, targetURL string, evalValue any) string {
	t.Helper()
	var wsURL string
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]devToolsTarget{{
			ID:                   "page-1",
			Type:                 "page",
			URL:                  targetURL,
			Title:                "Gemini",
			WebSocketDebuggerURL: wsURL,
		}})
	})
	mux.HandleFunc("/devtools/page/1", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var request struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		if request.Method != "Runtime.evaluate" {
			t.Errorf("CDP method = %q", request.Method)
		}
		_ = conn.WriteJSON(map[string]any{
			"id": request.ID,
			"result": map[string]any{
				"result": map[string]any{
					"type":  "object",
					"value": evalValue,
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	wsURL = "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/page/1"
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	profile := t.TempDir()
	if err := os.WriteFile(filepath.Join(profile, devToolsActivePortFile), []byte(port+"\n/devtools/browser/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return profile
}

func newFastGeminiProbe() *DevToolsReadinessProbe {
	probe := NewGeminiDevToolsReadinessProbe()
	probe.discoverTimeout = time.Second
	probe.pollInterval = time.Millisecond
	return probe
}

func TestCDPEvaluationTimeoutOutlivesGeminiSubmitBoundedWait(t *testing.T) {
	// geminiSubmitExpressionTemplate allows the page up to 6s to enable its send
	// control. CDP must never expire first or a valid delayed submit becomes an
	// ambiguous transport timeout.
	if cdpEvaluationTimeout <= 6*time.Second {
		t.Fatalf("cdpEvaluationTimeout = %s, must exceed Gemini submit bounded wait", cdpEvaluationTimeout)
	}
}

func TestDevToolsGeminiReadinessReady(t *testing.T) {
	profile := fakeDevToolsProfile(t, "https://gemini.google.com/app", map[string]any{
		"host":                "gemini.google.com",
		"path":                "/app",
		"has_account_control": true,
		"has_composer":        true,
		"has_sign_in":         false,
		"security_challenge":  false,
	})
	result, err := newFastGeminiProbe().Probe(context.Background(), SessionSnapshot{ProfileDir: profile})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if result.State != SessionReady {
		t.Fatalf("state = %s, want READY (%s)", result.State, result.Reason)
	}
}

func TestDevToolsGeminiPublicComposerStillRequiresAuth(t *testing.T) {
	profile := fakeDevToolsProfile(t, "https://gemini.google.com/app", map[string]any{
		"host":                "gemini.google.com",
		"path":                "/app",
		"has_account_control": false,
		"has_composer":        true,
		"has_sign_in":         true,
		"security_challenge":  false,
	})
	result, err := newFastGeminiProbe().Probe(context.Background(), SessionSnapshot{ProfileDir: profile})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if result.State != SessionAuthRequired {
		t.Fatalf("state = %s, want AUTH_REQUIRED (%s)", result.State, result.Reason)
	}
}

func TestDevToolsGeminiSignedInWithoutComposerIsDegraded(t *testing.T) {
	profile := fakeDevToolsProfile(t, "https://gemini.google.com/app", map[string]any{
		"host":                "gemini.google.com",
		"path":                "/app",
		"has_account_control": true,
		"has_composer":        false,
		"has_sign_in":         false,
		"security_challenge":  false,
	})
	result, err := newFastGeminiProbe().Probe(context.Background(), SessionSnapshot{ProfileDir: profile})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if result.State != SessionDegraded {
		t.Fatalf("state = %s, want DEGRADED (%s)", result.State, result.Reason)
	}
}

func TestDevToolsTargetWebSocketMustStayLoopback(t *testing.T) {
	for _, raw := range []string{
		"ws://example.com:9222/devtools/page/1",
		"wss://127.0.0.1:9222/devtools/page/1",
		"ws://127.0.0.1:9333/devtools/page/1",
	} {
		if _, err := safeDevToolsWebSocketURL(raw, 9222); err == nil {
			t.Fatalf("URL %q should be rejected", raw)
		}
	}
	if _, err := safeDevToolsWebSocketURL("ws://127.0.0.1:9222/devtools/page/1", 9222); err != nil {
		t.Fatalf("loopback DevTools URL rejected: %v", err)
	}
}

func TestSelectDevToolsTargetUsesSiteScore(t *testing.T) {
	adapter := sites.Gemini{}
	target, err := selectDevToolsTarget([]devToolsTarget{
		{Type: "page", URL: "https://example.com", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/1"},
		{Type: "page", URL: "https://accounts.google.com/signin?continue=https%3A%2F%2Fgemini.google.com%2Fapp", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/2"},
		{Type: "page", URL: "https://gemini.google.com/app", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/3"},
	}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if target.URL != "https://gemini.google.com/app" {
		t.Fatalf("selected target = %q", target.URL)
	}
}
