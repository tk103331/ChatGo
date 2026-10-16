//go:build windows

package ui

import "syscall"

// acpSysProcAttr is a no-op on Windows, which has no POSIX process groups.
func acpSysProcAttr() *syscall.SysProcAttr { return nil }

// acpKillProcessGroup is a no-op on Windows; the caller kills the process.
func acpKillProcessGroup(int) {}
