package main

import (
"context"
"flag"
"fmt"
"io"
"log"
"os"
"os/signal"
"strings"

"forge/internal/app"
"forge/internal/cli"
"forge/internal/config"
"forge/internal/inference"
"forge/internal/server"
"forge/internal/session"
"forge/internal/store"
)

var version = "dev"

func main() {
// Subcommand dispatch
if len(os.Args) > 1 {
switch os.Args[1] {
case "help", "--help", "-h":
cli.PrintUsage(os.Stdout, version)
return
case "version", "--version":
cli.PrintVersion(os.Stdout, version)
return
case "run":
runCmd()
return
case "chat":
chatCmd()
return
case "sessions":
sessionsCmd()
return
case "models":
// TODO: WU-10 will implement this
fmt.Fprintln(os.Stderr, "Error: 'forge models' is not yet implemented")
os.Exit(1)
default:
cli.PrintUnknownCommand(os.Stderr, os.Args[1])
os.Exit(1)
}
}

// No args → start HTTP server (existing behavior)
application, err := app.New(app.WithVersion(version))
if err != nil {
log.Fatalf("Failed to initialize: %v", err)
}
defer application.Close()

// Startup banner
models := application.Registry.ListAllModels(context.Background())
fmt.Printf("\n🔥 Forge %s\n", application.Config.Version)
fmt.Printf("  API:    http://localhost%s/v1/chat/completions\n", application.Config.Addr)
fmt.Printf("  Health: http://localhost%s/api/health\n", application.Config.Addr)
fmt.Printf("  Chat UI: http://localhost%s/chat\n", application.Config.Addr)
fmt.Println()
fmt.Printf("  Providers: %d registered\n", len(application.Registry.Providers()))
fmt.Printf("  Models:    %d available\n", len(models))
fmt.Println()

// Create and start server with all dependencies
srv := server.New(application.Config, application.Registry, application.Store, application.SessionMgr, application.Auth)
srv.StartAndServe()
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

if len(registry.Providers()) == 0 {
registry.Register(inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"}))
registry.Register(inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"}))
registry.SetDefault("qwen")
}

useModel := cfg.DefaultModel
if *model != "" {
useModel = *model
}

ctx := context.Background()
_, err = cli.RunOnce(ctx, registry, useModel, *system, promptText, os.Stdout)
if err != nil {
fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
os.Exit(1)
}
}

func chatCmd() {
fs := flag.NewFlagSet("chat", flag.ExitOnError)
model := fs.String("model", "", "Model to use")
sessionID := fs.String("session", "", "Resume session by ID")
system := fs.String("system", "", "System prompt for new sessions")
fs.Parse(os.Args[2:])

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

if len(registry.Providers()) == 0 {
registry.Register(inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"}))
registry.Register(inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"}))
registry.SetDefault("qwen")
}

sessionMgr := session.NewManager(db, cfg.DefaultModel)

useModel := cfg.DefaultModel
if *model != "" {
useModel = *model
}

ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()

if err := cli.RunREPL(ctx, registry, sessionMgr, useModel, *sessionID, *system, os.Stdin, os.Stdout); err != nil {
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
os.Exit(1)
}
}

func sessionsCmd() {
fs := flag.NewFlagSet("sessions", flag.ExitOnError)
limit := fs.Int("limit", 0, "Limit number of results")
jsonOut := fs.Bool("json", false, "Output as JSON")
fs.Parse(os.Args[2:])

args := fs.Args()
action := ""
targetID := ""
if len(args) > 0 {
action = args[0]
}
if len(args) > 1 {
targetID = args[1]
}

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
sessionMgr := session.NewManager(db, cfg.DefaultModel)

if err := cli.RunSessions(context.Background(), sessionMgr, action, targetID, *limit, *jsonOut, os.Stdout); err != nil {
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
os.Exit(1)
}
}
