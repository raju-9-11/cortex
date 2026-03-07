package main

import (
"context"
"fmt"
"log"
"os"

"forge/internal/app"
"forge/internal/cli"
"forge/internal/server"
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
// TODO: WU-06 will implement this
fmt.Fprintln(os.Stderr, "Error: 'forge run' is not yet implemented")
os.Exit(1)
case "chat":
// TODO: WU-07 will implement this
fmt.Fprintln(os.Stderr, "Error: 'forge chat' is not yet implemented")
os.Exit(1)
case "sessions":
// TODO: WU-08 will implement this
fmt.Fprintln(os.Stderr, "Error: 'forge sessions' is not yet implemented")
os.Exit(1)
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
