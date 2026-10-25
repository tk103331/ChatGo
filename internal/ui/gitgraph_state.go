package ui

import (
	"strconv"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
)

// gitGraphState is the GitGraph tab's own view of a repository, kept apart
// from the Git panel so browsing history never disturbs what the panel shows
// and vice versa: each has its own history page, branch list, changes, filters
// and selection.
type gitGraphState struct {
	// dir is the repository the state was loaded for.
	dir string
	// commits is the history loaded so far, newest first; rows is the graph
	// laid over it.
	commits []kit.Commit
	rows    []graphRow

	branch   string
	branches []kit.Branch
	changes  []kit.ChangedFile
	stat     [2]int
	repo     bool

	loaded bool
	// limit grows as the list nears its end, so history pages in.
	limit   int
	hasMore bool

	// query filters the list by subject, hash or author.
	query string
	// sel is the selected commit's hash, "" when none.
	sel string
	// detail caches each commit's message and files as it is opened.
	detail map[string]*commitInfo
	// list keeps the scroll position and keyboard cursor.
	list mgo.ListState
	// listH is the history's height in the split above the details, which the
	// divider moves.
	listH float32
}

// graphPageSize is how many commits a page of history holds.
const graphPageSize = 100

// resetGraphState points the GitGraph tab at dir, reloading when it changed.
func (cw *ChatWindow) resetGraphState(dir string) {
	if cw.gg.dir == dir {
		return
	}
	cw.gg = gitGraphState{dir: dir, limit: graphPageSize, detail: map[string]*commitInfo{}}
	cw.loadGitGraph()
}

// loadGitGraph reads the repository state the GitGraph tab shows, on its own
// so the Git panel's data is never overwritten.
func (cw *ChatWindow) loadGitGraph() {
	dir := cw.gg.dir
	if dir == "" {
		return
	}
	cw.gg.loaded = false
	go func() {
		repo := gitOut(dir, "rev-parse", "--is-inside-work-tree") == "true"
		var (
			branch   string
			branches []kit.Branch
			changes  []kit.ChangedFile
			stat     [2]int
			commits  []kit.Commit
			hasMore  bool
		)
		if repo {
			branch = gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
			branches = gitLocalBranches(dir, branch)
			changes = gitChangedFiles(dir)
			stat = gitDiffStat(dir)
			commits, hasMore = gitHistoryPage(dir, 0, cw.gg.limit)
		}
		cw.postMain(func() {
			if cw.gg.dir != dir {
				return
			}
			cw.gg.repo, cw.gg.branch, cw.gg.branches = repo, branch, branches
			cw.gg.changes, cw.gg.stat = changes, stat
			cw.gg.commits, cw.gg.hasMore = commits, hasMore
			cw.gg.rows = buildGraph(commits)
			cw.gg.loaded = true
			cw.invalidate()
		})
	}()
}

// growGitGraph loads the next page of history as the list nears its end.
func (cw *ChatWindow) growGitGraph() {
	if !cw.gg.hasMore || !cw.gg.loaded {
		return
	}
	dir := cw.gg.dir
	skip, want := len(cw.gg.commits), graphPageSize
	cw.gg.hasMore = false
	go func() {
		more, hasMore := gitHistoryPage(dir, skip, want)
		cw.postMain(func() {
			if cw.gg.dir != dir || skip != len(cw.gg.commits) {
				return
			}
			cw.gg.hasMore = hasMore
			if len(more) == 0 {
				cw.invalidate()
				return
			}
			cw.gg.commits = append(cw.gg.commits, more...)
			cw.gg.rows = buildGraph(cw.gg.commits)
			cw.gg.limit += len(more)
			cw.invalidate()
		})
	}()
}

// gitHistoryPage reads n commits after skip, newest first, and reports whether
// more remain. Refs are attached so the graph can label branches and tags.
func gitHistoryPage(dir string, skip, n int) ([]kit.Commit, bool) {
	refs := gitCommitRefs(dir)
	// One extra commit tells whether another page exists.
	out := gitOut(dir, "log", "--no-color", "--format="+gitLogFormat,
		"--skip="+strconv.Itoa(skip), "-n", strconv.Itoa(n+1), "HEAD", "--")
	commits := parseGitLog(out, refs)
	hasMore := len(commits) > n
	if hasMore {
		commits = commits[:n]
	}
	return commits, hasMore
}

// gitLogFormat is the record git log prints for the graph: one field per line
// with unit separators and a record separator, so a body never breaks a row.
const gitLogFormat = "%H%x1f%h%x1f%P%x1f%an%x1f%at%x1f%D%x1f%s%x1f%b%x1e"

// parseGitLog turns git log output into commits, attaching the refs that point
// at each one.
func parseGitLog(out string, refs map[string][]string) []kit.Commit {
	var commits []kit.Commit
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}
		f := strings.SplitN(record, "\x1f", 8)
		if len(f) < 8 {
			continue
		}
		sec, _ := strconv.ParseInt(f[4], 10, 64)
		commit := kit.Commit{
			Hash:    f[0],
			Author:  f[3],
			When:    time.Unix(sec, 0),
			Subject: f[6],
			Refs:    refs[f[0]],
		}
		if f[2] != "" {
			commit.Parents = strings.Fields(f[2])
		}
		for _, decorated := range strings.Split(f[5], ", ") {
			name := strings.TrimSpace(decorated)
			if name == "" || name == "HEAD" || strings.HasPrefix(name, "HEAD -> ") {
				continue
			}
			commit.Refs = appendUnique(commit.Refs, name)
		}
		commits = append(commits, commit)
	}
	return commits
}

// appendUnique adds a value to a slice unless it is already there.
func appendUnique(values []string, v string) []string {
	for _, existing := range values {
		if existing == v {
			return values
		}
	}
	return append(values, v)
}

// graphVisibleRows is the history the tab lists, after filtering.
func (cw *ChatWindow) graphVisibleRows() []graphRow {
	q := strings.ToLower(strings.TrimSpace(cw.gg.query))
	if q == "" {
		return cw.gg.rows
	}
	out := make([]graphRow, 0, len(cw.gg.rows))
	for _, row := range cw.gg.rows {
		c := row.Commit
		if strings.Contains(strings.ToLower(c.Subject), q) ||
			strings.HasPrefix(c.Hash, q) ||
			strings.Contains(strings.ToLower(c.Author), q) {
			out = append(out, row)
		}
	}
	return out
}

// graphRow is one row of the drawn history: a commit and where its branch
// lines run across the graph column.
type graphRow struct {
	Commit kit.Commit
	// Lane is the column the commit's dot sits in.
	Lane int
	// Lanes is how many columns this row needs.
	Lanes int
	// Segments are the lines this row draws through its own height.
	Segments []graphSegment
}

// graphSegment is one line across a row: from column From at the row's top
// edge to column To at its bottom edge.
type graphSegment struct {
	From, To int
	// Merging marks a second or later parent's line, running to another
	// branch.
	Merging bool
}

// laneWidth is the width one branch column takes, in DIPs.
const laneWidth float32 = 14

// maxGraphLanes caps how wide the graph column grows, so a history with many
// open branches still leaves room for the subjects.
const maxGraphLanes = 10

// buildGraph assigns a column to every commit and works out the lines joining
// them, walking the history newest first as git log prints it.
//
// active holds, per column, the hash that column waits to draw: a commit takes
// the column already waiting for it, else the first free one. Its first parent
// keeps that column and later parents open new ones, so a merge fans out
// sideways rather than folding back into the line above.
func buildGraph(commits []kit.Commit) []graphRow {
	rows := make([]graphRow, len(commits))
	var active []string

	// firstFree is the lowest idle column, appending one when every column is
	// waiting for a commit.
	firstFree := func() int {
		for col, want := range active {
			if want == "" {
				return col
			}
		}
		active = append(active, "")
		return len(active) - 1
	}

	for i, commit := range commits {
		// The column already waiting for this commit, else a free one.
		lane := -1
		for col, want := range active {
			if want == commit.Hash {
				lane = col
				break
			}
		}
		if lane < 0 {
			lane = firstFree()
		}

		// Lines of the columns already in flight pass through this row.
		var segments []graphSegment
		for col, want := range active {
			if want != "" && col != lane {
				segments = append(segments, graphSegment{From: col, To: col})
			}
		}

		// The first parent keeps this column; the rest fan out sideways.
		parents := commit.Parents
		active[lane] = ""
		if len(parents) > 0 {
			active[lane] = parents[0]
			segments = append(segments, graphSegment{From: lane, To: lane})
		}
		for _, parent := range parents[min(1, len(parents)):] {
			col := firstFree()
			active[col] = parent
			segments = append(segments, graphSegment{From: lane, To: col, Merging: true})
		}

		for len(active) > 0 && active[len(active)-1] == "" {
			active = active[:len(active)-1]
		}
		lanes := lane + 1
		if len(active) > lanes {
			lanes = len(active)
		}
		rows[i] = graphRow{Commit: commit, Lane: lane, Lanes: lanes, Segments: segments}
	}
	return rows
}

// graphWidth is the width the graph column needs for the given rows.
func graphWidth(rows []graphRow) float32 {
	lanes := 1
	for _, row := range rows {
		if row.Lanes > lanes {
			lanes = row.Lanes
		}
	}
	if lanes > maxGraphLanes {
		lanes = maxGraphLanes
	}
	return float32(lanes)*laneWidth + laneWidth
}

// findCommit looks a commit up by hash in a history slice.
func findCommit(commits []kit.Commit, hash string) (kit.Commit, bool) {
	for _, commit := range commits {
		if commit.Hash == hash {
			return commit, true
		}
	}
	return kit.Commit{}, false
}
