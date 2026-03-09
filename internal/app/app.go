package app

import (
	"context"
	"log"
	"net/http"

	"cortex/internal/auth"
	"cortex/internal/config"
	"cortex/internal/inference"
	"cortex/internal/session"
	"cortex/internal/store"
)

// App holds all initialized application dependencies.
type App struct {
	Config     *config.Config
	Store      store.Store
	Registry   *inference.ProviderRegistry
	SessionMgr session.Manager
	Auth       func(http.Handler) http.Handler
}

// Option is a functional option for configuring App initialization.
type Option func(*options)

type options struct {
	version string
}

// WithVersion sets the application version string (typically from ldflags).
func WithVersion(v string) Option {
	return func(o *options) {
		o.version = v
	}
}

// New loads config, opens the store, runs migrations, registers providers,
// refreshes the model map, and creates the session manager.
// This is the shared bootstrap used by both HTTP server and CLI modes.
func New(opts ...Option) (*App, error) {
	o := &options{
		version: "dev",
	}
	for _, fn := range opts {
		fn(o)
	}

	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	cfg.Version = o.version

	// 1b. Load JSON config file (if any) for additional providers
	cfgPath := config.DiscoverConfigFile(cfg.ConfigFilePath)
	fileCfg, err := config.LoadConfigFile(cfgPath)
	if err != nil {
		log.Printf("[WARN] Config file error: %v", err)
	}

	// 2. Initialize SQLite store
	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}

	// 3. Run migrations
	if err := db.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	// 4. Create provider registry
	registry := inference.NewProviderRegistry()

	// 5. Register providers
	// Ollama — auto-detect at configured URL
	ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
	if ollamaProvider.Probe(context.Background()) {
		registry.Register(ollamaProvider)
		log.Printf("✅ Detected Ollama at %s", cfg.OllamaURL)
	} else {
		log.Printf("⚠️  Ollama not detected at %s", cfg.OllamaURL)
	}

	// OpenAI-compatible providers
	if cfg.OpenAIKey != "" {
		registry.Register(inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey))
	}
	if cfg.QwenKey != "" {
		registry.Register(inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey))
	}
	if cfg.LlamaKey != "" {
		registry.Register(inference.NewOpenAIProvider("llama", cfg.LlamaBaseURL, cfg.LlamaKey))
	}
	if cfg.MinimaxKey != "" {
		registry.Register(inference.NewOpenAIProvider("minimax", cfg.MinimaxBaseURL, cfg.MinimaxKey))
	}
	if cfg.OSSKey != "" {
		registry.Register(inference.NewOpenAIProvider("oss", cfg.OSSBaseURL, cfg.OSSKey))
	}

	// 5b. Register providers from config file
	if fileCfg != nil {
		for _, p := range fileCfg.Providers {
			if p.Name != "" && p.BaseURL != "" {
				registry.Register(inference.NewOpenAIProvider(p.Name, p.BaseURL, p.APIKey))
				log.Printf("✅ Registered provider %q from config file", p.Name)
			}
		}
		// Config file defaults (env vars take precedence)
		if fileCfg.DefaultProvider != "" && cfg.DefaultProvider == "ollama" {
			cfg.DefaultProvider = fileCfg.DefaultProvider
		}
		if fileCfg.DefaultModel != "" && cfg.DefaultModel == "llama3.2:latest" {
			cfg.DefaultModel = fileCfg.DefaultModel
		}
	}

	// Set default provider from config
	if cfg.DefaultProvider != "" {
		registry.SetDefault(cfg.DefaultProvider)
	}

	// 6. Refresh model map
	if err := registry.RefreshModelMap(context.Background()); err != nil {
		log.Printf("[WARN] Initial model map refresh failed: %v", err)
	}

	// Fall back to mocks if no real providers are configured
	if len(registry.Providers()) == 0 {
		log.Println("No providers configured — using mock providers")
		registry.Register(inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"}))
		registry.Register(inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"}))
		registry.Register(inference.NewMockProvider("minimax", []string{"Minimax", " ", "says", " ", "hello", "!"}))
		registry.Register(inference.NewMockProvider("oss", []string{"OSS", " ", "power", "!"}))
		registry.SetDefault("qwen")
	}

	// 7. Create session manager
	sessionMgr := session.NewManager(db, cfg.DefaultModel)

	// 8. Create auth middleware
	authMiddleware := auth.NewMiddleware(cfg.APIKey)

	return &App{
		Config:     cfg,
		Store:      db,
		Registry:   registry,
		SessionMgr: sessionMgr,
		Auth:       authMiddleware,
	}, nil
}

// Close releases resources (closes the store).
func (a *App) Close() error {
	if a.Store != nil {
		return a.Store.Close()
	}
	return nil
}
