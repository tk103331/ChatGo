package ui

import (
	"fmt"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// gitGraphBody is the GitGraph tab: the repository's branches, a history drawn
// as a commit graph, and the selected commit's details. It reads and keeps its
// own state in cw.gg, so nothing it does changes the Git panel beside it.
func (cw *ChatWindow) gitGraphBody(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cw.resetGraphState(cw.workspace)
	if !cw.gg.loaded {
		cw.centerLoading(c)
		return
	}
	if !cw.gg.repo {
		mgo.Column(c).Fill().Center().Gap(u).Children(func() {
			mgo.Icon(c, icons.Must("git-branch")).Size(32, 32).TextColor(k.TextMuted)
			mgo.Text(c, "当前工作区不是 Git 仓库").TextColor(k.TextMuted).FontSize(caption)
		})
		return
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		cw.gitGraphBar(c)
		cw.gitGraphSearch(c)
		if cw.gg.sel == "" {
			cw.gitGraphList(c)
			return
		}
		// The details sit below the history with a divider the user drags, so
		// a long changed-file list is readable instead of a sliver.
		if cw.gg.listH == 0 {
			// Start with the details taking about a third of the tab.
			_, h := c.Size()
			cw.gg.listH = max(160, (h-navItemSize)*2/3)
		}
		mgo.SplitVertical(c, &cw.gg.listH,
			func() { cw.gitGraphList(c) },
			func() { cw.gitGraphDetail(c) }).Grow(1).MinHeight(0)
	})
}

// gitGraphBar is the tab's header: the branch selector, the working tree's
// changes and the refresh action, all driving this tab's own state.
func (cw *ChatWindow) gitGraphBar(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Gap(u).Children(func() {
		sel := kit.BranchSelector(c, &cw.gg.branch, cw.gg.branches,
			kit.BranchSelectorOptions{AllowCreate: true, Key: "gitgraph"})
		if sel.Changed() {
			cw.checkoutGraphBranch(cw.gg.branch)
		}
		if name, ok := sel.Created(); ok {
			cw.createGraphBranch(name)
		}
		kit.DiffStat(c, cw.gg.stat[0], cw.gg.stat[1], kit.DiffStatOptions{})
		mgo.Text(c, fmt.Sprintf("%d 个提交", len(cw.gg.rows))).
			FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		mgo.Spacer(c)
		if kit.Button(c, "刷新", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("refresh-cw")}).Clicked() {
			cw.loadGitGraph()
		}
	})
}

// gitGraphSearch filters the history by subject, hash or author.
func (cw *ChatWindow) gitGraphSearch(c *mgo.Context) {
	in := kit.TextInput(c, &cw.gg.query, kit.TextInputOptions{
		Placeholder: "搜索提交、哈希或作者",
		Label:       "搜索提交",
	})
	in.Element.FillWidth()
}

// gitGraphList draws the filtered history as a scrollable commit graph. Each
// row draws the lanes that pass through it, so branches and merges show as
// lines rather than as a flat list.
func (cw *ChatWindow) gitGraphList(c *mgo.Context) {
	rows := cw.graphVisibleRows()
	if len(rows) == 0 {
		mgo.Text(c, "没有匹配的提交").TextColor(mcore.Tokens(c).TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Padding(mcore.Density(c).Unit() * 2)
		return
	}
	if cw.gg.list.Selected == nil {
		cw.gg.list.Selected = new(int)
	}
	cw.gg.list.Key = func(i int) any { return rows[i].Commit.Hash }
	cw.gg.list.Label = func(i int) string { return rows[i].Commit.Subject }
	cw.gitGraphRows(c, rows)
}

// gitGraphRows builds the virtual list over the visible rows.
func (cw *ChatWindow) gitGraphRows(c *mgo.Context, rows []graphRow) {
	width := graphWidth(rows)
	list := mgo.List(c, &cw.gg.list, len(rows), func(i int) {
		cw.gitGraphRow(c, rows, i, width)
	})
	list.Grow(1).MinHeight(0).Label("提交历史")
	// Page in more history well before the end is reached, so scrolling does
	// not appear to stop.
	if cw.gg.query == "" {
		if _, last := cw.gg.list.Visible(); last >= len(rows)-graphPageSize/2 {
			cw.growGitGraph()
		}
	}
}

// gitGraphRow draws one commit: its lane lines, the ref labels and the
// subject with its author and time.
func (cw *ChatWindow) gitGraphRow(c *mgo.Context, rows []graphRow, i int, width float32) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	row := rows[i]
	selected := cw.gg.sel == row.Commit.Hash
	el := mgo.Row(c).FillWidth().Padding(u/2, u).Gap(u).AlignItems(mgo.Center).
		Radius(mtheme.ControlRadius).Label(row.Commit.Subject)
	subjectColor := k.Text
	switch {
	case selected:
		el.Background(k.Selection)
	case el.Hovered():
		el.Background(k.SurfaceHover)
	}
	if el.Clicked() {
		cw.gg.sel = row.Commit.Hash
	}
	el.Children(func() {
		cw.drawGraph(c, row, width, k.Accent)
		mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
			mgo.Row(c).FillWidth().MinWidth(0).Gap(u / 2).
				AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, row.Commit.Subject).SingleLine().Shrink(1).MinWidth(0).
					Ellipsis("…").TextColor(subjectColor)
				mgo.Box(c).Grow(1)
				cw.graphRefLabels(c, row.Commit.Refs, selected)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, shortHash(row.Commit.Hash)).Font(mtheme.MonoFont).
					FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				mgo.Text(c, row.Commit.Author).FontSize(caption).TextColor(k.TextMuted).
					SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				mgo.Text(c, relativeTime(row.Commit.When)).FontSize(caption).
					TextColor(k.TextMuted).Shrink(0).
					Tooltip(row.Commit.When.Format("2006-01-02 15:04:05"))
			})
		})
	})
}

// graphRefLabels shows a commit's refs, folding the extras into a count so a
// commit carrying several branches still leaves room for its subject.
func (cw *ChatWindow) graphRefLabels(c *mgo.Context, refs []string, selected bool) {
	if len(refs) == 0 {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	fg, border := k.Accent, k.Border
	if selected {
		fg, border = k.Text, k.Text
	}
	shown := refs
	if len(shown) > maxRowRefs {
		shown = shown[:maxRowRefs]
	}
	for _, ref := range shown {
		mgo.Text(c, ref).FontSize(mcore.FontSize(c, mtheme.CaptionSize-1)).
			TextColor(fg).Padding(0, u/2).Radius(mtheme.ControlRadius).
			Border(mtheme.BorderWidth, border).SingleLine().Shrink(0)
	}
	if extra := len(refs) - len(shown); extra > 0 {
		mgo.Text(c, "+"+itoa(extra)).FontSize(mcore.FontSize(c, mtheme.CaptionSize-1)).
			TextColor(k.TextMuted).Shrink(0)
	}
}

// maxRowRefs bounds how many ref labels a row shows before counting the rest.
const maxRowRefs = 2

// drawGraph paints one row's lanes: the lines running through it and its dot.
func (cw *ChatWindow) drawGraph(c *mgo.Context, row graphRow, width float32, accent mgo.Color) {
	// The rail takes the row's own height, which the two lines of text set:
	// a fixed height would stop the lanes short of the next row.
	rail := mgo.Box(c).Width(width).FillHeight().Shrink(0)
	palette := graphPalette(accent)
	lane := row.Lane
	if lane >= maxGraphLanes {
		lane = maxGraphLanes - 1
	}
	rail.Draw(func(p *mgo.Painter, r mgo.Rect) {
		mid := r.Y + r.H/2
		dotX := r.X + laneWidth/2 + float32(lane)*laneWidth
		for _, seg := range row.Segments {
			from, to := seg.From, seg.To
			if from >= maxGraphLanes {
				from = maxGraphLanes - 1
			}
			if to >= maxGraphLanes {
				to = maxGraphLanes - 1
			}
			x0 := r.X + laneWidth/2 + float32(from)*laneWidth
			x1 := r.X + laneWidth/2 + float32(to)*laneWidth
			color := palette(laneColorIndex(seg, lane))
			if seg.From == seg.To {
				// A line passing straight through the row.
				p.Line(x0, r.Y, x1, r.Y+r.H, 1.5, color)
				continue
			}
			// A fork or merge: down to the middle, across, then down.
			p.Line(x0, r.Y, x0, mid, 1.5, color)
			p.Line(x0, mid, x1, mid, 1.5, color)
			p.Line(x1, mid, x1, r.Y+r.H, 1.5, color)
		}
		p.Fill(mgo.Rect{X: dotX - 3.5, Y: mid - 3.5, W: 7, H: 7}, palette(lane), 3.5)
	})
}

// laneColorIndex picks the palette entry a segment draws in: its origin column,
// so a branch keeps one colour along its length.
func laneColorIndex(seg graphSegment, own int) int {
	if seg.Merging {
		return seg.To
	}
	return seg.From
}

// graphPalette returns a colour per lane, cycling a fixed set so neighbouring
// branches stay distinguishable.
func graphPalette(accent mgo.Color) func(int) mgo.Color {
	colors := []mgo.Color{
		accent,
		mgo.RGBA(0x2E, 0x9E, 0x6E, 0xFF),
		mgo.RGBA(0xD0, 0x7A, 0x1F, 0xFF),
		mgo.RGBA(0x9B, 0x59, 0xC4, 0xFF),
		mgo.RGBA(0xC0, 0x40, 0x5B, 0xFF),
		mgo.RGBA(0x2F, 0x86, 0xC0, 0xFF),
	}
	return func(lane int) mgo.Color {
		if lane < 0 {
			lane = -lane
		}
		return colors[lane%len(colors)]
	}
}

// gitGraphDetail shows the selected commit: its message, its refs, its files
// and a way to open the full diff.
func (cw *ChatWindow) gitGraphDetail(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	commit, found := findCommit(cw.gg.commits, cw.gg.sel)
	if !found {
		return
	}
	info := cw.graphDetailOf(commit)
	mgo.Box(c).Fill().MinHeight(0).Children(func() {
		mgo.Scroll(c).Fill().Children(func() {
			mgo.Column(c).FillWidth().Gap(u).Padding(u, 0).Children(func() {
				mgo.Text(c, commit.Subject).FontWeight(600).MaxLines(3)
				if len(commit.Refs) > 0 {
					mgo.Row(c).Wrap().Gap(u / 2).Children(func() {
						for _, ref := range commit.Refs {
							mgo.Text(c, ref).FontSize(mcore.FontSize(c, mtheme.CaptionSize-1)).
								TextColor(k.Accent).Padding(0, u/2).
								Radius(mtheme.ControlRadius).
								Border(mtheme.BorderWidth, k.Border).SingleLine()
						}
					})
				}
				meta := shortHash(commit.Hash) + " · " + commit.Author
				if !commit.When.IsZero() {
					meta += " · " + commit.When.Format("2006-01-02 15:04")
				}
				mgo.Text(c, meta).TextColor(k.TextMuted).FontSize(caption).Font(mtheme.MonoFont)
				if info != nil && info.Body != "" {
					mgo.Text(c, info.Body).TextColor(k.TextMuted).FontSize(caption).MaxLines(4)
				}
				if info != nil && len(info.Files) > 0 {
					mgo.Text(c, fmt.Sprintf("%d 个文件变更", len(info.Files))).
						TextColor(k.TextMuted).FontSize(caption)
					for _, f := range info.Files {
						mgo.Text(c, f.Path).Font(mtheme.MonoFont).FontSize(caption).
							SingleLine().Ellipsis("…")
					}
				}
				if kit.Button(c, "打开完整对比", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
					cw.openCommitDiffInGraph(commit.Hash)
				}
			})
		})
	})
}

// graphDetailOf returns a commit's cached details, loading them once in the
// background.
func (cw *ChatWindow) graphDetailOf(commit kit.Commit) *commitInfo {
	if cw.gg.detail == nil {
		cw.gg.detail = map[string]*commitInfo{}
	}
	if info, ok := cw.gg.detail[commit.Hash]; ok {
		return info
	}
	cw.gg.detail[commit.Hash] = nil
	dir := cw.gg.dir
	go func() {
		info := loadCommit(dir, commit.Hash)
		cw.postMain(func() {
			if cw.gg.detail == nil {
				cw.gg.detail = map[string]*commitInfo{}
			}
			cw.gg.detail[commit.Hash] = info
			cw.invalidate()
		})
	}()
	return nil
}

// checkoutGraphBranch switches branches for the GitGraph tab, then reloads its
// own state without touching the Git panel's.
func (cw *ChatWindow) checkoutGraphBranch(name string) {
	if out, err := gitRun(cw.gg.dir, "checkout", name); err != nil {
		cw.toast(out, mcore.SeverityError)
	}
	cw.loadGitGraph()
}

// createGraphBranch creates and checks out a branch for the GitGraph tab.
func (cw *ChatWindow) createGraphBranch(name string) {
	if out, err := gitRun(cw.gg.dir, "checkout", "-b", name); err != nil {
		cw.toast(out, mcore.SeverityError)
	}
	cw.loadGitGraph()
}

// openCommitDiffInGraph opens a commit's diff as a center tab.
func (cw *ChatWindow) openCommitDiffInGraph(hash string) {
	cw.openCommitDiff(hash)
}

// relativeTime writes how long ago t was: 刚刚, 5 分钟前, 3 天前.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}
