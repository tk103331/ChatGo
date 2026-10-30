package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/config"
)

// rtkRewriteTarget is a stub rtk whose "rewrite" maps known commands, and
// whose "gain" check passes so rtkStatus treats it as the real tool.
func rtkRewriteStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rtk")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--version) echo 'rtk 9.9.9' ;;\n" +
		"gain) echo '{}' ;;\n" +
		"rewrite) case \"$2\" in\n" +
		"  'git status') echo 'rtk git status' ;;\n" +
		"  *) exit 1 ;;\n" +
		"  esac ;;\n" +
		"*) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// rtkProbed seeds the window's cached probe so rtkRewrite skips detection.
func rtkProbed(cw *ChatWindow, path string) {
	cw.rtk.probe = rtkProbe{Path: path, Version: "rtk 9.9.9"}
	cw.rtk.probed = true
}

// commandlineHandler builds a real builtin commandline handler for a config.
func commandlineHandler(cw *ChatWindow, allowed string, root string) func(context.Context, string) (string, error) {
	return cw.builtinToolHandler(&config.BuiltinTool{
		Name: "commandline", Type: "commandline",
		Config: map[string]string{"allowed_commands": allowed},
	}, root, nil)
}

// A rewrite applies when RTK is on and an unknown command passes the allow
// list, but rtk's filter does not touch commands it does not support.
func TestRTKOptionallyRewritesCommand(t *testing.T) {
	rtkRewriteStub(t)
	dir := newTestRepo(t)
	probe := detectRTK()

	cwOn := testWindow()
	cwOn.cfg.RTKEnabled = true
	rtkProbed(cwOn, probe.Path)
	h := commandlineHandler(cwOn, "*", dir)
	out, err := h(context.Background(), "git status")
	if err != nil {
		t.Fatalf("git status under RTK: %v", err)
	}
	// rtk's own git filter prints a leading "* main" line and bare status
	// codes, unlike git's padded "On branch" wording.
	if !strings.Contains(out, "branch") && !strings.HasPrefix(out, "*") {
		t.Fatalf("an rtk-supportable command was not rewritten: %q", out)
	}

	cwOff := testWindow()
	cwOff.cfg.RTKEnabled = false
	h = commandlineHandler(cwOff, "*", dir)
	out, err = h(context.Background(), "git status")
	if err != nil {
		t.Fatalf("git status without RTK: %v", err)
	}
	if !strings.Contains(out, "branch") {
		t.Fatalf("an unchanged run lost git's own wording: %q", out)
	}
}

// The allow list is checked against the command the model asked for, before
// any rtk rewrite, so a command added by rtk's prefix cannot slip past it.
func TestRTKAllowListChecksOriginalCommand(t *testing.T) {
	rtkRewriteStub(t)
	dir := t.TempDir()
	probe := detectRTK()

	cw := testWindow()
	cw.cfg.RTKEnabled = true
	rtkProbed(cw, probe.Path)
	// git is allowed, so the unrewritten command passes; the rewrite is
	// what actually runs.
	h := commandlineHandler(cw, "git", dir)
	if _, err := h(context.Background(), "git status"); err != nil {
		t.Fatalf("git status with git allowed should pass: %v", err)
	}
	// ls is not allowed; the rewrite would be "rtk ls", but the original
	// already fails the gate.
	h = commandlineHandler(cw, "git", dir)
	if _, err := h(context.Background(), "ls"); err == nil || !strings.Contains(err.Error(), "不在允许列表") {
		t.Fatalf("ls with only git allowed should be refused, got %v", err)
	}
}

// RTK off skips rewrite entirely, so even an rtk-supported command runs raw
// while the tool is installed and reachable.
func TestRTKDisableSkipsRewrite(t *testing.T) {
	rtkRewriteStub(t)
	dir := newTestRepo(t)
	probe := detectRTK()
	cw := testWindow()
	cw.cfg.RTKEnabled = false
	rtkProbed(cw, probe.Path)
	h := commandlineHandler(cw, "*", dir)
	out, err := h(context.Background(), "git status")
	if err != nil {
		t.Fatalf("%v", err)
	}
	// git's own wording, not rtk's compact "* main" form.
	if !strings.Contains(out, "branch") {
		t.Fatalf("rewrite ran with RTK disabled: %q", out)
	}
}
