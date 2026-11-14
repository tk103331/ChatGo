package ui

import (
	"strings"
	"testing"
)

// acpCatalogEntryOf finds a catalog entry by name.
func acpCatalogEntryOf(t *testing.T, name string) acpCatalogEntry {
	t.Helper()
	for _, e := range acpCatalog() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("catalog has no %q entry", name)
	return acpCatalogEntry{}
}

// Every catalog entry names a command, and none of them starts a CLI the
// way its interactive session would: the commands were checked against
// paseo's catalog, where a missing acp subcommand (or a wrong binary name)
// is the difference between the protocol and a REPL that never answers.
func TestACPCatalogCommandsMatchPaseo(t *testing.T) {
	for _, e := range acpCatalog() {
		if strings.TrimSpace(e.Command) == "" {
			t.Fatalf("%q has no command", e.Name)
		}
	}
	// Cursor's binary is cursor-agent; "agent" collides with other tools.
	if e := acpCatalogEntryOf(t, "Cursor"); e.Command != "cursor-agent" || !containsStr(e.Args, "acp") {
		t.Fatalf("Cursor should run cursor-agent acp, got %s %v", e.Command, e.Args)
	}
	// Gemini, Qwen and Goose speak ACP behind a flag or subcommand; the bare
	// command would open their interactive sessions.
	if e := acpCatalogEntryOf(t, "GeminiCLI"); !containsStr(e.Args, "--acp") {
		t.Fatalf("Gemini needs --acp, got %s %v", e.Command, e.Args)
	}
	if e := acpCatalogEntryOf(t, "QwenCode"); !containsStr(e.Args, "--acp") {
		t.Fatalf("Qwen needs --acp, got %s %v", e.Command, e.Args)
	}
	for _, name := range []string{"Goose", "KimiCLI", "Kilo", "Hermes"} {
		if e := acpCatalogEntryOf(t, name); !containsStr(e.Args, "acp") {
			t.Fatalf("%s needs its acp subcommand, got %s %v", name, e.Command, e.Args)
		}
	}
	// Cline speaks ACP natively now; the cline-acp wrapper is outdated.
	e := acpCatalogEntryOf(t, "Cline")
	if e.Command != "npx" || !containsStr(e.Args, "cline") || containsStr(e.Args, "cline-acp") {
		t.Fatalf("Cline should run npx cline --acp, got %s %v", e.Command, e.Args)
	}
	// MiniMax publishes its binary under the package's unscoped name, so the
	// npx invocation must name both.
	if e := acpCatalogEntryOf(t, "MiniMaxCode"); !containsStr(e.Args, "-p") || !containsStr(e.Args, "mcode") {
		t.Fatalf("MiniMax needs -p <package> mcode acp, got %s %v", e.Command, e.Args)
	}
	// Factory Droid runs the daemon output format with its own updater off.
	e = acpCatalogEntryOf(t, "FactoryDroid")
	if !containsStr(e.Args, "acp-daemon") {
		t.Fatalf("Factory Droid needs --output-format acp-daemon, got %s %v", e.Command, e.Args)
	}
	if e.Env["DROID_DISABLE_AUTO_UPDATE"] != "true" {
		t.Fatalf("Factory Droid should disable its own updater, got %v", e.Env)
	}
}

// A catalog entry's env rides the connection it adds, so adapters that need
// a variable set get it without the user knowing it exists.
func TestAddACPFromCatalogCarriesEnv(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.addACPFromCatalog(acpCatalogEntryOf(t, "AugmentCode"))
	if len(cw.cfg.ACPAgents) != 1 {
		t.Fatalf("the entry was not added: %+v", cw.cfg.ACPAgents)
	}
	conn := cw.cfg.ACPAgents[0]
	if conn.Env["AUGMENT_DISABLE_AUTO_UPDATE"] != "1" {
		t.Fatalf("the env was lost: %+v", conn.Env)
	}
	if conn.Command != "npx" {
		t.Fatalf("the command was lost: %+v", conn)
	}
}
