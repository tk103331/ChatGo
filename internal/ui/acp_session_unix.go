//go:build !windows

package ui

import "syscall"

// acpSysProcAttr puts the child in its own process group so the whole tree
// (npx → node) can be killed together.
func acpSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// acpKillProcessGroup kills the process group led by pid.
func acpKillProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
