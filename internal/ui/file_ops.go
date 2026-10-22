package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "chatgo/internal/ui/uikit"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
)

// inputState is a reusable single-field prompt dialog.
type inputState struct {
	open   bool
	active bool
	title  string
	label  string
	value  string
	action func(string)
}

func (cw *ChatWindow) askInput(title, label, initial string, action func(string)) {
	cw.input = inputState{open: true, active: true, title: title, label: label, value: initial, action: action}
}

// inputDialog shows the pending prompt and runs its action on confirm.
func (cw *ChatWindow) inputDialog(c *mgo.Context) {
	if !cw.input.open && !cw.input.active {
		return
	}
	submit, cancel := false, false
	moverlay.Dialog(c, &cw.input.open, moverlay.DialogOptions{
		Title: cw.input.title,
		Width: 440,
		Actions: func() {
			if minput.Button(c, "取消", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cancel = true
			}
			if minput.Button(c, "确定", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				submit = true
			}
		},
	}, func() {
		in := minput.TextInput(c, &cw.input.value, minput.TextInputOptions{Label: cw.input.label, Placeholder: cw.input.label})
		in.Input.AutoFocus()
		if in.Submitted() {
			submit = true
		}
	})
	cw.input.active = cw.input.open
	if cancel {
		cw.input.open, cw.input.action = false, nil
		return
	}
	if submit && cw.input.action != nil {
		action := cw.input.action
		cw.input.action = nil
		cw.input.open = false
		action(strings.TrimSpace(cw.input.value))
	}
}

// treeContextMenu adds the file tree's right-click actions for a file or directory.
func (cw *ChatWindow) treeContextMenu(c *mgo.Context, target mgo.Element, path string, isDir bool) {
	if !target.Valid() {
		return
	}
	moverlay.ContextMenu(c, target, moverlay.ContextMenuOptions{}, func(m *moverlay.PopupMenu) {
		if !isDir && m.Item("打开预览", moverlay.PopupMenuItemOptions{}) {
			cw.openPreview(path)
		}
		if m.Item("添加到输入框上下文", moverlay.PopupMenuItemOptions{}) {
			cw.addWorkspaceContext(path)
		}
		if isDir {
			if m.Item("在此打开终端", moverlay.PopupMenuItemOptions{}) {
				cw.addTermIn(path)
			}
			if m.Item("新建文件", moverlay.PopupMenuItemOptions{}) {
				cw.newFileIn(path)
			}
			if m.Item("新建文件夹", moverlay.PopupMenuItemOptions{}) {
				cw.newDirIn(path)
			}
		}
		if m.Item("复制相对路径", moverlay.PopupMenuItemOptions{}) {
			c.WriteClipboard(cw.workspaceRel(path))
		}
		if m.Item("复制绝对路径", moverlay.PopupMenuItemOptions{}) {
			abs := path
			if v, err := filepath.Abs(path); err == nil {
				abs = v
			}
			c.WriteClipboard(abs)
		}
		if m.Item("在文件管理器中显示", moverlay.PopupMenuItemOptions{}) {
			revealPath(path)
		}
		if !isDir && m.Item("用默认程序打开", moverlay.PopupMenuItemOptions{}) {
			openPath(path)
		}
		if m.Item("重命名", moverlay.PopupMenuItemOptions{}) {
			cw.renamePath(path)
		}
		if m.Item("删除", moverlay.PopupMenuItemOptions{}) {
			cw.deletePath(path)
		}
	})
}

func (cw *ChatWindow) renamePath(path string) {
	cw.askInput("重命名", "新名称", filepath.Base(path), func(name string) {
		if name == "" || name == filepath.Base(path) {
			return
		}
		if err := os.Rename(path, filepath.Join(filepath.Dir(path), name)); err != nil {
			cw.toast("重命名失败："+err.Error(), mcore.SeverityError)
			return
		}
		cw.invalidate()
	})
}

func (cw *ChatWindow) deletePath(path string) {
	cw.askConfirm("删除", "确定删除「"+filepath.Base(path)+"」？此操作不可撤销。", func() {
		if err := os.RemoveAll(path); err != nil {
			cw.toast("删除失败："+err.Error(), mcore.SeverityError)
			return
		}
		cw.invalidate()
	})
}

func (cw *ChatWindow) newFileIn(dir string) {
	cw.askInput("新建文件", "文件名", "", func(name string) {
		if name == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			cw.toast("新建失败："+err.Error(), mcore.SeverityError)
			return
		}
		cw.invalidate()
	})
}

func (cw *ChatWindow) newDirIn(dir string) {
	cw.askInput("新建文件夹", "文件夹名", "", func(name string) {
		if name == "" {
			return
		}
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			cw.toast("新建失败："+err.Error(), mcore.SeverityError)
			return
		}
		cw.invalidate()
	})
}

// workspaceRel is path relative to the workspace root, or path when outside it.
func (cw *ChatWindow) workspaceRel(path string) string {
	rel, err := filepath.Rel(cw.fileRoot(), path)
	if err != nil || rel == "" || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func revealPath(path string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	case "windows":
		_ = exec.Command("explorer", "/select,"+path).Start()
	default:
		dir := path
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			dir = filepath.Dir(path)
		}
		_ = exec.Command("xdg-open", dir).Start()
	}
}

func openPath(path string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", path).Start()
	case "windows":
		_ = exec.Command("cmd", "/c", "start", "", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
	}
}
