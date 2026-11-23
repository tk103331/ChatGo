package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
	"github.com/gorilla/websocket"
	"github.com/hinshun/vt10x"

	"chatgo/internal/mobileapi"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
)

// projectTab values of the project page's segmented control.
const (
	projectFiles = iota
	projectGit
	projectTerm
)

// projectList shows the desktop's projects, the places a session can run in.
func (a *app) projectList(c *ui.Context) {
	if a.offline() {
		a.offlineState(c)
		return
	}
	if len(a.workspaces) == 0 {
		ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).Children(func() {
			emptyState(c, "folder", "还没有项目", "新建项目，或先在桌面添加一个工作目录。", func() {
				if primaryButton(c, "新建项目").Clicked() {
					a.openProjectForm()
				}
			})
		})
		return
	}
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(4, 0, 88, 0).Gap(6).Children(func() {
				for _, dir := range a.workspaces {
					dir := dir
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:      filepath.Base(dir),
						Subtitle:   dir,
						Disclosure: true,
						Leading: func() {
							listMark(c, "folder-git-2", false)
						},
					})
					if row.Clicked() {
						a.openProject(dir)
					}
				}
			})
		})
		fabButton(c, "plus", "新建项目", a.openProjectForm)
	})
}

func (a *app) openProject(dir string) {
	a.stopTerminalIO()
	a.clearTerminalInput()
	a.update(func() {
		a.screen = screenProject
		a.projectDir = dir
		a.projectTab = projectFiles
		a.fileDir = dir
		a.files = nil
		a.fileErr = ""
		a.gitStatus, a.gitLog = mobileapi.GitStatus{}, nil
		a.gitErr = ""
		a.gitCommitMsg = ""
		a.termID, a.termVT = "", nil
		a.termSince, a.termExited, a.termExitCode, a.termConnErr = 0, false, 0, ""
		a.termDraft, a.termSent = "", ""
		a.termClosed = false
	})
	a.loadFiles()
}

func (a *app) projectView(c *ui.Context) {
	if a.offline() {
		a.offlineState(c)
		return
	}
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Padding(6, 12).Gap(6).Children(func() {
			for _, tab := range []struct {
				value int
				label string
			}{{projectFiles, "文件"}, {projectGit, "Git"}, {projectTerm, "终端"}} {
				tab := tab
				selected := a.projectTab == tab.value
				b := kit.Button(c, tab.label, kit.ButtonOptions{Variant: chipVariant(selected)}).
					Height(32).Padding(0, 14)
				if b.Clicked() {
					a.projectTab = tab.value
					switch tab.value {
					case projectFiles:
						if a.fileDir == "" {
							a.fileDir = a.projectDir
						}
						a.loadFiles()
					case projectGit:
						a.loadGit()
					case projectTerm:
						a.ensureTerminal()
					}
					if tab.value != projectTerm {
						a.stopTerminalIO()
					}
					a.invalidate()
				}
			}
		})
		switch a.projectTab {
		case projectFiles:
			a.projectFiles(c)
		case projectGit:
			a.projectGitTab(c)
		case projectTerm:
			a.projectTerminal(c)
		}
	})
}

// --- files ---

func (a *app) projectFiles(c *ui.Context) {
	t := c.Theme()
	if a.fileLoading && len(a.files) == 0 {
		ui.Text(c, "正在读取目录…").FontSize(13).TextColor(t.TextMuted).Padding(16)
		return
	}
	if a.fileErr != "" {
		ui.Column(c).Grow(1).Justify(ui.Center).Children(func() {
			emptyState(c, "folder", "无法读取目录", a.fileErr, func() {
				if secondaryButton(c, "重试").Clicked() {
					a.loadFiles()
				}
			})
		})
		return
	}
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Padding(4, 16).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("folder")).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, a.projectRel(a.fileDir)).FontSize(12).TextColor(t.TextMuted).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if smallIconButton(c, "refresh-cw", "刷新").Clicked() {
				a.loadFiles()
			}
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if a.fileDir != a.projectDir {
					if mobile.ListRow(c, mobile.RowOptions{
						Title: "..", Subtitle: "上一级",
						Leading: func() { listMark(c, "folder", false) },
					}).Clicked() {
						a.fileDir = filepath.Dir(a.fileDir)
						a.loadFiles()
					}
				}
				if len(a.files) == 0 {
					ui.Text(c, "这个目录是空的").FontSize(13).TextColor(t.TextMuted).Padding(16)
				}
				for _, entry := range a.files {
					entry := entry
					title, subtitle := entry.Name, ""
					icon := "file-text"
					if entry.Dir {
						icon = "folder"
					} else {
						subtitle = humanSize(entry.Size)
					}
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:    title,
						Subtitle: subtitle,
						Leading: func() {
							listMark(c, icon, false)
						},
						Trailing: func() {
							if entry.Dir {
								ui.Icon(c, icons.Must("chevron-right")).Size(14, 14).TextColor(t.TextMuted)
							}
						},
					})
					if row.Clicked() {
						if entry.Dir {
							a.fileDir = entry.Path
							a.loadFiles()
						} else {
							a.openFile(entry.Path)
						}
					}
				}
			})
		})
	})
}

func (a *app) loadFiles() {
	client := a.client
	if client == nil || a.fileDir == "" {
		return
	}
	dir := a.fileDir
	a.update(func() {
		a.fileLoading = true
		a.fileErr = ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		files, err := client.Files(ctx, dir)
		a.update(func() {
			if a.client != client || a.fileDir != dir {
				return
			}
			a.fileLoading = false
			if err != nil {
				a.fileErr = err.Error()
				return
			}
			a.files = files
		})
	}()
}

func (a *app) openFile(path string) {
	client := a.client
	if client == nil {
		return
	}
	a.update(func() {
		a.filePreviewOpen = true
		a.filePreview = mobileapi.FileContent{Path: path, Name: filepath.Base(path)}
		a.filePreviewErr = ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		file, err := client.ReadFile(ctx, path)
		a.update(func() {
			if a.client != client || a.filePreview.Path != path {
				return
			}
			if err != nil {
				a.filePreviewErr = err.Error()
				return
			}
			a.filePreview = file
		})
	}()
}

func (a *app) filePreviewSheet(c *ui.Context) {
	t := c.Theme()
	sheet(c, &a.filePreviewOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "文件"}, func() {
		a.sheetBack(c)
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.filePreview.Name).FontSize(17).FontWeight(600).Grow(1).MinWidth(0).
					SingleLine().Ellipsis("…")
				if a.filePreview.Size > 0 {
					ui.Text(c, humanSize(a.filePreview.Size)).FontSize(11).TextColor(t.TextMuted).Shrink(0)
				}
			})
			switch {
			case a.filePreviewErr != "":
				ui.Text(c, a.filePreviewErr).FontSize(13).TextColor(t.Danger).MaxLines(4)
			case a.filePreview.Binary:
				ui.Text(c, "二进制文件，手机上暂不预览。"+humanSize(a.filePreview.Size)).
					FontSize(13).TextColor(t.TextMuted)
			case a.filePreview.Content == "" && a.filePreview.Size == 0:
				ui.Text(c, "正在读取…").FontSize(13).TextColor(t.TextMuted)
			default:
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Text(c, a.filePreview.Content).Font(mtheme.MonoFont).FontSize(12).
						TextColor(t.Text).PreserveSpaces()
					if a.filePreview.Truncated {
						ui.Text(c, "文件较大，只显示前 1 MB。").FontSize(11).TextColor(t.TextMuted)
					}
				})
			}
			if ghostButton(c, "关闭").Clicked() {
				a.filePreviewOpen = false
			}
		})
	})
}

// --- Git ---

func (a *app) projectGitTab(c *ui.Context) {
	t := c.Theme()
	if a.gitLoading && a.gitStatus.Branch == "" {
		ui.Text(c, "正在读取 Git…").FontSize(13).TextColor(t.TextMuted).Padding(16)
		return
	}
	if a.gitErr != "" {
		ui.Column(c).Grow(1).Justify(ui.Center).Children(func() {
			emptyState(c, "git-branch", "Git 不可用", a.gitErr, func() {
				ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Children(func() {
					if secondaryButton(c, "重试").Clicked() {
						a.loadGit()
					}
					if strings.Contains(a.gitErr, "不是 Git 仓库") {
						if primaryButton(c, "初始化 Git").Clicked() {
							a.initGit()
						}
					}
				})
			})
		})
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(12, 16, 24, 16).Gap(10).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Padding(3, 8).Radius(8).
					Background(t.Surface).Children(func() {
					ui.Icon(c, icons.Must("git-branch")).Size(12, 12).TextColor(t.TextMuted).Shrink(0)
					ui.Text(c, a.gitStatus.Branch).FontSize(12).TextColor(t.Text)
				})
				if len(a.gitStatus.Changes) > 0 {
					ui.Text(c, fmt.Sprintf("%d 个变更", len(a.gitStatus.Changes))).
						FontSize(12).TextColor(t.TextMuted)
				}
				ui.Spacer(c)
				if smallIconButton(c, "refresh-cw", "刷新").Clicked() {
					a.loadGit()
				}
			})
			if len(a.gitStatus.Changes) == 0 {
				ui.Text(c, "工作区干净").FontSize(13).TextColor(t.TextMuted)
			} else {
				if secondaryButton(c, "暂存全部").Clicked() {
					a.stageAll()
				}
				for _, change := range a.gitStatus.Changes {
					change := change
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:    change.Path,
						Subtitle: strings.TrimSpace(change.Status) + statusSuffix(change.Staged),
						Leading: func() {
							listMark(c, "file-text", false)
						},
						Disclosure: true,
					})
					if row.Clicked() {
						a.openGitDiff(change.Path)
					}
				}
			}
			ui.Text(c, "提交").FontSize(13).TextColor(t.TextMuted)
			ui.TextInput(c, &a.gitCommitMsg).Placeholder("提交信息")
			if primaryButton(c, "提交变更").Clicked() {
				a.commitGit()
			}
			ui.Text(c, "最近提交").FontSize(13).TextColor(t.TextMuted)
			if len(a.gitLog) == 0 {
				ui.Text(c, "还没有提交记录").FontSize(12).TextColor(t.TextMuted)
			}
			for _, commit := range a.gitLog {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
					ui.Text(c, commit.Hash).Font(mtheme.MonoFont).FontSize(11).
						TextColor(t.TextMuted).Shrink(0)
					ui.Column(c).Grow(1).Basis(0).Gap(1).Children(func() {
						ui.Text(c, commit.Subject).FontSize(13).SingleLine().Ellipsis("…")
						ui.Text(c, commit.Author+" · "+commit.Date).FontSize(10).TextColor(t.TextMuted)
					})
				})
			}
		})
	})
}

// initGit turns the current project into a repository, then reloads it.
func (a *app) initGit() {
	client := a.client
	dir := a.projectDir
	a.update(func() { a.gitLoading = true; a.gitErr = "" })
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := client.RunCommand(ctx, dir, "git init"); err != nil {
			a.update(func() {
				if a.client == client && a.projectDir == dir {
					a.gitLoading, a.gitErr = false, err.Error()
				}
			})
			return
		}
		a.loadGit()
	}()
}

func (a *app) loadGit() {
	client := a.client
	if client == nil || a.projectDir == "" {
		return
	}
	dir := a.projectDir
	a.update(func() {
		a.gitLoading = true
		a.gitErr = ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		status, err := client.GitStatus(ctx, dir)
		if err != nil {
			a.update(func() {
				if a.client == client && a.projectDir == dir {
					a.gitLoading, a.gitErr = false, err.Error()
				}
			})
			return
		}
		log, _ := client.GitLog(ctx, dir)
		a.update(func() {
			if a.client != client || a.projectDir != dir {
				return
			}
			a.gitLoading = false
			a.gitStatus, a.gitLog = status, log
		})
	}()
}

func (a *app) openGitDiff(path string) {
	client := a.client
	if client == nil {
		return
	}
	a.update(func() {
		a.gitDiffOpen = true
		a.gitDiffTitle = path
		a.gitDiff = ""
		a.gitDiffErr = ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		diff, err := client.GitDiff(ctx, a.projectDir, path)
		a.update(func() {
			if a.client != client || a.gitDiffTitle != path {
				return
			}
			if err != nil {
				a.gitDiffErr = err.Error()
				return
			}
			a.gitDiff = diff
		})
	}()
}

func (a *app) gitDiffSheet(c *ui.Context) {
	t := c.Theme()
	sheet(c, &a.gitDiffOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "差异"}, func() {
		a.sheetBack(c)
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Children(func() {
			ui.Text(c, a.gitDiffTitle).Font(mtheme.MonoFont).FontSize(14).FontWeight(600).
				SingleLine().Ellipsis("…")
			switch {
			case a.gitDiffErr != "":
				ui.Text(c, a.gitDiffErr).FontSize(13).TextColor(t.Danger)
			case a.gitDiff == "":
				ui.Text(c, "正在读取…").FontSize(13).TextColor(t.TextMuted)
			default:
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Text(c, a.gitDiff).Font(mtheme.MonoFont).FontSize(11).TextColor(t.Text).PreserveSpaces()
				})
			}
			if ghostButton(c, "关闭").Clicked() {
				a.gitDiffOpen = false
			}
		})
	})
}

func (a *app) stageAll() {
	client := a.client
	dir := a.projectDir
	a.update(func() { a.gitLoading = true; a.gitErr = "" })
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := client.GitStage(ctx, dir, ""); err != nil {
			a.update(func() {
				if a.client == client && a.projectDir == dir {
					a.gitLoading, a.gitErr = false, err.Error()
				}
			})
			return
		}
		a.loadGit()
	}()
}

func (a *app) commitGit() {
	client := a.client
	dir := a.projectDir
	message := strings.TrimSpace(a.gitCommitMsg)
	if message == "" {
		a.toast("提交信息不能为空")
		return
	}
	a.update(func() { a.gitLoading = true; a.gitErr = "" })
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.GitCommit(ctx, dir, message); err != nil {
			a.update(func() {
				if a.client == client && a.projectDir == dir {
					a.gitLoading, a.gitErr = false, err.Error()
				}
			})
			return
		}
		a.update(func() {
			if a.client == client && a.projectDir == dir {
				a.gitCommitMsg = ""
			}
		})
		a.loadGit()
	}()
}

// --- terminal ---

const (
	terminalCols = 40
	terminalRows = 26
)

func (a *app) projectTerminal(c *ui.Context) {
	t := c.Theme()
	if a.termID == "" {
		if a.termClosed {
			ui.Column(c).Grow(1).Justify(ui.Center).Children(func() {
				emptyState(c, "terminal", "终端已关闭", "重新打开一个交互式终端。", func() {
					if primaryButton(c, "新建终端").Clicked() {
						a.newTerminal()
					}
				})
			})
			return
		}
		if a.termConnErr != "" {
			ui.Column(c).Grow(1).Justify(ui.Center).Children(func() {
				emptyState(c, "terminal", "终端连接失败", a.termConnErr, func() {
					if secondaryButton(c, "重试").Clicked() {
						a.termStarting = false
						a.ensureTerminal()
					}
				})
			})
			return
		}
		ui.Text(c, "正在连接终端…").FontSize(13).TextColor(t.TextMuted).Padding(16)
		a.ensureTerminal()
		return
	}
	ui.Column(c).Grow(1).MinHeight(0).Background(t.Surface).Children(func() {
		ui.Row(c).FillWidth().Padding(4, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("terminal")).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, firstNonEmpty(a.termInfo.Title, filepath.Base(a.projectDir))).
				FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if smallGhostButton(c, "新终端").Clicked() {
				a.newTerminal()
			}
			if smallIconButton(c, "x", "关闭终端").Clicked() {
				a.closeTerminal()
			}
		})
		if a.termConnErr != "" {
			ui.Text(c, "连接中断，正在重连…").FontSize(11).TextColor(t.Warning).Padding(0, 12, 2, 12)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&a.termScroll).Children(func() {
			ui.Text(c, a.termScreen()).Font(mtheme.MonoFont).FontSize(11).
				TextColor(t.Text).PreserveSpaces().Padding(4, 8)
		})
		if a.termExited {
			ui.Row(c).FillWidth().Padding(4, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "进程已退出").FontSize(12).TextColor(t.TextMuted).Grow(1)
				if secondaryButton(c, "重新启动").Clicked() {
					a.newTerminal()
				}
			})
		}
		ui.Row(c).FillWidth().Padding(4, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			if smallGhostButton(c, "Ctrl+C").Clicked() {
				a.queueTermInput([]byte{0x03})
			}
			if smallGhostButton(c, "Esc").Clicked() {
				a.queueTermInput([]byte{0x1b})
			}
			if smallGhostButton(c, "Tab").Clicked() {
				a.queueTermInput([]byte{'\t'})
			}
		})
		ui.Row(c).FillWidth().Padding(3, 12, 8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c, &a.termDraft).Grow(1).Placeholder("终端输入…")
			input.OnTextKey(func(k ui.TextKey) bool {
				seq, clear := terminalKey(k)
				if seq == nil {
					return false // Backspace and printable text edit locally
				}
				a.queueTermInput(seq)
				if clear {
					k.SetText("")
					a.termSent = ""
				}
				return true
			})
			if input.Changed() && !input.Composing() {
				a.sendTermDiff()
			}
		})
	})
}

// terminalKey translates a key the soft keyboard does not type into the bytes
// a shell expects. clear says the local line should reset.
func terminalKey(k ui.TextKey) ([]byte, bool) {
	if k.Mods&ui.Ctrl != 0 && k.Key >= ui.KeyA && k.Key <= ui.KeyZ {
		return []byte{byte(k.Key-ui.KeyA) + 1}, true
	}
	switch k.Key {
	case ui.KeyEnter:
		return []byte{'\r'}, true
	case ui.KeyTab:
		return []byte{'\t'}, false
	case ui.KeyEscape:
		return []byte{0x1b}, false
	case ui.KeyDelete:
		return []byte("\x1b[3~"), false
	case ui.KeyUp:
		return []byte("\x1b[A"), false
	case ui.KeyDown:
		return []byte("\x1b[B"), false
	case ui.KeyRight:
		return []byte("\x1b[C"), false
	case ui.KeyLeft:
		return []byte("\x1b[D"), false
	case ui.KeyHome:
		return []byte("\x1b[H"), false
	case ui.KeyEnd:
		return []byte("\x1b[F"), false
	case ui.KeyPageUp:
		return []byte("\x1b[5~"), false
	case ui.KeyPageDown:
		return []byte("\x1b[6~"), false
	}
	return nil, false
}

// sendTermDiff sends the change from the previous local line to the current
// one: printable input as UTF-8, deletions as backspaces. An input method's
// composition waits until it commits.
func (a *app) sendTermDiff() {
	prev, next := a.termSent, a.termDraft
	if prev == next {
		return
	}
	prefix := 0
	for prefix < len(prev) && prefix < len(next) && prev[prefix] == next[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(prev)-prefix && suffix < len(next)-prefix &&
		prev[len(prev)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	var out []byte
	for i := prefix; i < len(prev)-suffix; i++ {
		out = append(out, 0x7f)
	}
	out = append(out, next[prefix:len(next)-suffix]...)
	if len(out) > 0 {
		a.queueTermInput(out)
	}
	a.termSent = next
}

// clearTerminalInput drops keystrokes queued for a previous session.
func (a *app) clearTerminalInput() {
	a.termInputMu.Lock()
	a.termInput = nil
	a.termInputMu.Unlock()
}

// queueTermInput appends keys for the writer, keeping them across a reconnect.
func (a *app) queueTermInput(data []byte) {
	if len(data) == 0 {
		return
	}
	a.termInputMu.Lock()
	a.termInput = append(a.termInput, data...)
	if len(a.termInput) > 64<<10 {
		a.termInput = a.termInput[len(a.termInput)-64<<10:]
	}
	a.termInputMu.Unlock()
	if a.termInputWake != nil {
		select {
		case a.termInputWake <- struct{}{}:
		default:
		}
	}
}

// ensureTerminal attaches to the project's existing shell or starts one. The
// session outlives the phone, so reconnecting reuses it.
func (a *app) ensureTerminal() {
	if a.termClosed {
		return
	}
	if a.termID != "" {
		if a.termVT == nil {
			a.termVT = vt10x.New(vt10x.WithSize(a.termInfo.Cols, a.termInfo.Rows))
		}
		a.startTerminalIO()
		return
	}
	if a.termStarting || a.client == nil || a.projectDir == "" {
		return
	}
	client := a.client
	dir := a.projectDir
	a.termStarting = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		sessions, err := client.Terminals(ctx)
		var info mobileapi.TerminalInfo
		if err == nil {
			for _, s := range sessions {
				if s.Dir == dir && !s.Exited {
					info = s
					break
				}
			}
			if info.ID == "" {
				info, err = client.StartTerminal(ctx, dir, terminalCols, terminalRows, filepath.Base(dir))
			}
		}
		a.update(func() {
			if a.client != client || a.projectDir != dir {
				return
			}
			a.termStarting = false
			if err != nil {
				a.termConnErr = err.Error()
				return
			}
			a.termID = info.ID
			a.termInfo = info
			a.termVT = vt10x.New(vt10x.WithSize(info.Cols, info.Rows))
			a.termSince = 0
			a.termExited, a.termExitCode = info.Exited, info.ExitCode
			a.termConnErr = ""
			a.termDraft, a.termSent = "", ""
			a.startTerminalIO()
		})
	}()
}

// startTerminalIO attaches a WebSocket to the shell. Reconnects are handled
// here: the session stays on the desktop, and queued keys wait for the next
// connection.
func (a *app) startTerminalIO() {
	a.stopTerminalIO()
	if a.client == nil || a.termID == "" {
		return
	}
	client, id := a.client, a.termID
	ctx, cancel := context.WithCancel(context.Background())
	a.termPollCancel = cancel
	a.termInputWake = make(chan struct{}, 1)
	wake := a.termInputWake
	go a.terminalWSLoop(ctx, client, id, wake)
}

func (a *app) stopTerminalIO() {
	if a.termPollCancel != nil {
		a.termPollCancel()
		a.termPollCancel = nil
	}
	a.termSendCancel = nil
}

// terminalWSLoop keeps the terminal WebSocket connected. The desktop replays
// the session's scrollback on every attach, so a reconnect restores the screen.
func (a *app) terminalWSLoop(ctx context.Context, client *mobileapi.Client, id string, wake chan struct{}) {
	for ctx.Err() == nil {
		conn, err := client.DialTerminal(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.update(func() {
				if a.client == client && a.termID == id {
					a.termConnErr = err.Error()
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(700 * time.Millisecond):
			}
			continue
		}
		a.update(func() {
			if a.client == client && a.termID == id {
				a.termConnErr = ""
			}
		})
		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			a.terminalWriter(ctx, conn, client, wake)
		}()
		exited := false
		for ctx.Err() == nil {
			kind, payload, err := mobileapi.ReadTerminalFrame(conn, client.E2EKey, client.DesktopID, "s2c")
			if err != nil {
				break
			}
			if kind == 1 {
				var control struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
					Code int    `json:"code"`
				}
				if json.Unmarshal(payload, &control) == nil {
					switch control.Type {
					case "reset":
						cols, rows := control.Cols, control.Rows
						if cols <= 0 || rows <= 0 {
							cols, rows = a.termInfo.Cols, a.termInfo.Rows
						}
						a.update(func() {
							if a.client == client && a.termID == id {
								a.termVT = vt10x.New(vt10x.WithSize(cols, rows))
								a.termConnErr = ""
							}
						})
					case "exit":
						exited = true
						a.update(func() {
							if a.client == client && a.termID == id {
								a.termExited, a.termExitCode = true, control.Code
							}
						})
					}
				}
				continue
			}
			if len(payload) > 0 {
				a.update(func() {
					if a.client != client || a.termID != id {
						return
					}
					if a.termVT == nil {
						a.termVT = vt10x.New(vt10x.WithSize(a.termInfo.Cols, a.termInfo.Rows))
					}
					_, _ = a.termVT.Write(payload)
					a.termScroll.Y = math.MaxFloat32
				})
			}
		}
		_ = conn.Close()
		<-writeDone
		if exited || ctx.Err() != nil {
			return
		}
		a.update(func() {
			if a.client == client && a.termID == id {
				a.termConnErr = "连接中断，正在重连…"
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(700 * time.Millisecond):
		}
	}
}

// terminalWriter owns all writes on one connection, flushing queued keys in
// order. A failed write puts them back for the next connection.
func (a *app) terminalWriter(ctx context.Context, conn *websocket.Conn, client *mobileapi.Client, wake chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
		a.termInputMu.Lock()
		data := append([]byte(nil), a.termInput...)
		a.termInput = nil
		a.termInputMu.Unlock()
		if len(data) == 0 {
			continue
		}
		if err := mobileapi.WriteTerminalFrame(conn, client.E2EKey, client.DesktopID, "c2s", 0, data); err != nil {
			a.termInputMu.Lock()
			a.termInput = append(data, a.termInput...)
			a.termInputMu.Unlock()
			return
		}
	}
}

// termScreen renders the emulated screen, with a block at the cursor.
func (a *app) termScreen() string {
	if a.termVT == nil {
		return "正在连接…"
	}
	a.termVT.Lock()
	defer a.termVT.Unlock()
	cols, rows := a.termVT.Size()
	cx, cy := a.termVT.Cursor().X, a.termVT.Cursor().Y
	showCursor := a.termVT.CursorVisible()
	var b strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			ch := a.termVT.Cell(x, y).Char
			if ch == 0 {
				ch = ' '
			}
			if showCursor && x == cx && y == cy {
				ch = '█'
			}
			b.WriteRune(ch)
		}
		if y < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (a *app) newTerminal() {
	client := a.client
	dir := a.projectDir
	oldID := a.termID
	a.stopTerminalIO()
	a.clearTerminalInput()
	a.update(func() {
		a.termID, a.termVT = "", nil
		a.termSince, a.termExited, a.termExitCode, a.termConnErr = 0, false, 0, ""
		a.termDraft, a.termSent = "", ""
		a.termStarting = false
		a.termClosed = false
	})
	if client == nil || dir == "" {
		return
	}
	a.termStarting = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if oldID != "" {
			_ = client.CloseTerminal(ctx, oldID)
		}
		info, err := client.StartTerminal(ctx, dir, terminalCols, terminalRows, filepath.Base(dir))
		a.update(func() {
			if a.client != client || a.projectDir != dir {
				return
			}
			a.termStarting = false
			if err != nil {
				a.termConnErr = err.Error()
				return
			}
			a.termID, a.termInfo = info.ID, info
			a.termVT = vt10x.New(vt10x.WithSize(info.Cols, info.Rows))
			a.termSince, a.termExited, a.termExitCode = 0, info.Exited, info.ExitCode
			a.termConnErr = ""
			a.termDraft, a.termSent = "", ""
			a.startTerminalIO()
		})
	}()
}

func (a *app) closeTerminal() {
	client := a.client
	id := a.termID
	a.stopTerminalIO()
	a.clearTerminalInput()
	a.update(func() {
		a.termID, a.termVT = "", nil
		a.termSince, a.termExited, a.termExitCode, a.termConnErr = 0, false, 0, ""
		a.termDraft, a.termSent = "", ""
		a.termClosed = true
	})
	if client != nil && id != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = client.CloseTerminal(ctx, id)
		}()
	}
}

// --- new project ---

func (a *app) openProjectForm() {
	a.update(func() {
		a.projectFormOpen = true
		a.projectName = ""
		a.projectParent = ""
		a.projectInitGit = true
		a.projectFormErr = ""
	})
}

func (a *app) projectFormSheet(c *ui.Context) {
	t := c.Theme()
	sheet(c, &a.projectFormOpen, mobile.SheetOptions{Detents: []float32{0.85}, Label: "新建项目"}, func() {
		a.sheetBack(c)
		ui.Column(c).FillWidth().Padding(16).Gap(10).Children(func() {
			ui.Text(c, "新建项目").FontSize(17).FontWeight(600)
			ui.TextInput(c, &a.projectName).Placeholder("项目名称")
			label := "用户目录"
			if a.projectParent != "" {
				label = filepath.Base(a.projectParent)
			}
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons.Must("folder")).Size(16, 16).TextColor(t.TextMuted).Shrink(0)
				ui.Column(c).Grow(1).Basis(0).Children(func() {
					ui.Text(c, "位置").FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, label).FontSize(13).SingleLine().Ellipsis("…")
				})
				if smallSecondaryButton(c, "浏览").Clicked() {
					a.openProjectPick()
				}
			})
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
					ui.Text(c, "初始化 Git").FontSize(13)
					ui.Text(c, "创建后运行 git init。").FontSize(11).TextColor(t.TextMuted)
				})
				mobile.Switch(c, &a.projectInitGit)
			})
			if a.projectFormErr != "" {
				ui.Text(c, a.projectFormErr).FontSize(12).TextColor(t.Danger).MaxLines(4)
			}
			ui.Row(c).Gap(8).Children(func() {
				if primaryButton(c, "创建").Clicked() {
					a.createProject()
				}
				if ghostButton(c, "取消").Clicked() {
					a.projectFormOpen = false
				}
			})
		})
	})
}

func (a *app) createProject() {
	client := a.client
	name := strings.TrimSpace(a.projectName)
	if name == "" {
		a.toast("项目名称不能为空")
		return
	}
	if client == nil || a.projectCreating {
		return
	}
	parent := a.projectParent
	gitInit := a.projectInitGit
	a.update(func() {
		a.projectCreating = true
		a.projectFormErr = ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		path, err := client.CreateProject(ctx, parent, name, gitInit)
		opened := false
		a.update(func() {
			if a.client != client {
				return
			}
			a.projectCreating = false
			if err != nil {
				a.projectFormErr = err.Error()
				return
			}
			a.projectFormOpen = false
			a.workspaces = append(a.workspaces, path)
			a.newWorkspace = path
			if a.screen == screenProjects {
				a.screen = screenProject
				a.projectDir = path
				a.projectTab = projectFiles
				a.fileDir = path
				a.files = nil
				a.gitStatus, a.gitLog = mobileapi.GitStatus{}, nil
				a.termID, a.termVT = "", nil
				a.termSince, a.termExited, a.termExitCode, a.termConnErr = 0, false, 0, ""
				a.termDraft, a.termSent = "", ""
				opened = true
			}
		})
		if opened {
			a.loadFiles()
		}
	}()
}

func (a *app) openProjectPick() {
	dir := a.projectParent
	a.update(func() {
		a.projectPickOpen = true
		a.projectPickLoading = true
		a.projectPickErr = ""
	})
	a.loadProjectDirs(dir)
}

func (a *app) loadProjectDirs(dir string) {
	client := a.client
	if client == nil {
		return
	}
	a.update(func() { a.projectPickLoading = true; a.projectPickErr = "" })
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		listing, err := client.Directories(ctx, dir)
		a.update(func() {
			if a.client != client {
				return
			}
			a.projectPickLoading = false
			if err != nil {
				a.projectPickErr = err.Error()
				return
			}
			a.projectPickDir = listing.Dir
			a.projectPickParent = listing.Parent
			a.projectPickDirs = listing.Entries
		})
	}()
}

func (a *app) projectPickSheet(c *ui.Context) {
	t := c.Theme()
	sheet(c, &a.projectPickOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "选择位置"}, func() {
		a.sheetBack(c)
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Children(func() {
			ui.Text(c, "选择位置").FontSize(18).FontWeight(600)
			ui.Text(c, a.projectPickDir).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			if a.projectPickErr != "" {
				ui.Text(c, a.projectPickErr).FontSize(12).TextColor(t.Danger).MaxLines(3)
			}
			if a.projectPickLoading {
				ui.Text(c, "正在读取…").FontSize(13).TextColor(t.TextMuted)
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				if a.projectPickParent != "" {
					if mobile.ListRow(c, mobile.RowOptions{Title: "..", Subtitle: "上一级"}).Clicked() {
						a.loadProjectDirs(a.projectPickParent)
					}
				}
				for _, dir := range a.projectPickDirs {
					dir := dir
					if mobile.ListRow(c, mobile.RowOptions{Title: dir.Name, Subtitle: dir.Path}).Clicked() {
						a.loadProjectDirs(dir.Path)
					}
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				if primaryButton(c, "选择此目录").Clicked() {
					a.projectParent = a.projectPickDir
					a.projectPickOpen = false
				}
				if ghostButton(c, "取消").Clicked() {
					a.projectPickOpen = false
				}
			})
		})
	})
}

func (a *app) projectRel(path string) string {
	rel, err := filepath.Rel(a.projectDir, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return filepath.Base(a.projectDir)
	}
	return rel
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func statusSuffix(staged bool) string {
	if staged {
		return " · 已暂存"
	}
	return ""
}
