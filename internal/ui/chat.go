package ui

import (
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/ui/assets"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	acp "github.com/coder/acp-go-sdk"
)

// chatView lays out the sidebar, the tabbed center area and the file panel.
func (cw *ChatWindow) chatView(c *mgo.Context) {
	if cw.leftW == 0 {
		cw.leftW = 280
	}
	if cw.treeW == 0 {
		cw.treeW = 300
	}
	winW, _ := c.Size()
	tree := float32(0)
	if cw.rightPanel {
		tree = cw.treeW
	}
	left := cw.leftW
	if cw.leftHidden {
		left = 0
	}
	if winW != cw.lastWinW || tree != cw.lastRightW || cw.mainW == 0 {
		cw.lastWinW, cw.lastRightW = winW, tree
		cw.mainW = max(240, winW-60-left-tree-4)
	}
	if cw.leftHidden {
		cw.chatContent(c)
		return
	}
	mgo.Split(c, &cw.leftW, func() { cw.sidebar(c) }, func() { cw.chatContent(c) }).Fill()
}

// chatContent lays out the center area.
func (cw *ChatWindow) chatContent(c *mgo.Context) {
	cw.chatMainArea(c)
}

// chatMainArea lays out the tabbed center area and the file panel.
func (cw *ChatWindow) chatMainArea(c *mgo.Context) {
	if cw.rightPanel {
		if cw.mainW == 0 {
			cw.mainW = 480
		}
		mgo.Split(c, &cw.mainW, func() { cw.centerArea(c) }, func() { cw.sourcePanel(c) }).Fill()
		return
	}
	cw.centerArea(c)
}

// messageArea is the message list and the composer, without the sidebar.
func (cw *ChatWindow) messageArea(c *mgo.Context) {
	msgs := cw.messages()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	row := func(i int) { cw.messageRow(c, i, msgs[i]) }
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Background(k.Background).Children(func() {
		res := kit.MessageList(c, &cw.msgList, len(msgs), kit.MessageListOptions{
			ID:    func(i int) string { return msgs[i].ID },
			Date:  func(i int) time.Time { return msgs[i].Timestamp },
			Label: func(i int) string { return msgs[i].Role },
		}, row)
		res.Element.Grow(1).MinHeight(0)
		mgo.Column(c).Shrink(0).Padding(u*3, u*4).BorderWidth(mtheme.BorderWidth, 0, 0, 0).
			BorderColor(k.Border).Children(func() { cw.composer(c) })
	})
}

// isACP reports whether the current conversation runs through an ACP CLI.
func (cw *ChatWindow) isACP() bool {
	return cw.cur != nil && strings.HasPrefix(cw.cur.Agent, "acp:")
}

func (cw *ChatWindow) messages() []models.Message {
	if cw.cur == nil {
		return nil
	}
	return cw.cur.Messages
}

// composer builds the message input with its model, mode and tool controls.
func (cw *ChatWindow) composer(c *mgo.Context) {
	initializing := cw.isACP() && cw.cur != nil && cw.acpInitializing(cw.cur.ID)
	in := composer(c, &cw.draft, composerOptions{
		Placeholder: "输入消息，Enter 发送，Shift+Enter 换行",
		MaxHeight:   220,
		Disabled:    initializing,
		Tools: func() {
			if cw.isACP() {
				cw.acpControls(c)
				return
			}
			cw.agentSelector(c)
			cw.modelSelector(c)
			cw.toolMenu(c)
			cw.mcpMenu(c)
			cw.skillMenu(c)
		},
		Actions: func() {
			if initializing {
				mgo.Row(c).Gap(mcore.Density(c).Unit()).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
					mgo.Text(c, "正在初始化 ACP 进程…").
						FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
				})
			}
			streaming := cw.streaming && cw.cur != nil && cw.streamingConvID == cw.cur.ID
			r := kit.SendButton(c, kit.SendButtonOptions{
				Streaming: streaming,
				Disabled:  initializing || strings.TrimSpace(cw.draft) == "",
			})
			if r.Sent() {
				cw.sendDraft()
			}
			if r.Stopped() {
				cw.stopStreaming()
			}
		},
	})
	if !initializing {
		cw.chatMenus(c, in)
	}
	if in.Submitted() && !initializing {
		cw.sendDraft()
	}
	cw.modelSetupNotice(c)
	if cw.status != "" {
		k := mcore.Tokens(c)
		mgo.Text(c, cw.status).TextColor(k.Danger).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	}
}

// modelSetupNotice explains a model configuration problem under the composer
// and offers the one action that fixes it.
func (cw *ChatWindow) modelSetupNotice(c *mgo.Context) {
	msg := cw.modelSetupError()
	if msg == "" {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().MinWidth(0).Gap(u).AlignItems(mgo.Center).
		Padding(u, u*2).Radius(mtheme.ControlRadius).Background(k.Surface).Children(func() {
		mgo.Icon(c, icons.Must("triangle-alert")).Size(16, 16).TextColor(k.Danger).Shrink(0)
		mgo.Text(c, msg).TextColor(k.Danger).FontSize(caption).
			Grow(1).MinWidth(0).MaxLines(2)
		if kit.Button(c, "去设置", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
			cw.settings.tab = 0
			cw.goConfigure(sectionSettings)
		}
	})
}

// modelSelector chooses among the models the active agent may use.
func (cw *ChatWindow) modelSelector(c *mgo.Context) {
	refs := cw.agentModelRefs()
	if len(refs) == 0 {
		if kit.Button(c, "配置模型", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
			cw.settings.tab = 0
			cw.goConfigure(sectionSettings)
		}
		return
	}
	opts := make([]kit.SearchSelectOption[string], 0, len(refs))
	for _, ref := range refs {
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: ref, Label: cw.refLabel(ref), Leading: cw.providerLeading(c, ref),
		})
	}
	cur := cw.currentModelRef()
	if !containsStr(refs, cur) {
		cur = refs[0]
	}
	r := kit.SearchSelect(c, &cur, opts, kit.SearchSelectOptions[string]{
		Label: "模型", Placeholder: "选择模型", SearchPlaceholder: "搜索模型",
	})
	if r.Changed() {
		cw.setModelRef(cur)
		if cw.currentAgent == "" {
			cw.saveChatSelection()
		}
		cw.switchProvider(cw.provider)
	}
}

// providerLeading draws the brand mark of the provider a model reference
// belongs to, so the model list reads like the provider settings page.
func (cw *ChatWindow) providerLeading(c *mgo.Context, ref string) func() {
	svg := assets.ProviderLogo(cw.providerForRef(ref).Type)
	if svg == nil {
		return nil
	}
	return func() {
		mgo.Icon(c, svg).Size(16, 16).Shrink(0).TextColor(mcore.Tokens(c).Text)
	}
}

// providerForRef returns the provider a model reference belongs to.
func (cw *ChatWindow) providerForRef(ref string) config.Provider {
	name, _ := splitModelRef(ref)
	if name == "" {
		name = cw.provider
	}
	if p, ok := cw.findProvider(name); ok {
		return p
	}
	return config.Provider{}
}

// acpControls shows the ACP session's modes and configuration options.
func (cw *ChatWindow) acpControls(c *mgo.Context) {
	if cw.cur == nil {
		return
	}
	rt := cw.acpRuntimeFor(cw.cur.ID)
	if rt == nil {
		return
	}
	u := mcore.Density(c).Unit()
	skipModes := acpConfigCoversModes(rt.configs)
	mgo.Row(c).Gap(u * 2).Wrap().AlignItems(mgo.End).Children(func() {
		if len(rt.modes) > 0 && !skipModes {
			opts := make([]kit.SelectOption[string], 0, len(rt.modes))
			for _, m := range rt.modes {
				opts = append(opts, kit.SelectOption[string]{Value: string(m.Id), Label: m.Name})
			}
			sel := string(rt.modeID)
			r := kit.Select(c, &sel, opts, kit.SelectOptions{})
			r.Element.Width(110)
			if r.Changed() {
				cw.acpSetMode(rt, acp.SessionModeId(sel))
			}
		}
		for _, opt := range rt.configs {
			switch {
			case opt.Select != nil:
				s := opt.Select
				sel := string(s.CurrentValue)
				r := kit.Select(c, &sel, acpSelectOptions(s.Options), kit.SelectOptions{})
				r.Element.Width(140)
				if r.Changed() {
					s.CurrentValue = acp.SessionConfigValueId(sel)
					cw.acpSetConfigValue(rt, s.Id, acp.SessionConfigValueId(sel))
				}
			case opt.Boolean != nil:
				b := opt.Boolean
				val := b.CurrentValue
				if mgo.Switch(c, &val).Label(b.Name).Changed() {
					b.CurrentValue = val
					cw.acpSetConfigBool(rt, b.Id, val)
				}
			}
		}
	})
}

// toolMenu toggles the builtin tools in a popover.
func (cw *ChatWindow) toolMenu(c *mgo.Context) {
	cw.toolPopover(c, "tools", "工具", "sliders-horizontal", "工具", "builtin:")
}

// mcpMenu toggles the MCP servers in a popover, separate from builtin tools.
func (cw *ChatWindow) mcpMenu(c *mgo.Context) {
	cw.toolPopover(c, "mcp", "MCP", "layout-grid", "MCP 服务器", "mcp:")
}

// toolPopover builds a button and popover listing the tools whose IDs begin
// with prefix, keeping the builtin and MCP controls independent.
func (cw *ChatWindow) toolPopover(c *mgo.Context, key, buttonLabel, icon, title, prefix string) {
	var list []toolMeta
	for _, mt := range cw.toolMetas() {
		if strings.HasPrefix(mt.ID, prefix) {
			list = append(list, mt)
		}
	}
	btnIcon := icons.Must(icon)
	if prefix == "mcp:" {
		if svg := assets.MCPLogo(); svg != nil {
			btnIcon = svg
		}
	}
	if len(list) == 0 {
		// A configure fallback, styled like the model selector's own.
		if kit.Button(c, "配置"+buttonLabel, kit.ButtonOptions{Variant: kit.Outline, Icon: btnIcon}).Clicked() {
			if prefix == "mcp:" {
				cw.goConfigure(sectionMCP)
			} else {
				cw.settings.tab = 1
				cw.goConfigure(sectionSettings)
			}
		}
		return
	}
	u := mcore.Density(c).Unit()
	on := 0
	for _, mt := range list {
		if cw.toolEnabled(mt.ID) {
			on++
		}
	}
	label := buttonLabel
	if on > 0 {
		label = buttonLabel + " (" + strconv.Itoa(on) + ")"
	}
	// The same secondary trigger as the model selector, so the four select
	// buttons in the composer read as one family.
	b := kit.Button(c, label, kit.ButtonOptions{Variant: kit.Secondary, Icon: btnIcon})
	open := mgo.Local(b, key, func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	query := &cw.toolQuery
	if prefix == "mcp:" {
		query = &cw.mcpQuery
	}
	items := make([]selectItem, 0, len(list))
	for _, mt := range list {
		items = append(items, selectItem{ID: mt.ID, Title: mt.Label, Desc: mt.Desc})
	}
	changed := false
	kit.Popover(c, b, open, kit.PopoverOptions{Title: title, Width: 360, MaxHeight: u * 88, Up: true}, func() {
		changed = selectList(c, query, "搜索"+buttonLabel, items,
			cw.toolEnabled,
			func(id string, on bool) { cw.toolOn[id] = on })
	})
	if changed {
		if cw.currentAgent == "" {
			cw.saveChatSelection()
		}
		cw.clientDirty.Store(true)
		c.Invalidate()
	}
}
