package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/agentcore"
)

// TestSessionStore_MetaInjected_AssistantFallsBackToLookupWithoutUsage verifies
// that browser-backed assistant messages still persist provider/model provenance
// even when the model cannot report token Usage.
func TestSessionStore_MetaInjected_AssistantFallsBackToLookupWithoutUsage(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))
	lookup := ModelLookup(func(agentName string) (string, string) {
		return "meme", "gpt-5.4"
	})
	logger := s.SubAgentLogger(lookup)

	logger("writer", "写第 1 章", agentcore.Message{
		Role:  agentcore.RoleUser,
		Usage: nil,
	})
	logger("writer", "写第 1 章", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Input: 1000, Output: 200, CacheRead: 800, TotalTokens: 1200,
		},
	})
	logger("writer", "写第 1 章", agentcore.Message{
		Role:  agentcore.RoleAssistant,
		Usage: nil,
	})

	entries := readJSONL(t, filepath.Join(dir, "meta/sessions/agents/writer-ch01.jsonl"))
	if len(entries) != 3 {
		t.Fatalf("entries=%d want 3", len(entries))
	}
	if _, has := entries[0]["_meta"]; has {
		t.Errorf("user message should NOT have _meta")
	}
	for _, index := range []int{1, 2} {
		meta, ok := entries[index]["_meta"].(map[string]any)
		if !ok {
			t.Fatalf("assistant entry[%d] should have _meta map, got %T %v", index, entries[index]["_meta"], entries[index]["_meta"])
		}
		if meta["provider"] != "meme" || meta["model"] != "gpt-5.4" {
			t.Errorf("entry[%d] _meta = %v want provider=meme model=gpt-5.4", index, meta)
		}
	}
}

func TestSessionStore_UsageProvenanceOverridesLookup(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))
	logger := s.SubAgentLogger(func(agentName string) (string, string) {
		return "lookup-provider", "lookup-model"
	})
	logger("writer", "写第 1 章", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Provider: "runtime-provider",
			Model:    "runtime-model",
			Input:    10,
			Output:   5,
		},
	})

	entries := readJSONL(t, filepath.Join(dir, "meta/sessions/agents/writer-ch01.jsonl"))
	meta, ok := entries[0]["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("assistant should have _meta map, got %T %v", entries[0]["_meta"], entries[0]["_meta"])
	}
	if meta["provider"] != "runtime-provider" || meta["model"] != "runtime-model" {
		t.Fatalf("_meta = %v want runtime usage provenance", meta)
	}
}

func TestSessionStore_D08WebRoleProvenanceWithoutUsage(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))
	lookup := ModelLookup(func(agentName string) (string, string) {
		if agentName == "architect_short" {
			return "chatgpt-web", "chatgpt-web"
		}
		if agentName == "writer" {
			return "web", "gemini-web"
		}
		return "", ""
	})
	logger := s.SubAgentLogger(lookup)
	logger("architect_short", "建立短篇基础", agentcore.Message{Role: agentcore.RoleAssistant})
	logger("writer", "写第 1 章", agentcore.Message{Role: agentcore.RoleAssistant})

	cases := []struct {
		path     string
		provider string
		model    string
	}{
		{filepath.Join(dir, "meta/sessions/agents/architect_short-001.jsonl"), "chatgpt-web", "chatgpt-web"},
		{filepath.Join(dir, "meta/sessions/agents/writer-ch01.jsonl"), "web", "gemini-web"},
	}
	for _, tc := range cases {
		entries := readJSONL(t, tc.path)
		if len(entries) != 1 {
			t.Fatalf("%s entries=%d want 1", tc.path, len(entries))
		}
		meta, ok := entries[0]["_meta"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing _meta: %v", tc.path, entries[0])
		}
		if meta["provider"] != tc.provider || meta["model"] != tc.model {
			t.Fatalf("%s _meta=%v want provider=%s model=%s", tc.path, meta, tc.provider, tc.model)
		}
	}
}

// TestSessionStore_MetaModelSwitch 验证运行中切换模型后，后续消息的 _meta 也跟着变。
// 这是 B 方案对"同进程内 /model 切换"的精确支持。
func TestSessionStore_MetaModelSwitch(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))

	current := "model-a"
	lookup := ModelLookup(func(agentName string) (string, string) {
		return "meme", current
	})
	logger := s.SubAgentLogger(lookup)

	logger("writer", "写第 1 章", makeAssistantWithUsage())
	current = "model-b" // 模拟 /model 切换
	logger("writer", "写第 1 章", makeAssistantWithUsage())

	entries := readJSONL(t, filepath.Join(dir, "meta/sessions/agents/writer-ch01.jsonl"))
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2", len(entries))
	}
	for i, want := range []string{"model-a", "model-b"} {
		meta, ok := entries[i]["_meta"].(map[string]any)
		if !ok {
			t.Fatalf("entry[%d] missing _meta", i)
		}
		if got := meta["model"]; got != want {
			t.Errorf("entry[%d] model = %v want %s", i, got, want)
		}
	}
}

// TestSessionStore_NilLookup 验证 lookup=nil 时写入仍然正常，
// 只是不带 _meta。
func TestSessionStore_NilLookup(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))
	logger := s.SubAgentLogger(nil)
	logger("writer", "写第 1 章", makeAssistantWithUsage())

	rel, err := s.subAgentPath("writer", "写第 1 章")
	if err != nil {
		t.Fatal(err)
	}
	entries := readJSONL(t, filepath.Join(dir, rel))
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1", len(entries))
	}
	if _, has := entries[0]["_meta"]; has {
		t.Errorf("nil lookup should not produce _meta")
	}
	// 但其他字段（role/usage）必须正常
	if entries[0]["role"] != "assistant" {
		t.Errorf("role lost: %v", entries[0]["role"])
	}
}

func TestSessionStoreContinuesAgentSequenceAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	first := NewSessionStore(newIO(dir)).SubAgentLogger(nil)
	first("architect_long", "处理反馈", makeAssistantWithUsage())

	second := NewSessionStore(newIO(dir)).SubAgentLogger(nil)
	second("architect_long", "扩展大纲", makeAssistantWithUsage())

	if got := len(readJSONL(t, filepath.Join(dir, "meta/sessions/agents/architect_long-001.jsonl"))); got != 1 {
		t.Fatalf("first session entries = %d, want 1", got)
	}
	if got := len(readJSONL(t, filepath.Join(dir, "meta/sessions/agents/architect_long-002.jsonl"))); got != 1 {
		t.Fatalf("second session entries = %d, want 1", got)
	}
}

func makeAssistantWithUsage() agentcore.Message {
	return agentcore.Message{
		Role:  agentcore.RoleAssistant,
		Usage: &agentcore.Usage{Input: 1000, Output: 200, TotalTokens: 1200},
	}
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("unmarshal line: %v\n%s", err, string(line))
		}
		out = append(out, m)
	}
	return out
}
