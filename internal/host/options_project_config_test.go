package host

import "testing"

func TestWithConfigPathSetsExplicitTarget(t *testing.T) {
	var opts newOptions
	WithConfigPath("C:/projects/demo/.ainovel/config.json")(&opts)
	if opts.configPath != "C:/projects/demo/.ainovel/config.json" {
		t.Fatalf("configPath = %q", opts.configPath)
	}
}
