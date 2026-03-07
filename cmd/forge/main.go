package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"forge/internal/app"
	"forge/internal/cli"
	"forge/internal/server"
)

var version = "dev"

func main() {
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
			modelsCmd()
			return
		default:
			cli.PrintUnknownCommand(os.Stderr, os.Args[1])
			os.Exit(1)
		}
	}

	// No args → start HTTP server
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

	srv := server.New(application.Config, application.Registry, application.Store, application.SessionMgr, application.Auth)
	srv.StartAndServe()
}

func runCmd() {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	prompt := fs.String("prompt", "", "Prompt text")
	model := fs.String("model", "", "Model to use (e.g., ollama/llama3.2)")
	system := fs.String("system", "", "System prompt")
	fs.Parse(os.Args[2:])

	promptText, err := cli.ReadPrompt(os.Stdin, *prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if err == cli.ErrNoPrompt {
			fmt.Fprintln(os.Stderr, "Usage: forge run --prompt \"your question here\"")
			fmt.Fprintln(os.Stderr, "       echo \"your question\" | forge run")
		}
		os.Exit(1)
	}

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	useModel := application.Config.DefaultModel
	if *model != "" {
		useModel = *model
	}

	_, err = cli.RunOnce(context.Background(), application.Registry, useModel, *system, promptText, os.Stdout)
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

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	useModel := application.Config.DefaultModel
	if *model != "" {
		useModel = *model
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cli.RunREPL(ctx, application.Registry, application.SessionMgr, useModel, *sessionID, *system, os.Stdin, os.Stdout); err != nil {
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

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	if err := cli.RunSessions(context.Background(), application.SessionMgr, action, targetID, *limit, *jsonOut, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func modelsCmd() {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	provider := fs.String("provider", "", "Filter by provider name")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.Parse(os.Args[2:])

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	if err := cli.RunModels(context.Background(), application.Registry, *provider, application.Config.DefaultProvider, application.Config.DefaultModel, *jsonOut, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
