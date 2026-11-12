package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"chatgo/internal/shellenv"
)

// shellCommandContext builds a shell invocation that runs script in dir. The
// shell is chosen per platform so command execution works on Windows, where
// sh.exe is not part of the system and may never have been installed.
func shellCommandContext(ctx context.Context, script, dir string) *exec.Cmd {
	name, args := shellInvocation(runtime.GOOS)
	c := exec.CommandContext(ctx, name, append(args, script)...)
	if dir != "" {
		c.Dir = dir
	}
	return c
}

// shellInvocation returns the shell program and the flags that come before the
// script. POSIX sh stays the first choice on every platform: the command allow
// list and the commands agents write assume its syntax, and Git Bash/MSYS
// provide it on Windows. Without sh, Windows falls back to the native
// COMSPEC (cmd.exe), which is always present.
func shellInvocation(goos string) (string, []string) {
	return shellInvocationWith(goos, findPOSIXShell, os.Getenv)
}

// shellInvocationWith is shellInvocation with its platform inputs injected so
// the Windows fallback can be tested on any host.
func shellInvocationWith(goos string, findShell func() (string, bool), getenv func(string) string) (string, []string) {
	if goos != "windows" {
		return "sh", []string{"-c"}
	}
	if sh, ok := findShell(); ok {
		return sh, []string{"-c"}
	}
	comspec := strings.TrimSpace(getenv("COMSPEC"))
	if comspec == "" {
		comspec = "cmd.exe"
	}
	return comspec, []string{"/d", "/s", "/c"}
}

// findPOSIXShell looks for an sh on PATH and in the usual per-user tool
// directories, including Git for Windows installs that are not on PATH.
func findPOSIXShell() (string, bool) {
	return shellenv.ResolveCommand("sh")
}

// shellDescription tells the model which command syntax the commandline tool
// runs, so it does not keep sending Unix commands to a Windows cmd shell.
func shellDescription() string {
	name, _ := shellInvocation(runtime.GOOS)
	switch strings.ToLower(filepath.Base(name)) {
	case "cmd", "cmd.exe":
		return "Windows 命令提示符（cmd）语法，如 dir、type、where、findstr"
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return "PowerShell 语法"
	default:
		return "POSIX shell（sh）语法"
	}
}
