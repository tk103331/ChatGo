// Command gen-plugin-catalog packs the shipped plugins into per-plugin
// zips for the release and writes the index. Incremental: a plugin whose
// manifest version matches the previous index keeps its zip, so a release
// uploads only what changed. The previous index is the committed
// catalog-index.json (and, when present, the release's own published
// index); pass -prev to name the file to diff against.
//
// Usage:
//
//	go run ./tools/gen-plugin-catalog                 # pack changed, write index
//	go run ./tools/gen-plugin-catalog -full           # pack everything
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"chatgo/internal/plugin"
)

func main() {
	catalog := flag.String("catalog", "plugins/catalog", "the shipped plugin sources")
	out := flag.String("out", "dist/plugins", "where the per-plugin zips are written")
	indexOut := flag.String("index", "internal/plugin/catalog-index.json", "where the embedded index is written")
	prev := flag.String("prev", "internal/plugin/catalog-index.json", "the previous index to diff against")
	full := flag.Bool("full", false, "pack every plugin, not only the changed ones")
	flag.Parse()

	var previous []plugin.CatalogEntry
	if !*full {
		if data, err := os.ReadFile(*prev); err == nil {
			if err := json.Unmarshal(data, &previous); err != nil {
				fmt.Fprintf(os.Stderr, "gen-plugin-catalog: the previous index is unreadable (%v); packing everything\n", err)
			}
		}
	}

	entries, err := plugin.PackCatalog(*catalog, *out, previous)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pack:", err)
		os.Exit(1)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*indexOut), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*indexOut, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The index itself travels to the release as an asset, so the app can
	// refresh its catalog metadata from there.
	if err := os.WriteFile(filepath.Join(*out, plugin.CatalogIndexFile), append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("index written to %s (also to %s)\n", *indexOut, filepath.Join(*out, plugin.CatalogIndexFile))
}
