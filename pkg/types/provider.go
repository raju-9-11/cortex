package types

// ProviderInfo represents a configured inference provider as exposed to the frontend.
// SECURITY: The api_key field is NEVER included. Only has_api_key (bool) is exposed.
type ProviderInfo struct {
	ID        string   `json:"id"`          // "ollama", "openai", "anthropic", etc.
	Type      string   `json:"type"`        // "ollama" | "openai" | "anthropic" | "gemini" | "openai_compat"
	BaseURL   string   `json:"base_url"`    // e.g. "http://localhost:11434"
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status"`      // "connected" | "offline" | "error" | "unconfigured"
	Models    []string `json:"models"`      // Model IDs available from this provider
	HasAPIKey bool     `json:"has_api_key"` // true if an API key is configured (never expose the key!)
	IsEnvVar  bool     `json:"is_env_var"`  // true if configured via environment variable (read-only in UI)
}

// Provider status constants
const (
	ProviderStatusConnected    = "connected"
	ProviderStatusOffline      = "offline"
	ProviderStatusError        = "error"
	ProviderStatusUnconfigured = "unconfigured"
)

// Provider type constants
const (
	ProviderTypeOllama       = "ollama"
	ProviderTypeOpenAI       = "openai"
	ProviderTypeAnthropic    = "anthropic"
	ProviderTypeGemini       = "gemini"
	ProviderTypeOpenAICompat = "openai_compat"
)

// ProviderListResponse is the response for GET /api/providers.
type ProviderListResponse struct {
	Data []ProviderInfo `json:"data"`
}

// TestProviderResponse is the response for POST /api/providers/{id}/test.
type TestProviderResponse struct {
	Status      string `json:"status"`       // "ok" | "error"
	LatencyMs   int64  `json:"latency_ms"`
	ModelsFound int    `json:"models_found,omitempty"`
	Error       string `json:"error,omitempty"`
}
