package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// Server
	Addr    string `env:"CORTEX_ADDR" envDefault:":8080"`
	DevMode bool   `env:"CORTEX_DEV" envDefault:"false"`
	Version string // Set via ldflags

	// Database
	DatabaseURL string `env:"DATABASE_URL"` // If set -> PostgreSQL; if empty -> SQLite
	SQLitePath  string `env:"CORTEX_DB_PATH" envDefault:"cortex.db"`

	// Auth
	APIKey string `env:"CORTEX_API_KEY"` // If set -> require auth; if empty -> no auth

	// Config file
	ConfigFilePath string `env:"CORTEX_CONFIG"` // Path to JSON config file

	// Inference Providers
	DefaultProvider string `env:"CORTEX_PROVIDER" envDefault:"ollama"`
	OllamaURL       string `env:"OLLAMA_URL" envDefault:"http://localhost:11434"`
	ModelsDir       string `env:"CORTEX_MODELS_DIR" envDefault:"./models"`
	LocalContextSize int    `env:"CORTEX_LOCAL_CTX" envDefault:"4096"`
	OpenAIKey       string `env:"OPENAI_API_KEY"`
	OpenAIBaseURL   string `env:"OPENAI_BASE_URL" envDefault:"https://api.openai.com/v1"`
	AnthropicKey    string `env:"ANTHROPIC_API_KEY"`
	DefaultModel    string `env:"CORTEX_MODEL" envDefault:"llama3.2:latest"`

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
	MaxToolTimeout time.Duration `env:"CORTEX_MAX_TOOL_TIMEOUT" envDefault:"60s"`
	MaxOutputBytes int           `env:"CORTEX_MAX_TOOL_OUTPUT" envDefault:"65536"` // 64KB
	MaxMessageSize int           `env:"CORTEX_MAX_MESSAGE_SIZE" envDefault:"102400"` // 100KB

	// Logging
	LogLevel  string `env:"CORTEX_LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"CORTEX_LOG_FORMAT" envDefault:"json"` // "json" | "pretty"

	// CORS
	CORSOrigins []string `env:"CORTEX_CORS_ORIGINS" envSeparator:"," envDefault:"*"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}
