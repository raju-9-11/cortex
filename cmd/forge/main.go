package main

import (
	"context"
	"fmt"
	"log"

	"forge/internal/app"
	"forge/internal/server"
)

var version = "dev"

func main() {
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
