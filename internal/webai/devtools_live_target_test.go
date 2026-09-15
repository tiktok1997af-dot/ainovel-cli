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
	"strconv"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

func TestWaitForLiveDevToolsTargetSurvivesStalePortRewrite(t *testing.T) {
	profileDir := t.TempDir()
	locator := filepath.Join(profileDir, devToolsActivePortFile)

	staleListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stalePort := staleListener.Addr().(*net.TCPAddr).Port
	if err := staleListener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(locator, []byte(strconv.Itoa(stalePort)+"\n/devtools/browser/stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]devToolsTarget{{
			ID:                   "page-live",
			Type:                 "page",
			URL:                  "https://gemini.google.com/app",
			Title:                "Gemini",
			WebSocketDebuggerURL: "ws://127.0.0.1:1/devtools/page/live",
		}})
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	livePort, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	rewriteDone := make(chan error, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		rewriteDone <- os.WriteFile(locator, []byte(strconv.Itoa(livePort)+"\n/devtools/browser/live\n"), 0o600)
	}()

	port, target, err := waitForLiveDevToolsTarget(
		context.Background(),
		profileDir,
		server.Client(),
		sites.Gemini{},
		750*time.Millisecond,
		5*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForLiveDevToolsTarget: %v", err)
	}
	if err := <-rewriteDone; err != nil {
		t.Fatalf("rewrite DevToolsActivePort: %v", err)
	}
	if port != livePort {
		t.Fatalf("port = %d, want rewritten live port %d (stale was %d)", port, livePort, stalePort)
	}
	if target.ID != "page-live" {
		t.Fatalf("target ID = %q, want page-live", target.ID)
	}
}
