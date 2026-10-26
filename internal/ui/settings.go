package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

type settingsState struct {
	tab         int
	prov        providerForm
	group       groupForm
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
	tools     []config.ChannelTool
}

type shortcutForm struct {
	value string
	err   string
}

type providerForm struct {
	editing  int
	name     string
	typ      string
	apiType  string
	apiKey   string
	baseURL  string
	models   []string
	newModel string
	enabled  bool
	preset   bool
}

type groupForm struct {
	editing  int
	name     string
	strategy string
	members  []config.GroupMember
	enabled  bool
}

type mcpForm struct {
	editing int
	name    string
	typ     string
	enabled bool
	command string
	args    string
	env     []kit.KeyValue
	url     string
	headers []kit.KeyValue
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
	// label is the confirm button's text; empty means "删除".
	label  string
	action func()
}

var mcpTypes = []string{"stdio", "sse", "streamable_http"}

// settingsPage shows the tabbed configuration page.

func (cw *ChatWindow) settingsPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "设置").
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		res := kit.Tabs(c, &cw.settings.tab, []kit.Tab{
			{Label: "模型", Panel: func() { cw.providersTab(c) }},
			{Label: "工具", Panel: func() { cw.builtinTab(c) }},
			{Label: "ACP", Panel: func() { cw.acpTab(c) }},
			{Label: "渠道", Panel: func() { cw.channelTab(c) }},
			{Label: "快捷键", Panel: func() { cw.shortcutTab(c) }},
			{Label: "外观", Panel: func() { cw.appearanceTab(c) }},
			{Label: "实验", Panel: func() { cw.experimentalTab(c) }},
			{Label: "系统", Panel: func() { cw.systemTab(c) }},
		}, kit.TabsOptions{})
		res.Element.Grow(1).MinHeight(0)
	})
	cw.importDialog(c)
	cw.modelRefreshDialog(c)
}

// confirmDialog shows the pending confirmation and runs its action. It keeps
// building for one frame after closing so the dialog releases the focus.

func (cw *ChatWindow) confirmDialog(c *mgo.Context) {
	if !cw.confirm.open && !cw.confirm.active {
		return
	}
	res := kit.AlertDialog(c, &cw.confirm.open, kit.AlertDialogOptions{
		Title:        cw.confirm.title,
		Description:  cw.confirm.desc,
		ConfirmLabel: cw.confirm.label,
	})
	cw.confirm.active = cw.confirm.open
	if res.Confirmed && cw.confirm.action != nil {
		cw.confirm.action()
		cw.confirm.action = nil
	}
}

func (cw *ChatWindow) askConfirm(title, desc string, action func()) {
	cw.askConfirmAs(title, desc, "", action)
}

// askConfirmAs is askConfirm with a custom confirm-button label.
func (cw *ChatWindow) askConfirmAs(title, desc, label string, action func()) {
	cw.confirm = confirmState{open: true, active: true, title: title, desc: desc, label: label, action: action}
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
		cw.closeConvTabs(id)
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
