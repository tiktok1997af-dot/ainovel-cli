package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

const devToolsActivePortFile = "DevToolsActivePort"

// DevToolsReadinessProbe inspects a locally launched Chrome page through the
// loopback DevTools endpoint. It is read-only and never submits model prompts.
type DevToolsReadinessProbe struct {
	adapter         sites.Adapter
	httpClient      *http.Client
	dialer          *websocket.Dialer
	discoverTimeout time.Duration
	pollInterval    time.Duration
}

func NewDevToolsReadinessProbe(adapter sites.Adapter) *DevToolsReadinessProbe {
	return &DevToolsReadinessProbe{
		adapter:         adapter,
		httpClient:      &http.Client{Timeout: 3 * time.Second},
		dialer:          &websocket.Dialer{HandshakeTimeout: 3 * time.Second},
		discoverTimeout: 6 * time.Second,
		pollInterval:    100 * time.Millisecond,
	}
}

func NewGeminiDevToolsReadinessProbe() *DevToolsReadinessProbe {
	return NewDevToolsReadinessProbe(sites.Gemini{})
}

func (p *DevToolsReadinessProbe) Probe(ctx context.Context, session SessionSnapshot) (ReadinessResult, error) {
	if p == nil || p.adapter == nil {
		return ReadinessResult{}, protocolError("probe browser readiness", fmt.Errorf("site adapter is required"))
	}
	profileDir := strings.TrimSpace(session.ProfileDir)
	if profileDir == "" {
		return ReadinessResult{}, protocolError("probe browser readiness", fmt.Errorf("browser profile directory is required"))
	}

	port, err := waitForDevToolsPort(ctx, profileDir, p.discoverTimeout, p.pollInterval)
	if err != nil {
		return ReadinessResult{}, readinessTransportError("discover Chrome DevTools", err)
	}
	targets, err := listDevToolsTargets(ctx, p.httpClient, port)
	if err != nil {
		return ReadinessResult{}, readinessTransportError("list Chrome targets", err)
	}
	target, err := selectDevToolsTarget(targets, p.adapter)
	if err != nil {
		return ReadinessResult{}, readinessTransportError("select site tab", err)
	}
	wsURL, err := safeDevToolsWebSocketURL(target.WebSocketDebuggerURL, port)
	if err != nil {
		return ReadinessResult{}, protocolError("validate Chrome target", err)
	}

	evaluator, err := newCDPEvaluator(ctx, p.dialer, wsURL)
	if err != nil {
		return ReadinessResult{}, readinessTransportError("connect Chrome target", err)
	}
	defer evaluator.Close()

	result, err := p.adapter.Probe(ctx, evaluator)
	if err != nil {
		return ReadinessResult{}, readinessTransportError("inspect "+p.adapter.Name()+" readiness", err)
	}
	mapped, ok := mapSiteReadiness(result.State)
	if !ok {
		return ReadinessResult{}, protocolError("map site readiness", fmt.Errorf("unsupported site readiness %q", result.State))
	}
	return ReadinessResult{State: mapped, Reason: strings.TrimSpace(result.Reason)}, nil
}

func readinessTransportError(op string, cause error) error {
	return &Error{
		Kind:       ErrorTransport,
		Op:         op,
		Cause:      cause,
		Retry:      true,
		RetryDelay: 500 * time.Millisecond,
	}
}

func mapSiteReadiness(state sites.Readiness) (SessionState, bool) {
	switch state {
	case sites.ReadinessAuthRequired:
		return SessionAuthRequired, true
	case sites.ReadinessReady:
		return SessionReady, true
	case sites.ReadinessDegraded:
		return SessionDegraded, true
	case sites.ReadinessFailed:
		return SessionFailed, true
	default:
		return "", false
	}
}

func waitForDevToolsPort(ctx context.Context, profileDir string, timeout, poll time.Duration) (int, error) {
	if timeout <= 0 {
		timeout = 6 * time.Second
	}
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		port, err := readDevToolsActivePort(profileDir)
		if err == nil {
			return port, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("DevToolsActivePort unavailable: %w", lastErr)
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}

func readDevToolsActivePort(profileDir string) (int, error) {
	path := filepath.Join(profileDir, devToolsActivePortFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[0]) == "" {
		return 0, fmt.Errorf("invalid %s", devToolsActivePortFile)
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid DevTools port %q", strings.TrimSpace(lines[0]))
	}
	return port, nil
}

type devToolsTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	Title                string `json:"title"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func listDevToolsTargets(ctx context.Context, client *http.Client, port int) ([]devToolsTarget, error) {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DevTools target list returned HTTP %d", resp.StatusCode)
	}
	var targets []devToolsTarget
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(&targets); err != nil {
		return nil, fmt.Errorf("decode DevTools target list: %w", err)
	}
	return targets, nil
}

func selectDevToolsTarget(targets []devToolsTarget, adapter sites.Adapter) (devToolsTarget, error) {
	bestScore := 0
	var best devToolsTarget
	for _, target := range targets {
		if target.Type != "page" || strings.TrimSpace(target.WebSocketDebuggerURL) == "" {
			continue
		}
		score := adapter.TargetScore(target.URL)
		if score > bestScore {
			bestScore = score
			best = target
		}
	}
	if bestScore == 0 {
		return devToolsTarget{}, fmt.Errorf("no matching %s page target", adapter.Name())
	}
	return best, nil
}

func safeDevToolsWebSocketURL(raw string, expectedPort int) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "ws" {
		return "", fmt.Errorf("DevTools target must use ws loopback transport")
	}
	if u.User != nil || u.Fragment != "" {
		return "", fmt.Errorf("DevTools target contains unsupported URL credentials/fragment")
	}
	host := strings.TrimSpace(u.Hostname())
	if !isLoopbackHost(host) {
		return "", fmt.Errorf("DevTools target host %q is not loopback", host)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port != expectedPort {
		return "", fmt.Errorf("DevTools target port does not match active Chrome port")
	}
	if strings.TrimSpace(u.Path) == "" {
		return "", fmt.Errorf("DevTools target path is empty")
	}
	return u.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type cdpEvaluator struct {
	conn   *websocket.Conn
	nextID atomic.Int64
}

func newCDPEvaluator(ctx context.Context, dialer *websocket.Dialer, wsURL string) (*cdpEvaluator, error) {
	if dialer == nil {
		dialer = &websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	}
	conn, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(2 << 20)
	return &cdpEvaluator{conn: conn}, nil
}

func (e *cdpEvaluator) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

func (e *cdpEvaluator) Eval(ctx context.Context, expression string) (json.RawMessage, error) {
	if e == nil || e.conn == nil {
		return nil, errors.New("CDP evaluator is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := e.conn.SetWriteDeadline(deadline); err != nil {
		return nil, err
	}
	if err := e.conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}

	id := e.nextID.Add(1)
	request := map[string]any{
		"id":     id,
		"method": "Runtime.evaluate",
		"params": map[string]any{
			"expression":    expression,
			"returnByValue": true,
			"awaitPromise":  true,
		},
	}
	if err := e.conn.WriteJSON(request); err != nil {
		return nil, err
	}

	for {
		var response struct {
			ID    int64 `json:"id"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Result struct {
				Result struct {
					Value json.RawMessage `json:"value"`
				} `json:"result"`
				ExceptionDetails *struct {
					Text string `json:"text"`
				} `json:"exceptionDetails"`
			} `json:"result"`
		}
		if err := e.conn.ReadJSON(&response); err != nil {
			return nil, err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("CDP error %d: %s", response.Error.Code, response.Error.Message)
		}
		if response.Result.ExceptionDetails != nil {
			return nil, fmt.Errorf("CDP evaluation exception: %s", response.Result.ExceptionDetails.Text)
		}
		if len(response.Result.Result.Value) == 0 {
			return nil, fmt.Errorf("CDP evaluation returned no by-value result")
		}
		return response.Result.Result.Value, nil
	}
}
