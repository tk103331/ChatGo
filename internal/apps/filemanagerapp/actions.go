package filemanagerapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/files"
)

// currentDir returns the explorer's current folder.
func (cl *Client) currentDir(st *state) string {
	if st.explorer.Current != "" {
		return st.explorer.Current
	}
	return st.root
}

// navigate switches the current folder.
func (cl *Client) navigate(dir string) {
	abs, err := resolvePath(dir)
	if err != nil {
		cl.setErr(err)
		return
	}
	cl.mu.Lock()
	cl.st.explorer.Navigate(abs)
	cl.mu.Unlock()
}

// onNavigate refreshes the preview when the folder changes.
func (cl *Client) onNavigate(dir string) {
	cl.mu.Lock()
	cl.st.previewPath = ""
	cl.st.previewText = ""
	cl.st.previewErr = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// onSelect records the current selection.
func (cl *Client) onSelect(paths []string) {
	cl.mu.Lock()
	cl.st.selected = len(paths)
	cl.mu.Unlock()
}

// openPreview reads a file for preview.
func (cl *Client) openPreview(path string) {
	info, err := os.Stat(path)
	if err != nil {
		cl.setErr(err)
		return
	}
	if info.IsDir() {
		cl.navigate(path)
		return
	}
	cl.mu.Lock()
	cl.st.previewPath = path
	cl.st.previewName = info.Name()
	cl.st.previewSize = info.Size()
	cl.st.previewText = ""
	cl.st.previewErr = ""
	cl.st.previewLoad = true
	cl.mu.Unlock()
	cl.host.Invalidate()
	go func() {
		text, err := readText(path)
		cl.mu.Lock()
		cl.st.previewLoad = false
		if err != nil {
			cl.st.previewErr = err.Error()
		} else {
			cl.st.previewText = text
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// closePreview hides the preview pane.
func (cl *Client) closePreview() {
	cl.mu.Lock()
	cl.st.previewPath = ""
	cl.st.previewText = ""
	cl.st.previewErr = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// refresh clears the cache and reloads the explorer.
func (cl *Client) refresh() {
	cl.mu.Lock()
	cl.st.revision++
	cl.st.cache = map[string][]files.FileEntry{}
	cl.st.err = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// toggleHidden shows or hides dot files.
func (cl *Client) toggleHidden() {
	cl.mu.Lock()
	cl.st.showHidden = !cl.st.showHidden
	cl.st.revision++
	cl.st.cache = map[string][]files.FileEntry{}
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// setErr shows a one-line error.
func (cl *Client) setErr(err error) {
	cl.mu.Lock()
	cl.st.err = err.Error()
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// promptMkdir asks for a folder name and creates it.
func (cl *Client) promptMkdir() {
	cl.mu.Lock()
	dir := cl.currentDir(cl.st)
	cl.mu.Unlock()
	cl.host.Prompt("新建文件夹", "名称", "", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		target, err := resolvePath(filepath.Join(dir, name))
		if err != nil {
			cl.setErr(err)
			return
		}
		go func() {
			if err := makeDir(target); err != nil {
				cl.host.Post(func() {
					cl.host.Toast("新建失败: "+err.Error(), mcore.SeverityError)
				})
				return
			}
			cl.host.Post(func() {
				cl.host.Toast("已创建 "+name, mcore.SeveritySuccess)
				cl.refresh()
			})
		}()
	})
}

// promptRename renames the selected entry.
func (cl *Client) promptRename() {
	sel := cl.selectedPaths()
	if len(sel) != 1 {
		cl.setErr(fmt.Errorf("请先选择一个文件或文件夹"))
		return
	}
	oldPath := sel[0]
	cl.host.Prompt("重命名", "新名称", filepath.Base(oldPath), func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		target, err := resolvePath(filepath.Join(filepath.Dir(oldPath), name))
		if err != nil {
			cl.setErr(err)
			return
		}
		go func() {
			if err := renamePath(oldPath, target); err != nil {
				cl.host.Post(func() {
					cl.host.Toast("重命名失败: "+err.Error(), mcore.SeverityError)
				})
				return
			}
			cl.host.Post(func() {
				cl.host.Toast("已重命名为 "+name, mcore.SeveritySuccess)
				cl.mu.Lock()
				if cl.st.previewPath == oldPath {
					cl.st.previewPath = target
					cl.st.previewName = name
				}
				cl.mu.Unlock()
				cl.refresh()
			})
		}()
	})
}

// confirmRemove deletes the selected entries after confirmation.
func (cl *Client) confirmRemove() {
	sel := cl.selectedPaths()
	if len(sel) == 0 {
		cl.setErr(fmt.Errorf("请先选择要删除的项目"))
		return
	}
	desc := "确定删除 " + filepath.Base(sel[0]) + " 吗？"
	if len(sel) > 1 {
		desc = fmt.Sprintf("确定删除选中的 %d 项吗？", len(sel))
	}
	cl.host.Confirm("删除", desc, func() {
		go func() {
			var failed []string
			for _, p := range sel {
				if _, err := resolvePath(p); err != nil {
					failed = append(failed, filepath.Base(p))
					continue
				}
				if err := removePath(p); err != nil {
					failed = append(failed, filepath.Base(p))
				}
			}
			cl.host.Post(func() {
				if len(failed) > 0 {
					cl.host.Toast("部分删除失败: "+strings.Join(failed, ", "), mcore.SeverityError)
				} else {
					cl.host.Toast("已删除", mcore.SeveritySuccess)
				}
				cl.mu.Lock()
				cl.st.previewPath = ""
				cl.st.previewText = ""
				cl.mu.Unlock()
				cl.refresh()
			})
		}()
	})
}

// selectedPaths returns the explorer's selected paths.
func (cl *Client) selectedPaths() []string {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.st.explorer.Selected()
}
