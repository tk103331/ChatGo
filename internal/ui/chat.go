package ui

import (
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "github.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
	acp "github.com/coder/acp-go-sdk"
)

// chatView lays out the sidebar, message list and composer.
func (cw *ChatWindow) chatView(c *mgo.Context) {
	if cw.leftW == 0 {
		cw.leftW = 280
	}
	if cw.treeW == 0 {
		cw.treeW = 300
	}
	if cw.previewW == 0 {
		cw.previewW = 320
	}
	winW, _ := c.Size()
	tree := float32(0)
	if cw.rightPanel {
		tree = cw.treeW
	}
	hasPreview := len(cw.previewTabs) > 0
	left := cw.leftW
	if cw.leftHidden {
		left = 0
	}
	if winW != cw.lastWinW || tree != cw.lastRightW || hasPreview != cw.lastPreview || cw.chatW == 0 {
		cw.lastWinW, cw.lastRightW, cw.lastPreview = winW, tree, hasPreview
		flex := winW - 60 - left - tree - 4
		if hasPreview {
			cw.chatW = max(240, flex*0.6)
		} else {
			cw.chatW = max(240, flex)
		}
	}
	if cw.leftHidden {
		cw.chatContent(c, hasPreview)
		return
	}
	mgo.Split(c, &cw.leftW, func() { cw.sidebar(c) }, func() { cw.chatContent(c, hasPreview) }).Fill()
}

// chatContent lays out the message area, preview column and file panel.
func (cw *ChatWindow) chatContent(c *mgo.Context, hasPreview bool) {
	if cw.termOpen {
		if cw.termH == 0 {
			_, winH := c.Size()
			cw.termH = max(120, winH-navItemSize-260)
		}
		mgo.SplitVertical(c, &cw.termH, func() { cw.chatMainArea(c, hasPreview) }, func() { cw.terminalPanel(c) }).Fill()
		return
	}
	cw.chatMainArea(c, hasPreview)
}

// chatMainArea lays out the message area, preview column and file panel.
func (cw *ChatWindow) chatMainArea(c *mgo.Context, hasPreview bool) {
	mgo.Row(c).Fill().AlignItems(mgo.Stretch).Children(func() {
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			if hasPreview {
				mgo.Split(c, &cw.chatW, func() { cw.messageArea(c) }, func() { cw.previewPanel(c) }).Fill()
				return
			}
			cw.messageArea(c)
		})
		if cw.rightPanel {
			cw.sourcePanel(c)
		}
	})
}

// messageArea is the message list and the composer, without the sidebar.
func (cw *ChatWindow) messageArea(c *mgo.Context) {
	msgs := cw.messages()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	row := func(i int) { cw.messageRow(c, i, msgs[i]) }
	if cw.isACP() {
		row = func(i int) { cw.messageRowACP(c, i, msgs[i]) }
	}
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Background(k.Background).Children(func() {
		res := mchat.MessageList(c, &cw.msgList, len(msgs), mchat.MessageListOptions{
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
	in := mchat.PromptComposer(c, &cw.draft, mchat.PromptComposerOptions{
		Placeholder: "输入消息，Enter 发送，Shift+Enter 换行",
		Tools: func() {
			if cw.isACP() {
				cw.acpBadge(c)
				cw.acpControls(c)
				return
			}
			cw.agentSelector(c)
			cw.modelSelector(c)
			cw.toolMenu(c)
			cw.skillMenu(c)
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

// modelSelector chooses among the models the active agent may use.
func (cw *ChatWindow) modelSelector(c *mgo.Context) {
	refs := cw.agentModelRefs()
	var models []mchat.ModelOption
	for _, ref := range refs {
		models = append(models, mchat.ModelOption{ID: ref, Name: modelLabel(ref), Capabilities: capsOf(cw.providerForRef(ref))})
	}
	if len(models) == 0 {
		if minput.Button(c, "配置模型", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
			cw.section = sectionSettings
			cw.settings.tab = 0
		}
		return
	}
	cur := cw.currentModelRef()
	if !containsStr(refs, cur) {
		cur = refs[0]
	}
	r := mchat.ModelSelector(c, &cur, models, mchat.ModelSelectorOptions{Label: "模型"})
	if r.Changed() {
		cw.setModelRef(cur)
		if cw.currentAgent == "" {
			cw.saveChatSelection()
		}
		cw.switchProvider(cw.provider)
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
	mgo.Row(c).Gap(u * 2).Wrap().AlignItems(mgo.End).Children(func() {
		if len(rt.modes) > 0 {
			opts := make([]minput.SelectOption[string], 0, len(rt.modes))
			for _, m := range rt.modes {
				opts = append(opts, minput.SelectOption[string]{Value: string(m.Id), Label: m.Name})
			}
			sel := string(rt.modeID)
			if minput.Select(c, &sel, opts, minput.SelectOptions{Label: "模式"}).Changed() {
				cw.acpSetMode(rt, acp.SessionModeId(sel))
			}
		}
		for _, opt := range rt.configs {
			switch {
			case opt.Select != nil:
				s := opt.Select
				sel := string(s.CurrentValue)
				if minput.Select(c, &sel, acpSelectOptions(s.Options), minput.SelectOptions{Label: s.Name}).Changed() {
					s.CurrentValue = acp.SessionConfigValueId(sel)
					cw.acpSetConfigValue(rt, s.Id, acp.SessionConfigValueId(sel))
				}
			case opt.Boolean != nil:
				b := opt.Boolean
				val := b.CurrentValue
				if minput.Switch(c, &val, b.Name, minput.SwitchOptions{}).Changed() {
					b.CurrentValue = val
					cw.acpSetConfigBool(rt, b.Id, val)
				}
			}
		}
	})
}

// acpBadge names the ACP connection driving the current conversation.
func (cw *ChatWindow) acpBadge(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	name := ""
	if cw.cur != nil {
		name = strings.TrimPrefix(cw.cur.Agent, "acp:")
	}
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		if logo := acpLogo(name); logo != nil {
			mgo.Image(c, logo).Size(16, 16)
		} else {
			mgo.Icon(c, icons.Must("terminal")).Size(14, 14).TextColor(k.AccentText)
		}
		mgo.Text(c, "ACP · "+name).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.AccentText).SingleLine()
	})
}

func capsOf(p config.Provider) []string {
	if p.Type == "" {
		return nil
	}
	return []string{p.Type}
}

// toolMenu toggles builtin tools and MCP servers, grouped in a popover.
func (cw *ChatWindow) toolMenu(c *mgo.Context) {
	metas := cw.toolMetas()
	if len(metas) == 0 {
		if minput.Button(c, "配置工具", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("sliders-horizontal")}).Clicked() {
			cw.section = sectionSettings
			cw.settings.tab = 1
		}
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	on := 0
	for _, mt := range metas {
		if cw.toolEnabled(mt.ID) {
			on++
		}
	}
	label := "工具"
	if on > 0 {
		label = "工具 (" + strconv.Itoa(on) + ")"
	}
	b := mgo.ButtonBase(c).MinHeight(mtheme.MinHitSize).Padding(0, u*2).Gap(u).
		Radius(mtheme.ControlRadius).Label(label).TextColor(k.TextMuted).Role(mgo.RolePopUpButton)
	open := mgo.Local(b, "tools", func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must("sliders-horizontal")).Size(16, 16)
		if on > 0 {
			mgo.Text(c, strconv.Itoa(on)).FontWeight(600).TextColor(k.AccentText).SingleLine()
		}
	})
	var builtins, mcp []toolMeta
	for _, mt := range metas {
		if strings.HasPrefix(mt.ID, "mcp:") {
			mcp = append(mcp, mt)
		} else {
			builtins = append(builtins, mt)
		}
	}
	changed := false
	moverlay.Popover(c, b, open, moverlay.PopoverOptions{Title: "工具", Width: 340}, func() {
		agentSearch(c, &cw.toolQuery, "搜索工具").Height(mcore.Density(c).ControlHeight() - 6)
		section := func(title string, list []toolMeta) {
			list = filterMetas(list, cw.toolQuery)
			if len(list) == 0 {
				return
			}
			mgo.Text(c, title).FontWeight(600).FontSize(small).TextColor(k.TextMuted)
			for _, mt := range list {
				mt := mt
				v := cw.toolEnabled(mt.ID)
				mgo.Column(c).Gap(0).Children(func() {
					if minput.Switch(c, &v, mt.Label, minput.SwitchOptions{}).Changed() {
						cw.toolOn[mt.ID] = v
						changed = true
					}
					if mt.Desc != "" {
						mgo.Text(c, mt.Desc).FontSize(small).TextColor(k.TextMuted).MaxLines(1).Ellipsis("…")
					}
				})
			}
		}
		section("内置工具", builtins)
		section("MCP", mcp)
	})
	if changed {
		if cw.currentAgent == "" {
			cw.saveChatSelection()
		}
		cw.clientDirty.Store(true)
		c.Invalidate()
	}
}
