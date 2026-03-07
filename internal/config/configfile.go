package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileConfig represents the JSON configuration file structure.
type FileConfig struct {
	DefaultProvider string          `json:"default_provider,omitempty"`
	DefaultModel    string          `json:"default_model,omitempty"`
	Providers       []ProviderEntry `json:"providers,omitempty"`
}

// ProviderEntry defines a single provider configuration.
type ProviderEntry struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key,omitempty"`
}

// DiscoverConfigFile finds the first config file that exists.
// Search order: envPath (from $FORGE_CONFIG) > ~/.forge/config.json > ./forge.config.json
// Returns empty string if no config file found.
// Warns if the config file has overly permissive permissions.
func DiscoverConfigFile(envPath string) string {
	candidates := []string{}

	if envPath != "" {
		candidates = append(candidates, envPath)
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".forge", "config.json"))
	}

	candidates = append(candidates, "forge.config.json")

	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil {
			if info.Mode().Perm()&0077 != 0 {
				fmt.Fprintf(os.Stderr, "WARNING: %s has permissions %o, should be 0600 to protect API keys\n", path, info.Mode().Perm())
			}
			return path
		}
	}
	return ""
}

// LoadConfigFile reads and parses a JSON config file.
// Returns nil FileConfig (not error) if path is empty.
func LoadConfigFile(path string) (*FileConfig, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	return &fc, nil
}
