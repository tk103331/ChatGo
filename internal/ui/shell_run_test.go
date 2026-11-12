package ui

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestShellInvocationUnixUsesSH(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		name, args := shellInvocationWith(goos,
			func() (string, bool) { return `C:\Git\bin\sh.exe`, true },
			func(string) string { return `C:\WINDOWS\system32\cmd.exe` })
		if name != "sh" {
			t.Fatalf("%s uses shell %q, want sh", goos, name)
		}
		if len(args) != 1 || args[0] != "-c" {
			t.Fatalf("%s arguments are %v, want [-c]", goos, args)
		}
	}
}

func TestShellInvocationWindowsPrefersSH(t *testing.T) {
	const sh = `C:\Program Files\Git\bin\sh.exe`
	name, args := shellInvocationWith("windows",
		func() (string, bool) { return sh, true },
		func(string) string { return `C:\WINDOWS\system32\cmd.exe` })
	if name != sh {
		t.Fatalf("shell is %q, want the sh found on PATH %q", name, sh)
	}
	if len(args) != 1 || args[0] != "-c" {
		t.Fatalf("arguments are %v, want [-c]", args)
	}
}

func TestShellInvocationWindowsFallsBackToCmd(t *testing.T) {
	const comspec = `C:\WINDOWS\system32\cmd.exe`
	name, args := shellInvocationWith("windows",
		func() (string, bool) { return "", false },
		func(k string) string {
			if k == "COMSPEC" {
				return comspec
			}
			return ""
		})
	if name != comspec {
		t.Fatalf("shell is %q, want COMSPEC %q", name, comspec)
	}
	if strings.Join(args, " ") != "/d /s /c" {
		t.Fatalf("arguments are %v, want [/d /s /c]", args)
	}
}

func TestShellInvocationWindowsDefaultsToCmdExe(t *testing.T) {
	name, args := shellInvocationWith("windows",
		func() (string, bool) { return "", false },
		func(string) string { return "" })
	if name != "cmd.exe" {
		t.Fatalf("shell is %q, want cmd.exe", name)
	}
	if len(args) != 3 || args[2] != "/c" {
		t.Fatalf("arguments are %v, want the cmd /c form", args)
	}
}

// The shared helper actually runs the script and reports its output, which is
// what the commandline tool and the terminal app rely on.
func TestShellCommandContextRunsScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script below uses POSIX sh syntax")
	}
	out, err := shellCommandContext(context.Background(), "printf shell-ok", t.TempDir()).CombinedOutput()
	if err != nil {
		t.Fatalf("running the script: %v (%s)", err, out)
	}
	if string(out) != "shell-ok" {
		t.Fatalf("output is %q, want shell-ok", out)
	}
}

func TestContainsCommandWindowsIsCaseInsensitive(t *testing.T) {
	allowed := []string{"dir", "powershell.exe"}
	for _, name := range []string{"dir", "DIR", "dir.exe", "Dir.Exe"} {
		if !containsCommandFor("windows", allowed, name) {
			t.Fatalf("%q was refused on Windows", name)
		}
	}
	if !containsCommandFor("windows", allowed, "PowerShell") {
		t.Fatal("powershell without .exe was refused on Windows")
	}
	if containsCommandFor("windows", allowed, "rm") {
		t.Fatal("rm passed a Windows allow list that does not name it")
	}
}

func TestContainsCommandUnixIsCaseSensitive(t *testing.T) {
	if containsCommandFor("linux", []string{"dir"}, "DIR") {
		t.Fatal("a Unix allow list matched a differently cased command")
	}
}
