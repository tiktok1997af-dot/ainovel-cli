package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/webai"
)

const (
	expectedText        = "W3_LIVE_OK"
	initialReadyTimeout = 45 * time.Second
)

type w2EvidenceSummary struct {
	Schema      string `json:"schema"`
	ProfileName string `json:"profile_name"`
	Result      string `json:"result"`
}

type w3Evidence struct {
	Schema       string               `json:"schema"`
	ProfileName  string               `json:"profile_name"`
	StartedAt    time.Time            `json:"started_at"`
	CompletedAt  time.Time            `json:"completed_at"`
	Result       string               `json:"result"`
	States       []webai.SessionState `json:"states"`
	OutputSHA256 string               `json:"output_sha256"`
	OutputLength int                  `json:"output_length"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("ainovel-w3-verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", "", "Đường dẫn Chrome (để trống để tự tìm)")
	profile := fs.String("profile", "", "Tên W2 profile đã đăng nhập; để trống để tự tìm evidence W2 PASS mới nhất")
	timeout := fs.Duration("timeout", 2*time.Minute, "Thời gian tối đa cho một prompt thật")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "ainovel-w3-verify không nhận tham số vị trí")
		return 2
	}

	profileName := strings.TrimSpace(*profile)
	if profileName == "" {
		var err error
		profileName, err = latestPassedW2Profile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "W3E FAILED: %v\n", err)
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	started := time.Now().UTC()
	fmt.Println("W3 — REAL GEMINI WEB PROMPT VERIFICATION")
	fmt.Println("WEB-ONLY / NO-API — dùng profile W2 đã đăng nhập, gửi đúng một prompt kiểm thử qua Gemini Web.")
	fmt.Printf("Profile: %s\n", profileName)
	fmt.Println()

	session := webai.NewSessionManager(webai.SessionConfig{
		Site:        "gemini-web",
		BrowserPath: *browser,
		ProfileName: profileName,
	})
	defer session.Stop()
	initial, err := session.Start(ctx)
	fmt.Printf("[W3] start  %-14s %s\n", initial.State, initial.Reason)
	if err != nil && initial.State != webai.SessionDegraded && initial.State != webai.SessionAuthRequired {
		fmt.Fprintf(os.Stderr, "W3E FAILED: %v\n", err)
		return 1
	}
	ready, err := waitReady(ctx, session, initialReadyTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "W3E FAILED: %v\n", err)
		return 1
	}
	fmt.Printf("[W3] ready  %-14s %s\n", ready.State, ready.Reason)

	transport, err := webai.NewGeminiWebTransport(webai.GeminiWebTransportConfig{
		Session:         session,
		ResponseTimeout: *timeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "W3E FAILED: %v\n", err)
		return 1
	}
	model, err := webai.NewModel(webai.ModelConfig{
		Site:      "gemini-web",
		Model:     "gemini-web-session",
		Transport: transport,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "W3E FAILED: %v\n", err)
		return 1
	}

	states, stopObserve := observeStates(session)
	resp, err := model.Generate(ctx, []agentcore.Message{agentcore.UserMsg(
		"W3 live verification. Return exactly the text W3_LIVE_OK in the protocol text response. Do not call tools and do not add any other text.",
	)}, nil)
	stopObserve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "W3E FAILED: %v\n", err)
		_, _ = writeW3Evidence(profileName, started, "FAILED", states(), "")
		return 1
	}
	text := strings.TrimSpace(resp.Message.TextContent())
	if text != expectedText {
		fmt.Fprintf(os.Stderr, "W3E FAILED: parsed text = %q, want %q\n", text, expectedText)
		_, _ = writeW3Evidence(profileName, started, "FAILED_PROTOCOL_TEXT", states(), text)
		return 1
	}
	finalStates := states()
	if !containsTransition(finalStates, webai.SessionReady, webai.SessionBusy, webai.SessionReady) {
		fmt.Fprintf(os.Stderr, "W3E FAILED: state timeline did not contain READY -> BUSY -> READY: %v\n", finalStates)
		_, _ = writeW3Evidence(profileName, started, "FAILED_STATE_TIMELINE", finalStates, text)
		return 1
	}
	evidencePath, err := writeW3Evidence(profileName, started, "PASS", finalStates, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "W3E FAILED writing evidence: %v\n", err)
		return 1
	}
	fmt.Printf("[W3] final  %-14s parsed=%s\n", session.Snapshot().State, text)
	fmt.Println("\nW3 PASS: READY -> BUSY -> READY; WebChatModel parsed W3_LIVE_OK")
	fmt.Printf("Evidence: %s\n", evidencePath)
	return 0
}

func waitReady(ctx context.Context, session *webai.SessionManager, timeout time.Duration) (webai.SessionSnapshot, error) {
	if timeout <= 0 {
		return session.Snapshot(), fmt.Errorf("Gemini READY timeout must be positive")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		snap := session.Snapshot()
		if snap.State == webai.SessionReady {
			return snap, nil
		}
		if readyWaitTerminalState(snap.State) {
			return snap, fmt.Errorf("browser session entered %s while waiting for Gemini READY: %s", snap.State, snap.Reason)
		}
		select {
		case <-ctx.Done():
			return session.Snapshot(), ctx.Err()
		case <-deadline.C:
			last := session.Snapshot()
			if last.State == webai.SessionAuthRequired {
				return last, fmt.Errorf("W2 profile remained AUTH_REQUIRED for %s; run W2 login verification again", timeout)
			}
			return last, fmt.Errorf("timed out after %s waiting for Gemini READY (last state %s: %s)", timeout, last.State, last.Reason)
		case <-ticker.C:
			// Chrome DevTools and Gemini account controls can appear a few seconds
			// after the process starts. DEGRADED/AUTH_REQUIRED during this startup
			// window are transitional; keep polling instead of declaring logout on
			// the first unauthenticated-looking DOM snapshot.
			_, _ = session.Refresh(ctx)
		}
	}
}

func readyWaitTerminalState(state webai.SessionState) bool {
	switch state {
	case webai.SessionFailed, webai.SessionStopped:
		return true
	default:
		return false
	}
}

func observeStates(session *webai.SessionManager) (func() []webai.SessionState, func()) {
	var mu sync.Mutex
	states := []webai.SessionState{session.Snapshot().State}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				state := session.Snapshot().State
				mu.Lock()
				if len(states) == 0 || states[len(states)-1] != state {
					states = append(states, state)
					fmt.Printf("[W3] state  %-14s\n", state)
				}
				mu.Unlock()
			}
		}
	}()
	get := func() []webai.SessionState {
		mu.Lock()
		defer mu.Unlock()
		return append([]webai.SessionState(nil), states...)
	}
	stop := func() {
		once.Do(func() { close(done) })
		time.Sleep(60 * time.Millisecond)
		state := session.Snapshot().State
		mu.Lock()
		if len(states) == 0 || states[len(states)-1] != state {
			states = append(states, state)
		}
		mu.Unlock()
	}
	return get, stop
}

func containsTransition(states []webai.SessionState, want ...webai.SessionState) bool {
	if len(want) == 0 {
		return true
	}
	index := 0
	for _, state := range states {
		if state == want[index] {
			index++
			if index == len(want) {
				return true
			}
		}
	}
	return false
}

func latestPassedW2Profile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".ainovel", "browser", "evidence")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read W2 evidence directory: %w", err)
	}
	type candidate struct {
		name    string
		modTime time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		info, err := entry.Info()
		if err == nil {
			candidates = append(candidates, candidate{name: entry.Name(), modTime: info.ModTime()})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].modTime.After(candidates[j].modTime) })
	for _, item := range candidates {
		data, err := os.ReadFile(filepath.Join(dir, item.name))
		if err != nil {
			continue
		}
		var evidence w2EvidenceSummary
		if json.Unmarshal(data, &evidence) != nil {
			continue
		}
		if strings.HasPrefix(evidence.Schema, "ainovel.webai.w2e.") && evidence.Result == "PASS_RESTART_READY" && strings.TrimSpace(evidence.ProfileName) != "" {
			return evidence.ProfileName, nil
		}
	}
	return "", fmt.Errorf("no passed W2 evidence/profile found under %s", dir)
}

func writeW3Evidence(profile string, started time.Time, result string, states []webai.SessionState, output string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".ainovel", "browser", "evidence")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(output))
	evidence := w3Evidence{
		Schema:       "ainovel.webai.w3.v1",
		ProfileName:  profile,
		StartedAt:    started,
		CompletedAt:  time.Now().UTC(),
		Result:       result,
		States:       states,
		OutputSHA256: hex.EncodeToString(sum[:]),
		OutputLength: len(output),
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "w3-verify-"+time.Now().UTC().Format("20060102-150405")+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
