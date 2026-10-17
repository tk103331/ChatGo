package gitapp

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"time"

	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
)

// gitOut runs git and returns trimmed stdout, or "" on failure.
func gitOut(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitRun runs git and returns stdout or stderr with the error.
func gitRun(dir string, args ...string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return strings.TrimSpace(stderr.String()), err
	}
	return strings.TrimSpace(string(out)), nil
}

func splitRaw(s string) []string {
	s = strings.TrimRight(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// parseCommits reads the last n commits, newest first.
func parseCommits(dir string, n int) []mgit.Commit {
	raw := gitOut(dir, "log", "--pretty=format:%H%x09%P%x09%an%x09%at%x09%s", "-n", strconv.Itoa(n))
	var commits []mgit.Commit
	for _, line := range splitRaw(raw) {
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 5 {
			continue
		}
		sec, _ := strconv.ParseInt(parts[3], 10, 64)
		var parents []string
		if parts[1] != "" {
			parents = strings.Fields(parts[1])
		}
		commits = append(commits, mgit.Commit{
			Hash: parts[0], Parents: parents, Author: parts[2], When: time.Unix(sec, 0), Subject: parts[4],
		})
	}
	return commits
}

// gitBlameLines parses git blame --line-porcelain into per-line commits.
func gitBlameLines(dir, rel string) []mgit.BlameLine {
	raw := gitOut(dir, "blame", "--line-porcelain", "--", rel)
	var lines []mgit.BlameLine
	var cur mgit.BlameLine
	flush := func() {
		if cur.Hash != "" {
			lines = append(lines, cur)
		}
		cur = mgit.BlameLine{}
	}
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case isHexHash(line):
			cur.Hash = line[:40]
		case strings.HasPrefix(line, "author "):
			cur.Author = line[len("author "):]
		case strings.HasPrefix(line, "author-time "):
			sec, _ := strconv.ParseInt(line[len("author-time "):], 10, 64)
			cur.When = time.Unix(sec, 0)
		case strings.HasPrefix(line, "summary "):
			cur.Summary = line[len("summary "):]
		case strings.HasPrefix(line, "\t"):
			cur.Text = line[1:]
			flush()
		}
	}
	return lines
}

func isHexHash(line string) bool {
	if len(line) < 40 || (len(line) > 40 && line[40] != ' ') {
		return false
	}
	for _, ch := range line[:40] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
