package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// addDroppedFiles uploads files dropped on the composer. The browser exposes a
// dropped file's bytes under a /mygo-drop path, readable only synchronously in
// the drop handler, so the bytes are read here before the upload starts.
func (a *app) addDroppedFiles(paths []string) {
	id := a.activeID()
	if id == "" || len(paths) == 0 {
		return
	}
	type upload struct {
		name string
		data []byte
	}
	ups := make([]upload, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			a.status = "读取附件失败：" + err.Error()
			continue
		}
		ups = append(ups, upload{name: filepath.Base(path), data: data})
	}
	if len(ups) == 0 {
		return
	}
	a.attaching = true
	a.win.Update(func() {})
	go func() {
		for _, up := range ups {
			body := map[string]string{
				"name": up.name,
				"data": base64.StdEncoding.EncodeToString(up.data),
			}
			var resp struct {
				Attachment attachmentRef `json:"attachment"`
			}
			if err := apiPost(sessionPath(id)+"/attachments", body, &resp); err != nil {
				a.win.Update(func() { a.status = "上传附件失败：" + err.Error() })
				continue
			}
			if resp.Attachment.Path == "" {
				continue
			}
			a.win.Update(func() {
				if a.activeID() == id {
					a.attachments = append(a.attachments, resp.Attachment)
				}
			})
		}
		a.win.Update(func() { a.attaching = false })
	}()
}

// attachmentChips shows the files the next message carries.
func (a *app) attachmentChips(c *ui.Context, t *ui.Theme) {
	if len(a.attachments) == 0 && !a.attaching {
		return
	}
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Row(c).FillWidth().Gap(u).Wrap().AlignItems(ui.Center).Children(func() {
		if a.attaching {
			uikit.Spinner(c, uikit.SpinnerOptions{Size: 12})
			ui.Text(c, "上传中…").FontSize(caption).TextColor(t.TextMuted)
		}
		for i := range a.attachments {
			item := a.attachments[i]
			ui.Row(c).Shrink(0).Gap(u).AlignItems(ui.Center).
				Padding(u/2, u).Radius(mtheme.ControlRadius).
				Background(t.Surface).Border(mtheme.BorderWidth, t.Border).Children(func() {
				ui.Icon(c, icons.Must("file")).Size(12, 12).
					TextColor(t.Accent).Shrink(0)
				ui.Text(c, item.Name).FontSize(caption).SingleLine().Ellipsis("…").MaxWidth(180)
				if uikit.IconAction(c, icons.Must("x"), "移除附件").Clicked() {
					a.attachments = append(a.attachments[:i], a.attachments[i+1:]...)
				}
			})
		}
	})
}

// attachmentButton explains the drop target and lists the pending files.
func (a *app) attachmentButton(c *ui.Context, t *ui.Theme, open *bool) {
	b := kit.IconButton(c, icons.Must("file"), "附件", kit.IconButtonOptions{
		Variant: kit.Ghost,
	})
	if len(a.attachments) > 0 {
		b.Tooltip(fmt.Sprintf("附件（%d）", len(a.attachments)))
	} else {
		b.Tooltip("附件：把文件拖入输入框")
	}
	if b.Clicked() {
		*open = !*open
	}
	kit.Popover(c, b, open, kit.PopoverOptions{
		Title: "附件", Width: 300, Up: true,
	}, func() {
		u := uikit.Unit(c)
		caption := uikit.FontSize(c, mtheme.CaptionSize)
		ui.Text(c, "把文件拖入输入框即可添加，发送时会随消息交给模型。").
			FontSize(caption).TextColor(t.TextMuted)
		for _, item := range a.attachments {
			ui.Text(c, item.Name).FontSize(caption).SingleLine().Ellipsis("…")
		}
		if a.attaching {
			ui.Row(c).Gap(u).AlignItems(ui.Center).Children(func() {
				uikit.Spinner(c, uikit.SpinnerOptions{Size: 12})
				ui.Text(c, "上传中…").FontSize(caption).TextColor(t.TextMuted)
			})
		}
	})
}
