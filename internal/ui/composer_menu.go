package ui

import (
	"io/fs"
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	acp "github.com/coder/acp-go-sdk"
)

func (cw *ChatWindow) chatMenus(c *mgo.Context, in minput.InputResult) {
	if cw.isACP() {
		if _, ok := mchat.SlashCommandMenu(c, in, &cw.draft, cw.acpCommands()).Chosen(); ok {
			cw.invalidate()
		}
	} else {
		cmds, tokens := cw.slashSkills()
		if cmd, ok := mchat.SlashCommandMenu(c, in, &cw.draft, cmds).Chosen(); ok {
			cw.selectSkill(tokens[cmd.Name])
			return
		}
	}
	if it, ok := mchat.ContextMentionMenu(c, in, &cw.draft, cw.contextItems(), mchat.ContextMentionOptions{}).Chosen(); ok {
		cw.insertContextMention(it)
	}
}

func (cw *ChatWindow) acpCommands() []mchat.SlashCommand {
	if cw.cur == nil {
		return nil
	}
	cw.acpMu.Lock()
	rt := cw.acpConvs[cw.cur.ID]
	var cmds []acp.AvailableCommand
	if rt != nil {
		cmds = rt.commands
	}
	cw.acpMu.Unlock()
	out := make([]mchat.SlashCommand, 0, len(cmds))
	for _, c := range cmds {
		if c.Name == "" || strings.ContainsAny(c.Name, " \t\n") {
			continue
		}
		out = append(out, mchat.SlashCommand{Name: c.Name, Description: c.Description})
	}
	return out
}

func (cw *ChatWindow) slashSkills() ([]mchat.SlashCommand, map[string]string) {
	if cw.cur == nil {
		return nil, nil
	}
	ids := cw.slashSkillIDs()
	cmds := make([]mchat.SlashCommand, 0, len(ids))
	tokens := make(map[string]string, len(ids))
	for _, id := range ids {
		s := cw.skillByID(id)
		if s == nil {
			continue
		}
		token := skillToken(s.Name)
		if token == "" {
			continue
		}
		if _, dup := tokens[token]; dup {
			continue
		}
		tokens[token] = id
		cmds = append(cmds, mchat.SlashCommand{Name: token, Description: s.Description})
	}
	return cmds, tokens
}

func (cw *ChatWindow) slashSkillIDs() []string {
	if !cw.skillsLoaded {
		cw.loadSkills()
	}
	a, ok := cw.currentAgentDef()
	if !ok {
		out := make([]string, 0, len(cw.skills))
		for _, s := range cw.skills {
			out = append(out, s.ID)
		}
		return out
	}
	var out []string
	for _, id := range cw.skillIDsByName(a.Skills) {
		if cw.skillByID(id) != nil {
			out = append(out, id)
		}
	}
	return out
}

func skillToken(name string) string {
	t := strings.TrimSpace(name)
	return strings.NewReplacer(" ", "-", "\t", "-", "\n", "-").Replace(t)
}

func (cw *ChatWindow) selectSkill(id string) {
	cw.draft = ""
	if id == "" {
		return
	}
	s := cw.skillByID(id)
	name := id
	if s != nil {
		name = s.Name
	}
	if cw.skillOn == nil {
		cw.skillOn = map[string]bool{}
	}
	cw.skillOn[id] = true
	if !cw.skillEnabled(id) {
		cw.setSkillEnabled(id, true)
	}
	if cw.currentAgent == "" {
		cw.saveChatSelection()
	}
	cw.clientDirty.Store(true)
	cw.toast("已启用 Skill："+name, mcore.SeveritySuccess)
	cw.invalidate()
}

func (cw *ChatWindow) contextItems() []mchat.ContextItem {
	root := cw.fileRoot()
	if cw.mentionRoot != root {
		cw.mentionRoot = root
		cw.mentionFiles = listWorkspaceFiles(root)
	}
	q := strings.ToLower(mentionQuery(cw.draft))
	if q == "" {
		return cw.mentionFiles
	}
	out := make([]mchat.ContextItem, 0, len(cw.mentionFiles))
	for _, it := range cw.mentionFiles {
		if strings.Contains(strings.ToLower(it.Label), q) {
			out = append(out, it)
		}
	}
	return out
}

func (cw *ChatWindow) insertContextMention(it mchat.ContextItem) {
	if cw.draft != "" && !strings.HasSuffix(cw.draft, " ") {
		cw.draft += " "
	}
	cw.draft += "@" + it.Label + " "
	cw.invalidate()
}

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
