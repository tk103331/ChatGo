package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/data"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "github.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"

	"gopkg.in/yaml.v3"
)

// skill is one discovered SKILL.md on disk.
type skill struct {
	ID          string
	Name        string
	Description string
	Path        string
	Source      string
	Body        string
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func skillDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".agents", "skills"))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, ".agents", "skills"), filepath.Join(wd, ".claude", "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, d := range []string{".claude/skills", ".opencode/skill", ".opencode/skills"} {
			dirs = append(dirs, filepath.Join(home, d))
		}
	}
	return dirs
}

// loadSkills scans the skill directories, deduplicating by name.
func (cw *ChatWindow) loadSkills() {
	seenPath := map[string]bool{}
	seenName := map[string]bool{}
	var out []skill
	for _, dir := range skillDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name(), "SKILL.md")
			data, err := os.ReadFile(path)
			if err != nil || seenPath[path] {
				continue
			}
			seenPath[path] = true
			s := parseSkill(path, dir, data)
			key := strings.ToLower(s.Name)
			if seenName[key] {
				continue
			}
			seenName[key] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	cw.skills = out
	cw.skillsLoaded = true
}

func parseSkill(path, source string, data []byte) skill {
	s := skill{ID: path, Path: path, Source: source, Name: filepath.Base(filepath.Dir(path))}
	body := strings.TrimSpace(string(data))
	if strings.HasPrefix(body, "---") {
		rest := body[3:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			var f skillFrontmatter
			if err := yaml.Unmarshal([]byte(rest[:i]), &f); err == nil {
				if f.Name != "" {
					s.Name = f.Name
				}
				s.Description = f.Description
			}
			body = strings.TrimSpace(rest[i+4:])
		}
	}
	s.Body = body
	return s
}

func (cw *ChatWindow) skillsPage(c *mgo.Context) {
	if !cw.skillsLoaded {
		cw.loadSkills()
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "Skill").Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			mgo.Text(c, "共 "+strconv.Itoa(len(cw.skills))+" 个").TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			if minput.Button(c, "重新扫描", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cw.loadSkills()
			}
		})
		mgo.Row(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
			cw.skillsList(c)
			cw.skillViewer(c)
		})
	})
}

func (cw *ChatWindow) skillsList(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(340).Shrink(0).Gap(u * 2).Children(func() {
		in := minput.TextInput(c, &cw.skillsQuery, minput.TextInputOptions{Label: "搜索", Placeholder: "搜索 Skill"})
		in.Element.FillWidth()
		items := cw.filteredSkills()
		mgo.List(c, &cw.skillsListState, len(items), func(i int) {
			s := items[i]
			selected := cw.skillsSel == s.ID
			b := mgo.ButtonBase(c).Key(s.ID).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
				Padding(u, u*2).Radius(mtheme.ControlRadius)
			if selected {
				b.Background(k.Selection)
			}
			b.Children(func() {
				mgo.Column(c).Gap(u / 2).MinWidth(0).Children(func() {
					mgo.Text(c, s.Name).FontWeight(600).SingleLine()
					if s.Description != "" {
						mgo.Text(c, s.Description).TextColor(k.TextMuted).
							FontSize(mcore.FontSize(c, mtheme.CaptionSize)).MaxLines(1)
					}
				})
			})
			if b.Clicked() {
				cw.skillsSel = s.ID
			}
		}).Grow(1).MinHeight(0)
	})
}

func (cw *ChatWindow) filteredSkills() []skill {
	q := strings.ToLower(strings.TrimSpace(cw.skillsQuery))
	if q == "" {
		return cw.skills
	}
	var out []skill
	for _, s := range cw.skills {
		if strings.Contains(strings.ToLower(s.Name), q) || strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}

func (cw *ChatWindow) skillViewer(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		var s *skill
		for i := range cw.skills {
			if cw.skills[i].ID == cw.skillsSel {
				s = &cw.skills[i]
			}
		}
		if s == nil {
			mgo.Text(c, "从左侧选择一个 Skill 查看详情").TextColor(k.TextMuted)
			return
		}
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, s.Name).Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).Grow(1).SingleLine()
			on := cw.skillEnabled(s.ID)
			if minput.Switch(c, &on, "启用", minput.SwitchOptions{}).Changed() {
				cw.setSkillEnabled(s.ID, on)
			}
		})
		if s.Description != "" {
			mgo.Text(c, s.Description).TextColor(k.TextMuted)
		}
		mgo.Text(c, s.Path).Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).
			SingleLine().Ellipsis("…")
		dir := filepath.Dir(s.Path)
		if cw.skillFilesFor != s.ID {
			cw.skillFiles = listSkillFiles(dir)
			cw.skillFilesFor = s.ID
			cw.skillFileSel = "SKILL.md"
			cw.skillFileBodyOf = ""
		}
		if cw.skillFileSel == "" {
			cw.skillFileSel = "SKILL.md"
		}
		if len(cw.skillFiles) > 0 {
			mgo.Scroll(c).MaxHeight(180).Children(func() {
				cw.skillFileTree(c, dir)
			})
		}
		if cw.skillFileBodyOf != cw.skillFileSel {
			data, err := os.ReadFile(filepath.Join(dir, cw.skillFileSel))
			if err != nil {
				cw.skillFileBody = "读取失败：" + err.Error()
			} else {
				cw.skillFileBody = string(data)
			}
			cw.skillFileBodyOf = cw.skillFileSel
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if strings.HasSuffix(strings.ToLower(cw.skillFileSel), ".md") {
				mchat.MarkdownView(c, cw.skillFileBody)
			} else {
				mgo.Text(c, cw.skillFileBody).Font(mtheme.MonoFont).FontSize(caption)
			}
		})
	})
}

func (cw *ChatWindow) skillFileTree(c *mgo.Context, dir string) {
	select_ := func(f string) {
		cw.skillFileSel = f
		cw.skillFileBodyOf = ""
	}
	skillTreeInto(c, dir, "", cw.skillFiles, cw.skillFileSel, select_)
}

// skillTreeInto renders files of a directory as a tree of folders and files.
func skillTreeInto(c *mgo.Context, dir, prefix string, files []string, sel string, onSelect func(string)) {
	u := mcore.Density(c).Unit()
	var root []string
	sub := map[string][]string{}
	for _, f := range files {
		if i := strings.IndexByte(f, '/'); i >= 0 {
			sub[f[:i]] = append(sub[f[:i]], f[i+1:])
		} else {
			root = append(root, f)
		}
	}
	sort.Strings(root)
	for _, f := range root {
		skillFileButton(c, prefix+f, sel, onSelect)
	}
	names := make([]string, 0, len(sub))
	for n := range sub {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		n := name
		open := mgo.Local(c.Root(), "skilldir:"+dir+"/"+prefix+n, func() bool { return true })
		data.Collapsible(c, open, data.CollapsibleOptions{Title: n}, func() {
			mgo.Column(c).Margin(0, 0, 0, u*3).Children(func() {
				skillTreeInto(c, dir, prefix+n+"/", sub[n], sel, onSelect)
			})
		})
	}
}

func skillFileButton(c *mgo.Context, rel, sel string, onSelect func(string)) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).Row().FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).Radius(mtheme.ControlRadius).Label(rel)
	switch {
	case sel == rel:
		b.Background(k.Selection)
	case b.Hovered():
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must("file-text")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, filepath.Base(rel)).SingleLine()
	})
	if b.Clicked() {
		onSelect(rel)
	}
}

// listSkillFiles lists a skill directory's files, relative and sorted.
func listSkillFiles(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != dir && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if rel, err := filepath.Rel(dir, path); err == nil {
			files = append(files, rel)
		}
		if len(files) >= 300 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func (cw *ChatWindow) skillIDsByName(names []string) []string {
	if !cw.skillsLoaded {
		cw.loadSkills()
	}
	var out []string
	for _, n := range names {
		for i := range cw.skills {
			if cw.skills[i].Name == n {
				out = append(out, cw.skills[i].ID)
			}
		}
	}
	return out
}

func (cw *ChatWindow) skillEnabled(id string) bool {
	return containsStr(cw.cfg.SkillsEnabled, id)
}

func (cw *ChatWindow) setSkillEnabled(id string, on bool) {
	cw.cfg.SkillsEnabled = setTool(cw.cfg.SkillsEnabled, id, on)
	_ = config.SaveConfig(cw.cfg)
}

// skillPrompt returns the bodies of the conversation's selected skills.
func (cw *ChatWindow) skillPrompt() string {
	if cw.cur == nil || len(cw.cur.Skills) == 0 {
		return ""
	}
	var parts []string
	for _, id := range cw.cur.Skills {
		for i := range cw.skills {
			if cw.skills[i].ID == id && cw.skills[i].Body != "" {
				parts = append(parts, "## Skill: "+cw.skills[i].Name+"\n\n"+cw.skills[i].Body)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// skillMenu chooses the skills injected into the current conversation.
func (cw *ChatWindow) skillMenu(c *mgo.Context) {
	if len(cw.skills) == 0 || cw.cur == nil {
		return
	}
	count := len(cw.cur.Skills)
	label := "Skills"
	if count > 0 {
		label = "Skills (" + strconv.Itoa(count) + ")"
	}
	b := mgo.ButtonBase(c).MinHeight(mtheme.MinHitSize).Padding(0, mcore.Density(c).Unit()*2).
		Gap(mcore.Density(c).Unit()).Radius(mtheme.ControlRadius).Label(label).
		TextColor(mcore.Tokens(c).TextMuted).Role(mgo.RolePopUpButton)
	open := mgo.Local(b, "skills", func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must("brain")).Size(16, 16)
		if count > 0 {
			mgo.Text(c, strconv.Itoa(count)).FontWeight(600).TextColor(mcore.Tokens(c).AccentText).SingleLine()
		}
	})
	moverlay.PopupMenuOpen(c, b, open, func(m *moverlay.PopupMenu) {
		for i := range cw.skills {
			s := cw.skills[i]
			on := containsStr(cw.cur.Skills, s.ID)
			mark := "  "
			if on {
				mark = "✓ "
			}
			if m.Item(mark+s.Name, moverlay.PopupMenuItemOptions{}) {
				cw.cur.Skills = setTool(cw.cur.Skills, s.ID, !on)
				_ = cw.convManager.SaveConversation(cw.cur)
			}
		}
	})
}
