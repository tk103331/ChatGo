package config

import (
	"runtime"
	"strings"
	"testing"
)

// A new install gets commands its own platform can run: the Unix list alone
// leaves a Windows cmd shell unable to do anything but echo.
func TestCommandlineDefaultMatchesPlatform(t *testing.T) {
	tool := defaultBuiltinTool("commandline")
	got := tool.Config["allowed_commands"]
	if runtime.GOOS == "windows" {
		for _, want := range []string{"dir", "type", "findstr", "where"} {
			if !strings.Contains(got, want) {
				t.Fatalf("Windows allow list %q is missing %q", got, want)
			}
		}
		return
	}
	if got != defaultAllowedCommands {
		t.Fatalf("allow list is %q, want the Unix default %q", got, defaultAllowedCommands)
	}
}

// The old Unix-only default is upgraded on Windows, while a list the user
// edited is left alone.
func TestMigrateBuiltinToolOnlyRewritesShippedDefault(t *testing.T) {
	shipped := BuiltinTool{
		Name: "commandline", Type: "commandline",
		Config: map[string]string{"allowed_commands": defaultAllowedCommands},
	}
	got := migrateBuiltinTool(shipped)
	if runtime.GOOS == "windows" {
		if got.Config["allowed_commands"] != defaultAllowedCommandsWindows {
			t.Fatalf("Windows kept the Unix-only default: %q", got.Config["allowed_commands"])
		}
	} else if got.Config["allowed_commands"] != defaultAllowedCommands {
		t.Fatalf("the Unix default changed to %q", got.Config["allowed_commands"])
	}

	custom := BuiltinTool{
		Name: "commandline", Type: "commandline",
		Config: map[string]string{"allowed_commands": "git, ls"},
	}
	if got := migrateBuiltinTool(custom); got.Config["allowed_commands"] != "git, ls" {
		t.Fatalf("a custom allow list was rewritten to %q", got.Config["allowed_commands"])
	}
}
