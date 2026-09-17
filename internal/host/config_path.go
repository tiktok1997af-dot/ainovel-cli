package host

// configPathForOptions resolves the config-write target during Host construction.
// Explicit project ownership wins; callers that omit WithConfigPath retain the
// existing EffectiveConfigPath fallback.
func configPathForOptions(fallback string, opts newOptions) string {
	if opts.configPath != "" {
		return opts.configPath
	}
	return fallback
}
