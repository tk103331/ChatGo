package chatpanel

import (
	"io/fs"
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
)

// menus overlays the slash-command and @-mention menus on the composer.
func (p *Panel) menus(c *mgo.Context, in minput.InputResult, opts Options) {
	if opts.SlashCommands != nil {
		if cmd, ok := mchat.SlashCommandMenu(c, in, &p.Draft, opts.SlashCommands()).Chosen(); ok {
			if opts.OnSlashCommand != nil {
				opts.OnSlashCommand(cmd.Name)
			}
			in.Input.Focus()
			if opts.Invalidate != nil {
				opts.Invalidate()
			}
			return
		}
	}
	if opts.ContextRoot != nil {
		root := opts.ContextRoot()
		if root != "" {
			if it, ok := mchat.ContextMentionMenu(c, in, &p.Draft, p.contextItems(root), mchat.ContextMentionOptions{}).Chosen(); ok {
				p.InsertMention(it)
				in.Input.Focus()
				if opts.Invalidate != nil {
					opts.Invalidate()
				}
			}
		}
	}
}

// contextItems returns the @-mentionable files under root, filtered by the
// current @-query in the draft. The file list is cached per root.
func (p *Panel) contextItems(root string) []mchat.ContextItem {
	if p.mentionRoot != root {
		p.mentionRoot = root
		p.mentionFiles = listWorkspaceFiles(root)
	}
	q := strings.ToLower(mentionQuery(p.Draft))
	if q == "" {
		return p.mentionFiles
	}
	out := make([]mchat.ContextItem, 0, len(p.mentionFiles))
	for _, it := range p.mentionFiles {
		if strings.Contains(strings.ToLower(it.Label), q) {
			out = append(out, it)
		}
	}
	return out
}

// mentionQuery extracts the text after the last @ that starts a word.
func mentionQuery(s string) string {
	at := strings.LastIndexByte(s, '@')
	if at < 0 {
		return ""
	}
	q := s[at+1:]
	if strings.ContainsAny(q, " \t\n") {
		return ""
	}
	if at > 0 && !strings.ContainsRune(" \t\n", rune(s[at-1])) {
		return ""
	}
	return q
}

// listWorkspaceFiles walks root and returns up to 300 file entries,
// skipping hidden directories and files.
func listWorkspaceFiles(root string) []mchat.ContextItem {
	var out []mchat.ContextItem
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, mchat.ContextItem{ID: rel, Label: rel, Kind: mchat.ContextFile})
		if len(out) >= 300 {
			return filepath.SkipAll
		}
		return nil
	})
	return out
}
