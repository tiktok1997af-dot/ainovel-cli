package webai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD08PackagedSmokeReadsAuthoritativeModelGraphLog(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "scripts", "d08-dual-provider-packaged-smoke.ps1")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read D08 smoke script: %v", err)
	}
	script := string(scriptBytes)

	for _, required := range []string{
		`(Join-Path $outputRoot 'logs\headless.log')`,
		`default=web/gemini-web`,
		`specialist=chatgpt-web/chatgpt-web`,
		`strict-role model graph was not projected by packaged runtime`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("D08 smoke model-graph evidence contract missing %q", required)
		}
	}
	if strings.Contains(script, `(Join-Path $outputRoot 'headless.log')`) {
		t.Fatal("D08 smoke still reads the obsolete output-root headless.log path")
	}

	hostPath := filepath.Join("..", "host", "host.go")
	hostBytes, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatalf("read host runtime: %v", err)
	}
	if !strings.Contains(string(hostBytes), `"summary", models.Summary()`) {
		t.Fatal("host runtime no longer projects authoritative ModelSet.Summary()")
	}

	loggerPath := filepath.Join("..", "logger", "logger.go")
	loggerBytes, err := os.ReadFile(loggerPath)
	if err != nil {
		t.Fatalf("read runtime logger: %v", err)
	}
	if !strings.Contains(string(loggerBytes), `filepath.Join(outputDir, "logs", filename)`) {
		t.Fatal("runtime logger no longer writes file logs under outputDir/logs")
	}
}
