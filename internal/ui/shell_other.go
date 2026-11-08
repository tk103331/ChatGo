//go:build !windows

package ui

// explorerMenuSupported reports whether the folder context menu exists here.
func explorerMenuSupported() bool { return false }

// setExplorerMenu does nothing outside Windows.
func setExplorerMenu(bool) error { return nil }
