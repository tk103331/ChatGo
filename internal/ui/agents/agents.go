// Package agents renders the Agent management page: a card grid of configured
// agents and a create/edit editor with collapsible capability sections. The
// State struct (editor form + section toggles) is owned by the host window;
// data and side effects flow through the Options callbacks.
package agents

import (
	"fmt"

	"chatgo/internal/config"
	"chatgo/internal/ui/common"
	skillspkg "chatgo/internal/ui/skills"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Navigation targets for the GoConfigure callback.
const (
	NavModels = iota
	NavTools
	NavMCP
	NavSkills
)

// ChatName is the reserved display name of the built-in Chat agent.
const ChatName = "Chat"

// Form is the create/edit form for one agent preset.
type Form struct {
	Editing      int
	Name         string
	Description  string
	SystemPrompt string
	Workspace    string
	Icon         string
	AvatarSeed   string
	Models       []string
	BuiltinTools []string
	ToolQuery    string
	MCPServers   []string
	MCPQuery     string
	Skills       []string
	SkillQuery   string
}

// State is the agent page's mutable state, owned by the host window.
type State struct {
	Form       Form
	EditorOpen bool
	SecModels  bool
	SecTools   bool
	SecMCP     bool
	SecSkills  bool
}

// Options supplies the page with host data and actions.
type Options struct {
	Config              func() *config.Config
	ModelOptions        func(custom []string) []string
	ModelLabel          func(ref string) string
	AllModelRefs        func() []string
	EnabledBuiltinTools func() []string
	EnabledMCPServers   func() []string
	EnabledSkills       func() []skillspkg.Skill
	DrawMark            func(c *mgo.Context, icon, avatarSeed, name string, size float32)
	PickIconFile        func(cb func(string))
	PickDirectory       func(title string, cb func(string))
	Invalidate          func()
	Toast               func(msg string, sev mcore.Severity)
	AskConfirm          func(title, desc string, action func())
	SaveConfig          func() error
	GoConfigure         func(target int)
	OnDeleted           func(name string)
}

// View renders the agent page: the card grid or the editor.
func View(c *mgo.Context, state *State, opts Options) {
	if state.EditorOpen {
		editor(c, state, opts)
		return
	}
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "Agent 管理").Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if minput.Button(c, "新建 Agent", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
				ClearForm(state, opts)
				state.EditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if len(opts.Config().Agents) == 0 {
				emptyState(c, state, opts)
				return
			}
			mgo.Grid(c).Columns(3).Gap(u * 3).Children(func() {
				for i := range opts.Config().Agents {
					card(c, state, opts, i)
				}
			})
		})
	})
}

// emptyState shows a centered placeholder when no agents exist.
func emptyState(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u*2).
		Padding(u*8, 0).Children(func() {
		mgo.Icon(c, icons.Must("bot")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有 Agent").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "创建自定义 Agent，配置模型、工具和技能").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if minput.Button(c, "新建 Agent", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
			ClearForm(state, opts)
			state.EditorOpen = true
		}
	})
}

// card renders one agent preset as a card with edit and delete actions.
func card(c *mgo.Context, state *State, opts Options, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	a := opts.Config().Agents[i]
	mgo.Column(c).Key("agent:"+a.Name).Gap(u*2).Padding(u*3).FillWidth().
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).
		Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				opts.DrawMark(c, a.Icon, a.AvatarSeed, a.Name, 20)
				mgo.Text(c, a.Name).Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).Grow(1).SingleLine()
			})
			if a.Description != "" {
				mgo.Text(c, a.Description).TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).MaxLines(2)
			}
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				mode := "对话"
				if a.Mode == "agent" {
					mode = "Agents"
				}
				common.Tag(c, mode)
				if len(a.Models) > 0 {
					common.Tag(c, opts.ModelLabel(a.Models[0]))
				}
				if n := len(a.BuiltinTools) + len(a.MCPServers); n > 0 {
					common.Tag(c, fmt.Sprintf("%d 个工具", n))
				}
			})
			mgo.Row(c).Gap(u).Children(func() {
				if minput.Button(c, "编辑", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
					LoadForm(state, opts, i)
					state.EditorOpen = true
				}
				if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
					DeleteAt(state, opts, i)
				}
			})
		})
}
