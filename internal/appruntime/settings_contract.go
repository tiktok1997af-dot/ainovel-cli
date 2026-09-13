package appruntime

const (
	QuerySettingsGet      QueryKind   = "settings.get"
	CommandSettingsUpdate CommandKind = "settings.update"
)

type SettingsGetQuery struct{}

type SettingsIdentityDTO struct {
	WebEnabled bool   `json:"web_enabled"`
	WebSite    string `json:"web_site"`
}

type SettingsValuesDTO struct {
	BrowserPath     string   `json:"browser_path,omitempty"`
	ProfileName     string   `json:"profile_name,omitempty"`
	StartURL        string   `json:"start_url,omitempty"`
	Language        string   `json:"language,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Style           string   `json:"style,omitempty"`
	ContextWindow   int      `json:"context_window,omitempty"`
	NotifyEnabled   bool     `json:"notify_enabled"`
	NotifyEvents    []string `json:"notify_events,omitempty"`
}

type SettingsRuntimeDTO struct {
	Values SettingsValuesDTO `json:"values"`
}

type SettingsGetResultDTO struct {
	Identity        SettingsIdentityDTO `json:"identity"`
	Values          SettingsValuesDTO   `json:"values"`
	Runtime         SettingsRuntimeDTO  `json:"runtime"`
	ConfigScope     string              `json:"config_scope"`
	ConfigPathClass string              `json:"config_path_class"`
	Fingerprint     string              `json:"fingerprint"`
	RestartRequired bool                `json:"restart_required"`
	ApplyMode       string              `json:"apply_mode"`
}

type SettingsUpdateCommandPayload struct {
	ExpectedFingerprint string            `json:"expected_fingerprint"`
	Values              SettingsValuesDTO `json:"values"`
}

type SettingsUpdateResultDTO struct {
	Saved       bool   `json:"saved"`
	Fingerprint string `json:"fingerprint"`
}
