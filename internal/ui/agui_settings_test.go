package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
)

func TestAGUISettingsSavesConnection(t *testing.T) {
	home := isolateHome(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsAGUITab
	cw.cfg.AGUIAgents = []config.AGUIConnection{{
		Name: "Local", URL: "https://agent.example.com/agui", Disabled: true,
	}}
	cw.loadAGUIForm(0)

	tr := mgo.NewTester(cw.view, 1000, 800)
	for range 6 {
		tr.Frame()
	}
	for _, want := range []string{"AG-UI", "运行端点 URL", "https://agent.example.com/agui"} {
		if !tr.HasText(want) {
			t.Fatalf("the AG-UI tab lacks %q; texts: %v", want, tr.Texts())
		}
	}

	cw.settings.agui.description = "local agent"
	cw.settings.agui.apiKey = "sk-1"
	cw.settings.agui.headers = "X-Tenant=team-a"
	cw.saveAGUI()
	got := cw.cfg.AGUIAgents[0]
	if got.Description != "local agent" || got.APIKey != "sk-1" || got.Headers["X-Tenant"] != "team-a" {
		t.Fatalf("saved connection = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(home, ".chatgo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agui_agents:") || !strings.Contains(string(data), "https://agent.example.com/agui") {
		t.Fatalf("config not persisted:\n%s", data)
	}
}

func TestAGUIAgentJoinsPickerAndWebRuntime(t *testing.T) {
	cw := testWindow()
	cw.cfg.AGUIAgents = []config.AGUIConnection{{Name: "Local", URL: "https://agent.example.com/agui"}}
	found := false
	for _, item := range cw.agentPickItems() {
		if item.ref == "agui:Local" && item.label == "Local" {
			found = true
		}
	}
	if !found {
		t.Fatalf("picker items lack the AG-UI agent: %+v", cw.agentPickItems())
	}
	if got := agentLabel("agui:Local"); got != "AG-UI · Local" {
		t.Fatalf("agent label = %q", got)
	}
	if got := cw.agentIconPath("agui:Local"); got != "" {
		t.Fatalf("icon path = %q", got)
	}
}
