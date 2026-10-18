// Package settings renders the tabbed configuration page: model providers,
// built-in tools, ACP connections, IM channels, shortcuts, appearance and
// system options. The State struct (tab index + per-tab forms) is owned by
// the host window; data and side effects flow through the Options callbacks.
package settings

import (
	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// ProviderForm is the create/edit form for one model provider.
type ProviderForm struct {
	Editing  int
	Name     string
	Typ      string
	APIKey   string
	BaseURL  string
	Models   []string
	NewModel string
	Enabled  bool
	Preset   bool
}

// BuiltinForm is the edit form for one built-in tool.
type BuiltinForm struct {
	Editing int
	Enabled bool
	Config  map[string]string
}

// ShortcutForm holds the global shortcut value being edited.
type ShortcutForm struct {
	Value string
}

// ChannelForm is the create/edit form for one IM channel.
type ChannelForm struct {
	Editing   int
	Name      string
	Typ       string
	Agent     string
	Workspace string
	AppID     string
	AppSecret string
	BaseURL   string
	Enabled   bool
	Scanning  bool
	QR        *mgo.Bitmap
	QRTip     string
}

// State is the settings page's mutable state, owned by the host window.
type State struct {
	Tab      int
	Prov     ProviderForm
	Builtin  BuiltinForm
	Shortcut ShortcutForm
	Channel  ChannelForm
}

// Options supplies the page with host data and actions.
type Options struct {
	Config              func() *config.Config
	Status              func() string
	SetStatus           func(s string)
	Fail                func(msg string)
	Toast               func(msg string, sev mcore.Severity)
	AskConfirm          func(title, desc string, action func())
	SaveConfig          func() error
	MarkDirty           func()
	PickDirectory       func(title string, cb func(string))
	Invalidate          func()
	ChatAgentName       string
	CurrentProvider     func() string
	FeishuStatusView    func() (mfeedback.Status, string)
	WechatStatusView    func() (mfeedback.Status, string)
	StartWechatLogin    func()
	StartFeishuRegister func()
	RestartChannels     func()
	ACPTab              func(c *mgo.Context)
	ThemeMode           func() mcore.Mode
	SetThemeMode        func(m mcore.Mode)
	ColorThemeName      func() string
	SetColorTheme       func(name string)
	DefaultShortcut     string
	SaveQuickShortcut   func()
	AppVersion          func() string
	SaveSystemSettings  func()
}

// ProviderTypes lists the selectable provider types.
var ProviderTypes = []string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}

// View renders the tabbed settings page.
func View(c *mgo.Context, state *State, opts Options) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "设置").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		if s := opts.Status(); s != "" {
			mgo.Text(c, s).TextColor(mcore.Tokens(c).Danger).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Shrink(0)
		}
		res := mnavigation.Tabs(c, &state.Tab, []mnavigation.Tab{
			{Label: "模型", Panel: func() { providersTab(c, state, opts) }},
			{Label: "工具", Panel: func() { builtinTab(c, state, opts) }},
			{Label: "ACP", Panel: func() { opts.ACPTab(c) }},
			{Label: "渠道", Panel: func() { channelTab(c, state, opts) }},
			{Label: "快捷键", Panel: func() { shortcutTab(c, state, opts) }},
			{Label: "外观", Panel: func() { appearanceTab(c, state, opts) }},
			{Label: "系统", Panel: func() { systemTab(c, state, opts) }},
		}, mnavigation.TabsOptions{})
		res.Element.Grow(1).MinHeight(0)
	})
}
