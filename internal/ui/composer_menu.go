package ui

import (
	"strings"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	acp "github.com/coder/acp-go-sdk"
)

func (cw *ChatWindow) acpCommands() []mchat.SlashCommand {
	if cw.cur == nil {
		return nil
	}
	cw.acp.mu.Lock()
	rt := cw.acp.convs[cw.cur.ID]
	var cmds []acp.AvailableCommand
	if rt != nil {
		cmds = rt.commands
	}
	cw.acp.mu.Unlock()
	out := make([]mchat.SlashCommand, 0, len(cmds))
	for _, c := range cmds {
		if c.Name == "" || strings.ContainsAny(c.Name, " \t\n") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Name), "skill:") {
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
	if !cw.skills.Loaded {
		cw.loadSkills()
	}
	a, ok := cw.currentAgentDef()
	if !ok {
		out := make([]string, 0, len(cw.skills.List))
		for _, s := range cw.skills.List {
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
	cw.panel.Draft = ""
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
