package ui

import (
	"strconv"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/ui/common"
	skillspkg "chatgo/internal/ui/skills"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// loadSkills scans the skill directories and auto-enables all skills when
// none are configured yet.
func (cw *ChatWindow) loadSkills() {
	skillspkg.Load(&cw.skills)
	if len(cw.cfg.SkillsEnabled) == 0 && len(cw.skills.List) > 0 {
		for _, s := range cw.skills.List {
			cw.cfg.SkillsEnabled = append(cw.cfg.SkillsEnabled, s.ID)
		}
		_ = config.SaveConfig(cw.cfg)
	}
}

func (cw *ChatWindow) skillIDsByName(names []string) []string {
	if !cw.skills.Loaded {
		cw.loadSkills()
	}
	var out []string
	for _, n := range names {
		for i := range cw.skills.List {
			if cw.skills.List[i].Name == n {
				out = append(out, cw.skills.List[i].ID)
			}
		}
	}
	return out
}

func (cw *ChatWindow) skillEnabled(id string) bool {
	return common.ContainsStr(cw.cfg.SkillsEnabled, id)
}

// skillRuntime reports whether a skill is switched on for the active agent;
// unknown skills default on.
func (cw *ChatWindow) skillRuntime(id string) bool {
	if v, ok := cw.skillOn[id]; ok {
		return v
	}
	return true
}

func (cw *ChatWindow) setSkillEnabled(id string, on bool) {
	cw.cfg.SkillsEnabled = common.SetTool(cw.cfg.SkillsEnabled, id, on)
	_ = config.SaveConfig(cw.cfg)
}

// skillPrompt returns the bodies of the active agent's enabled skills.
func (cw *ChatWindow) skillPrompt() string {
	if cw.cur == nil {
		return ""
	}
	var parts []string
	for _, id := range cw.visibleSkillIDs() {
		if !cw.skillRuntime(id) {
			continue
		}
		for i := range cw.skills.List {
			if cw.skills.List[i].ID == id && cw.skills.List[i].Body != "" {
				parts = append(parts, "## Skill: "+cw.skills.List[i].Name+"\n\n"+cw.skills.List[i].Body)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// skillMenu chooses the skills active for the current agent.
func (cw *ChatWindow) skillMenu(c *mgo.Context) {
	ids := cw.visibleSkillIDs()
	if len(ids) == 0 || cw.cur == nil {
		if cw.cur != nil {
			if minput.Button(c, "配置 Skill", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
				cw.goConfigure(sectionSkill)
			}
		}
		return
	}
	k := mcore.Tokens(c)
	small := mcore.FontSize(c, mtheme.CaptionSize)
	count := 0
	for _, id := range ids {
		if cw.skillRuntime(id) {
			count++
		}
	}
	label := "Skills"
	if count > 0 {
		label = "Skills (" + strconv.Itoa(count) + ")"
	}
	b := mgo.ButtonBase(c).MinHeight(mtheme.MinHitSize).Padding(0, mcore.Density(c).Unit()*2).
		Gap(mcore.Density(c).Unit()).Radius(mtheme.ControlRadius).Label(label).
		TextColor(k.TextMuted).Role(mgo.RolePopUpButton)
	open := mgo.Local(b, "skills", func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must("brain")).Size(16, 16)
		if count > 0 {
			mgo.Text(c, strconv.Itoa(count)).FontWeight(600).TextColor(k.AccentText).SingleLine()
		}
	})
	changed := false
	moverlay.Popover(c, b, open, moverlay.PopoverOptions{Title: "Skills", Width: 320}, func() {
		mgo.Column(c).Gap(mcore.Density(c).Unit() * 2).Children(func() {
			common.SearchField(c, &cw.skills.MenuQuery, "搜索 Skill").Height(mcore.Density(c).ControlHeight() - 6)
			q := strings.ToLower(strings.TrimSpace(cw.skills.MenuQuery))
			for _, id := range ids {
				s := cw.skillByID(id)
				if s == nil {
					continue
				}
				if q != "" && !strings.Contains(strings.ToLower(s.Name+" "+s.Description), q) {
					continue
				}
				v := cw.skillRuntime(id)
				mgo.Column(c).Gap(0).Children(func() {
					if minput.Switch(c, &v, s.Name, minput.SwitchOptions{}).Changed() {
						cw.skillOn[id] = v
						changed = true
					}
					if s.Description != "" {
						mgo.Text(c, s.Description).FontSize(small).TextColor(k.TextMuted).MaxLines(1).Ellipsis("…")
					}
				})
			}
		})
	})
	if changed {
		if cw.currentAgent == "" {
			cw.saveChatSelection()
		}
		cw.clientDirty.Store(true)
		c.Invalidate()
	}
}

// skillByID returns the loaded skill with the given ID.
func (cw *ChatWindow) skillByID(id string) *skillspkg.Skill {
	if !cw.skills.Loaded {
		cw.loadSkills()
	}
	for i := range cw.skills.List {
		if cw.skills.List[i].ID == id {
			return &cw.skills.List[i]
		}
	}
	return nil
}
