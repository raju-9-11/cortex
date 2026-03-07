package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"forge/internal/auth"
	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/inference"
	"forge/internal/server"
	"forge/internal/session"
	"forge/internal/store"
)

var version = "dev"

func main() {
	// Check for subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			runCmd()
			return
		}
	}

	// No args (or unrecognized) → start HTTP server (existing behavior)
	startServer()
}

func runCmd() {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	prompt := fs.String("prompt", "", "Prompt text")
	model := fs.String("model", "", "Model to use (e.g., ollama/llama3.2)")
	system := fs.String("system", "", "System prompt")
	fs.Parse(os.Args[2:])

	// Determine prompt: flag > stdin pipe
	promptText := *prompt
	if promptText == "" {
		// Check if stdin is a pipe
		info, _ := os.Stdin.Stat()
		if (info.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 100*1024))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}
			promptText = strings.TrimSpace(string(data))
		}
	}

	if promptText == "" {
		fmt.Fprintln(os.Stderr, "Error: no prompt provided. Use --prompt flag or pipe input via stdin.")
		fmt.Fprintln(os.Stderr, "Usage: forge run --prompt \"your question here\"")
		fmt.Fprintln(os.Stderr, "       echo \"your question\" | forge run")
		os.Exit(1)
	}

	// Bootstrap (inline — same as server startup)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := inference.NewProviderRegistry()

	// Register providers (same logic as server mode)
	ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
	if ollamaProvider.Probe(context.Background()) {
		registry.Register(ollamaProvider)
	}
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

	if cfg.DefaultProvider != "" {
		registry.SetDefault(cfg.DefaultProvider)
	}

	registry.RefreshModelMap(context.Background())

	// Mock fallback
	if len(registry.Providers()) == 0 {
		registry.Register(inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"}))
		registry.Register(inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"}))
		registry.SetDefault("qwen")
	}

	// Resolve model
	useModel := cfg.DefaultModel
	if *model != "" {
		useModel = *model
	}

	// Execute
	ctx := context.Background()
	_, err = cli.RunOnce(ctx, registry, useModel, *system, promptText, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

func startServer() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Version = version

	// 2. Initialize SQLite store
	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 3. Run migrations
	if err := db.Migrate(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
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

	// Startup banner
	models := registry.ListAllModels(context.Background())
	fmt.Printf("\n🔥 Forge %s\n", cfg.Version)
	fmt.Printf("  API:    http://localhost%s/v1/chat/completions\n", cfg.Addr)
	fmt.Printf("  Health: http://localhost%s/api/health\n", cfg.Addr)
	fmt.Printf("  Chat UI: http://localhost%s/chat\n", cfg.Addr)
	fmt.Println()
	fmt.Printf("  Providers: %d registered\n", len(registry.Providers()))
	fmt.Printf("  Models:    %d available\n", len(models))
	fmt.Println()

	// 9. Create and start server with all dependencies
	srv := server.New(cfg, registry, db, sessionMgr, authMiddleware)
	srv.StartAndServe()
}
