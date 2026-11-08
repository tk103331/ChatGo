//go:build darwin || linux

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// cliInstallSupported reports whether the app can install a chatgo command.
func cliInstallSupported() bool { return true }

// installCLI links ~/.local/bin/chatgo to this executable and returns the
// link's path. A development build is refused on macOS: its executable is a
// temporary file that disappears after the next build.
func installCLI() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if runtime.GOOS == "darwin" && !strings.Contains(exe, ".app/Contents/MacOS/") {
		return "", fmt.Errorf("请先打包成 ChatGo.app 再安装命令行工具")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, "chatgo")
	_ = os.Remove(link)
	if err := os.Symlink(exe, link); err != nil {
		return "", err
	}
	return link, nil
}
