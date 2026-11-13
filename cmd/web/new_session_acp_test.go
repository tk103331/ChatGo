package main

import (
	"testing"

	ui "github.com/egoist/mygo/ui"
)

// Picking an ACP agent in the new-session dialog shows the runtime's model
// and its config options, and the choices ride the new conversation.
func TestWebNewSessionACPOptions(t *testing.T) {
	a := testApp()
	a.newSessionOpen = true
	a.newAgent = "acp:OpenCode"
	a.agents = append(a.agents, agentInfo{
		Ref: "acp:OpenCode", Name: "OpenCode", Kind: "acp", Description: "opencode acp",
	})
	a.acpOptions = map[string]acpOptionsState{"OpenCode": {options: []acpConfigOptionInfo{
		{ID: "model", Name: "Model", Kind: "model", Current: "m1", Options: []acpConfigValue{
			{Value: "m1", Name: "小模型"}, {Value: "m2", Name: "大模型"},
		}},
		{ID: "mode", Name: "Mode", Kind: "select", Current: "fast", Options: []acpConfigValue{
			{Value: "fast", Name: "快速"},
		}},
	}}}
	tr := ui.NewTester(a.view, 1180, 820)
	for range 4 {
		tr.Frame()
	}
	for _, want := range []string{"模型", "Mode", "小模型"} {
		if !tr.HasText(want) {
			t.Fatalf("the dialog lacks %q; texts: %v", want, tr.Texts())
		}
	}
	// The choices made in the dialog ride the new conversation.
	a.setNewACPConfig("model", "m2")
	body := a.newSessionBody()
	cfg, ok := body["acp_config"].(map[string]string)
	if !ok || cfg["model"] != "m2" {
		t.Fatalf("the ACP choice did not reach the request: %v", body)
	}
}
