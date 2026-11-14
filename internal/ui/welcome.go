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

// homeView is the welcome page: the brand, the same agent, project and
// worktree pickers the "新会话" tab uses, the composer and recent chats.
func (cw *ChatWindow) homeView(c *mgo.Context) {
	if cw.create.ws == "" {
		cw.create.ws = cw.workspace
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	// The block centers when the window is tall enough and scrolls when it
	// is not.
	viewportCenter(c, "home", func() {
		mgo.Column(c).FillWidth().MaxWidth(createWidth).AlignSelf(mgo.Center).
			AlignItems(mgo.Center).Gap(u*2).Padding(u*5, u*2).Children(func() {
			mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				mgo.Icon(c, assets.BrandMark()).Size(44, 44).TextColor(k.Accent)
				mgo.Text(c, "ChatGo").FontSize(mcore.FontSize(c, mtheme.H1Size)).
					FontWeight(600).TextColor(k.Text)
			})
			cw.createSelectors(c, "首页")
			cw.startComposer(c, "输入消息，Enter 开始对话", cw.startFromCreate)
			cw.recentConversations(c)
		})
	})
}

// recentConversations lists the ten most recent saved conversations in a
// scrollable area.
func (cw *ChatWindow) recentConversations(c *mgo.Context) {
	if len(cw.convs) == 0 {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	open := mgo.Local(c.Root(), "home:recent-conversations:open", func() bool { return false })
	mgo.Column(c).FillWidth().Gap(u * 1.5).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "最近会话").FontWeight(600).TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Grow(1)
			arrow := "chevron-right"
			label := "展开最近会话"
			if *open {
				arrow = "chevron-down"
				label = "收起最近会话"
			}
			if mcore.IconAction(c, icons.Must(arrow), label).Clicked() {
				*open = !*open
			}
		})
		if !*open {
			return
		}
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
			if workspaceMeaningful(conv.Workspace) {
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
