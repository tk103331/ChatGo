package ui

import (
	"chatgo/pkg/models"
	"strings"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "chatgo/internal/ui/uikit"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
)

func (cw *ChatWindow) toast(msg string, sev mcore.Severity) {
	cw.toastText = msg
	cw.toastSeverity = sev
	cw.invalidate()
}

func (cw *ChatWindow) flushToast(c *mgo.Context) {
	if cw.toastText == "" {
		return
	}
	mfeedback.ShowToast(c, mfeedback.ToastMessage{Message: cw.toastText, Severity: cw.toastSeverity})
	cw.toastText = ""
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
		return
	}
	_ = mygo.NewNotification(mygo.NotificationOptions{
		Title: title,
		Body:  body,
	}).Show()
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
