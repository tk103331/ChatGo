package gitapp

import (
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// filterBox draws a search field above a list.
func filterBox(c *mgo.Context, query *string, placeholder string) {
	if query == nil {
		return
	}
	in := minput.TextInput(c, query, minput.TextInputOptions{
		Placeholder: placeholder,
		Label:       placeholder,
		Leading: func() {
			mgo.Icon(c, icons.Must("search")).Size(14, 14).
				TextColor(mcore.Tokens(c).TextMuted)
		},
	})
	in.Element.FillWidth()
}

// matchFold reports whether s contains q, ignoring case.
func matchFold(s, q string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(strings.TrimSpace(q)))
}

// filterChanges keeps the files whose path matches the query.
func filterChanges(files []mgit.ChangedFile, q string) []mgit.ChangedFile {
	if strings.TrimSpace(q) == "" {
		return files
	}
	out := make([]mgit.ChangedFile, 0, len(files))
	for _, f := range files {
		if matchFold(f.Path, q) {
			out = append(out, f)
		}
	}
	return out
}

// filterBranches keeps the branches whose name matches the query.
func filterBranches(branches []mgit.Branch, q string) []mgit.Branch {
	if strings.TrimSpace(q) == "" {
		return branches
	}
	out := make([]mgit.Branch, 0, len(branches))
	for _, b := range branches {
		if matchFold(b.Name, q) || matchFold(b.Upstream, q) {
			out = append(out, b)
		}
	}
	return out
}

// filterTags keeps the tags whose name or annotation matches the query.
func filterTags(tags []mgit.GitTag, q string) []mgit.GitTag {
	if strings.TrimSpace(q) == "" {
		return tags
	}
	out := make([]mgit.GitTag, 0, len(tags))
	for _, t := range tags {
		if matchFold(t.Name, q) || matchFold(t.Annotation, q) {
			out = append(out, t)
		}
	}
	return out
}

// filterCommits keeps commmits matching the author, message and time terms.
func filterCommits(commits []mgit.Commit, query, author, timeQuery string) []mgit.Commit {
	query, author, timeQuery = strings.TrimSpace(query), strings.TrimSpace(author), strings.TrimSpace(timeQuery)
	if query == "" && author == "" && timeQuery == "" {
		return commits
	}
	out := make([]mgit.Commit, 0, len(commits))
	for _, cm := range commits {
		if query != "" && !matchFold(cm.Subject, query) && !matchFold(cm.Hash, query) {
			continue
		}
		if author != "" && !matchFold(cm.Author, author) {
			continue
		}
		if timeQuery != "" && !matchTime(cm.When, timeQuery) {
			continue
		}
		out = append(out, cm)
	}
	return out
}

// matchTime accepts a date prefix or a relative age such as 7d or 2h.
func matchTime(t time.Time, q string) bool {
	if t.IsZero() {
		return false
	}
	stamp := t.Format("2006-01-02 15:04")
	if matchFold(stamp, q) {
		return true
	}
	if n, unit, ok := parseAge(q); ok {
		switch unit {
		case 'd':
			return time.Since(t) <= time.Duration(n)*24*time.Hour
		case 'h':
			return time.Since(t) <= time.Duration(n)*time.Hour
		case 'w':
			return time.Since(t) <= time.Duration(n)*7*24*time.Hour
		}
	}
	return false
}

// parseAge reads "7d", "12h" or "2w".
func parseAge(q string) (int, byte, bool) {
	if len(q) < 2 {
		return 0, 0, false
	}
	unit := q[len(q)-1]
	if unit != 'd' && unit != 'h' && unit != 'w' {
		return 0, 0, false
	}
	n, ok := parseInt(q[:len(q)-1])
	if !ok || n <= 0 {
		return 0, 0, false
	}
	return n, unit, true
}

// emptyHint shows a dimmed line when a filter matches nothing.
func emptyHint(c *mgo.Context, msg string) {
	mgo.Text(c, msg).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
		TextColor(mcore.Tokens(c).TextMuted).Padding(mcore.Density(c).Unit() * 2)
}
