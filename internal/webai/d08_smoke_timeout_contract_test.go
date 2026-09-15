package webai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD08PackagedSmokeTimeoutPreservesSanitizedDiagnostics(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "d08-dual-provider-packaged-smoke.ps1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read D08 smoke script: %v", err)
	}
	script := string(data)

	startMarker := "if (-not $process.WaitForExit($RunTimeoutSeconds * 1000)) {"
	start := strings.Index(script, startMarker)
	if start < 0 {
		t.Fatalf("timeout branch marker %q not found", startMarker)
	}
	rest := script[start:]
	endMarker := "    try { $process.WaitForExit() } catch { }"
	end := strings.Index(rest, endMarker)
	if end < 0 {
		t.Fatalf("timeout branch end marker %q not found", endMarker)
	}
	block := rest[:end]

	for _, required := range []string{
		"Get-SanitizedBoundarySummary $RuntimeDir",
		"Get-SanitizedDiagnosticTail $stderrPath",
		"Get-SanitizedDiagnosticTail $stdoutPath",
		"packaged strict-role run timed out; boundary=",
	} {
		if !strings.Contains(block, required) {
			t.Fatalf("timeout branch does not preserve %q", required)
		}
	}
	if strings.Contains(block, "Fail 'packaged strict-role run timed out'") {
		t.Fatal("timeout branch still drops runtime diagnostics")
	}

	helperMarker := "function Get-SanitizedBoundarySummary([string]$RuntimeRoot) {"
	helper := strings.Index(script, helperMarker)
	if helper < 0 {
		t.Fatalf("sanitized boundary helper %q not found", helperMarker)
	}
	helperBody := script[helper:]
	for _, required := range []string{
		"progress=<missing>",
		"completed=",
		"architect_chatgpt=",
		"writer_gemini=",
	} {
		if !strings.Contains(helperBody, required) {
			t.Fatalf("sanitized boundary helper missing %q", required)
		}
	}
}
