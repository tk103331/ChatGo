package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	"chatgo/internal/ui/uikit"
)

// flash reports a short-lived message in the header, the web client's stand-in
// for the desktop toast.
func (a *app) flash(msg string) {
	a.status = msg
	a.win.Update(func() {})
	go func() {
		time.Sleep(1600 * time.Millisecond)
		a.win.Update(func() {
			if a.status == msg {
				a.status = ""
			}
		})
	}()
}

// beginRename opens the rename dialog for a session.
func (a *app) beginRename(s sessionSummary) {
	a.menuFor = ""
	a.renameID = s.ID
	a.renameDraft = s.Title
	a.renameOpen = true
	a.win.Update(func() {})
}

// commitRename saves the draft title and updates the list in place.
func (a *app) commitRename() {
	id := a.renameID
	title := strings.TrimSpace(a.renameDraft)
	if id == "" || title == "" {
		return
	}
	a.renameOpen = false
	go func() {
		var out struct {
			OK    bool   `json:"ok"`
			Title string `json:"title"`
		}
		if err := apiPatch(sessionPath(id), map[string]string{"title": title}, &out); err != nil {
			a.win.Update(func() { a.status = "重命名失败：" + err.Error() })
			return
		}
		saved := out.Title
		if saved == "" {
			saved = title
		}
		a.win.Update(func() {
			for i := range a.sessions {
				if a.sessions[i].ID == id {
					a.sessions[i].Title = saved
				}
			}
			if a.cur != nil && a.cur.ID == id {
				a.cur.Title = saved
			}
			a.status = ""
		})
		a.refreshSessions()
	}()
}

// beginDelete asks for confirmation before deleting a session.
func (a *app) beginDelete(s sessionSummary) {
	a.menuFor = ""
	a.deleteID = s.ID
	a.deleteTitle = s.Title
	a.deleteOpen = true
	a.win.Update(func() {})
}

// commitDelete removes the session and selects the next one.
func (a *app) commitDelete() {
	id := a.deleteID
	if id == "" {
		return
	}
	a.deleteOpen = false
	go func() {
		if err := apiDelete(sessionPath(id), nil); err != nil {
			a.win.Update(func() { a.status = "删除失败：" + err.Error() })
			return
		}
		if a.getSelected() == id {
			a.setSelected("")
		}
		a.win.Update(func() {
			if a.cur != nil && a.cur.ID == id {
				a.cur = nil
			}
			a.setRunning(false)
			a.status = ""
		})
		a.refreshSessions()
	}()
}

// overlays draws the modal dialogs on top of the conversation.
func (a *app) overlays(c *ui.Context) {
	a.renameDialog(c)
	a.deleteDialog(c)
	a.newSessionDialog(c)
	a.revertDialog(c)
}

// renameDialog edits a session title. It renders the closed state once more
// after closing, so the dialog restores the focus it trapped.
func (a *app) renameDialog(c *ui.Context) {
	if a.renameID == "" {
		return
	}
	wasOpen := a.renameOpen
	kit.Dialog(c, &a.renameOpen, kit.DialogOptions{
		Title: "重命名会话",
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				a.renameOpen = false
			}
			if kit.Button(c, "保存", kit.ButtonOptions{
				Disabled: strings.TrimSpace(a.renameDraft) == "",
			}).Clicked() {
				a.commitRename()
			}
		},
	}, func() {
		in := kit.TextInput(c, &a.renameDraft, kit.TextInputOptions{
			Placeholder: "会话标题", Label: "会话标题",
		})
		in.Element.FillWidth()
		uikit.AutoFocus(c, in.Input)
		if in.Submitted() {
			a.commitRename()
		}
	})
	if a.renameOpen {
		return
	}
	if wasOpen {
		return
	}
	a.renameID = ""
	a.renameDraft = ""
}

// deleteDialog confirms deleting a session.
func (a *app) deleteDialog(c *ui.Context) {
	if a.deleteID == "" {
		return
	}
	wasOpen := a.deleteOpen
	title := a.deleteTitle
	if title == "" {
		title = "该会话"
	}
	res := kit.AlertDialog(c, &a.deleteOpen, kit.AlertDialogOptions{
		Title:        "删除会话",
		Description:  fmt.Sprintf("将删除「%s」及其全部消息，此操作不可撤销。", title),
		ConfirmLabel: "删除",
	})
	if res.Confirmed {
		a.commitDelete()
	}
	if a.deleteOpen {
		return
	}
	if wasOpen {
		return
	}
	a.deleteID = ""
	a.deleteTitle = ""
}
