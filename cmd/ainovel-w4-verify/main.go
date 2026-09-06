package main

import (
	"context"
	"crypto/rand"
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
	"sync/atomic"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/subagent"
	"github.com/voocel/ainovel-cli/internal/webai"
)

const (
	proofToolName       = "w4_local_proof"
	proofChallenge      = "W4_LOCAL_TOOL_CHALLENGE"
	initialReadyTimeout = 45 * time.Second
)

type w3EvidenceSummary struct {
	Schema      string `json:"schema"`
	ProfileName string `json:"profile_name"`
	Result      string `json:"result"`
}

type w4Evidence struct {
	Schema         string               `json:"schema"`
	ProfileName    string               `json:"profile_name"`
	StartedAt      time.Time            `json:"started_at"`
	CompletedAt    time.Time            `json:"completed_at"`
	Result         string               `json:"result"`
	States         []webai.SessionState `json:"states"`
	ToolExecutions int32                `json:"tool_executions"`
	OutputSHA256   string               `json:"output_sha256"`
	OutputLength   int                  `json:"output_length"`
	ReceiptSHA256  string               `json:"receipt_sha256"`
	ReceiptLength  int                  `json:"receipt_length"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("ainovel-w4-verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", "", "Đường dẫn Chrome (để trống để tự tìm)")
	profile := fs.String("profile", "", "Tên profile W3 đã PASS; để trống để tự tìm evidence W3 PASS mới nhất")
	timeout := fs.Duration("timeout", 2*time.Minute, "Thời gian tối đa cho mỗi lượt Gemini Web")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "ainovel-w4-verify không nhận tham số vị trí")
		return 2
	}

	profileName := strings.TrimSpace(*profile)
	if profileName == "" {
		var err error
		profileName, err = latestPassedW3Profile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "W4 FAILED: %v\n", err)
			return 1
		}
	}

	receipt, err := randomReceipt()
	if err != nil {
		fmt.Fprintf(os.Stderr, "W4 FAILED generating local receipt: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	started := time.Now().UTC()

	fmt.Println("W4 — REAL GEMINI WEB <-> LOCAL TOOL E2E VERIFICATION")
	fmt.Println("WEB-ONLY / NO-API — Gemini chỉ yêu cầu Tool; ainovel local runtime mới được phép thực thi Tool.")
	fmt.Printf("Profile: %s\n\n", profileName)

	session := webai.NewSessionManager(webai.SessionConfig{
		Site:        "gemini-web",
		BrowserPath: *browser,
		ProfileName: profileName,
	})
	defer session.Stop()
	initial, startErr := session.Start(ctx)
	fmt.Printf("[W4] start  %-14s %s\n", initial.State, initial.Reason)
	if startErr != nil && initial.State != webai.SessionDegraded && initial.State != webai.SessionAuthRequired {
		fmt.Fprintf(os.Stderr, "W4 FAILED: %v\n", startErr)
		return 1
	}
	ready, err := waitReady(ctx, session, initialReadyTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "W4 FAILED: %v\n", err)
		return 1
	}
	fmt.Printf("[W4] ready  %-14s %s\n", ready.State, ready.Reason)

	transport, err := webai.NewGeminiWebTransport(webai.GeminiWebTransportConfig{
		Session:         session,
		ResponseTimeout: *timeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "W4 FAILED: %v\n", err)
		return 1
	}
	model, err := webai.NewModel(webai.ModelConfig{
		Site:      "gemini-web",
		Model:     "gemini-web-session",
		Transport: transport,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "W4 FAILED: %v\n", err)
		return 1
	}

	var executions atomic.Int32
	var seenMu sync.Mutex
	seenChallenge := ""
	tool := agentcore.NewFuncTool(proofToolName, "Local-only W4 verification tool. You MUST call this tool exactly once before answering. The ainovel runtime executes it locally; the browser must never pretend it ran.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"challenge": map[string]any{"type": "string"},
		},
		"required":             []string{"challenge"},
		"additionalProperties": false,
	}, func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		executions.Add(1)
		var input struct {
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("decode W4 proof args: %w", err)
		}
		seenMu.Lock()
		seenChallenge = input.Challenge
		seenMu.Unlock()
		if input.Challenge != proofChallenge {
			return nil, fmt.Errorf("unexpected W4 challenge %q", input.Challenge)
		}
		return json.Marshal(map[string]string{
			"receipt": receipt,
			"status":  "LOCAL_TOOL_EXECUTED",
		})
	})

	runner := subagent.NewRunner(subagent.Config{
		Name:         "w4-verifier",
		Description:  "W4 real browser local tool-call verifier",
		Model:        model,
		SystemPrompt: "You are verifying the ainovel WEB-ONLY local tool loop. Follow the tool protocol exactly. Never claim a tool ran unless the local tool result is present in the conversation.",
		Tools:        []agentcore.Tool{tool},
		MaxTurns:     4,
	})

	states, stopObserve := observeStates(session)
	result, err := runner.Run(ctx, "w4-verifier", w4Instruction())
	stopObserve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "W4 FAILED: %v\n", err)
		_, _ = writeW4Evidence(profileName, started, "FAILED_RUNNER", states(), executions.Load(), "", receipt)
		return 1
	}

	seenMu.Lock()
	challenge := seenChallenge
	seenMu.Unlock()
	if executions.Load() != 1 {
		fmt.Fprintf(os.Stderr, "W4 FAILED: local tool executions=%d, want 1\n", executions.Load())
		_, _ = writeW4Evidence(profileName, started, "FAILED_TOOL_COUNT", states(), executions.Load(), result.Output, receipt)
		return 1
	}
	if challenge != proofChallenge {
		fmt.Fprintf(os.Stderr, "W4 FAILED: local tool challenge=%q, want %q\n", challenge, proofChallenge)
		_, _ = writeW4Evidence(profileName, started, "FAILED_TOOL_ARGS", states(), executions.Load(), result.Output, receipt)
		return 1
	}

	want := expectedFinal(receipt)
	got := strings.TrimSpace(result.Output)
	if got != want {
		fmt.Fprintf(os.Stderr, "W4 FAILED: final output=%q, want exact local receipt confirmation\n", got)
		_, _ = writeW4Evidence(profileName, started, "FAILED_FINAL_RECEIPT", states(), executions.Load(), got, receipt)
		return 1
	}

	finalStates := states()
	if countState(finalStates, webai.SessionBusy) < 2 || !containsTransition(finalStates, webai.SessionReady, webai.SessionBusy, webai.SessionReady) {
		fmt.Fprintf(os.Stderr, "W4 FAILED: expected two browser round trips and READY/BUSY lifecycle, states=%v\n", finalStates)
		_, _ = writeW4Evidence(profileName, started, "FAILED_STATE_TIMELINE", finalStates, executions.Load(), got, receipt)
		return 1
	}
	if session.Snapshot().State != webai.SessionReady {
		fmt.Fprintf(os.Stderr, "W4 FAILED: final session state=%s, want READY\n", session.Snapshot().State)
		_, _ = writeW4Evidence(profileName, started, "FAILED_FINAL_STATE", finalStates, executions.Load(), got, receipt)
		return 1
	}

	evidencePath, err := writeW4Evidence(profileName, started, "PASS", finalStates, executions.Load(), got, receipt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "W4 FAILED writing evidence: %v\n", err)
		return 1
	}
	fmt.Printf("[W4] tool   LOCAL_EXECUTIONS=%d challenge=OK\n", executions.Load())
	fmt.Printf("[W4] final  %-14s receipt=VERIFIED\n", session.Snapshot().State)
	fmt.Println("\nW4 PASS: Gemini Web -> WebChatModel -> local Tool -> tool result -> Gemini Web -> final receipt")
	fmt.Printf("Evidence: %s\n", evidencePath)
	return 0
}

func w4Instruction() string {
	return "W4 live local-tool verification. You MUST call the local tool " + proofToolName + " exactly once with arguments {\"challenge\":\"" + proofChallenge + "\"}. Do not answer before the tool result exists. After the local tool result arrives, read its receipt field and return exactly the text W4_TOOL_OK:<receipt> in the protocol text response. Do not invent or alter the receipt."
}

func expectedFinal(receipt string) string {
	return "W4_TOOL_OK:" + receipt
}

func randomReceipt() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
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
				return last, fmt.Errorf("browser profile remained AUTH_REQUIRED for %s; rerun W2 login verification", timeout)
			}
			return last, fmt.Errorf("timed out after %s waiting for Gemini READY (last state %s: %s)", timeout, last.State, last.Reason)
		case <-ticker.C:
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
					fmt.Printf("[W4] state  %-14s\n", state)
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

func countState(states []webai.SessionState, want webai.SessionState) int {
	count := 0
	for _, state := range states {
		if state == want {
			count++
		}
	}
	return count
}

func latestPassedW3Profile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".ainovel", "browser", "evidence")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read W3 evidence directory: %w", err)
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
		var evidence w3EvidenceSummary
		if json.Unmarshal(data, &evidence) != nil {
			continue
		}
		if evidence.Schema == "ainovel.webai.w3.v1" && evidence.Result == "PASS" && strings.TrimSpace(evidence.ProfileName) != "" {
			return evidence.ProfileName, nil
		}
	}
	return "", fmt.Errorf("no passed W3 evidence/profile found under %s", dir)
}

func writeW4Evidence(profile string, started time.Time, result string, states []webai.SessionState, executions int32, output, receipt string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".ainovel", "browser", "evidence")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	outputSum := sha256.Sum256([]byte(output))
	receiptSum := sha256.Sum256([]byte(receipt))
	evidence := w4Evidence{
		Schema:         "ainovel.webai.w4.v1",
		ProfileName:    profile,
		StartedAt:      started,
		CompletedAt:    time.Now().UTC(),
		Result:         result,
		States:         states,
		ToolExecutions: executions,
		OutputSHA256:   hex.EncodeToString(outputSum[:]),
		OutputLength:   len(output),
		ReceiptSHA256:  hex.EncodeToString(receiptSum[:]),
		ReceiptLength:  len(receipt),
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "w4-verify-"+time.Now().UTC().Format("20060102-150405")+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
