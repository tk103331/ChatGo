package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/a2a"
	"chatgo/internal/config"
	"chatgo/pkg/models"
)

func TestA2ASettingsSavesConnection(t *testing.T) {
	home := isolateHome(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsA2ATab
	cw.cfg.A2AAgents = []config.A2AConnection{{
		Name: "Remote", URL: "https://a.example.com", Disabled: true,
	}}
	cw.loadA2AForm(0)

	tr := mgo.NewTester(cw.view, 1000, 800)
	for range 6 {
		tr.Frame()
	}
	for _, want := range []string{"A2A", "Agent Card 地址", "https://a.example.com"} {
		if !tr.HasText(want) {
			t.Fatalf("the A2A tab lacks %q; texts: %v", want, tr.Texts())
		}
	}

	cw.settings.a2a.description = "research"
	cw.settings.a2a.apiKey = "sk-1"
	cw.settings.a2a.headers = "X-Tenant=team-a"
	cw.saveA2A()
	got := cw.cfg.A2AAgents[0]
	if got.Description != "research" || got.APIKey != "sk-1" || got.Headers["X-Tenant"] != "team-a" {
		t.Fatalf("saved connection = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(home, ".chatgo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "a2a_agents:") || !strings.Contains(string(data), "https://a.example.com") {
		t.Fatalf("config not persisted:\n%s", data)
	}
}

func TestRoomA2ASessionSurvivesPerMember(t *testing.T) {
	conv := &models.Conversation{}
	roomA2ASession(conv, "a2a:A", &a2a.Result{TaskID: "t1", ContextID: "c1"})
	roomA2ASession(conv, "a2a:B", &a2a.Result{TaskID: "t2", ContextID: "c2"})
	if s := conv.A2ASessionFor("a2a:A"); s.TaskID != "t1" || s.ContextID != "c1" {
		t.Fatalf("A session = %#v", s)
	}
	if s := conv.A2ASessionFor("a2a:B"); s.TaskID != "t2" {
		t.Fatalf("B session = %#v", s)
	}
	roomA2ASession(conv, "a2a:A", &a2a.Result{TaskID: "t1", ContextID: "c1", State: "TASK_STATE_COMPLETED"})
	if s := conv.A2ASessionFor("a2a:A"); s.TaskID != "" || s.ContextID != "c1" {
		t.Fatalf("terminal session = %#v", s)
	}
}

func TestA2AJoinsRoomsTasksAndChannels(t *testing.T) {
	cw := testWindow()
	cw.cfg.A2AAgents = []config.A2AConnection{{Name: "Remote", URL: "https://a.example.com"}}

	if refs := cw.roomCandidates(&roomForm{}); len(refs) != 1 || refs[0] != "a2a:Remote" {
		t.Fatalf("room candidates = %v", refs)
	}
	if got := agentLabel("a2a:Remote"); got != "A2A · Remote" {
		t.Fatalf("agent label = %q", got)
	}
	if got := channelAgentLabel("a2a:Remote"); got != "A2A · Remote" {
		t.Fatalf("channel label = %q", got)
	}
	if got, msg := cw.resolveChannelAgent("a2a:Remote"); got != "a2a:Remote" || msg != "" {
		t.Fatalf("resolve a2a = %q, %q", got, msg)
	}
}
