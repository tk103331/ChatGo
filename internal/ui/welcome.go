package ui

import (
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// homeView is the welcome page: capability cards, the composer and recent chats.
func (cw *ChatWindow) homeView(c *mgo.Context) {
	k := mcore.Tokens(c)
	var view mchat.CapabilityCardsView
	mgo.Column(c).Fill().Justify(mgo.Center).AlignItems(mgo.Center).Gap(mcore.Density(c).Unit() * 3).Children(func() {
		mgo.Image(c, brandBitmap()).Size(72, 72)
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
				MinHeight:   72,
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
		if len(wsOpts) > 0 {
			label("项目")
			if minput.Select(c, &cw.workspace, wsOpts, minput.SelectOptions{Label: "项目", Placeholder: "选择项目"}).Changed() && cw.currentAgent == "" {
				cw.saveChatSelection()
			}
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

// recentConversations lists the five most recent saved conversations.
func (cw *ChatWindow) recentConversations(c *mgo.Context) {
	if len(cw.convs) == 0 {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().MaxWidth(640).Gap(u).Children(func() {
		mgo.Text(c, "最近会话").FontWeight(600).TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		for i, conv := range cw.convs {
			if i >= 5 {
				break
			}
			conversation := conv
			if minput.Button(c, conversation.Title, minput.ButtonOptions{
				Variant: minput.Ghost,
				Label:   conversation.Title,
			}).Clicked() {
				cw.selectConversation(conversation.ID)
			}
		}
	})
}
