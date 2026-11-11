package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	"chatgo/pkg/models"
)

// beginRevert asks for confirmation before rewinding the workspace to the
// checkpoint of a reply.
func (a *app) beginRevert(m models.Message) {
	if a.getSelected() == "" || m.ID == "" {
		return
	}
	a.revertID = m.ID
	a.revertOpen = true
	a.win.Update(func() {})
}

// revertDialog confirms and performs the rewind.
func (a *app) revertDialog(c *ui.Context) {
	if a.revertID == "" {
		return
	}
	wasOpen := a.revertOpen
	res := kit.AlertDialog(c, &a.revertOpen, kit.AlertDialogOptions{
		Title: "回退到此点",
		Description: "将把代码回退到该回复完成时的 checkpoint。" +
			"此回复之后的所有改动都会丢失，且不可撤销。",
		ConfirmLabel: "回退",
	})
	if res.Confirmed {
		a.commitRevert()
	}
	if a.revertOpen {
		return
	}
	if wasOpen {
		return
	}
	a.revertID = ""
}

func (a *app) commitRevert() {
	id := a.getSelected()
	mid := a.revertID
	if id == "" || mid == "" {
		return
	}
	a.revertOpen = false
	a.status = "正在回退…"
	go func() {
		if err := apiPost(messagePath(id, mid, "revert"), nil, nil); err != nil {
			a.win.Update(func() { a.status = strings.TrimSpace(err.Error()) })
			return
		}
		a.win.Update(func() { a.flash("已回退到该 checkpoint") })
		a.loadSession(id, false)
	}()
}
