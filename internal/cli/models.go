package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"forge/internal/inference"
)

// RunModels lists all available models from the registry.
func RunModels(ctx context.Context, registry *inference.ProviderRegistry,
	filterProvider string, defaultProvider string, defaultModel string,
	jsonOutput bool, w io.Writer) error {

	models := registry.ListAllModels(ctx)

	// Filter by provider if requested
	if filterProvider != "" {
		filtered := models[:0]
		for _, m := range models {
			if m.Provider == filterProvider {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}

	if len(models) == 0 {
		fmt.Fprintln(w, "No models available. Check provider configuration.")
		return nil
	}

	// Sort by provider, then model ID
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})

	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(models)
	}

	// Table output
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PROVIDER\tMODEL\tCREATED")
	for _, m := range models {
		fmt.Fprintf(tw, "%s\t%s\t%d\n", m.Provider, m.ID, m.Created)
	}
	tw.Flush()

	// Count unique providers
	providers := make(map[string]bool)
	for _, m := range models {
		providers[m.Provider] = true
	}

	fmt.Fprintf(w, "\n%d provider(s), %d model(s) available\n", len(providers), len(models))
	fmt.Fprintf(w, "Default: %s / %s\n", defaultProvider, defaultModel)

	return nil
}
