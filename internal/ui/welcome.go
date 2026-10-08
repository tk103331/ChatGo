package ui

import (
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
	mgo.Column(c).Fill().Justify(mgo.Center).Children(func() {
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
			{Title: "多模型", Description: "OpenAI / Claude / Qwen / DeepSeek / Gemini / Ollama", Icon: icons.Must("bot"), Examples: []string{"用一句话介绍你自己"}},
			{Title: "Agents 工具", Description: "内置工具与 MCP 服务器，按需自动调用", Icon: icons.Must("terminal"), Examples: []string{"帮我看看这个项目有哪些文件"}},
			{Title: "多会话", Description: "会话自动保存，支持搜索、重命名与导出", Icon: icons.Must("message-square"), Examples: []string{"写一个 Go 的快速排序"}},
		},
		Prompts: func() {
			in := mchat.PromptComposer(c, &cw.draft, mchat.PromptComposerOptions{
				Placeholder: "输入消息，Enter 开始对话",
				MaxHeight:   160,
				Actions: func() {
					r := mchat.SendButton(c, mchat.SendButtonOptions{Disabled: strings.TrimSpace(cw.draft) == ""})
					if r.Sent() {
						cw.startFromHome()
					}
				},
			})
			in.Element.FillWidth().MaxWidth(640)
			if in.Submitted() {
				cw.startFromHome()
			}
			cw.recentConversations(c)
		},
	})
}

// startFromHome creates a conversation and sends whatever is in the composer.
func (cw *ChatWindow) startFromHome() {
	if strings.TrimSpace(cw.draft) == "" {
		return
	}
	cw.newConversation()
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
