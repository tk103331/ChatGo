package ui

import (
	mgo "github.com/egoist/mygo/ui"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/devtools"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
)

type settingsState struct {
	tab         int
	prov        providerForm
	mcp         mcpForm
	builtin     builtinForm
	shortcut    shortcutForm
	channel     channelForm
	provList    mgo.ListState
	builtinList mgo.ListState
	channelList mgo.ListState
}

type channelForm struct {
	editing   int
	name      string
	typ       string
	agent     string
	workspace string
	appID     string
	appSecret string
	baseURL   string
	enabled   bool
	scanning  bool
	qr        *mgo.Bitmap
	qrTip     string
}

type shortcutForm struct {
	value string
}

type providerForm struct {
	editing  int
	name     string
	typ      string
	apiKey   string
	baseURL  string
	models   []string
	newModel string
	enabled  bool
	preset   bool
}

type mcpForm struct {
	editing int
	name    string
	typ     string
	enabled bool
	command string
	args    string
	env     []devtools.KeyValue
	url     string
	headers []devtools.KeyValue
	timeout string
	hint    string
}

type builtinForm struct {
	editing int
	enabled bool
	config  map[string]string
}

// confirmState is a reusable confirmation dialog.

type confirmState struct {
	open   bool
	active bool
	title  string
	desc   string
	action func()
}

var providerTypes = []string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}

var mcpTypes = []string{"stdio", "sse", "streamable_http"}

// settingsPage shows the tabbed configuration page.

func (cw *ChatWindow) settingsPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "设置").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		if cw.status != "" {
			mgo.Text(c, cw.status).TextColor(mcore.Tokens(c).Danger).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Shrink(0)
		}
		res := mnavigation.Tabs(c, &cw.settings.tab, []mnavigation.Tab{
			{Label: "模型", Panel: func() { cw.providersTab(c) }},
			{Label: "工具", Panel: func() { cw.builtinTab(c) }},
			{Label: "ACP", Panel: func() { cw.acpTab(c) }},
			{Label: "渠道", Panel: func() { cw.channelTab(c) }},
			{Label: "快捷键", Panel: func() { cw.shortcutTab(c) }},
			{Label: "外观", Panel: func() { cw.appearanceTab(c) }},
			{Label: "系统", Panel: func() { cw.systemTab(c) }},
		}, mnavigation.TabsOptions{})
		res.Element.Grow(1).MinHeight(0)
	})
}

// confirmDialog shows the pending confirmation and runs its action. It keeps
// building for one frame after closing so the dialog releases the focus.

func (cw *ChatWindow) confirmDialog(c *mgo.Context) {
	if !cw.confirm.open && !cw.confirm.active {
		return
	}
	res := moverlay.AlertDialog(c, &cw.confirm.open, moverlay.AlertDialogOptions{
		Title:        cw.confirm.title,
		Description:  cw.confirm.desc,
		ConfirmLabel: "删除",
	})
	cw.confirm.active = cw.confirm.open
	if res.Confirmed && cw.confirm.action != nil {
		cw.confirm.action()
		cw.confirm.action = nil
	}
}

func (cw *ChatWindow) askConfirm(title, desc string, action func()) {
	cw.confirm = confirmState{open: true, active: true, title: title, desc: desc, action: action}
}

func removeStr(s []string, v string) []string {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func (cw *ChatWindow) askDeleteConversation(id, title string) {
	cw.askConfirm("删除会话", "确定删除「"+title+"」？此操作不可撤销。", func() {
		cw.acpConvClose(id)
		_ = cw.convManager.DeleteConversation(id)
		if cw.cur != nil && cw.cur.ID == id {
			cw.cur = nil
			cw.mode = modeHome
		}
		cw.reloadConvs()
		cw.toast("会话已删除", mcore.SeveritySuccess)
	})
}

// acpTab configures the ACP connections used by ACP agents.

func (cw *ChatWindow) fail(msg string) {
	cw.status = msg
	cw.toast(msg, mcore.SeverityError)
}
