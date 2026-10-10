package ui

import (
	"os"
	"strconv"

	term "github.com/egoist/mygo/plugins/terminal"
	mgo "github.com/egoist/mygo/ui"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// termTab is one terminal tab, its shell and its last reported state.
type termTab struct {
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
	if !cw.termOpen {
		cw.termOpen = true
	}
	tb := &termTab{}
	cw.terms = append(cw.terms, tb)
	cw.termActive = len(cw.terms) - 1
	go cw.startTerm(tb)
}

// startTerm builds one terminal; New may load a native library, so it runs off
// the main thread and hands the result back through postMain.
func (cw *ChatWindow) startTerm(tb *termTab) {
	dir := cw.workspace
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

// closeTerm closes a tab and picks the neighbour to show.
func (cw *ChatWindow) closeTerm(i int) {
	if i < 0 || i >= len(cw.terms) {
		return
	}
	if tb := cw.terms[i]; tb.term != nil {
		_ = tb.term.Close()
	}
	cw.terms = append(cw.terms[:i], cw.terms[i+1:]...)
	switch {
	case len(cw.terms) == 0:
		cw.termOpen = false
	case cw.termActive > i:
		cw.termActive--
	case cw.termActive >= len(cw.terms):
		cw.termActive = len(cw.terms) - 1
	}
}

// terminalPanel draws the bottom terminal with its tab bar and shell view.
func (cw *ChatWindow) terminalPanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().Background(k.Surface).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			mgo.Icon(c, icons.Must("terminal")).Size(14, 14).TextColor(k.TextMuted)
			closing := -1
			for i, tb := range cw.terms {
				i, tb := i, tb
				variant := minput.Ghost
				if i == cw.termActive {
					variant = minput.Secondary
				}
				if minput.Button(c, tb.label(i), minput.ButtonOptions{Variant: variant, Label: tb.label(i)}).Clicked() {
					cw.termActive = i
				}
				if mcore.IconAction(c, icons.Must("x"), "关闭标签").Clicked() {
					closing = i
				}
			}
			if minput.Button(c, "新建终端", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("plus")}).Clicked() {
				cw.addTerm()
			}
			mgo.Box(c).Grow(1)
			if mcore.IconAction(c, icons.Must("x"), "关闭终端").Clicked() {
				cw.termOpen = false
			}
			if closing >= 0 {
				cw.closeTerm(closing)
			}
		})
		cw.termBody(c, caption, u)
	})
}

// termBody draws the active tab: its shell, a loading hint or an error.
func (cw *ChatWindow) termBody(c *mgo.Context, caption, u float32) {
	k := mcore.Tokens(c)
	mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if cw.termActive < 0 || cw.termActive >= len(cw.terms) {
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
			return
		}
		tb := cw.terms[cw.termActive]
		switch {
		case tb.err != "":
			mgo.Text(c, "终端不可用："+tb.err).Padding(u * 3).FontSize(caption).TextColor(k.Danger)
		case tb.term == nil:
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
		default:
			term.View(c, tb.term).Fill()
		}
	})
}
