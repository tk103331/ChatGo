// Package shellenv widens the process environment so child processes resolve
// commands from the user's login shell, even when the app is launched from
// Finder/Dock on macOS with a minimal launchd PATH.
package shellenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const loginShellTimeout = 3 * time.Second

// AugmentPath merges the login shell PATH and common per-user bin directories
// into the process PATH so exec.Command (ACP agents, MCP servers, git, ...)
// can find tools installed outside the default system directories.
func AugmentPath() {
	current := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	extra := append(loginShellPath(), commonBinDirs()...)
	merged := mergePaths(current, extra)
	_ = os.Setenv("PATH", strings.Join(merged, string(os.PathListSeparator)))
}

// ResolveCommand returns the absolute path of an executable by name, searching
// the process PATH first and then the login shell PATH and common bin
// directories. A name that is already a path is returned when it is executable.
func ResolveCommand(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if strings.ContainsRune(name, os.PathSeparator) {
		if !isExecutable(name) {
			return "", false
		}
		if abs, err := filepath.Abs(name); err == nil {
			return abs, true
		}
		return name, true
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	names := []string{name}
	if runtime.GOOS == "windows" {
		names = append(names, name+".exe", name+".cmd", name+".bat")
	}
	for _, dir := range append(loginShellPath(), commonBinDirs()...) {
		if dir == "" {
			continue
		}
		for _, n := range names {
			if p := filepath.Join(dir, n); isExecutable(p) {
				return p, true
			}
		}
	}
	return "", false
}

// isExecutable reports whether path names a runnable regular file.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// commonBinDirs lists directories where per-user toolchains install binaries.
func commonBinDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	dirs := []string{
		filepath.Join(home, ".opencode", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".deno", "bin"),
		filepath.Join(home, ".cargo", "bin"),
		filepath.Join(home, ".volta", "bin"),
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, "go", "bin"),
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
	}
	if home == "" {
		var filtered []string
		for _, d := range dirs {
			if filepath.IsAbs(d) {
				filtered = append(filtered, d)
			}
		}
		return filtered
	}
	return dirs
}

// loginShellPath asks the user's login shell for its PATH, giving up after a
// short timeout so a slow rc file never blocks startup. A marker isolates the
// value from any banner the rc file prints.
func loginShellPath() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginShellTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '\n%s%s' __CHATGO_PATH__ "$PATH"`).Output()
	if err != nil {
		return nil
	}
	const marker = "__CHATGO_PATH__"
	text := string(out)
	i := strings.LastIndex(text, marker)
	if i < 0 {
		return nil
	}
	value := text[i+len(marker):]
	if j := strings.IndexByte(value, '\n'); j >= 0 {
		value = value[:j]
	}
	if value == "" {
		return nil
	}
	return strings.Split(value, string(os.PathListSeparator))
}

// mergePaths concatenates base and extra, dropping empties and duplicates while
// keeping the first occurrence's order.
func mergePaths(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, p := range append(append([]string{}, base...), extra...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
