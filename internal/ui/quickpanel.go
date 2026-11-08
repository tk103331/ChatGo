package ui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgo/internal/capture"
	"chatgo/internal/config"

	"chatgo/internal/ui/icons"
	mcore "chatgo/internal/ui/uikit"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"
)

// The quick panel's two shapes. The bar is a small, inactive strip next to a
// selection; the compose panel takes the keyboard and runs a task.
const (
	quickBarWidth      = 380
	quickBarHeight     = 52
	quickPanelWidth    = 480
	quickPanelHeight   = 278
	quickRunningHeight = 300
)

// quickBarLifetime hides the bar after the user ignores it.
const quickBarLifetime = 8 * time.Second

type quickPanelMode int

const (
	quickPanelHidden quickPanelMode = iota
	quickPanelBar
	quickPanelCompose
	quickPanelRunning
)

// quickAgentOption is one agent chip in the panel.
type quickAgentOption struct {
	Ref  string
	Name string
}

// quickPanel is the system-wide quick task window: a MyGo panel floating over
// every app, which either straddles a selection or takes a directory and a
// prompt.
type quickPanel struct {
	cw  *ChatWindow
	win *mygo.Window

	mode quickPanelMode
	// seq marks the newest capture attempt, so a slow read cannot replace a
	// panel the user already reopened.
	seq uint64

	selection    capture.Selection
	hasSelection bool

	draft     string
	agent     string
	model     string
	workspace string
	worktree  bool

	focusInput   bool
	suppressBlur bool
	// barHovered pauses the bar's auto-hide while the pointer is on it.
	barHovered bool

	// lastSel* skip a second bar for the same selection in quick succession.
	lastSelText   string
	lastSelBundle string
	lastSelAt     time.Time

	task *quickTask
	// dismiss removes the mouse-down monitor that closes the bar when the
	// user moves on.
	dismiss func()
}

func newQuickPanel(cw *ChatWindow) *quickPanel {
	return &quickPanel{cw: cw}
}

// create makes the window. It must run on the main thread after the app is
// ready, and does nothing when the window already exists.
func (qp *quickPanel) create() {
	if qp.win != nil || qp.cw == nil {
		return
	}
	qp.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         "ChatGo 快捷任务",
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		DisableResize: true,
		SkipTaskbar:   true,
		Width:         quickPanelWidth,
		Height:        quickPanelHeight,
		Content:       mgo.View(qp.view),
	})
	qp.win.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		qp.hide()
	})
	qp.win.OnBlur(func() {
		if qp.suppressBlur {
			return
		}
		switch qp.mode {
		case quickPanelCompose, quickPanelRunning:
			qp.hide()
		}
	})
}

// destroy closes the window for good, when the panel is turned off.
func (qp *quickPanel) destroy() {
	qp.stopDismiss()
	if qp.win == nil {
		return
	}
	qp.win.Destroy()
	qp.win = nil
	qp.mode = quickPanelHidden
	qp.task = nil
}

// openQuickPanel is the global shortcut's action: show the panel, or hide it
// when it is already up.
func (cw *ChatWindow) openQuickPanel() {
	if !cw.cfg.QuickPanel.IsEnabled() {
		return
	}
	cw.postMain(func() {
		if cw.quick == nil {
			cw.quick = newQuickPanel(cw)
			cw.quick.create()
		}
		if cw.quick == nil {
			return
		}
		cw.quick.open()
	})
}

// applyQuickPanelSetting applies an enable/auto-popup change from settings.
func (cw *ChatWindow) applyQuickPanelSetting() {
	_ = config.SaveConfig(cw.cfg)
	if cw.cfg.QuickPanel.IsEnabled() {
		if cw.quick == nil {
			cw.quick = newQuickPanel(cw)
			cw.quick.create()
		}
		cw.startQuickWatch()
	} else {
		cw.stopQuickWatch()
		if cw.quick != nil {
			cw.quick.destroy()
			cw.quick = nil
		}
	}
	cw.applyPanelShortcut()
	cw.updateTrayMenu()
	cw.invalidate()
}

// startQuickWatch installs the selection monitor when AutoPopup is on.
func (cw *ChatWindow) startQuickWatch() {
	cw.stopQuickWatch()
	if cw.grabber == nil || !cw.cfg.QuickPanel.AutoPopup {
		return
	}
	cw.quickWatch = cw.grabber.Watch(func(sel capture.Selection) {
		cw.postMain(func() {
			cfg := cw.cfg.QuickPanel
			if !cfg.AutoPopup || cw.quick == nil {
				return
			}
			if !capture.AcceptText(sel.Text, cfg.MinSelection()) {
				return
			}
			if containsStr(cfg.ExcludeApps, sel.BundleID) {
				return
			}
			cw.quick.onSelection(sel)
		})
	})
}

// stopQuickWatch removes the monitor, if one is installed.
func (cw *ChatWindow) stopQuickWatch() {
	if cw.quickWatch != nil {
		cw.quickWatch()
		cw.quickWatch = nil
	}
}

// open shows the panel, or hides it when it is up; a task still running is
// shown again rather than started over.
func (qp *quickPanel) open() {
	if qp.win == nil {
		qp.create()
	}
	if qp.win == nil {
		return
	}
	if qp.win.IsVisible() {
		qp.hide()
		return
	}
	if qp.task != nil && qp.task.running(qp.cw) {
		qp.mode = quickPanelRunning
		qp.resizeAndPlace(quickPanelWidth, quickRunningHeight)
		qp.win.Show()
		qp.win.Invalidate()
		return
	}
	qp.reset()
	qp.prepare()
}

// openAt shows the panel with a directory and a prompt already filled, as a
// command line or a chatgo:// URL asks.
func (qp *quickPanel) openAt(dir, prompt string) {
	if qp.win == nil {
		qp.create()
	}
	if qp.win == nil {
		return
	}
	qp.reset()
	if dir != "" {
		qp.workspace = dir
	}
	qp.draft = prompt
	qp.seq++
	qp.showCompose()
}

// reset clears the panel for a new task.
func (qp *quickPanel) reset() {
	cfg := qp.cw.cfg.QuickPanel
	qp.selection = capture.Selection{}
	qp.hasSelection = false
	qp.draft = ""
	qp.task = nil
	qp.agent = cfg.DefaultAgent
	qp.model = qp.defaultModelRef(qp.agent)
	qp.worktree = cfg.DefaultWorktree
	qp.workspace = qp.cw.workspace
	if qp.workspace == "" {
		qp.workspace = qp.cw.fileRoot()
	}
	qp.focusInput = true
}

// prepare reads the selection, the directory and the clipboard off the main
// thread, then shows the compose panel with what it found.
func (qp *quickPanel) prepare() {
	grabber := qp.cw.grabber
	qp.seq++
	seq := qp.seq
	if grabber == nil {
		qp.showCompose()
		return
	}
	go func() {
		sel, selErr := grabber.Selection()
		// Windows has no readable selection API here; the optional copy makes
		// one by asking the app to copy its selection.
		if selErr != nil && qp.cw.cfg.QuickPanel.AutoCopySelection {
			if text, ok := grabber.CopySelection(); ok {
				sel = capture.Selection{Text: text, Source: capture.SourceClipboard}
				selErr = nil
			}
		}
		// A folder on the clipboard is not the context of a selection made
		// somewhere else, so only read a directory when the selection came
		// from Finder or there is none.
		var dir string
		var dirErr error = capture.ErrNoDirectory
		if selErr != nil || sel.BundleID == "com.apple.finder" {
			dir, dirErr = grabber.Directory()
		}
		clip := ""
		if qp.cw.cfg.QuickPanel.IsClipboardFallback() {
			clip = grabber.ClipboardText()
		}
		qp.cw.postMain(func() {
			if seq != qp.seq || qp.mode != quickPanelHidden {
				return
			}
			qp.applyCapture(sel, selErr, dir, dirErr, clip)
		})
	}()
}

// applyCapture fills the panel from what prepare read.
func (qp *quickPanel) applyCapture(sel capture.Selection, selErr error, dir string, dirErr error, clip string) {
	min := qp.cw.cfg.QuickPanel.MinSelection()
	if selErr == nil && capture.AcceptText(sel.Text, min) {
		qp.selection = sel
		qp.hasSelection = true
	}
	if !qp.hasSelection && qp.cw.cfg.QuickPanel.IsClipboardFallback() && capture.AcceptText(clip, min) {
		qp.selection = capture.Selection{Text: clip, Source: capture.SourceClipboard}
		qp.hasSelection = true
	}
	if dirErr == nil && capture.ExistingDir(dir) {
		qp.workspace = dir
		// A file name Finder happens to expose as selected text is not the
		// context the user means; the folder they picked is.
		if qp.hasSelection && qp.selection.BundleID == "com.apple.finder" {
			qp.hasSelection = false
			qp.selection = capture.Selection{}
		}
	}
	qp.showCompose()
}

// onSelection is the AutoPopup path: show the bar next to the selection.
func (qp *quickPanel) onSelection(sel capture.Selection) {
	if qp.win == nil {
		return
	}
	if sel.PID == os.Getpid() {
		return
	}
	if qp.win.IsVisible() && qp.mode != quickPanelBar {
		return
	}
	// Double-clicking the same word twice should not flash two bars.
	now := time.Now()
	if qp.lastSelText == sel.Text && qp.lastSelBundle == sel.BundleID &&
		now.Sub(qp.lastSelAt) < 2*time.Second {
		return
	}
	qp.lastSelText, qp.lastSelBundle, qp.lastSelAt = sel.Text, sel.BundleID, now
	// An element that reports no bounds still gets a bar at the pointer.
	if sel.Rect.Width <= 0 && sel.Rect.Height <= 0 {
		p := mygo.Screen.CursorScreenPoint()
		sel.Rect = capture.Rect{X: p.X, Y: p.Y, Width: 1, Height: 1}
	}
	qp.reset()
	qp.selection = sel
	qp.hasSelection = true
	qp.showBar()
}

func (qp *quickPanel) showBar() {
	if qp.win == nil {
		return
	}
	qp.mode = quickPanelBar
	qp.focusInput = false
	qp.resizeAndPlace(quickBarWidth, quickBarHeight)
	qp.win.ShowInactive()
	qp.win.Invalidate()
	qp.watchDismiss()
	qp.scheduleBarHide()
}

// scheduleBarHide hides the bar after it is ignored, but waits again while the
// pointer rests on it.
func (qp *quickPanel) scheduleBarHide() {
	time.AfterFunc(quickBarLifetime, func() {
		qp.cw.postMain(func() {
			if qp.mode != quickPanelBar {
				return
			}
			if qp.barHovered {
				qp.scheduleBarHide()
				return
			}
			qp.hide()
		})
	})
}

func (qp *quickPanel) showCompose() {
	if qp.win == nil {
		return
	}
	qp.stopDismiss()
	qp.mode = quickPanelCompose
	qp.focusInput = true
	qp.resizeAndPlace(quickPanelWidth, quickPanelHeight)
	qp.win.Show()
	qp.win.Invalidate()
}

func (qp *quickPanel) showRunning() {
	if qp.win == nil {
		return
	}
	qp.stopDismiss()
	qp.mode = quickPanelRunning
	qp.resizeAndPlace(quickPanelWidth, quickRunningHeight)
	qp.win.Show()
	qp.win.Invalidate()
}

func (qp *quickPanel) hide() {
	qp.stopDismiss()
	// A capture still in flight must not reopen the panel after it hides.
	qp.seq++
	if qp.win != nil {
		qp.win.Hide()
	}
	// A finished task is read and dismissed; one still running is kept so
	// the shortcut shows it again instead of starting over.
	if qp.task == nil || !qp.task.running(qp.cw) {
		qp.task = nil
	}
	qp.mode = quickPanelHidden
	qp.focusInput = false
	qp.suppressBlur = false
}

// watchDismiss closes the inactive bar when the user presses the mouse
// anywhere outside this application. Events in this app never reach the
// monitor, so clicking the bar itself still works.
func (qp *quickPanel) watchDismiss() {
	if qp.dismiss != nil || qp.cw.grabber == nil {
		return
	}
	qp.dismiss = qp.cw.grabber.WatchDismiss(func() {
		qp.cw.postMain(func() {
			if qp.mode == quickPanelBar {
				qp.hide()
			}
		})
	})
}

func (qp *quickPanel) stopDismiss() {
	if qp.dismiss != nil {
		stop := qp.dismiss
		qp.dismiss = nil
		stop()
	}
}

func (qp *quickPanel) invalidate() {
	if qp.win != nil {
		qp.win.Invalidate()
	}
}

// resizeAndPlace resizes the window and puts it next to the selection (or the
// pointer, when there is none).
func (qp *quickPanel) resizeAndPlace(width, height int) {
	if qp.win == nil {
		return
	}
	qp.win.SetContentSize(width, height)
	work := mygo.Screen.PrimaryDisplay().WorkArea
	anchor := capture.Rect{}
	if qp.hasSelection {
		anchor = qp.selection.Rect
	}
	if anchor.Width > 0 || anchor.Height > 0 {
		d := mygo.Screen.DisplayMatching(mygo.Rectangle{
			X: anchor.X, Y: anchor.Y, Width: max(anchor.Width, 1), Height: max(anchor.Height, 1),
		})
		if d.WorkArea.Width > 0 {
			work = d.WorkArea
		}
	} else {
		d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
		if d.WorkArea.Width > 0 {
			work = d.WorkArea
		}
	}
	x, y := capture.PlaceNear(anchor, capture.Size{Width: width, Height: height}, capture.Rect{
		X: work.X, Y: work.Y, Width: work.Width, Height: work.Height,
	})
	qp.win.SetPosition(x, y)
}

// agents lists the chips in display order: favorites first when configured,
// otherwise the built-in chat and every enabled agent.
func (qp *quickPanel) agents() []quickAgentOption {
	out := allQuickAgents(qp.cw.cfg)
	favorites := qp.cw.cfg.QuickPanel.Favorites
	if len(favorites) == 0 {
		return out
	}
	byRef := make(map[string]quickAgentOption, len(out))
	for _, o := range out {
		byRef[o.Ref] = o
	}
	var ordered []quickAgentOption
	seen := map[string]bool{}
	push := func(ref string) {
		if seen[ref] {
			return
		}
		if o, ok := byRef[ref]; ok {
			seen[ref] = true
			ordered = append(ordered, o)
		}
	}
	push(qp.cw.cfg.QuickPanel.DefaultAgent)
	for _, ref := range favorites {
		push(ref)
	}
	if len(ordered) == 0 {
		return out
	}
	return ordered
}

// allQuickAgents lists the agents a panel can run: the built-in chat, every
// custom agent and every enabled ACP agent.
func allQuickAgents(cfg *config.Config) []quickAgentOption {
	out := []quickAgentOption{{Ref: "", Name: chatAgentName}}
	for _, a := range cfg.Agents {
		if strings.TrimSpace(a.Name) == "" {
			continue
		}
		out = append(out, quickAgentOption{Ref: "agent:" + a.Name, Name: a.Name})
	}
	for _, a := range cfg.ACPAgents {
		if a.Disabled || strings.TrimSpace(a.Name) == "" {
			continue
		}
		out = append(out, quickAgentOption{Ref: "acp:" + a.Name, Name: a.Name})
	}
	return out
}

// visibleAgents caps the chips; limit 0 means all of them.
func (qp *quickPanel) visibleAgents(limit int) []quickAgentOption {
	all := qp.agents()
	if limit > 0 && len(all) > limit {
		return all[:limit]
	}
	return all
}

// agentChipLeading draws an agent's mark for a chip, in the panel or in
// settings.
func (cw *ChatWindow) agentChipLeading(c *mgo.Context, a quickAgentOption) func() {
	switch {
	case strings.HasPrefix(a.Ref, "acp:"):
		name := strings.TrimPrefix(a.Ref, "acp:")
		return func() { cw.acpLeading(c, name, 16) }
	case strings.HasPrefix(a.Ref, "agent:"):
		name := strings.TrimPrefix(a.Ref, "agent:")
		return func() { cw.agentMarkByName(c, name, 16) }
	default:
		return func() {
			mgo.Icon(c, icons.Must(chatAgentIcon)).Size(16, 16).TextColor(mcore.Tokens(c).Accent)
		}
	}
}

// modelRefsForAgent lists the models an agent may run; ACP agents choose
// their own, so they get none.
func (cw *ChatWindow) modelRefsForAgent(agent string) []string {
	if strings.HasPrefix(agent, "acp:") {
		return nil
	}
	if name, ok := strings.CutPrefix(agent, "agent:"); ok {
		if a, found := cw.agentByName(name); found && len(a.Models) > 0 {
			return append([]string(nil), a.Models...)
		}
	}
	return cw.allModelRefs()
}

// defaultModelRef is the model the panel shows before the user picks one.
func (qp *quickPanel) defaultModelRef(agent string) string {
	refs := qp.cw.modelRefsForAgent(agent)
	if len(refs) == 0 {
		return ""
	}
	if agent == "" {
		if cur := qp.cw.currentModelRef(); containsStr(refs, cur) {
			return cur
		}
	}
	return refs[0]
}

// setAgent switches the agent and follows its default model.
func (qp *quickPanel) setAgent(ref string) {
	if qp.agent == ref {
		return
	}
	qp.agent = ref
	qp.model = qp.defaultModelRef(ref)
}

// effectiveModel is the reference the selector shows.
func (qp *quickPanel) effectiveModel() string {
	refs := qp.cw.modelRefsForAgent(qp.agent)
	if qp.model != "" && containsStr(refs, qp.model) {
		return qp.model
	}
	return qp.defaultModelRef(qp.agent)
}

// submit turns the draft and the captured context into a task.
func (qp *quickPanel) submit() {
	text := strings.TrimSpace(qp.draft)
	if text == "" {
		return
	}
	if qp.workspace != "" && !capture.ExistingDir(qp.workspace) {
		qp.cw.toast("工作目录不存在："+qp.workspace, mcore.SeverityError)
		return
	}
	prompt := text
	if qp.hasSelection {
		if q := capture.Quote(qp.selection.Text, 6000); q != "" {
			prompt = text + "\n\n" + q
		}
	}
	task := qp.cw.quickStartTask(prompt, qp.agent, qp.workspace, qp.model, qp.worktree)
	if task == nil {
		return
	}
	qp.task = task
	qp.draft = ""
	qp.showRunning()
}

// newTask starts a fresh capture without waiting for the running reply, which
// keeps streaming into its conversation.
func (qp *quickPanel) newTask() {
	if qp.win == nil {
		return
	}
	qp.stopDismiss()
	qp.win.Hide()
	qp.seq++
	qp.reset()
	qp.mode = quickPanelHidden
	qp.prepare()
}

// pickWorkspace lets the user choose a directory from a native dialog. The
// dialog is app-modal rather than parented to the main window, which may be
// hidden while the panel runs.
func (qp *quickPanel) pickWorkspace() {
	qp.suppressBlur = true
	time.AfterFunc(2*time.Minute, func() { qp.cw.postMain(func() { qp.suppressBlur = false }) })
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:     "选择工作目录",
			Directory: true,
		})
		qp.cw.postMain(func() {
			qp.suppressBlur = false
			if err != nil || len(paths) == 0 {
				return
			}
			qp.workspace = paths[0]
			qp.showCompose()
		})
	}()
}

// openInMain shows the task's conversation in the main window. It hands the
// live conversation over rather than loading it from disk, so a reply still
// streaming keeps filling the same message.
func (qp *quickPanel) openInMain() {
	if qp.task == nil || qp.task.Conv == nil {
		return
	}
	conv := qp.task.Conv
	qp.hide()
	qp.cw.postMain(func() {
		qp.cw.openConversation(conv)
		qp.cw.section = sectionChat
		qp.cw.showMainWindow()
	})
}

// placeholder is the composer's hint, which follows what the panel captured.
func (qp *quickPanel) placeholder() string {
	switch {
	case qp.hasSelection:
		return "让 Agent 对选中的内容做什么？Enter 运行"
	case qp.workspace != "":
		return "在这份代码里做什么？Enter 运行"
	default:
		return "让 Agent 做什么？Enter 运行"
	}
}

// HandleLaunchArgs opens the panel for the directory or prompt a command line
// named, for example `chatgo .` or `chatgo --dir ~/repo --prompt "修一下测试"`.
func (cw *ChatWindow) HandleLaunchArgs(args []string, wd string) {
	cw.postMain(func() { cw.handleLaunchArgs(args, wd) })
}

func (cw *ChatWindow) handleLaunchArgs(args []string, wd string) {
	dir, prompt := "", ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--dir" || a == "-C":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		case a == "--prompt":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--prompt="):
			prompt = strings.TrimPrefix(a, "--prompt=")
		case strings.HasPrefix(a, "-psn"):
			// LaunchServices' process serial number.
		case strings.HasPrefix(a, "-"):
			// Other framework flags.
		default:
			if dir == "" {
				dir = existingDir(a, wd)
			}
		}
	}
	if dir == "" && prompt == "" {
		cw.openNormal()
		return
	}
	cw.openQuickAt(dir, prompt)
}

// openQuickAt shows the panel with pre-filled context, or queues the request
// when the window does not exist yet.
func (cw *ChatWindow) openQuickAt(dir, prompt string) {
	if !cw.hasUI() {
		cw.pendingQuick = &quickLaunch{Dir: dir, Prompt: prompt}
		return
	}
	if cw.quick == nil {
		cw.quick = newQuickPanel(cw)
		cw.quick.create()
	}
	if cw.quick != nil {
		cw.quick.openAt(dir, prompt)
	}
}

// quickLaunch is a panel request that arrived before the window existed.
type quickLaunch struct {
	Dir    string
	Prompt string
}

// HandleQuickURL opens the panel for chatgo://quick?dir=…&q=… links.
func (cw *ChatWindow) HandleQuickURL(raw string) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "chatgo") {
		return
	}
	q := u.Query()
	dir := q.Get("dir")
	if dir == "" {
		dir = q.Get("path")
	}
	prompt := q.Get("q")
	if prompt == "" {
		prompt = q.Get("prompt")
	}
	// A URL that started the app arrives before its window exists.
	if !cw.hasUI() {
		cw.pendingQuick = &quickLaunch{Dir: dir, Prompt: prompt}
		return
	}
	cw.postMain(func() { cw.openQuickAt(dir, prompt) })
}

// existingDir resolves a command line path against the process's directory.
func existingDir(path, wd string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(wd, path)
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return filepath.Clean(path)
	}
	return ""
}
