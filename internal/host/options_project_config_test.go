package host

import "testing"

func TestWithConfigPathSetsExplicitTarget(t *testing.T) {
	var opts newOptions
	WithConfigPath("C:/projects/demo/.ainovel/config.json")(&opts)
	if opts.configPath != "C:/projects/demo/.ainovel/config.json" {
		t.Fatalf("configPath = %q", opts.configPath)
	}
}

func TestConfigPathForOptionsPrefersExplicitTarget(t *testing.T) {
	var opts newOptions
	WithConfigPath("C:/projects/demo/.ainovel/config.json")(&opts)
	got := configPathForOptions("C:/global/.ainovel/config.json", opts)
	if got != "C:/projects/demo/.ainovel/config.json" {
		t.Fatalf("configPathForOptions() = %q, want explicit project target", got)
	}
}

func TestConfigPathForOptionsFallsBackWhenUnset(t *testing.T) {
	got := configPathForOptions("C:/global/.ainovel/config.json", newOptions{})
	if got != "C:/global/.ainovel/config.json" {
		t.Fatalf("configPathForOptions() = %q, want fallback target", got)
	}
}
