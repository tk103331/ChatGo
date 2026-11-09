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
//
// quickPanelHeight fits the compose panel's context header, agent chips,
// composer, and the project and model row.
const (
	quickBarWidth      = 380
	quickBarHeight     = 52
	quickPanelWidth    = 480
	quickPanelHeight   = 280
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
	// selectionInDraft records that the draft already carries the quoted
	// selection, so submit does not attach the same text twice.
	selectionInDraft bool

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
	// A destroyed panel cannot close itself later, so bring the main window
	// back now instead of leaving it transparent.
	qp.cw.restoreMainAfterQuickPanel()
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
		if qp.mode == quickPanelBar {
			// The bar is only a preview; the shortcut opens the input panel
			// with the selection already captured, rather than hiding it and
			// making the user press again.
			qp.showCompose()
			return
		}
		qp.hide()
		return
	}
	if qp.task != nil && qp.task.running(qp.cw) {
		qp.cw.hideMainForQuickPanel()
		qp.mode = quickPanelRunning
		qp.resizeAndPlace(quickPanelWidth, quickRunningHeight)
		qp.showWindow(false)
		qp.win.Invalidate()
		return
	}
	qp.reset()
	qp.prepare()
}

// openQuickSelection shows the panel with text a system integration captured,
// such as the "向 ChatGo 提问" Service, quoted in the composer. It queues the
// request when the window does not exist yet, so a service that launched the
// app still opens the panel.
func (cw *ChatWindow) openQuickSelection(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !cw.hasUI() {
		cw.pendingQuick = &quickLaunch{Text: text}
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
		cw.quick.openSelection(capture.Selection{Text: text, Source: capture.SourceService})
	})
}

// openSelection shows the compose panel with sel already captured, as the
// Services menu integration asks.
func (qp *quickPanel) openSelection(sel capture.Selection) {
	if qp.win == nil {
		qp.create()
	}
	if qp.win == nil {
		return
	}
	qp.showSelection(sel)
	qp.showCompose()
}

// showSelection fills the panel with a captured selection, leaving the window
// alone so the state can be checked without one.
func (qp *quickPanel) showSelection(sel capture.Selection) {
	qp.reset()
	qp.selection = sel
	qp.hasSelection = true
	qp.seq++
	qp.prefillSelection()
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
	qp.selectionInDraft = false
	qp.draft = ""
	qp.task = nil
	qp.agent = cfg.DefaultAgent
	qp.model = qp.keepModel(qp.agent)
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
	qp.cw.hideMainForQuickPanel()
	qp.mode = quickPanelBar
	qp.focusInput = false
	qp.resizeAndPlace(quickBarWidth, quickBarHeight)
	qp.showWindow(true)
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
	qp.cw.hideMainForQuickPanel()
	qp.prefillSelection()
	qp.stopDismiss()
	qp.mode = quickPanelCompose
	qp.focusInput = true
	qp.resizeAndPlace(quickPanelWidth, quickPanelHeight)
	qp.showWindow(false)
	qp.win.Invalidate()
}

func (qp *quickPanel) showRunning() {
	if qp.win == nil {
		return
	}
	qp.cw.hideMainForQuickPanel()
	qp.stopDismiss()
	qp.mode = quickPanelRunning
	qp.resizeAndPlace(quickPanelWidth, quickRunningHeight)
	qp.showWindow(false)
	qp.win.Invalidate()
}

// showWindow reveals the panel. Hiding is done by clearing the window rather
// than by ordering it out alone, so showing it again must restore what the
// hide cleared.
func (qp *quickPanel) showWindow(inactive bool) {
	if qp.win == nil {
		return
	}
	qp.win.SetOpacity(1)
	qp.win.SetIgnoreMouseEvents(false)
	if inactive {
		qp.win.ShowInactive()
		return
	}
	qp.win.Show()
}

// hideWindow takes the panel off the screen and leaves its state hidden.
func (qp *quickPanel) hideWindow() {
	if qp.win == nil {
		return
	}
	qp.win.SetIgnoreMouseEvents(true)
	qp.win.SetOpacity(0)
	qp.win.Hide()
}

func (qp *quickPanel) hide() {
	qp.stopDismiss()
	// A capture still in flight must not reopen the panel after it hides.
	qp.seq++
	qp.hideWindow()
	qp.cw.restoreMainAfterQuickPanel()
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

// hideMainForQuickPanel keeps the main window out of the way while the panel
// is on screen. The panel is used over whichever app the user is in, so
// showing it must never bring the full interface forward with it.
//
// The window is cleared rather than ordered out: mygo's Hide leaves the window
// on screen on recent macOS, so hiding it visually and keeping it out of the
// mouse's way is what actually works.
func (cw *ChatWindow) hideMainForQuickPanel() {
	cw.winMu.Lock()
	w := cw.window
	already := cw.mainDimmed
	cw.mainDimmed = true
	cw.winMu.Unlock()
	if w == nil || w.IsDestroyed() || already || !w.IsVisible() {
		return
	}
	w.SetIgnoreMouseEvents(true)
	w.SetOpacity(0)
}

// restoreMainAfterQuickPanel brings back a main window the panel cleared.
func (cw *ChatWindow) restoreMainAfterQuickPanel() {
	cw.winMu.Lock()
	w := cw.window
	dimmed := cw.mainDimmed
	cw.mainDimmed = false
	cw.winMu.Unlock()
	if !dimmed || w == nil || w.IsDestroyed() {
		return
	}
	w.SetIgnoreMouseEvents(false)
	w.SetOpacity(1)
}

// quickPanelShowing reports whether the panel is up, so activating the app for
// the panel does not reopen the main window behind it.
func (cw *ChatWindow) quickPanelShowing() bool {
	return cw.quick != nil && cw.quick.mode != quickPanelHidden
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

// agents lists the panel's chips: the configured favorites in order, with the
// default agent first. Without favorites every agent is shown.
func (qp *quickPanel) agents() []quickAgentOption {
	all := allQuickAgents(qp.cw.cfg)
	favorites := qp.cw.cfg.QuickPanel.Favorites
	if len(favorites) == 0 {
		return all
	}
	byRef := make(map[string]quickAgentOption, len(all))
	for _, a := range all {
		byRef[a.Ref] = a
	}
	var out []quickAgentOption
	seen := map[string]bool{}
	push := func(ref string) {
		if seen[ref] {
			return
		}
		if a, ok := byRef[ref]; ok {
			seen[ref] = true
			out = append(out, a)
		}
	}
	// The default agent always has a chip, so a task never starts with an
	// agent the panel cannot show.
	push(qp.cw.cfg.QuickPanel.DefaultAgent)
	for _, ref := range favorites {
		push(ref)
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// allQuickAgents lists the built-in chat, every custom agent and every
// enabled ACP agent.
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

// agentChipLeading draws an agent's mark for a chip.
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

// defaultModelRef is the model a new task runs with: the current one for the
// built-in chat, otherwise the agent's first model.
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

// keepModel keeps a model the user picked while the agent still allows it,
// so starting another task does not silently go back to the default.
func (qp *quickPanel) keepModel(agent string) string {
	if qp.model != "" && containsStr(qp.cw.modelRefsForAgent(agent), qp.model) {
		return qp.model
	}
	return qp.defaultModelRef(agent)
}

// setModel switches the model the next task runs with.
func (qp *quickPanel) setModel(ref string) {
	if ref == "" || ref == qp.model {
		return
	}
	qp.model = ref
}

// setAgent switches the agent and follows its default model.
func (qp *quickPanel) setAgent(ref string) {
	if qp.agent == ref {
		return
	}
	qp.agent = ref
	qp.model = qp.defaultModelRef(ref)
}

// prefillSelection puts the captured selection into an empty draft as a
// quote block, so the user sees the context and can type the instruction
// around it. The bar and the shortcut both come through here.
func (qp *quickPanel) prefillSelection() {
	if !qp.hasSelection || strings.TrimSpace(qp.draft) != "" {
		return
	}
	if q := capture.Quote(qp.selection.Text, 6000); q != "" {
		qp.draft = q
		qp.selectionInDraft = true
	}
}

// prompt combines the draft with the selection, unless the draft already
// carries the quoted selection.
func (qp *quickPanel) prompt() string {
	text := strings.TrimSpace(qp.draft)
	if text == "" {
		return ""
	}
	if qp.hasSelection && !qp.selectionInDraft {
		if q := capture.Quote(qp.selection.Text, 6000); q != "" {
			return text + "\n\n" + q
		}
	}
	return text
}

// submit turns the draft and the captured context into a task.
func (qp *quickPanel) submit() {
	prompt := qp.prompt()
	if prompt == "" {
		return
	}
	if qp.workspace != "" && !capture.ExistingDir(qp.workspace) {
		qp.cw.toast("工作目录不存在："+qp.workspace, mcore.SeverityError)
		return
	}
	task := qp.cw.quickStartTask(prompt, qp.agent, qp.workspace, qp.model, false)
	if task == nil {
		return
	}
	qp.task = task
	qp.draft = ""
	qp.selectionInDraft = false
	qp.showRunning()
}

// newTask starts a fresh capture without waiting for the running reply, which
// keeps streaming into its conversation.
func (qp *quickPanel) newTask() {
	if qp.win == nil {
		return
	}
	qp.stopDismiss()
	qp.hideWindow()
	qp.seq++
	qp.reset()
	qp.mode = quickPanelHidden
	qp.prepare()
}

// pickWorkspace lets the user choose a project directory from a native
// dialog. The dialog is app-modal rather than parented to the main window,
// which may be hidden while the panel runs.
func (qp *quickPanel) pickWorkspace() {
	qp.suppressBlur = true
	time.AfterFunc(2*time.Minute, func() { qp.cw.postMain(func() { qp.suppressBlur = false }) })
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:     "选择项目",
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

// enterMain closes the panel and brings up the full interface, so a task can
// be continued or a new conversation started there.
func (qp *quickPanel) enterMain() {
	qp.hide()
	qp.cw.postMain(func() { qp.cw.openNormal() })
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
		// A service or URL launch can arrive with no arguments of its own;
		// the panel it queued is the whole interface it wants.
		if cw.pendingQuick == nil && !cw.quickPanelShowing() {
			cw.openNormal()
		}
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
	// Text is a selection captured by a service, quoted when the panel opens.
	Text string
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
