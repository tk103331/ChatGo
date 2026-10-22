package ui

import (
	"os"
	"strconv"

	term "github.com/egoist/mygo/plugins/terminal"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
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

// toggleTerminal shows or hides the bottom terminal, starting a shell tab once.
func (cw *ChatWindow) toggleTerminal() {
	cw.termOpen = !cw.termOpen
	if cw.termOpen && len(cw.terms) == 0 {
		cw.addTerm()
	}
}

// addTerm appends a shell tab and starts it, in the current workspace.
func (cw *ChatWindow) addTerm() {
	cw.addTermIn("")
}

// addTermIn is addTerm with an explicit starting directory.
func (cw *ChatWindow) addTermIn(dir string) {
	if !cw.termOpen {
		cw.termOpen = true
	}
	tb := &termTab{id: "term" + strconv.Itoa(cw.termSeq), dir: dir}
	cw.termSeq++
	cw.terms = append(cw.terms, tb)
	cw.termActiveID = tb.id
	go cw.startTerm(tb)
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
		if !cw.hasTerm(tb) {
			_ = t.Close()
			return
		}
		tb.term = t
	})
}

// hasTerm reports whether the tab is still open.
func (cw *ChatWindow) hasTerm(tb *termTab) bool {
	for _, t := range cw.terms {
		if t == tb {
			return true
		}
	}
	return false
}

// termByID returns the open tab with the given ID, if any.
func (cw *ChatWindow) termByID(id string) *termTab {
	for _, tb := range cw.terms {
		if tb.id == id {
			return tb
		}
	}
	return nil
}

// closeTerm closes a tab by its ID and picks the neighbour to show.
func (cw *ChatWindow) closeTerm(id string) {
	for i, tb := range cw.terms {
		if tb.id != id {
			continue
		}
		if tb.term != nil {
			_ = tb.term.Close()
		}
		cw.terms = append(cw.terms[:i], cw.terms[i+1:]...)
		switch {
		case len(cw.terms) == 0:
			cw.termOpen = false
		case !cw.hasTermID(cw.termActiveID):
			cw.termActiveID = cw.terms[min(i, len(cw.terms)-1)].id
		}
		return
	}
}

func (cw *ChatWindow) hasTermID(id string) bool {
	for _, tb := range cw.terms {
		if tb.id == id {
			return true
		}
	}
	return false
}

// syncTermTabs mirrors the terminal tabs into the TerminalTabs view, adding
// any tab opened since the last frame.
func (cw *ChatWindow) syncTermTabs() {
	for i := range cw.terms {
		tb := cw.terms[i]
		j := termTabsIndex(cw.termTabs, tb.id)
		if j < 0 {
			cw.termTabs = append(cw.termTabs, mcode.TerminalTab{ID: tb.id, Title: tb.label(i)})
			continue
		}
		cw.termTabs[j].Title = tb.label(i)
	}
}

func termTabsIndex(tabs []mcode.TerminalTab, id string) int {
	for i, t := range tabs {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// terminalPanel draws the bottom terminal with its tab bar and shell view.
func (cw *ChatWindow) terminalPanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cw.syncTermTabs()
	mgo.Row(c).Fill().Background(k.Surface).Gap(u).Children(func() {
		res := mcode.TerminalTabs(c, &cw.termTabs, &cw.termActiveID, mcode.TerminalTabsOptions{
			Actions: func() {
				if mcore.IconAction(c, icons.Must("plus"), "新建终端").Clicked() {
					cw.addTerm()
				}
				if mcore.IconAction(c, icons.Must("x"), "关闭终端").Clicked() {
					cw.termOpen = false
				}
			},
		}, func(id string) {
			if tb := cw.termByID(id); tb != nil {
				cw.termBody(c, tb, caption, u)
			}
		})
		res.Element.Grow(1).FillHeight()
		if res.Closed != "" {
			cw.closeTerm(res.Closed)
		}
		if res.Renamed != "" {
			if tb := cw.termByID(res.Renamed); tb != nil {
				if j := termTabsIndex(cw.termTabs, res.Renamed); j >= 0 {
					tb.title = cw.termTabs[j].Title
				}
			}
		}
	})
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
