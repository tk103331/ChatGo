package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// Settings tab indexes, in the order settingsPage lists them.
const (
	settingsModelsTab = iota
	settingsToolsTab
	settingsACPTab
	settingsA2ATab
	settingsAGUITab
	settingsChannelsTab
	settingsShortcutsTab
	settingsQuickTab
	settingsAppearanceTab
	settingsExperimentalTab
	settingsSystemTab
	settingsWebTab
	settingsMobileTab
	settingsPluginsTab
)

// settingsTabMeta is one settings tab by stable name. Jumps from elsewhere
// use these names, never a numeric position: adding a tab must not move a
// "from plugin" link to another page.
type settingsTabMeta struct {
	id    string
	label string
}

var settingsTabMetas = []settingsTabMeta{
	{"models", "模型"},
	{"tools", "工具"},
	{"acp", "ACP"},
	{"a2a", "A2A"},
	{"agui", "AG-UI"},
	{"channels", "渠道"},
	{"shortcuts", "快捷键"},
	{"quick", "快捷任务"},
	{"appearance", "外观"},
	{"experimental", "实验"},
	{"system", "系统"},
	{"web", "Web"},
	{"mobile", "手机"},
	{"plugins", "插件"},
}

// settingsTabIndex returns the position of a named settings tab, or the
// first tab when the name is unknown.
func settingsTabIndex(id string) int {
	for i, tab := range settingsTabMetas {
		if tab.id == id {
			return i
		}
	}
	return 0
}

// openSettingsTab opens settings on the named page.
func (cw *ChatWindow) openSettingsTab(id string) {
	cw.settings.tab = settingsTabIndex(id)
	cw.goConfigure(sectionSettings)
}

type settingsState struct {
	tab         int
	prov        providerForm
	group       groupForm
	mcp         mcpForm
	builtin     builtinForm
	shortcut    shortcutForm
	exclude     textForm
	channel     channelForm
	a2a         a2aForm
	agui        aguiForm
	proxy       proxyForm
	webToken    textForm
	relayURL    textForm
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
	// qrNotice is a warning the user must read before scanning, shown above
	// the code.
	qrNotice string
	tools    []config.ChannelTool
	// options holds a platform's extra credential fields, which do not fit
	// the AppID/AppSecret pair — Slack's app token, for instance.
	options map[string]string
}

type shortcutForm struct {
	value string
	err   string
	// panelValue and panelErr belong to the quick task panel's accelerator,
	// which is registered separately from the compact window's.
	panelValue string
	panelErr   string
}

// textForm keeps a field's draft across frames, for a value the user edits
// with delimiters that would otherwise be normalized while typing.
type textForm struct {
	value string
	ready bool
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
	// testing marks a connectivity probe in flight, so the button spins and
	// a second click cannot start another.
	testing bool
	// probeResults is the last per-model probe: one entry per model that was
	// tried, so a disabled model is named instead of failing the whole check.
	probeResults map[string]modelProbe
}

// modelProbe is one model's connectivity result.
type modelProbe struct {
	ok  bool
	err string
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
	tabs := make([]kit.Tab, 0, len(settingsTabMetas))
	for _, meta := range settingsTabMetas {
		meta := meta
		tabs = append(tabs, kit.Tab{
			Label: meta.label,
			Panel: func() { cw.settingsTabPanel(c, meta.id) },
		})
	}
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "设置").
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		res := kit.Tabs(c, &cw.settings.tab, tabs, kit.TabsOptions{})
		res.Element.Grow(1).MinHeight(0)
	})
	cw.importDialog(c)
	cw.acpRegistryDialog(c)
	cw.modelRefreshDialog(c)
}

// settingsTabPanel draws the page a named settings tab holds.
func (cw *ChatWindow) settingsTabPanel(c *mgo.Context, id string) {
	switch id {
	case "models":
		cw.providersTab(c)
	case "tools":
		cw.builtinTab(c)
	case "acp":
		cw.acpTab(c)
	case "a2a":
		cw.a2aTab(c)
	case "agui":
		cw.aguiTab(c)
	case "channels":
		cw.channelTab(c)
	case "shortcuts":
		cw.shortcutTab(c)
	case "quick":
		cw.quickPanelTab(c)
	case "appearance":
		cw.appearanceTab(c)
	case "experimental":
		cw.experimentalTab(c)
	case "system":
		cw.systemTab(c)
	case "web":
		cw.webTab(c)
	case "mobile":
		cw.mobileTab(c)
	case "plugins":
		cw.pluginsTab(c)
	}
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
		cw.emit("conversation.deleted", map[string]any{"conversationId": id, "title": title})
		cw.toast("会话已删除", mcore.SeveritySuccess)
	})
}

// acpTab configures the ACP connections used by ACP agents.

func (cw *ChatWindow) fail(msg string) {
	cw.toastErrorDetail("操作失败", msg)
}
