package ui

import (
	"testing"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"
)

// A failed warm-up is named on the connection's settings card, with a retry.
func TestACPWarmErrorShownInSettings(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = 2
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "Claude Code", Command: "claude-code-acp"}}
	cw.acp.modelErrs["Claude Code"] = "启动超时"
	cw.acpForm.editing = 0
	cw.loadACPForm(0)
	tr := mgo.NewTester(cw.view, 1280, 820)
	for range 5 {
		tr.Frame()
	}
	if !tr.HasText("预热失败") {
		t.Fatalf("the card does not show the warm-up failure; texts: %v", tr.Texts())
	}
	if !tr.HasText("启动超时") {
		t.Fatalf("the form does not show the warm-up error; texts: %v", tr.Texts())
	}
	if !tr.HasText("重试") {
		t.Fatalf("the form offers no retry; texts: %v", tr.Texts())
	}
}

// Enabling a connection warms it: the cached failure is cleared and a fresh
// probe starts.
func TestACPWarmConnectionResetsState(t *testing.T) {
	cw := testWindow()
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "Gemini", Command: "no-such-cli-xyz"}}
	cw.acp.modelErrs["Gemini"] = "旧错误"
	cw.acp.models["Gemini"] = []acpModelOption{{ID: "m1", Name: "旧版本"}}
	cw.acpWarmConnection("Gemini")

	cw.acp.mu.Lock()
	_, hasErr := cw.acp.modelErrs["Gemini"]
	_, hasModels := cw.acp.models["Gemini"]
	busy := cw.acp.modelsBusy["Gemini"]
	cw.acp.mu.Unlock()
	if hasErr {
		t.Fatal("enabling must clear the cached failure")
	}
	if hasModels {
		t.Fatal("enabling must drop the stale model list")
	}
	if !busy {
		t.Fatal("enabling must start a fresh probe")
	}
}

// The startup warm covers every enabled connection and skips the disabled
// ones.
func TestACPStartupInitWarmsEnabledOnly(t *testing.T) {
	cw := testWindow()
	cw.cfg.ACPAgents = []config.ACPConnection{
		{Name: "On", Command: "no-such-cli-xyz"},
		{Name: "Off", Command: "no-such-cli-xyz", Disabled: true},
	}
	cw.acpStartupInit()
	cw.acp.mu.Lock()
	_, on := cw.acp.modelsBusy["On"]
	_, off := cw.acp.modelsBusy["Off"]
	cw.acp.mu.Unlock()
	if !on {
		t.Fatal("an enabled connection must be probed at startup")
	}
	if off {
		t.Fatal("a disabled connection must stay untouched")
	}
}
