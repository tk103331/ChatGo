package ui

import (
	"os"
	"strconv"

	term "github.com/egoist/mygo/plugins/terminal"
	mgo "github.com/egoist/mygo/ui"

	mcore "chatgo/internal/ui/uikit"
)

// termTab is one terminal tab, its shell and its last reported state.
type termTab struct {
	id     string
	dir    string
	term   *term.Terminal
	title  string
	err    string
	exited bool
}

// label is the tab's title, a fallback name while the shell is silent.
func (tb *termTab) label(index int) string {
	switch {
	case tb.term != nil && tb.title != "":
		return tb.title
	case tb.exited:
		return "已退出"
	default:
		return "Shell " + strconv.Itoa(index+1)
	}
}

// newTermTab builds a shell tab for dir without starting it, for callers that
// own where the tab lives.
func (cw *ChatWindow) newTermTab(dir string) *termTab {
	tb := &termTab{id: "term" + strconv.Itoa(cw.termSeq), dir: dir}
	cw.termSeq++
	return tb
}

// releaseTerm closes a shell a center tab owned.
func (cw *ChatWindow) releaseTerm(tb *termTab) {
	if tb != nil && tb.term != nil {
		_ = tb.term.Close()
	}
}

// startTerm builds one terminal; New may load a native library, so it runs off
// the main thread and hands the result back through postMain.
func (cw *ChatWindow) startTerm(tb *termTab) {
	dir := tb.dir
	if dir == "" {
		dir = cw.workspace
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	t, err := term.New(term.Options{
		Dir: dir,
		OnTitle: func(title string) {
			cw.postMain(func() { tb.title = title; cw.invalidate() })
		},
		OnExit: func(int) {
			cw.postMain(func() { tb.exited = true; cw.invalidate() })
		},
	})
	cw.postMain(func() {
		if err != nil {
			tb.err = err.Error()
			return
		}
		if !cw.centerHoldsTerm(tb) {
			_ = t.Close()
			return
		}
		tb.term = t
	})
}

// centerHoldsTerm reports whether a center tab still owns the shell.
func (cw *ChatWindow) centerHoldsTerm(tb *termTab) bool {
	for _, t := range cw.centerTabsOfKind(centerTerm) {
		if t.Term == tb {
			return true
		}
	}
	return false
}

// termBody draws one tab: its shell, a loading hint or an error.
func (cw *ChatWindow) termBody(c *mgo.Context, tb *termTab, caption, u float32) {
	mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		switch {
		case tb.err != "":
			mgo.Text(c, "终端不可用："+tb.err).Padding(u * 3).FontSize(caption).TextColor(mcore.Tokens(c).Danger)
		case tb.term == nil:
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(mcore.Tokens(c).TextMuted)
		default:
			term.View(c, tb.term).Fill()
		}
	})
}
