// Command gen-plugin-catalog packs the shipped plugins into per-plugin
// zips for the release and writes the embedded index. Run it before a
// release; it prints the sha256 of every zip for the release notes.
//
// Usage: go run ./tools/gen-plugin-catalog -catalog plugins/catalog -out dist/plugins
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
	flag.Parse()

	entries, err := plugin.PackCatalog(*catalog, *out)
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
	fmt.Printf("packed %d plugins into %s\nindex written to %s\n", len(entries), *out, *indexOut)
}
