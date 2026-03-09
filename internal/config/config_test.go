package config

import (
	"os"
	"testing"
	"time"
)

// unsetEnv unsets an environment variable for the duration of the test,
// restoring it on cleanup. This complements t.Setenv when we need to
// guarantee a variable is absent (so that envDefault kicks in).
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	if orig, ok := os.LookupEnv(key); ok {
		os.Unsetenv(key)
		t.Cleanup(func() { os.Setenv(key, orig) })
	}
}

// allConfigEnvVars returns every env var key that Config reads, so tests
// can start from a clean slate.
func allConfigEnvVars() []string {
	return []string{
		"CORTEX_ADDR", "CORTEX_DEV",
		"DATABASE_URL", "CORTEX_DB_PATH",
		"CORTEX_API_KEY",
		"CORTEX_CONFIG",
		"CORTEX_PROVIDER", "OLLAMA_URL",
		"OPENAI_API_KEY", "OPENAI_BASE_URL",
		"ANTHROPIC_API_KEY",
		"CORTEX_MODEL",
		"QWEN_BASE_URL", "QWEN_API_KEY",
		"LLAMA_BASE_URL", "LLAMA_API_KEY",
		"MINIMAX_BASE_URL", "MINIMAX_API_KEY",
		"OSS_BASE_URL", "OSS_API_KEY",
		"CORTEX_MAX_TOOL_TIMEOUT", "CORTEX_MAX_TOOL_OUTPUT", "CORTEX_MAX_MESSAGE_SIZE",
		"CORTEX_LOG_LEVEL", "CORTEX_LOG_FORMAT",
		"CORTEX_CORS_ORIGINS",
	}
}

// clearAllConfigEnv unsets every config-related env var so Load() returns
// pure defaults.
func clearAllConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range allConfigEnvVars() {
		unsetEnv(t, key)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearAllConfigEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Server
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.DevMode != false {
		t.Errorf("DevMode = %v, want false", cfg.DevMode)
	}

	// Database
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.SQLitePath != "cortex.db" {
		t.Errorf("SQLitePath = %q, want %q", cfg.SQLitePath, "cortex.db")
	}

	// Auth
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}

	// Config file
	if cfg.ConfigFilePath != "" {
		t.Errorf("ConfigFilePath = %q, want empty", cfg.ConfigFilePath)
	}

	// Inference defaults
	if cfg.DefaultProvider != "ollama" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "ollama")
	}
	if cfg.DefaultModel != "llama3.2:latest" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "llama3.2:latest")
	}
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q, want %q", cfg.OllamaURL, "http://localhost:11434")
	}
	if cfg.OpenAIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("OpenAIBaseURL = %q, want %q", cfg.OpenAIBaseURL, "https://api.openai.com/v1")
	}

	// Keys should be empty
	if cfg.OpenAIKey != "" {
		t.Errorf("OpenAIKey = %q, want empty", cfg.OpenAIKey)
	}
	if cfg.AnthropicKey != "" {
		t.Errorf("AnthropicKey = %q, want empty", cfg.AnthropicKey)
	}

	// Limits
	if cfg.MaxToolTimeout != 60*time.Second {
		t.Errorf("MaxToolTimeout = %v, want %v", cfg.MaxToolTimeout, 60*time.Second)
	}
	if cfg.MaxOutputBytes != 65536 {
		t.Errorf("MaxOutputBytes = %d, want %d", cfg.MaxOutputBytes, 65536)
	}
	if cfg.MaxMessageSize != 102400 {
		t.Errorf("MaxMessageSize = %d, want %d", cfg.MaxMessageSize, 102400)
	}

	// Logging
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "json")
	}

	// CORS
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "*" {
		t.Errorf("CORSOrigins = %v, want [\"*\"]", cfg.CORSOrigins)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("CORTEX_ADDR", ":9090")
	t.Setenv("CORTEX_DEV", "true")
	t.Setenv("CORTEX_PROVIDER", "openai")
	t.Setenv("CORTEX_MODEL", "gpt-4")
	t.Setenv("OLLAMA_URL", "http://remote:11434")
	t.Setenv("CORTEX_DB_PATH", "/tmp/test.db")
	t.Setenv("DATABASE_URL", "postgres://localhost/cortex")
	t.Setenv("CORTEX_API_KEY", "secret-key")
	t.Setenv("CORTEX_LOG_LEVEL", "debug")
	t.Setenv("CORTEX_LOG_FORMAT", "pretty")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.DevMode != true {
		t.Errorf("DevMode = %v, want true", cfg.DevMode)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "openai")
	}
	if cfg.DefaultModel != "gpt-4" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4")
	}
	if cfg.OllamaURL != "http://remote:11434" {
		t.Errorf("OllamaURL = %q, want %q", cfg.OllamaURL, "http://remote:11434")
	}
	if cfg.SQLitePath != "/tmp/test.db" {
		t.Errorf("SQLitePath = %q, want %q", cfg.SQLitePath, "/tmp/test.db")
	}
	if cfg.DatabaseURL != "postgres://localhost/cortex" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://localhost/cortex")
	}
	if cfg.APIKey != "secret-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "secret-key")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.LogFormat != "pretty" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "pretty")
	}
}

func TestLoad_APIKeys(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("OPENAI_API_KEY", "sk-openai-test")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("QWEN_API_KEY", "sk-qwen-test")
	t.Setenv("LLAMA_API_KEY", "sk-llama-test")
	t.Setenv("MINIMAX_API_KEY", "sk-minimax-test")
	t.Setenv("OSS_API_KEY", "sk-oss-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"OpenAIKey", cfg.OpenAIKey, "sk-openai-test"},
		{"AnthropicKey", cfg.AnthropicKey, "sk-ant-test"},
		{"QwenKey", cfg.QwenKey, "sk-qwen-test"},
		{"LlamaKey", cfg.LlamaKey, "sk-llama-test"},
		{"MinimaxKey", cfg.MinimaxKey, "sk-minimax-test"},
		{"OSSKey", cfg.OSSKey, "sk-oss-test"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoad_BaseURLs(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("OPENAI_BASE_URL", "https://custom-openai.example.com/v1")
	t.Setenv("QWEN_BASE_URL", "https://qwen.example.com/v1")
	t.Setenv("LLAMA_BASE_URL", "https://llama.example.com/v1")
	t.Setenv("MINIMAX_BASE_URL", "https://minimax.example.com/v1")
	t.Setenv("OSS_BASE_URL", "https://oss.example.com/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"OpenAIBaseURL", cfg.OpenAIBaseURL, "https://custom-openai.example.com/v1"},
		{"QwenBaseURL", cfg.QwenBaseURL, "https://qwen.example.com/v1"},
		{"LlamaBaseURL", cfg.LlamaBaseURL, "https://llama.example.com/v1"},
		{"MinimaxBaseURL", cfg.MinimaxBaseURL, "https://minimax.example.com/v1"},
		{"OSSBaseURL", cfg.OSSBaseURL, "https://oss.example.com/v1"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoad_Limits(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("CORTEX_MAX_TOOL_TIMEOUT", "30s")
	t.Setenv("CORTEX_MAX_TOOL_OUTPUT", "32768")
	t.Setenv("CORTEX_MAX_MESSAGE_SIZE", "204800")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MaxToolTimeout != 30*time.Second {
		t.Errorf("MaxToolTimeout = %v, want %v", cfg.MaxToolTimeout, 30*time.Second)
	}
	if cfg.MaxOutputBytes != 32768 {
		t.Errorf("MaxOutputBytes = %d, want %d", cfg.MaxOutputBytes, 32768)
	}
	if cfg.MaxMessageSize != 204800 {
		t.Errorf("MaxMessageSize = %d, want %d", cfg.MaxMessageSize, 204800)
	}
}

func TestLoad_ConfigFilePath(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("CORTEX_CONFIG", "/tmp/test.json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ConfigFilePath != "/tmp/test.json" {
		t.Errorf("ConfigFilePath = %q, want %q", cfg.ConfigFilePath, "/tmp/test.json")
	}
}

func TestLoad_CORSOrigins(t *testing.T) {
	clearAllConfigEnv(t)

	t.Setenv("CORTEX_CORS_ORIGINS", "http://a.com,http://b.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{"http://a.com", "http://b.com"}
	if len(cfg.CORSOrigins) != len(want) {
		t.Fatalf("CORSOrigins length = %d, want %d", len(cfg.CORSOrigins), len(want))
	}
	for i, v := range want {
		if cfg.CORSOrigins[i] != v {
			t.Errorf("CORSOrigins[%d] = %q, want %q", i, cfg.CORSOrigins[i], v)
		}
	}
}

func TestLoad_EmptyEnvVars(t *testing.T) {
	clearAllConfigEnv(t)

	// caarlos0/env v11 treats empty env vars the same as unset and falls
	// back to envDefault. Verify that fields with defaults keep their
	// defaults when the env var is explicitly set to "".
	t.Setenv("CORTEX_ADDR", "")
	t.Setenv("CORTEX_PROVIDER", "")
	t.Setenv("CORTEX_MODEL", "")
	t.Setenv("CORTEX_DB_PATH", "")
	t.Setenv("CORTEX_LOG_LEVEL", "")
	t.Setenv("CORTEX_LOG_FORMAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Empty env vars → envDefault values are used.
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.DefaultProvider != "ollama" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "ollama")
	}
	if cfg.DefaultModel != "llama3.2:latest" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "llama3.2:latest")
	}
	if cfg.SQLitePath != "cortex.db" {
		t.Errorf("SQLitePath = %q, want %q", cfg.SQLitePath, "cortex.db")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "json")
	}

	// Fields without defaults should remain empty when set to "".
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("CORTEX_API_KEY", "")

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg2.OpenAIKey != "" {
		t.Errorf("OpenAIKey = %q, want empty", cfg2.OpenAIKey)
	}
	if cfg2.AnthropicKey != "" {
		t.Errorf("AnthropicKey = %q, want empty", cfg2.AnthropicKey)
	}
	if cfg2.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg2.DatabaseURL)
	}
	if cfg2.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg2.APIKey)
	}
}
