package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// Server
	Addr    string `env:"FORGE_ADDR" envDefault:":8080"`
	DevMode bool   `env:"FORGE_DEV" envDefault:"false"`
	Version string // Set via ldflags

	// Database
	DatabaseURL string `env:"DATABASE_URL"` // If set -> PostgreSQL; if empty -> SQLite
	SQLitePath  string `env:"FORGE_DB_PATH" envDefault:"forge.db"`

	// Auth
	APIKey string `env:"FORGE_API_KEY"` // If set -> require auth; if empty -> no auth

	// Config file
	ConfigFilePath string `env:"FORGE_CONFIG"` // Path to JSON config file

	// Inference Providers
	DefaultProvider string `env:"FORGE_PROVIDER" envDefault:"qwen"`
	OllamaURL       string `env:"OLLAMA_URL" envDefault:"http://localhost:11434"`
	OpenAIKey       string `env:"OPENAI_API_KEY"`
	OpenAIBaseURL   string `env:"OPENAI_BASE_URL" envDefault:"https://api.openai.com/v1"`
	AnthropicKey    string `env:"ANTHROPIC_API_KEY"`
	DefaultModel    string `env:"FORGE_MODEL" envDefault:"qwen2.5:0.5b"`

	// Qwen/Minimax/OSS Configs mapping to OpenAI compatible endpoints
	QwenBaseURL string `env:"QWEN_BASE_URL"`
	QwenKey     string `env:"QWEN_API_KEY"`

	LlamaBaseURL string `env:"LLAMA_BASE_URL"`
	LlamaKey     string `env:"LLAMA_API_KEY"`

	MinimaxBaseURL string `env:"MINIMAX_BASE_URL"`
	MinimaxKey     string `env:"MINIMAX_API_KEY"`

	OSSBaseURL string `env:"OSS_BASE_URL"`
	OSSKey     string `env:"OSS_API_KEY"`


	// Limits
	MaxToolTimeout time.Duration `env:"FORGE_MAX_TOOL_TIMEOUT" envDefault:"60s"`
	MaxOutputBytes int           `env:"FORGE_MAX_TOOL_OUTPUT" envDefault:"65536"` // 64KB
	MaxMessageSize int           `env:"FORGE_MAX_MESSAGE_SIZE" envDefault:"102400"` // 100KB

	// Logging
	LogLevel  string `env:"FORGE_LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"FORGE_LOG_FORMAT" envDefault:"json"` // "json" | "pretty"

	// CORS
	CORSOrigins []string `env:"FORGE_CORS_ORIGINS" envSeparator:"," envDefault:"*"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}
