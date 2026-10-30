package ui

import (
	"chatgo/internal/ui/assets"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// homeView is the welcome page: capability cards, the same agent, project and
// worktree pickers the "新会话" tab uses, the composer and recent chats.
func (cw *ChatWindow) homeView(c *mgo.Context) {
	if cw.create.ws == "" {
		cw.create.ws = cw.workspace
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	var example string
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().AlignItems(mgo.Stretch).Children(func() {
			mgo.Column(c).FillWidth().MaxWidth(createWidth).AlignSelf(mgo.Center).
				AlignItems(mgo.Center).Gap(u*3).Padding(u*6, u*4).Children(func() {
				mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
					mgo.Icon(c, assets.BrandMark()).Size(56, 56).TextColor(k.Accent)
					mgo.Text(c, "ChatGo").FontSize(mcore.FontSize(c, mtheme.H1Size)).
						FontWeight(600).TextColor(k.Text)
				})
				view := kit.CapabilityCards(c, []kit.CapabilityCard{
					{Title: "多模型", Description: "OpenAI、Claude、Gemini 等主流模型", Icon: icons.Must("bot"), Examples: []string{"用一句话介绍你自己"}},
					{Title: "Agents 工具", Description: "内置工具与 MCP 服务器，按需自动调用", Icon: icons.Must("terminal"), Examples: []string{"帮我看看这个项目有哪些文件"}},
					{Title: "多会话", Description: "会话自动保存，支持搜索、重命名与导出", Icon: icons.Must("message-square"), Examples: []string{"写一个 Go 的快速排序"}},
				}, kit.CapabilityCardsOptions{Columns: 3})
				view.Element.FillWidth()
				example = view.Example
				cw.createSelectors(c, "首页")
				cw.startComposer(c, "输入消息，Enter 开始对话", cw.startFromCreate)
				cw.recentConversations(c)
			})
		})
	})
	if example != "" {
		cw.draft = example
	}
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
		mgo.Scroll(c).FillWidth().MaxHeight(mcore.Density(c).ControlHeight() * 9).Children(func() {
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
				mgo.Text(c, kit.DateSeparatorWhen(convLast(conv), mcore.DateOf(c.Now()))).
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
