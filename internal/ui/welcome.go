package ui

import (
	"path/filepath"
	"strings"

	"chatgo/internal/ui/assets"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
)

// homeView is the welcome page: capability cards, the composer and recent chats.
func (cw *ChatWindow) homeView(c *mgo.Context) {
	k := mcore.Tokens(c)
	var view mchat.CapabilityCardsView
	mgo.Column(c).Fill().Justify(mgo.Center).AlignItems(mgo.Center).Gap(mcore.Density(c).Unit() * 3).Children(func() {
		mgo.Image(c, assets.BrandBitmap()).Size(72, 72)
		view = cw.welcome(c)
	})
	if view.Example != "" {
		cw.draft = view.Example
	}
	if cw.status != "" {
		mgo.Text(c, cw.status).TextColor(k.Danger).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	}
}

func (cw *ChatWindow) welcome(c *mgo.Context) mchat.CapabilityCardsView {
	return mchat.WelcomeScreen(c, mchat.WelcomeScreenOptions{
		Greeting: "ChatGo",
		Subtitle: "基于 MyGo 与 MujicaUI 的 AI 对话客户端",
		Cards: []mchat.CapabilityCard{
			{Title: "多模型", Description: "OpenAI、Claude、Gemini 等主流模型", Icon: icons.Must("bot"), Examples: []string{"用一句话介绍你自己"}},
			{Title: "Agents 工具", Description: "内置工具与 MCP 服务器，按需自动调用", Icon: icons.Must("terminal"), Examples: []string{"帮我看看这个项目有哪些文件"}},
			{Title: "多会话", Description: "会话自动保存，支持搜索、重命名与导出", Icon: icons.Must("message-square"), Examples: []string{"写一个 Go 的快速排序"}},
		},
		Prompts: func() {
			in := composer(c, &cw.draft, composerOptions{
				Placeholder: "输入消息，Enter 开始对话",
				MaxHeight:   160,
				Tools:       func() { cw.homeSelectors(c) },
				Actions: func() {
					r := mchat.SendButton(c, mchat.SendButtonOptions{Disabled: strings.TrimSpace(cw.draft) == ""})
					if r.Sent() {
						cw.startFromHome()
					}
				},
			})
			in.Element.FillWidth()
			if in.Submitted() {
				cw.startFromHome()
			}
			cw.recentConversations(c)
		},
	})
}

// homeSelectors chooses the workspace and agent for a chat started from home.
func (cw *ChatWindow) homeSelectors(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	label := func(text string) {
		mgo.Text(c, text).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
			TextColor(mcore.Tokens(c).TextMuted).Shrink(0)
	}
	mgo.Row(c).Gap(u * 3).AlignItems(mgo.Center).Children(func() {
		wsOpts := make([]minput.SelectOption[string], 0, len(cw.workspaces()))
		for _, ws := range cw.workspaces() {
			name := filepath.Base(ws)
			if name == "" || name == "." || name == string(filepath.Separator) {
				name = ws
			}
			wsOpts = append(wsOpts, minput.SelectOption[string]{Value: ws, Label: name})
		}
		label("项目")
		if len(wsOpts) > 0 {
			if minput.Select(c, &cw.workspace, wsOpts, minput.SelectOptions{Label: "项目", Placeholder: "选择项目"}).Changed() && cw.currentAgent == "" {
				cw.saveChatSelection()
			}
		}
		if mcore.IconAction(c, icons.Must("plus"), "新建项目").Tooltip("新建项目").Clicked() {
			cw.pickWorkspace()
		}
		agOpts := make([]minput.SelectOption[string], 0, len(cw.cfg.Agents)+len(cw.cfg.ACPAgents)+1)
		agOpts = append(agOpts, minput.SelectOption[string]{Value: "", Label: chatAgentName})
		for _, a := range cw.cfg.Agents {
			agOpts = append(agOpts, minput.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
		}
		for _, a := range cw.cfg.ACPAgents {
			if a.Disabled {
				continue
			}
			agOpts = append(agOpts, minput.SelectOption[string]{Value: "acp:" + a.Name, Label: "ACP · " + a.Name})
		}
		label("Agent")
		minput.Select(c, &cw.homeAgent, agOpts, minput.SelectOptions{Label: "Agent", Placeholder: "选择 Agent"})
	})
}

// startFromHome creates a conversation and sends whatever is in the composer.
func (cw *ChatWindow) startFromHome() {
	if strings.TrimSpace(cw.draft) == "" {
		return
	}
	cw.newConversationIn(cw.workspace, cw.homeAgent)
	cw.sendDraft()
}

// recentConversations lists the ten most recent saved conversations in a
// scrollable area.
func (cw *ChatWindow) recentConversations(c *mgo.Context) {
	if len(cw.convs) == 0 {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 1.5).Children(func() {
		mgo.Text(c, "最近会话").FontWeight(600).TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Scroll(c).MaxHeight(mcore.Density(c).ControlHeight() * 9).Children(func() {
			mgo.Column(c).FillWidth().Gap(u * 1.5).Children(func() {
				for i, conv := range cw.convs {
					if i >= 10 {
						break
					}
					cw.recentConversationRow(c, conv)
				}
			})
		})
	})
}

// recentConversationRow renders one recent conversation as a card: avatar,
// title, relative time and the workspace it belongs to.
func (cw *ChatWindow) recentConversationRow(c *mgo.Context, conv models.Conversation) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).FillWidth().Gap(u).AlignItems(mgo.Center).
		MinHeight(mcore.Density(c).ControlHeight()*1.15).Padding(u*0.5, u*2.5).Radius(mtheme.CardRadius).
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border)
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		cw.convLeading(c, conv, 24)
		mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, conv.Title).FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(500).
					SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
				mgo.Text(c, mchat.DateSeparatorWhen(c, convLast(conv), mcore.DateOf(c.Now()))).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).Shrink(0)
			})
			if conv.Workspace != "" {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Icon(c, icons.Must("folder")).Size(12, 12).TextColor(k.TextMuted).Shrink(0)
					mgo.Text(c, workspaceTitle(conv.Workspace)).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
						TextColor(k.TextMuted).SingleLine().Grow(1).MinWidth(0).Ellipsis("…").Tooltip(conv.Workspace)
				})
			}
		})
	})
	if b.Clicked() {
		cw.selectConversation(conv.ID)
	}
}
