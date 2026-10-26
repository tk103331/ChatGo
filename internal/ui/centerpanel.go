// Center tabs: the chat message area and every opened file, diff, terminal,
// web page, Git graph and editor live in one tabbed, splittable center area.
//
// The area holds up to four panes, each with its own tab strip. Pane 0 always
// owns the Chat tab, which is pinned first and cannot be closed; the Chat tab
// follows the current conversation's agent, so its icon and name tell a plain
// agent from an ACP one. Other tabs are created by the new-tab menu or opened
// from the left panel's file tree and Git panel.
package ui

import (
	mgo "github.com/egoist/mygo/ui"
)

// centerKind is what a center tab shows.
type centerKind string

const (
	// centerChat is the pinned conversation tab of pane 0.
	centerChat centerKind = "chat"
	// centerText is a file's source, read-only.
	centerText centerKind = "text"
	// centerImage is a decoded bitmap or SVG.
	centerImage centerKind = "image"
	// centerDiff is a file's working-tree diff against HEAD.
	centerDiff centerKind = "fdiff"
	// centerBlame is a file's line-by-line history.
	centerBlame centerKind = "blame"
	// centerCommit is one commit's message and per-file diffs.
	centerCommit centerKind = "commit"
	// centerWeb is a web view.
	centerWeb centerKind = "web"
	// centerTerm is a shell.
	centerTerm centerKind = "term"
	// centerGitGraph is the repository's branches and commit graph.
	centerGitGraph centerKind = "gitgraph"
	// centerWorktree is the repository's linked working trees.
	centerWorktree centerKind = "worktree"
	// centerEditor is a writable file editor.
	centerEditor centerKind = "editor"
	// centerCodeSearch searches the workspace's files.
	centerCodeSearch centerKind = "codesearch"
)

// centerLayout is how the center area divides itself into panes.
type centerLayout int

const (
	// layoutSingle is one pane filling the area.
	layoutSingle centerLayout = iota
	// layoutCols is two panes side by side.
	layoutCols
	// layoutLeftWide is one pane down the left, two stacked on the right.
	layoutLeftWide
)

// paneCount is how many panes the layout shows.
func (l centerLayout) paneCount() int {
	switch l {
	case layoutCols:
		return 2
	case layoutLeftWide:
		return 3
	default:
		return 1
	}
}

// maxPanes is the most panes any layout has, and so the length of the pane
// array: the layout decides how many of them are drawn.
const maxPanes = 4

// chatPaneIndex is the pane that owns the pinned Chat tab.
const chatPaneIndex = 0

// centerPane is one pane of the center area: its tabs and which is active.
type centerPane struct {
	tabs []*centerTab
	sel  int
}

// centerState is the whole center area: its panes, the focused one, the
// layout and the divider positions.
type centerState struct {
	panes  [maxPanes]centerPane
	focus  int
	layout centerLayout
	// colW splits the left column from the right one, and gridRight the
	// right column's own two rows.
	colW, gridRight float32
	// seq names new tabs.
	seq int
}

// activeTab is the focused pane's active tab, or nil.
func (s *centerState) activeTab() *centerTab {
	return s.panes[s.focus].active()
}

// active is the pane's active tab, or nil when it has none.
func (p *centerPane) active() *centerTab {
	if p.sel < 0 || p.sel >= len(p.tabs) {
		return nil
	}
	return p.tabs[p.sel]
}

// clampSel keeps the pane's selection on a tab it still holds.
func (p *centerPane) clampSel() {
	switch {
	case len(p.tabs) == 0:
		p.sel = -1
	case p.sel < 0:
		p.sel = 0
	case p.sel >= len(p.tabs):
		p.sel = len(p.tabs) - 1
	}
}

// indexOf is the position of tab in the pane, or -1.
func (p *centerPane) indexOf(tab *centerTab) int {
	for i, t := range p.tabs {
		if t == tab {
			return i
		}
	}
	return -1
}

// visiblePanes are the panes the layout draws, in order.
func (s *centerState) visiblePanes() []int {
	n := s.layout.paneCount()
	out := make([]int, 0, n)
	for i := range n {
		out = append(out, i)
	}
	return out
}

// collapseHiddenPanes moves the tabs of panes the layout no longer shows into
// the surviving pane nearest to where they were drawn, so shrinking the layout
// never loses a tab and keeps each one as close to its old place as it can.
func (s *centerState) collapseHiddenPanes() {
	n := s.layout.paneCount()
	for i := n; i < maxPanes; i++ {
		if len(s.panes[i].tabs) > 0 {
			target := nearestVisiblePane(s.layout, i)
			s.panes[target].tabs = append(s.panes[target].tabs, s.panes[i].tabs...)
		}
		s.panes[i] = centerPane{sel: -1}
	}
	// A pane the layout adds has never held a tab, and one that just gave its
	// tabs away may point past its new end; clamping settles both.
	for i := range s.panes {
		s.panes[i].clampSel()
	}
	if s.focus >= n {
		s.focus = nearestVisiblePane(s.layout, s.focus)
	}
}

// paneNeighbours records, per layout, which drawn pane each hidden pane folds
// into. Panes keep their index whenever the layout still draws them, so only
// the indices at or past paneCount are folded away; they go to the pane that
// now covers the region they last occupied.
var paneNeighbours = map[centerLayout]map[int]int{
	// One pane covers the whole area.
	layoutSingle: {1: 0, 2: 0},
	// Left and right columns: the second column's lower pane folds up into
	// the pane above it.
	layoutCols: {2: 1},
	// With one pane down the left, only pane 2 can be dropped, and it joins
	// the right column's top.
	layoutLeftWide: {},
}

// nearestVisiblePane is the drawn pane a hidden one folds into under the
// layout, or the same index when it is already visible.
func nearestVisiblePane(l centerLayout, i int) int {
	if i < l.paneCount() {
		return i
	}
	if m, ok := paneNeighbours[l]; ok {
		if target, ok := m[i]; ok {
			return target
		}
	}
	return max(l.paneCount()-1, 0)
}

// setLayout switches the layout, folding away the panes it drops.
func (cw *ChatWindow) setLayout(l centerLayout) {
	cw.center.layout = l
	// collapseHiddenPanes normalises every pane's selection, so panes the new
	// layout adds start with none and panes that lost tabs point at a
	// remaining one.
	cw.center.collapseHiddenPanes()
	cw.invalidate()
}

// newTabID names a tab uniquely within the window.
func (cw *ChatWindow) newTabID(prefix string) string {
	cw.center.seq++
	return prefix + ":" + itoa(cw.center.seq)
}

// centerArea draws the center area: the panes of the current layout.
func (cw *ChatWindow) centerArea(c *mgo.Context) {
	if cw.center.colW == 0 {
		w, _ := c.Size()
		cw.center.colW = max(240, w/2)
	}
	if cw.center.gridRight == 0 {
		_, h := c.Size()
		cw.center.gridRight = max(160, h/2)
	}
	switch cw.center.layout {
	case layoutCols:
		mgo.Split(c, &cw.center.colW,
			func() { cw.centerPaneView(c, 0) },
			func() { cw.centerPaneView(c, 1) }).Fill()
	case layoutLeftWide:
		mgo.Split(c, &cw.center.colW, func() {
			cw.centerPaneView(c, 0)
		}, func() {
			mgo.SplitVertical(c, &cw.center.gridRight,
				func() { cw.centerPaneView(c, 1) },
				func() { cw.centerPaneView(c, 2) }).Fill()
		}).Fill()
	default:
		cw.centerPaneView(c, 0)
	}
}

// centerStateKey names a workspace's saved center state; the empty workspace
// shares one state of its own.
func centerStateKey(ws string) string {
	if ws == "" {
		return "(global)"
	}
	return ws
}

// saveCenterState stores the center area as it is now under ws, keeping the
// tabs and layout each workspace opened.
func (cw *ChatWindow) saveCenterState(ws string) {
	if cw.centerStates == nil {
		cw.centerStates = map[string]*centerState{}
	}
	st := cw.centerStates[centerStateKey(ws)]
	if st == nil {
		st = &centerState{}
		cw.centerStates[centerStateKey(ws)] = st
	}
	*st = cw.center
}

// switchCenterState swaps the whole center area to the state saved under ws,
// stashing the one it leaves, so every workspace has its own tabs and layout.
func (cw *ChatWindow) switchCenterState(ws string) {
	if cw.workspace == ws {
		return
	}
	cw.saveCenterState(cw.workspace)
	if cw.centerStates == nil {
		cw.centerStates = map[string]*centerState{}
	}
	st := cw.centerStates[centerStateKey(ws)]
	if st == nil {
		st = &centerState{}
		cw.centerStates[centerStateKey(ws)] = st
	}
	cw.center = *st
}
