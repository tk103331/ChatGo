//go:build !darwin && !linux

package ui

import "fmt"

// cliInstallSupported reports whether the app can install a chatgo command.
func cliInstallSupported() bool { return false }

// installCLI does nothing outside macOS and Linux.
func installCLI() (string, error) {
	return "", fmt.Errorf("当前平台不支持安装命令行工具")
}
