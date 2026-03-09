package cli

import (
	"fmt"
	"io"
)

// PrintUsage writes comprehensive usage information to w.
func PrintUsage(w io.Writer, version string) {
	fmt.Fprintf(w, `Cortex %s — AI Chat Backend & CLI

Usage: cortex [command] [flags]

Commands:
  (default)    Start the Cortex HTTP API server
  chat         Start an interactive multi-turn chat REPL
  run          Execute a one-shot prompt and exit
  sessions     List and manage chat sessions
  models       List available models and providers
  version      Print version information
  help         Show this help message

Run 'cortex <command> --help' for command-specific flags.

Environment:
  CORTEX_PROVIDER    Default provider (default: ollama)
  CORTEX_MODEL       Default model (default: llama3.2:latest)
  CORTEX_ADDR        Server listen address (default: :8080)
  CORTEX_API_KEY     API authentication key (optional)
  CORTEX_DB_PATH     SQLite database path (default: cortex.db)
  CORTEX_CONFIG      Path to JSON config file
  OLLAMA_URL        Ollama API URL (default: http://localhost:11434)

Config File:
  Provider configuration can be defined in a JSON config file.
  Discovery order: --config flag > $CORTEX_CONFIG > ~/.cortex/config.json > ./cortex.config.json

`, version)
}

// PrintVersion writes version information to w.
func PrintVersion(w io.Writer, version string) {
	fmt.Fprintf(w, "cortex %s\n", version)
}

// PrintUnknownCommand writes an error for unrecognized commands to w.
func PrintUnknownCommand(w io.Writer, cmd string) {
	fmt.Fprintf(w, "Error: unknown command %q\n\nRun 'cortex help' for usage information.\n", cmd)
}
