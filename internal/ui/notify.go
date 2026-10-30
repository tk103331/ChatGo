package ui

import (
	"chatgo/pkg/models"
	"strings"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

func (cw *ChatWindow) toast(msg string, sev mcore.Severity) {
	cw.toastText = msg
	cw.toastSeverity = sev
	cw.toastAction = ""
	cw.toastActionFn = nil
	cw.invalidate()
}

// toastWithAction shows a toast carrying one button, for a message whose
// details live somewhere else: a short line here, the full text on click.
func (cw *ChatWindow) toastWithAction(msg string, sev mcore.Severity, action string, fn func()) {
	cw.toastText = msg
	cw.toastSeverity = sev
	cw.toastAction = action
	cw.toastActionFn = fn
	cw.invalidate()
}

// toastErrorDetail reports an error as a one-line toast whose "查看详情"
// opens the full text, so a long protocol error never floods the composer.
func (cw *ChatWindow) toastErrorDetail(summary, detail string) {
	cw.errDetail = detail
	cw.toastWithAction(summary, mcore.SeverityError, "查看详情", func() {
		cw.errDetailOpen = true
	})
}

func (cw *ChatWindow) flushToast(c *mgo.Context) {
	if cw.toastText == "" {
		return
	}
	kit.ShowToast(c, kit.ToastMessage{
		Message: cw.toastText, Severity: cw.toastSeverity,
		Action: cw.toastAction, OnAction: cw.toastActionFn,
	})
	cw.toastText = ""
	cw.toastAction = ""
	cw.toastActionFn = nil
}

// notifySessionComplete shows a desktop notification when a conversation has
// finished generating a reply, unless it is the conversation currently being
// viewed. Background sessions (auto tasks, channel agents, or a conversation
// the user navigated away from while it streamed) notify so the user knows the
// work is done. When ChatGo is in the foreground the notification is shown
// in-app as a toast; system desktop notification is used only when the window
// is not focused.

func (cw *ChatWindow) notifySessionComplete(conv *models.Conversation, reply string) {
	if conv == nil {
		return
	}
	if cw.cur != nil && cw.cur.ID == conv.ID {
		return
	}
	cw.notifyUser(notifySessionTitle(conv), notifySessionBody(conv, reply))
}

// notifyUser shows the notice in-app as a toast when the window is focused,
// otherwise as a system desktop notification.

func (cw *ChatWindow) notifyUser(title, body string) {
	cw.feishuLogf("notifyUser: title=%q body=%q focused=%v", title, body, cw.window != nil && cw.window.IsFocused())
	if cw.window != nil && cw.window.IsFocused() {
		cw.toast(title+"："+body, mcore.SeverityInfo)
		return
	}
	if !mygo.NotificationsSupported() {
		cw.feishuLogf("notifyUser: 系统通知不支持（未打包或权限未授权）")
		cw.bounceDock()
		return
	}
	if err := mygo.NewNotification(mygo.NotificationOptions{
		Title: title,
		Body:  body,
	}).Show(); err != nil {
		cw.feishuLogf("notifyUser: 系统通知发送失败: %v", err)
		cw.bounceDock()
	}
}

// notifyAttention reports a pending permission or question that blocks a
// conversation. The current conversation with a focused window already shows
// its dialog, so nothing is sent then; otherwise the notice leads back to it.
func (cw *ChatWindow) notifyAttention(convID, label, detail string) {
	focused := cw.window != nil && cw.window.IsFocused()
	viewing := focused && cw.section == sectionChat && cw.mode == modeChat &&
		convID != "" && cw.cur != nil && cw.cur.ID == convID
	if viewing {
		return
	}
	title := "会话"
	if convID != "" {
		if conv, err := cw.convManager.LoadConversation(convID); err == nil {
			title = notifySessionTitle(conv)
		}
	}
	body := label
	if detail != "" {
		body = label + "：" + detail
	}
	body = notifyPreview(body)
	if !focused {
		cw.systemNotify(convID, title, body)
		return
	}
	cw.toastWithAction(title+"："+body, mcore.SeverityInfo, "查看", func() {
		cw.openConversationByID(convID)
	})
}

func (cw *ChatWindow) systemNotify(convID, title, body string) {
	if !mygo.NotificationsSupported() {
		cw.feishuLogf("systemNotify: 系统通知不支持（未打包或权限未授权）")
		cw.bounceDock()
		return
	}
	id := ""
	if convID != "" {
		id = "chatgo-attention-" + convID
	}
	n := mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: title, Body: body})
	n.OnClick(func() {
		cw.postMain(func() {
			if convID != "" {
				cw.openConversationByID(convID)
			}
			if cw.window != nil {
				cw.window.Show()
			}
		})
	})
	if err := n.Show(); err != nil {
		cw.feishuLogf("systemNotify: 系统通知发送失败: %v", err)
		cw.bounceDock()
	}
}

func (cw *ChatWindow) bounceDock() {
	mygo.App.Dock.Bounce(false)
}

// notifySessionTitle is the notification title: agent name plus session title.

func notifySessionTitle(conv *models.Conversation) string {
	return agentLabel(conv.Agent) + " · " + convTitleLabel(conv.Title)
}

// convTitleLabel falls back to a generic label for untitled sessions.

func convTitleLabel(title string) string {
	if title == "" || title == defaultTitle {
		return "会话"
	}
	return title
}

// notifySessionBody is the notification body with the reply preview.

func notifySessionBody(conv *models.Conversation, reply string) string {
	body := notifyPreview(reply)
	if body == "" {
		return "会话已完成"
	}
	return body
}

// notifyPreview trims a reply for a notification body.

func notifyPreview(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// titleBar draws the self-drawn title bar under the window controls.

// errorDetailDialog shows the full text behind a toast's "查看详情", with a
// scrollable body so a long protocol error stays readable.
func (cw *ChatWindow) errorDetailDialog(c *mgo.Context) {
	if !cw.errDetailOpen {
		return
	}
	open := &cw.errDetailOpen
	kit.Dialog(c, open, kit.DialogOptions{
		Title: "错误详情", Width: 640,
		Actions: func() {
			if kit.Button(c, "关闭", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.errDetailOpen = false
			}
			if kit.Button(c, "复制", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				c.WriteClipboard(cw.errDetail)
				cw.toast("已复制", mcore.SeveritySuccess)
			}
		},
	}, func() {
		u := mcore.Density(c).Unit()
		mgo.Scroll(c).FillWidth().MaxHeight(u * 40).Children(func() {
			mgo.Text(c, cw.errDetail).Selectable().Font(mtheme.MonoFont).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
				TextColor(mcore.Tokens(c).Text)
		})
	})
}
