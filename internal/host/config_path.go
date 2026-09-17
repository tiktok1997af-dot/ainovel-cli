package host

// applyConfigPath applies the additive path-explicit construction option after
// the existing Host constructor has completed. It is kept in-package so the
// desktop composition layer never gains access to Host internals.
func applyConfigPath(h *Host, opts newOptions) {
	if h == nil || opts.configPath == "" {
		return
	}
	h.configPath = opts.configPath
}

// ApplyConfigOptions is an internal-composition helper for callers that need
// path-explicit config ownership without changing the legacy New constructor.
// It does not expose Host state to JavaScript; Wails binds only Gateway.
func ApplyConfigOptions(h *Host, options ...NewOption) {
	if h == nil {
		return
	}
	var opts newOptions
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	applyConfigPath(h, opts)
}
