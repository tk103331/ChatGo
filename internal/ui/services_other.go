//go:build !darwin

package ui

// installServiceProvider is macOS only; other platforms reach the quick panel
// through the command line and chatgo:// links.
func installServiceProvider(cw *ChatWindow) {}
