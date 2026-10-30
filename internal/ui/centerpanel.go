// Center tabs: the chat message area and every opened file, diff, terminal,
// web page, Git graph and editor live in one tabbed, splittable center area.
//
// The area holds any number of panes, each with its own tab strip. A tab lives
// in one pane and may be dragged to another; the Chat tab follows its
// conversation's agent, so its icon and name tell a plain agent from an ACP
// one. Tabs are created by the new-tab menu or opened from the left panel's
// file tree and Git panel.
package ui

import (
	mgo "github.com/egoist/mygo/ui"
)

// centerKind is what a center tab shows.
type centerKind string

const (
	// centerChat is a conversation tab, free to live in any pane.
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
	// centerCreate is the "新会话" tab that composes a conversation's agent,
	// project, worktree and branch.
	centerCreate centerKind = "create"
)

// centerSplitDir is how a split node lays its two children out.
type centerSplitDir int

const (
	// splitRight puts the children side by side, the second on the right.
	splitRight centerSplitDir = iota
	// splitDown stacks the children, the second below the first.
	splitDown
)

// centerNode is one node of the center area's split tree: a leaf drawing a
// pane, or a split drawing two children around a draggable divider.
type centerNode struct {
	// pane is the pane a leaf draws, and -1 for a split.
	pane int
	// dir is how a split arranges its children; size is the first child's
	// size in DIPs, which the divider moves.
	dir  centerSplitDir
	size float32
	// first and second are the split's children, in that order.
	first, second *centerNode
}

// newCenterNode returns a leaf drawing pane.
func newCenterNode(pane int) *centerNode { return &centerNode{pane: pane} }

// splitPaneNode turns the leaf drawing pane into a split of that leaf and a
// new leaf, and returns the new leaf, or nil when the tree has no such leaf.
func splitPaneNode(n *centerNode, pane int, dir centerSplitDir, size float32) *centerNode {
	if n == nil {
		return nil
	}
	if n.pane == pane {
		first := *n
		second := newCenterNode(-1)
		*n = centerNode{pane: -1, dir: dir, size: size, first: &first, second: second}
		return second
	}
	if n.pane >= 0 {
		return nil
	}
	if got := splitPaneNode(n.first, pane, dir, size); got != nil {
		return got
	}
	return splitPaneNode(n.second, pane, dir, size)
}

// removePaneNode returns the tree without the leaf drawing pane.
func removePaneNode(n *centerNode, pane int) *centerNode {
	if n == nil || n.pane >= 0 {
		return n
	}
	if n.first != nil && n.first.pane == pane {
		return n.second
	}
	if n.second != nil && n.second.pane == pane {
		return n.first
	}
	n.first = removePaneNode(n.first, pane)
	n.second = removePaneNode(n.second, pane)
	return n
}

// paneInTree reports whether the leaf drawing pane is in the tree.
func paneInTree(n *centerNode, pane int) bool {
	if n == nil {
		return false
	}
	if n.pane >= 0 {
		return n.pane == pane
	}
	return paneInTree(n.first, pane) || paneInTree(n.second, pane)
}

// walkPanes visits the leaves in the order they are drawn.
func walkPanes(n *centerNode, visit func(int)) {
	if n == nil {
		return
	}
	if n.pane >= 0 {
		visit(n.pane)
		return
	}
	walkPanes(n.first, visit)
	walkPanes(n.second, visit)
}

// centerPane is one pane of the center area: its tabs and which is active.
type centerPane struct {
	tabs []*centerTab
	sel  int
}

// centerState is the whole center area: its panes, the focused one, the split
// tree that arranges them and the size the last frame had.
type centerState struct {
	panes []centerPane
	focus int
	// root is the split tree of the area; a nil root is a single pane 0.
	root *centerNode
	// viewW and viewH are the last frame's content size, and paneW and paneH
	// each pane's size, so a new split starts near half the pane it splits.
	viewW, viewH float32
	paneW, paneH []float32
	// seq names new tabs.
	seq int
}

// newCenterState is a center area holding one empty pane.
func newCenterState() *centerState {
	return &centerState{root: newCenterNode(0), panes: []centerPane{{sel: -1}}}
}

// ensureRoot gives a state made without a tree its first leaf.
func (s *centerState) ensureRoot() {
	if s.root == nil {
		s.root = newCenterNode(0)
	}
	s.ensurePane(0)
	s.panes[0].clampSel()
}

// ensurePane grows the pane and size slices so pane is indexable.
func (s *centerState) ensurePane(pane int) {
	for len(s.panes) <= pane {
		s.panes = append(s.panes, centerPane{sel: -1})
	}
	for len(s.paneW) <= pane {
		s.paneW = append(s.paneW, 0)
		s.paneH = append(s.paneH, 0)
	}
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

// visiblePanes are the panes the tree draws, in order.
func (s *centerState) visiblePanes() []int {
	out := make([]int, 0, len(s.panes))
	walkPanes(s.root, func(pane int) { out = append(out, pane) })
	return out
}

// paneCount is how many panes the tree draws.
func (s *centerState) paneCount() int {
	n := 0
	walkPanes(s.root, func(int) { n++ })
	return n
}

// hasPane reports whether the tree draws pane.
func (s *centerState) hasPane(pane int) bool {
	return pane >= 0 && paneInTree(s.root, pane)
}

// freePane is the smallest pane index the tree does not draw, growing the
// array when every index is taken.
func (s *centerState) freePane() int {
	for i := range s.panes {
		if !paneInTree(s.root, i) {
			return i
		}
	}
	return len(s.panes)
}

// splitSize is a new split's first-child size in DIPs: about half the pane it
// splits, so both halves are usable at once.
func (s *centerState) splitSize(pane int, dir centerSplitDir) float32 {
	var w, h float32
	if pane >= 0 && pane < len(s.paneW) {
		w, h = s.paneW[pane], s.paneH[pane]
	}
	if w <= 0 {
		w = s.viewW
	}
	if h <= 0 {
		h = s.viewH
	}
	if w <= 0 {
		w = 1200
	}
	if h <= 0 {
		h = 800
	}
	if dir == splitDown {
		return max(80, h/2)
	}
	return max(80, w/2)
}

// splitPane adds an empty pane to the right or below pane, focusing it.
func (cw *ChatWindow) splitPane(pane int, dir centerSplitDir) {
	cw.center.ensureRoot()
	if !cw.center.hasPane(pane) {
		return
	}
	np := cw.center.freePane()
	leaf := splitPaneNode(cw.center.root, pane, dir, cw.center.splitSize(pane, dir))
	if leaf == nil {
		return
	}
	leaf.pane = np
	cw.center.ensurePane(np)
	cw.center.panes[np] = centerPane{sel: -1}
	cw.center.focus = np
	cw.invalidate()
}

// newTabID names a tab uniquely within the window.
func (cw *ChatWindow) newTabID(prefix string) string {
	cw.center.seq++
	return prefix + ":" + itoa(cw.center.seq)
}

// centerArea draws the center area: the panes of the split tree.
func (cw *ChatWindow) centerArea(c *mgo.Context) {
	cw.center.ensureRoot()
	cw.center.viewW, cw.center.viewH = c.Size()
	cw.centerNodeView(c, cw.center.root)
}

// centerNodeView draws one node of the split tree.
func (cw *ChatWindow) centerNodeView(c *mgo.Context, n *centerNode) {
	if n == nil {
		return
	}
	if n.pane >= 0 {
		cw.centerPaneView(c, n.pane)
		return
	}
	first := func() { cw.centerNodeView(c, n.first) }
	second := func() { cw.centerNodeView(c, n.second) }
	if n.dir == splitDown {
		mgo.SplitVertical(c, &n.size, first, second).Fill()
		return
	}
	mgo.Split(c, &n.size, first, second).Fill()
}

// centerStateKey names a workspace's saved center state; the empty workspace
// shares one state of its own.
func centerStateKey(ws string) string {
	if ws == "" {
		return "(global)"
	}
	return ws
}

// switchCenterState points the center area at the state saved under ws,
// creating an empty one the first time, so every workspace owns its tabs and
// split tree. The center is the state's pointer, not a copy: edits to the open
// tabs and splits write straight into the workspace's saved state.
func (cw *ChatWindow) switchCenterState(ws string) {
	if cw.centerStates == nil {
		cw.centerStates = map[string]*centerState{}
	}
	// The window's first center is not bound to any workspace yet. Adopt it
	// as the state of the workspace being left, so tabs opened before the
	// first switch — the main tree's files — survive coming back.
	if !cw.centerBound() {
		cw.centerStates[centerStateKey(cw.workspace)] = cw.center
	}
	st := cw.centerStates[centerStateKey(ws)]
	if st == nil {
		st = newCenterState()
		cw.centerStates[centerStateKey(ws)] = st
	}
	cw.center = st
}

// centerBound reports whether the active center already owns one of the
// per-workspace states, rather than the window's initial unbound one.
func (cw *ChatWindow) centerBound() bool {
	for _, st := range cw.centerStates {
		if st == cw.center {
			return true
		}
	}
	return false
}
