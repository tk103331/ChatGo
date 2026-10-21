// Package logapp is the Apps menu's log viewer: open log files in tabs,
// tail or full-load them, follow appended content, and filter by level.
package logapp

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是日志查看助手。你可以通过 open_log 打开日志文件（mode 为 full 完整加载或 tail 只读最后 N 行），通过 follow_log 开关跟踪文件新增内容，通过 read_log 读取当前日志行（可按级别过滤、限制行数），通过 reload_log 重新加载，通过 list_tabs 查看打开的日志标签。帮助用户排查问题、定位错误与异常。"

const (
	defaultTail = 500
	maxLines    = 200000
	followPoll  = 800 * time.Millisecond
)

// logFile is one open log.
type logFile struct {
	path     string
	lines    []mcode.LogLine
	state    mcode.LogViewerState
	follow   bool
	tailN    int
	tailText string
	lastSize int64
	loaded   bool
	err      string
	stopCh   chan struct{}
}

// Client is one log viewer bound to a Host.
type Client struct {
	host        Host
	mu          sync.Mutex
	tabs        appkit.Tabs
	files       map[string]*logFile
	openHandler func(path string)
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a log viewer driving the given host.
func New(h Host) *Client {
	return &Client{host: h, files: map[string]*logFile{}}
}

// SetOpenHandler registers a callback run when a file is opened into a new
// tab, so the shell can mirror it into its own tab bar.
func (cl *Client) SetOpenHandler(fn func(path string)) { cl.openHandler = fn }

// Title labels a tab with its file name.
func (cl *Client) Title(id string) string {
	if lf := cl.file(id); lf != nil && lf.path != "" {
		return filepath.Base(lf.path)
	}
	return "未命名"
}

// NewTab opens an empty log view.
func (cl *Client) NewTab() string { return cl.Open("") }

// Open opens a path in a tab, reusing an existing tab.
func (cl *Client) Open(path string) string {
	key := path
	if key == "" {
		key = "untitled-" + cl.host.NewID()
	}
	cl.mu.Lock()
	if _, ok := cl.files[key]; !ok {
		lf := &logFile{path: path, tailN: defaultTail, tailText: fmt.Sprintf("%d", defaultTail)}
		cl.files[key] = lf
		cl.tabs.Add(key)
		cl.mu.Unlock()
		if path != "" {
			cl.loadTailFile(lf, defaultTail)
		}
		return key
	}
	cl.tabs.Active = key
	cl.mu.Unlock()
	return key
}

// Close drops a tab and stops its follow goroutine.
func (cl *Client) Close(id string) {
	cl.mu.Lock()
	if lf := cl.files[id]; lf != nil {
		cl.stopFollowLocked(lf)
	}
	for i, key := range cl.tabs.Items {
		if key == id {
			cl.tabs.Close(i)
			break
		}
	}
	delete(cl.files, id)
	cl.mu.Unlock()
}

// Tabs exposes the tab list for an embedding tab bar.
func (cl *Client) Tabs() *appkit.Tabs { return &cl.tabs }

// View draws the active log: a toolbar above the LogViewer.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	lf := cl.file(cl.tabs.Active)
	if lf == nil {
		mgo.Column(c).Fill().Center().Children(func() {
			mgo.Text(c, "打开一个日志文件开始查看").FontSize(caption).TextColor(k.TextMuted)
		})
		return
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("folder"), Label: "打开"}).Clicked() {
				cl.openViaDialog()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "完整加载"}).Clicked() {
				cl.loadFullFile(lf)
			}
			mgo.Box(c).Width(1).Height(18).Shrink(0).Background(k.ControlBorder)
			minput.TextInput(c, &lf.tailText, minput.TextInputOptions{Placeholder: "行数"}).Element.Width(70).Shrink(0)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("list"), Label: "Tail"}).Clicked() {
				cl.loadTailFile(lf, parseTailN(lf.tailText))
			}
			mgo.Box(c).Width(1).Height(18).Shrink(0).Background(k.ControlBorder)
			followVariant := minput.Ghost
			if lf.follow {
				followVariant = minput.Secondary
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: followVariant, Icon: icons.Must("eye"), Label: "跟踪"}).Clicked() {
				cl.toggleFollow(lf)
			}
			mgo.Box(c).Grow(1)
			mgo.Text(c, fmt.Sprintf("%d 行", len(lf.lines))).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		})
		if lf.err != "" {
			mgo.Text(c, lf.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			mcode.LogViewer(c, lf.lines, &lf.state, mcode.LogViewerOptions{}).Element.Fill()
		})
		mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, lf.path).FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if lf.follow {
				mgo.Text(c, "跟踪中").FontSize(caption).TextColor(k.AccentText).Shrink(0)
			}
		})
	})
}

// openViaDialog picks a file with the native dialog and opens it in a tab.
func (cl *Client) openViaDialog() {
	go func() {
		picked := cl.host.OpenFileDialog(cl.host.Root())
		if picked == "" {
			return
		}
		rel := cl.relOf(picked)
		cl.host.Post(func() {
			cl.Open(rel)
			if cl.openHandler != nil {
				cl.openHandler(rel)
			}
			cl.host.Invalidate()
		})
	}()
}

// toggleFollow starts or stops the file-follow polling for one log.
func (cl *Client) toggleFollow(lf *logFile) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if lf.follow {
		cl.stopFollowLocked(lf)
	} else {
		cl.startFollowLocked(lf)
	}
}

// file returns the logFile for a tab id.
func (cl *Client) file(id string) *logFile {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.files[id]
}

// relOf converts an absolute path to a workspace-relative one when inside
// the root, keeping the absolute path otherwise.
func (cl *Client) relOf(abs string) string {
	if root := cl.host.Root(); root != "" {
		if r, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return abs
}

// abs joins a workspace-relative path to the host root, or keeps an
// absolute one.
func (cl *Client) abs(rel string) string {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Join(cl.host.Root(), rel)
}

// parseTailN reads a tail count from text, falling back to the default.
func parseTailN(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return defaultTail
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return defaultTail
	}
	if n > maxLines {
		return maxLines
	}
	return n
}
