package types

// HealthResponse is returned by GET /api/health.
// Used by the topbar connection status indicator.
//
// Status logic:
//   - "ok"       — Database healthy AND at least one provider connected
//   - "degraded" — Database healthy BUT some providers offline
//   - "error"    — Database unreachable OR all providers offline
type HealthResponse struct {
	Status    string                     `json:"status"` // "ok" | "degraded" | "error"
	Version   string                     `json:"version"`
	Uptime    int64                      `json:"uptime_seconds"`
	Database  HealthComponent            `json:"database"`
	Providers map[string]HealthComponent `json:"providers"`
}

// HealthComponent represents the health status of a single subsystem.
type HealthComponent struct {
	Status  string `json:"status"`           // "ok" | "error"
	Latency int64  `json:"latency_ms"`
	Error   string `json:"error,omitempty"`
}

// ConfigResponse is returned by GET /api/config.
// Exposes non-sensitive runtime configuration to the frontend.
// Used by the Settings page to show "Set via environment variable" badges.
type ConfigResponse struct {
	DefaultModel    string   `json:"default_model"`
	DefaultProvider string   `json:"default_provider"`
	AuthEnabled     bool     `json:"auth_enabled"`
	DevMode         bool     `json:"dev_mode"`
	Version         string   `json:"version"`
	EnvOverrides    []string `json:"env_overrides"` // Which config keys are set via env vars
}
