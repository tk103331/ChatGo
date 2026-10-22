package kit

import (
	"fmt"
	"path"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// GitStatus is how a file changed.
type GitStatus int

const (
	GitModified GitStatus = iota
	GitAdded
	GitDeleted
	GitUntracked
	GitRenamed
	GitCopied
)

// Letter is the git status code, as M.
func (s GitStatus) Letter() string {
	if s < GitModified || s > GitCopied {
		panic("kit: unknown GitStatus")
	}
	return string("MADURC"[int(s)])
}

func (s GitStatus) word() string {
	return [...]string{"已修改", "已添加", "已删除", "未跟踪", "已重命名", "已复制"}[int(s)]
}

// tone returns the color marking s.
func (s GitStatus) tone(k *mgo.Theme) mgo.Color {
	switch s {
	case GitAdded, GitUntracked:
		return k.Success
	case GitDeleted:
		return k.Danger
	case GitRenamed, GitCopied:
		return k.Accent
	}
	return k.Warning
}

// GitStatusBadge marks a status by letter, outline and color.
func GitStatusBadge(c *mgo.Context, s GitStatus) mgo.Element {
	k := uikit.Tokens(c)
	tone := s.tone(k)
	return mgo.Box(c).Size(18, 18).Shrink(0).Center().Radius(theme.ControlRadius).
		Border(theme.BorderWidth, tone).Role(mgo.RoleImage).Label(s.word()).
		Children(func() {
			mgo.Text(c, s.Letter()).Font(theme.MonoFont).FontWeight(600).
				FontSize(uikit.FontSize(c, theme.CaptionSize-1)).TextColor(tone)
		})
}

// ChangedFile is a changed file in the tree or index.
type ChangedFile struct {
	Path   string
	Status GitStatus
	// Staged puts the file in the index group.
	Staged bool
}

// Branch is a local or remote branch.
type Branch struct {
	// Name is unique; remote names carry the remote, as origin/main.
	Name   string
	Remote bool
	// Current marks the checked-out branch.
	Current bool
	// Upstream is the tracked remote branch, if any.
	Upstream      string
	Ahead, Behind int
}

// Commit is one commit of a history.
type Commit struct {
	Hash    string
	Parents []string
	Author  string
	When    time.Time
	Subject string
	// Refs are branch and tag names pointing at the commit.
	Refs []string
}

// BlameLine is a source line and its last commit.
type BlameLine struct {
	Hash    string
	Author  string
	When    time.Time
	Summary string
	Text    string
}

// GitListResult is a built version-control list and its events.
type GitListResult struct {
	Element            mgo.Element
	changed, submitted bool
	action, item       string
}

// Changed reports a new selection.
func (r GitListResult) Changed() bool { return r.changed }

// Submitted reports Enter or a double click on a row.
func (r GitListResult) Submitted() bool { return r.submitted }

// Action returns this frame's action ID and its item.
func (r GitListResult) Action() (action, item string, ok bool) {
	return r.action, r.item, r.action != ""
}

// gitRow styles one row of a version-control list.
func gitRow(c *mgo.Context, selected bool) mgo.Element {
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	row := mgo.Row(c).FillWidth().MinHeight(d.ControlHeight()-4).
		Padding(0, d.Unit()*2).Gap(d.Unit() * 1.5).AlignItems(mgo.Center).
		Radius(theme.ControlRadius).Cursor(mgo.CursorPointer).Selected(selected)
	switch {
	case selected:
		row.Background(k.Selection)
	case row.Hovered():
		row.Background(k.SurfaceHover)
	}
	return row
}

// gitCaption is small muted text.
func gitCaption(c *mgo.Context, s string) mgo.Element {
	return mgo.Text(c, s).FontSize(uikit.FontSize(c, theme.CaptionSize)).
		TextColor(uikit.Tokens(c).TextMuted).SingleLine()
}

// DiffStatOptions configure a DiffStat.
type DiffStatOptions struct {
	// Total draws the bar against this many changes; zero uses added+deleted.
	Total int
}

// DiffStat shows added and deleted line counts with a bar.
func DiffStat(c *mgo.Context, added, deleted int, opts DiffStatOptions) mgo.Element {
	if added < 0 || deleted < 0 {
		panic("kit: DiffStat needs counts of zero or more")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	total := opts.Total
	if total <= 0 {
		total = added + deleted
	}
	label := fmt.Sprintf("+%d −%d", added, deleted)
	row := mgo.Row(c).Gap(u).AlignItems(mgo.Center).Shrink(0).
		Role(mgo.RoleImage).Label(label)
	row.Children(func() {
		mgo.Text(c, fmt.Sprintf("+%d", added)).FontFeatures("tnum").
			FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.Success).SingleLine()
		mgo.Text(c, fmt.Sprintf("−%d", deleted)).FontFeatures("tnum").
			FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.Danger).SingleLine()
		if total == 0 {
			return
		}
		// Five blocks, filled green then red in proportion to the change.
		const blocks = 5
		greens := int(float32(added)/float32(total)*blocks + 0.5)
		mgo.Row(c).Gap(1).AlignItems(mgo.Center).Shrink(0).Children(func() {
			for i := range blocks {
				fill := k.Border
				switch {
				case i < greens:
					fill = k.Success
				case added+deleted > 0:
					fill = k.Danger
				}
				mgo.Box(c).Size(6, 6).Radius(1).Background(fill)
			}
		})
	})
	return row
}

// ChangesListState is a ChangesList's selection and grouping.
type ChangesListState struct {
	list    mgo.ListState
	cursor  int
	rows    []int // rows map to files; negatives are group headers
	staged  int
	n       int
	started bool
}

// Selected returns the chosen file's index, or -1.
func (s *ChangesListState) Selected() int {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return -1
	}
	return s.rows[s.cursor]
}

// ChangesListOptions configure a ChangesList.
type ChangesListOptions struct {
	Disabled bool
}

// ChangesList groups staged and unstaged files; give it a height.
func ChangesList(c *mgo.Context, s *ChangesListState, files []ChangedFile, opts ChangesListOptions) GitListResult {
	if s == nil {
		panic("kit: ChangesList needs a state")
	}
	if !s.started {
		s.started, s.cursor = true, -1
	}
	if len(files) != s.n {
		s.rows, s.staged = changesListRows(files)
		s.n, s.cursor = len(files), -1
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	var r GitListResult
	s.list.Selected = &s.cursor
	s.list.Header = func(i int) bool { return s.rows[i] < 0 }
	s.list.Key = func(i int) any {
		if f := s.rows[i]; f >= 0 {
			return fmt.Sprint(files[f].Staged, files[f].Path)
		}
		return s.rows[i]
	}
	r.Element = mgo.List(c, &s.list, len(s.rows), func(i int) {
		at := s.rows[i]
		if at < 0 {
			// A group header: staged files first, then the rest.
			title, action, label := "已暂存", "unstage_all", "全部取消暂存"
			n := s.staged
			if at == -2 {
				title, action, label = "未暂存", "stage_all", "全部暂存"
				n = len(files) - s.staged
			}
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).
				Padding(u, u*2, u/2, u*2).Children(func() {
				mgo.Text(c, title).FontWeight(600).
					FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)
				gitCaption(c, fmt.Sprint(n))
				mgo.Spacer(c)
				if n > 0 && uikit.IconAction(c, icons.Must("check"), label).
					Disabled(opts.Disabled).Clicked() && !opts.Disabled {
					r.action, r.item = action, ""
				}
			})
			return
		}
		f := files[at]
		row := gitRow(c, i == s.cursor)
		row.Label(f.Path + " " + f.Status.word())
		if row.Clicked() {
			s.cursor = i
			r.changed = true
		}
		if row.DoubleClicked() {
			r.submitted = true
		}
		row.Children(func() {
			GitStatusBadge(c, f.Status)
			mgo.Text(c, path.Base(f.Path)).SingleLine().Shrink(0)
			if dir := path.Dir(f.Path); dir != "." {
				gitCaption(c, dir).Shrink(1).MinWidth(0).Ellipsis("…")
			}
			mgo.Spacer(c)
			action, label := "stage", "暂存"
			if f.Staged {
				action, label = "unstage", "取消暂存"
			}
			if uikit.IconAction(c, icons.Must("plus"), label+" "+f.Path).
				Disabled(opts.Disabled).Clicked() && !opts.Disabled {
				r.action, r.item = action, f.Path
			}
		})
	})
	if r.Element.Changed() {
		r.changed = true
	}
	if r.Element.Submitted() && !opts.Disabled {
		r.submitted = true
	}
	return r
}

// changesListRows lays out group headers and file rows: -1 heads the staged
// group and -2 the unstaged one.
func changesListRows(files []ChangedFile) (rows []int, staged int) {
	for _, f := range files {
		if f.Staged {
			staged++
		}
	}
	if staged > 0 {
		rows = append(rows, -1)
		for i, f := range files {
			if f.Staged {
				rows = append(rows, i)
			}
		}
	}
	if staged < len(files) {
		rows = append(rows, -2)
		for i, f := range files {
			if !f.Staged {
				rows = append(rows, i)
			}
		}
	}
	return rows, staged
}

// BranchSelectorOptions configure a BranchSelector.
type BranchSelectorOptions struct {
	// AllowCreate offers the typed name as a new branch.
	AllowCreate bool
	Disabled    bool
}

// BranchSelectorResult is a built BranchSelector and its events.
type BranchSelectorResult struct {
	Element mgo.Element
	changed bool
	created string
}

// Changed reports a checkout of another branch.
func (r BranchSelectorResult) Changed() bool { return r.changed }

// Created returns a branch the user asked to create.
func (r BranchSelectorResult) Created() (string, bool) { return r.created, r.created != "" }

// BranchSelector picks a branch into *current, and can offer to create one.
func BranchSelector(c *mgo.Context, current *string, branches []Branch, opts BranchSelectorOptions) BranchSelectorResult {
	if current == nil {
		panic("kit: BranchSelector needs a current branch")
	}
	var r BranchSelectorResult
	open := mgo.Local(c.Root(), "chatgo.branch.open", func() bool { return false })
	query := mgo.Local(c.Root(), "chatgo.branch.query", func() string { return "" })

	label := *current
	if label == "" {
		label = "选择分支"
	}
	trigger := Button(c, label, ButtonOptions{
		Variant: Secondary, Icon: icons.Must("git-branch"), Disabled: opts.Disabled,
	})
	r.Element = trigger
	if trigger.Clicked() {
		*open = !*open
		*query = ""
	}
	if !*open {
		return r
	}
	u := uikit.Unit(c)
	q := strings.ToLower(strings.TrimSpace(*query))
	Popover(c, trigger, open, PopoverOptions{Label: "分支"}, func() {
		in := SearchInput(c, query, SearchInputOptions{Placeholder: "搜索分支", Label: "搜索分支"})
		in.Element.FillWidth()
		uikit.AutoFocus(c, in.Input)
		shown := 0
		mgo.Column(c).FillWidth().Gap(1).Children(func() {
			for _, b := range branches {
				if q != "" && !strings.Contains(strings.ToLower(b.Name), q) {
					continue
				}
				shown++
				row := gitRow(c, b.Name == *current)
				row.Label(b.Name)
				if row.Clicked() && b.Name != *current {
					*current, r.changed, *open = b.Name, true, false
				}
				row.Children(func() {
					icon := "git-branch"
					if b.Remote {
						icon = "globe"
					}
					mgo.Icon(c, icons.Must(icon)).Size(14, 14).
						TextColor(uikit.Tokens(c).TextMuted).Shrink(0)
					mgo.Text(c, b.Name).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
					if b.Ahead > 0 || b.Behind > 0 {
						gitCaption(c, fmt.Sprintf("↑%d ↓%d", b.Ahead, b.Behind))
					}
					if b.Current {
						mgo.Icon(c, icons.Must("check")).Size(14, 14).
							TextColor(uikit.Tokens(c).AccentText).Shrink(0)
					}
				})
			}
		})
		name := strings.TrimSpace(*query)
		switch {
		case shown == 0 && name == "":
			gitCaption(c, "没有分支").Padding(u * 2)
		case opts.AllowCreate && name != "" && !branchExists(branches, name):
			if Button(c, "创建分支 "+name, ButtonOptions{
				Variant: Ghost, Icon: icons.Must("plus"),
			}).AlignSelf(mgo.Start).Clicked() {
				r.created, *open = name, false
			}
		case shown == 0:
			gitCaption(c, "无匹配分支").Padding(u * 2)
		}
	})
	return r
}

func branchExists(branches []Branch, name string) bool {
	for _, b := range branches {
		if b.Name == name {
			return true
		}
	}
	return false
}

// CommitMessage is a commit's title and body.
type CommitMessage struct {
	Title string
	Body  string
}

// CommitInputOptions configure a CommitInput.
type CommitInputOptions struct {
	// TitleLimit warns past this many title characters; zero means 50.
	TitleLimit int
	// Busy shows a commit in progress and blocks another.
	Busy     bool
	Disabled bool
}

// CommitInputResult is a built CommitInput and its events.
type CommitInputResult struct {
	Element          mgo.Element
	committed, amend bool
}

// Committed reports Commit, or Cmd+Enter, this frame.
func (r CommitInputResult) Committed() bool { return r.committed }

// Amended reports Amend this frame.
func (r CommitInputResult) Amended() bool { return r.amend }

// CommitInput edits a commit message and reports Commit and Amend.
func CommitInput(c *mgo.Context, msg *CommitMessage, opts CommitInputOptions) CommitInputResult {
	if msg == nil {
		panic("kit: CommitInput needs a message")
	}
	limit := opts.TitleLimit
	if limit == 0 {
		limit = 50
	}
	u := uikit.Unit(c)
	blocked := opts.Disabled || opts.Busy
	var r CommitInputResult
	r.Element = mgo.Column(c).FillWidth().Shrink(0).Gap(u).Disabled(blocked)
	r.Element.Children(func() {
		title := TextInput(c, &msg.Title, TextInputOptions{
			Placeholder: "提交标题", Label: "提交标题", Disabled: blocked,
		})
		title.Element.FillWidth()
		body := TextArea(c, &msg.Body, TextAreaOptions{
			Placeholder: "详细说明（可选）", Label: "提交说明",
			MinHeight: 56, MaxHeight: 160, Disabled: blocked,
		})
		body.Element.FillWidth()
		ready := strings.TrimSpace(msg.Title) != "" && !blocked
		if (title.Submitted() || body.Submitted()) && ready {
			r.committed = true
		}
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			if n := len([]rune(msg.Title)); n > limit {
				mgo.Text(c, fmt.Sprintf("标题超出 %d 字", n-limit)).
					FontSize(uikit.FontSize(c, theme.CaptionSize)).
					TextColor(uikit.Tokens(c).Warning).SingleLine()
			}
			mgo.Spacer(c)
			if Button(c, "修补", ButtonOptions{Variant: Secondary, Disabled: !ready}).Clicked() {
				r.amend = true
			}
			commit := Button(c, "提交", ButtonOptions{
				Variant: Primary, Disabled: !ready, Loading: opts.Busy,
			})
			if commit.Clicked() {
				r.committed = true
			}
		})
	})
	return r
}

// KeyedListState holds a list's chosen key, so the choice survives rows
// being added or removed above it.
type KeyedListState[K comparable] struct {
	Rows mgo.ListState
	key  K
	has  bool
}

// Selected returns the chosen item's key.
func (s *KeyedListState[K]) Selected() (K, bool) { return s.key, s.has }

// choose records k as the chosen key.
func (s *KeyedListState[K]) choose(k K) { s.key, s.has = k, true }

// CommitGraphState keeps a CommitGraph's selection and place.
type CommitGraphState struct {
	List   KeyedListState[string]
	cursor int
}

// CommitGraphOptions configure a CommitGraph.
type CommitGraphOptions struct {
	// TimeFormat lays out commit times; empty means "2006-01-02 15:04".
	TimeFormat string
	Disabled   bool
}

// CommitGraph lists commits with their refs; give it a height.
func CommitGraph(c *mgo.Context, s *CommitGraphState, commits []Commit, opts CommitGraphOptions) GitListResult {
	if s == nil {
		panic("kit: CommitGraph needs a state")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	format := opts.TimeFormat
	if format == "" {
		format = "2006-01-02 15:04"
	}
	var r GitListResult
	s.List.Rows.Selected = &s.cursor
	s.List.Rows.Key = func(i int) any { return commits[i].Hash }
	r.Element = mgo.List(c, &s.List.Rows, len(commits), func(i int) {
		commit := commits[i]
		row := gitRow(c, i == s.cursor)
		row.Label(commit.Subject)
		if row.Clicked() {
			s.cursor = i
			s.List.choose(commit.Hash)
			r.changed = true
		}
		if row.DoubleClicked() {
			r.submitted = true
		}
		row.Children(func() {
			// The graph rail: a dot per commit, joined by a line.
			rail := mgo.Box(c).Size(10, 18).Shrink(0)
			dot, line := k.AccentText, k.Border
			first, last := i == 0, i == len(commits)-1
			rail.Draw(func(p *mgo.Painter, b mgo.Rect) {
				cx, cy := b.X+b.W/2, b.Y+b.H/2
				if !first {
					p.Line(cx, b.Y, cx, cy, 1, line)
				}
				if !last {
					p.Line(cx, cy, cx, b.Y+b.H, 1, line)
				}
				p.Fill(mgo.Rect{X: cx - 3, Y: cy - 3, W: 6, H: 6}, dot, 3)
			})
			mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, commit.Subject).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
					for _, ref := range commit.Refs {
						mgo.Text(c, ref).FontSize(uikit.FontSize(c, theme.CaptionSize-1)).
							TextColor(k.AccentText).Padding(0, u/2).Radius(theme.ControlRadius).
							Border(theme.BorderWidth, k.Border).SingleLine().Shrink(0)
					}
				})
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					gitCaption(c, shortHash(commit.Hash))
					gitCaption(c, commit.Author).Shrink(1).MinWidth(0).Ellipsis("…")
					gitCaption(c, commit.When.Format(format)).Shrink(0)
				})
			})
		})
	})
	if r.Element.Changed() {
		r.changed = true
		if s.cursor >= 0 && s.cursor < len(commits) {
			s.List.choose(commits[s.cursor].Hash)
		}
	}
	if r.Element.Submitted() && !opts.Disabled {
		r.submitted = true
	}
	return r
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// BlameViewState keeps a BlameView's place across frames.
type BlameViewState struct {
	list mgo.ListState
}

// BlameViewOptions configure a BlameView.
type BlameViewOptions struct {
	// Language picks syntax colors, as "go"; empty is plain.
	Language string
	// TimeFormat lays out commit dates; empty means "2006-01-02".
	TimeFormat string
}

// BlameView shows source lines beside the commit that last touched each.
func BlameView(c *mgo.Context, s *BlameViewState, lines []BlameLine, opts BlameViewOptions) mgo.Element {
	if s == nil {
		panic("kit: BlameView needs a state")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	format := opts.TimeFormat
	if format == "" {
		format = "2006-01-02"
	}
	font := codeFont(c)
	return mgo.List(c, &s.list, len(lines), func(i int) {
		l := lines[i]
		// A run of lines from one commit shows its heading once.
		head := i == 0 || lines[i-1].Hash != l.Hash
		row := mgo.Row(c.Key(i)).FillWidth().Gap(u * 2).AlignItems(mgo.Start)
		row.Children(func() {
			meta := mgo.Column(c).Width(200).Shrink(0).Padding(0, u, 0, u*2)
			meta.Children(func() {
				if !head {
					return
				}
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, shortHash(l.Hash)).Font(theme.MonoFont).
						FontSize(uikit.FontSize(c, theme.CaptionSize)).
						TextColor(k.AccentText).SingleLine().Shrink(0).
						Tooltip(l.Summary)
					gitCaption(c, l.Author).Shrink(1).MinWidth(0).Ellipsis("…")
				})
				gitCaption(c, l.When.Format(format))
			})
			mgo.Text(c, fmt.Sprint(i+1)).Font(theme.MonoFont).
				FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted).
				Width(44).TextAlign(mgo.End).Shrink(0).Unselectable()
			mgo.Text(c, l.Text).Font(font.Family).FontSize(font.Size).
				Grow(1).MinWidth(0).NoWrap().Selectable()
		})
	}).Role(mgo.RoleList).Label("逐行溯源")
}
