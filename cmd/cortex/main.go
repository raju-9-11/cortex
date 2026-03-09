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

	"cortex/internal/app"
	"cortex/internal/cli"
	"cortex/internal/server"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run dispatches subcommands and returns an exit code.
// Separated from main() for testability.
func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "help", "--help", "-h":
			cli.PrintUsage(stdout, version)
			return 0
		case "version", "--version":
			cli.PrintVersion(stdout, version)
			return 0
		case "run":
			return runCmd(args[1:], stdin, stdout, stderr)
		case "chat":
			return chatCmd(args[1:], stdin, stdout, stderr)
		case "sessions":
			return sessionsCmd(args[1:], stdout, stderr)
		case "models":
			return modelsCmd(args[1:], stdout, stderr)
		default:
			cli.PrintUnknownCommand(stderr, args[0])
			return 1
		}
	}
	return startServer(args, stdout, stderr)
}

// startServer starts the Cortex HTTP API server.
func startServer(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	model := fs.String("model", "", "Default model to use")
	provider := fs.String("provider", "", "Default provider to use")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	application, err := app.New(app.WithVersion(version))
	if err != nil {
		log.New(stderr, "", 0).Fatalf("Failed to initialize: %v", err)
		return 1
	}
	defer application.Close()

	// Override defaults from flags if provided
	if *model != "" {
		application.Config.DefaultModel = *model
	}
	if *provider != "" {
		application.Config.DefaultProvider = *provider
		application.Registry.SetDefault(*provider)
	}

	// Startup banner
	models := application.Registry.ListAllModels(context.Background())
	fmt.Fprintf(stdout, "\n🔥 Cortex %s\n", application.Config.Version)
	fmt.Fprintf(stdout, "  API:    http://localhost%s/v1/chat/completions\n", application.Config.Addr)
	fmt.Fprintf(stdout, "  Health: http://localhost%s/api/health\n", application.Config.Addr)
	fmt.Fprintf(stdout, "  Chat UI: http://localhost%s/chat\n", application.Config.Addr)
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "  Providers: %d registered\n", len(application.Registry.Providers()))
	fmt.Fprintf(stdout, "  Models:    %d available\n", len(models))
	fmt.Fprintf(stdout, "  Default:   %s / %s\n", application.Config.DefaultProvider, application.Config.DefaultModel)
	fmt.Fprintln(stdout)

	srv := server.New(application.Config, application.Registry, application.Store, application.SessionMgr, application.Auth)
	srv.StartAndServe()
	return 0
}

func runCmd(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prompt := fs.String("prompt", "", "Prompt text")
	model := fs.String("model", "", "Model to use (e.g., ollama/llama3.2)")
	system := fs.String("system", "", "System prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	promptText, err := cli.ReadPrompt(stdin, *prompt)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		if err == cli.ErrNoPrompt {
			fmt.Fprintln(stderr, "Usage: cortex run --prompt \"your question here\"")
			fmt.Fprintln(stderr, "       echo \"your question\" | cortex run")
		}
		return 1
	}

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	defer application.Close()

	useModel := application.Config.DefaultModel
	if *model != "" {
		useModel = *model
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	_, err = cli.RunOnce(ctx, application.Registry, useModel, *system, promptText, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "\nError: %v\n", err)
		return 1
	}
	return 0
}

func chatCmd(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(stderr)
	model := fs.String("model", "", "Model to use")
	sessionID := fs.String("session", "", "Resume session by ID")
	system := fs.String("system", "", "System prompt for new sessions")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	defer application.Close()

	useModel := application.Config.DefaultModel
	if *model != "" {
		useModel = *model
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cli.RunREPL(ctx, application.Registry, application.SessionMgr, useModel, *sessionID, *system, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func sessionsCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 0, "Limit number of results")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	remaining := fs.Args()
	action := ""
	targetID := ""
	if len(remaining) > 0 {
		action = remaining[0]
	}
	if len(remaining) > 1 {
		targetID = remaining[1]
	}

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	defer application.Close()

	if err := cli.RunSessions(context.Background(), application.SessionMgr, action, targetID, *limit, *jsonOut, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func modelsCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "Filter by provider name")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	defer application.Close()

	if err := cli.RunModels(context.Background(), application.Registry, *provider, application.Config.DefaultProvider, application.Config.DefaultModel, *jsonOut, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
