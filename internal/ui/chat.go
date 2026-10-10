package ui

import (
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	mnavigation "github.com/ZacharyZhang-NY/MujicaUI/navigation"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// chatView lays out the sidebar, message list and composer.
func (cw *ChatWindow) chatView(c *mgo.Context) {
	msgs := cw.messages()
	mchat.ChatContainer(c, &cw.msgList, len(msgs), mchat.ChatContainerOptions{
		List: mchat.MessageListOptions{
			ID:    func(i int) string { return msgs[i].ID },
			Date:  func(i int) time.Time { return msgs[i].Timestamp },
			Label: func(i int) string { return msgs[i].Role },
		},
		Sidebar:      func() { cw.sidebar(c) },
		Input:        func() { cw.composer(c) },
		SidebarWidth: 280,
	}, func(i int) { cw.messageRow(c, i, msgs[i]) })
}

func (cw *ChatWindow) messages() []models.Message {
	if cw.cur == nil {
		return nil
	}
	return cw.cur.Messages
}

// composer builds the message input with its model, mode and tool controls.
func (cw *ChatWindow) composer(c *mgo.Context) {
	in := mchat.PromptComposer(c, &cw.draft, mchat.PromptComposerOptions{
		Placeholder: "输入消息，Enter 发送，Shift+Enter 换行",
		Tools: func() {
			cw.modelSelector(c)
			cw.modeSelector(c)
			cw.toolMenu(c)
		},
		Actions: func() {
			r := mchat.SendButton(c, mchat.SendButtonOptions{
				Streaming: cw.streaming,
				Disabled:  strings.TrimSpace(cw.draft) == "",
			})
			if r.Sent() {
				cw.sendDraft()
			}
			if r.Stopped() {
				cw.stopStreaming()
			}
		},
	})
	if in.Submitted() {
		cw.sendDraft()
	}
	if cw.status != "" {
		k := mcore.Tokens(c)
		mgo.Text(c, cw.status).TextColor(k.Danger).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	}
}

// modelSelector chooses among the enabled providers.
func (cw *ChatWindow) modelSelector(c *mgo.Context) {
	var models []mchat.ModelOption
	for _, p := range cw.cfg.Providers {
		if !p.Enabled || p.Name == "" {
			continue
		}
		models = append(models, mchat.ModelOption{ID: p.Name, Name: p.Name, Capabilities: capsOf(p)})
	}
	if len(models) == 0 {
		if minput.Button(c, "配置模型", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
			cw.settings.open = true
		}
		return
	}
	r := mchat.ModelSelector(c, &cw.provider, models, mchat.ModelSelectorOptions{Label: "模型"})
	if r.Changed() {
		cw.switchProvider(cw.provider)
	}
}

func capsOf(p config.Provider) []string {
	if p.Type == "" {
		return nil
	}
	return []string{p.Type}
}

// modeSelector toggles between plain chat and the tool-enabled agent.
func (cw *ChatWindow) modeSelector(c *mgo.Context) {
	sel := 0
	if cw.agentMode {
		sel = 1
	}
	mnavigation.SegmentedControl(c, &sel, []mnavigation.Segment{
		{Label: "对话"},
		{Label: "Agents"},
	}, mnavigation.SegmentedControlOptions{Label: "模式"})
	if (sel == 1) != cw.agentMode {
		cw.setAgentMode(sel == 1)
	}
}

// toolMenu lists the available builtin and MCP tools as toggles.
func (cw *ChatWindow) toolMenu(c *mgo.Context) {
	metas := cw.toolMetas()
	if len(metas) == 0 {
		return
	}
	ons := make([]bool, len(metas))
	items := make([]mchat.ToolItem, len(metas))
	for i, mt := range metas {
		ons[i] = cw.toolEnabled(mt.ID)
		items[i] = mchat.ToolItem{ID: mt.ID, Label: mt.Label, Description: mt.Desc, On: &ons[i]}
	}
	r := mchat.ToolToggleMenu(c, items)
	if _, ok := r.Changed(); ok {
		for i, mt := range metas {
			cw.toolOn[mt.ID] = ons[i]
		}
		cw.clientDirty.Store(true)
		c.Invalidate()
	}
}
