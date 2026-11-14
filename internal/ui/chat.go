package ui

import (
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

// chatWidth caps the conversation's reading measure — messages, the panels
// and the composer — the way the home page caps its composer, so a wide pane
// keeps a readable line length.
const chatWidth float32 = 860

// messageArea is the message list and the composer, without the sidebar.
func (cw *ChatWindow) messageArea(c *mgo.Context, list *kit.MessageListState) {
	msgs := cw.messages()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	row := func(i int) { cw.messageRow(c, i, msgs[i]) }
	mgo.Column(c).Fill().AlignItems(mgo.Center).Background(k.Background).Children(func() {
		mgo.Column(c).Fill().MaxWidth(chatWidth).MinWidth(0).AlignItems(mgo.Stretch).Children(func() {
			res := kit.MessageList(c, list, len(msgs), kit.MessageListOptions{
				ID:    func(i int) string { return msgs[i].ID },
				Date:  func(i int) time.Time { return msgs[i].Timestamp },
				Label: func(i int) string { return msgs[i].Role },
			}, row)
			res.Element.Grow(1).MinHeight(0)
			cw.acpPermissionPanel(c)
			cw.askQuestionPanel(c)
			cw.planPanel(c)
			mgo.Column(c).Shrink(0).FillWidth().Padding(u*3, u*4).BorderWidth(mtheme.BorderWidth, 0, 0, 0).
				BorderColor(k.Border).Children(func() { cw.composer(c) })
		})
	})
}

// isACP reports whether the current conversation runs through an ACP CLI.
// scrollChatToEnd follows new messages in every chat surface: the focused
// quick/app area and each open chat tab. Each tab owns its own ListState, so
// two panes can show a chat at the same time.
func (cw *ChatWindow) scrollChatToEnd() {
	cw.msgList.ScrollToEnd()
	for _, tab := range cw.centerTabsOfKind(centerChat) {
		tab.msgList.ScrollToEnd()
	}
}

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
				cw.contextButton(c, false)
				cw.acpControls(c)
				return
			}
			cw.contextButton(c, true)
			cw.modelSelector(c)
			cw.toolsMenu(c)
		},
		Actions: func() {
			if initializing {
				mgo.Row(c).Gap(mcore.Density(c).Unit()).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
					mgo.Text(c, "正在初始化 ACP 进程…").
						FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
				})
			}
			streaming := cw.streamingCur()
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
	if files := in.Element.DroppedFiles(); len(files) > 0 {
		cw.addDroppedFiles(files)
	}
	if in.Element.FileDragOver() {
		in.Element.Border(mtheme.FocusWidth, mcore.Tokens(c).Focus).
			Background(mcore.Tokens(c).Selection)
	}
	if !initializing {
		cw.chatMenus(c, in)
	}
	if in.Submitted() && !initializing && !cw.streamingCur() {
		cw.sendDraft()
	}
	cw.modelSetupNotice(c)
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
	cw.modelPickerButton(c, refs)
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
