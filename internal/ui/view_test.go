package ui

import (
	"testing"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"
)

func testWindow() *ChatWindow {
	cfg := &config.Config{
		Providers:         []config.Provider{{Name: "OpenAI", Type: "openai", Model: "gpt-4", Enabled: true}},
		CurrentProvider:   "OpenAI",
		ReactAgentMaxStep: 20,
	}
	return &ChatWindow{
		cfg:        cfg,
		mcpManager: NewMCPManagerWrapper(),
		provider:   "OpenAI",
		toolOn:     map[string]bool{},
		editingIdx: -1,
	}
}

func TestHomeViewRenders(t *testing.T) {
	cw := testWindow()
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("home render failed: %v", img)
	}
}

func TestChatViewRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{
		ID:       "c1",
		Title:    "Test",
		Provider: "OpenAI",
		Messages: []models.Message{
			{ID: "u1", Role: "user", Content: "你好", Timestamp: time.Now()},
			{ID: "a1", Role: "assistant", Content: "# 标题\n\n**加粗** 与 `代码`\n\n- 一\n- 二", Timestamp: time.Now(),
				ToolCalls: []models.ToolCall{{ID: "t1", Name: "search", Arguments: "{}", Result: "ok"}}},
		},
	}
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("chat render failed: %v", img)
	}
}

func TestSettingsDialogRenders(t *testing.T) {
	cw := testWindow()
	cw.settings.open = true
	cw.settings.prov.editing = 0
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("settings render failed: %v", img)
	}
}

func TestSidebarSearchRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.searchQuery = "hello"
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "Hello world", Messages: []models.Message{{ID: "m1", Role: "user", Content: "hello there"}}},
		{ID: "c2", Title: "Other", Messages: []models.Message{{ID: "m2", Role: "user", Content: "nope"}}},
	}
	cw.cur = &cw.convs[0]
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("search render failed: %v", img)
	}
}

func TestAppearanceTabRenders(t *testing.T) {
	cw := testWindow()
	cw.settings.open = true
	cw.settings.tab = 3
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("appearance render failed: %v", img)
	}
}
