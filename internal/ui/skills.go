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

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"

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

func skillSourceLabel(source string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(source, home) {
		return "~" + source[len(home):]
	}
	return source
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
	cw.skills.list = out
	cw.skills.loaded = true
	if len(cw.cfg.SkillsEnabled) == 0 && len(out) > 0 {
		for _, s := range out {
			cw.cfg.SkillsEnabled = append(cw.cfg.SkillsEnabled, s.ID)
		}
		_ = config.SaveConfig(cw.cfg)
	}
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
	if !cw.skills.loaded {
		cw.loadSkills()
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "Skill").Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			mgo.Text(c, "共 "+strconv.Itoa(len(cw.skills.list))+" 个").TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			if minput.Button(c, "重新扫描", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("refresh-cw")}).Clicked() {
				cw.loadSkills()
			}
		})
		if len(cw.skills.list) == 0 {
			cw.skillsEmptyState(c)
			return
		}
		mgo.Row(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
			cw.skillsList(c)
			cw.skillViewer(c)
		})
	})
}

func (cw *ChatWindow) skillsEmptyState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("brain")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "未发现 Skill").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "将 SKILL.md 放入以下任意目录后重新扫描").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Column(c).Gap(u).AlignItems(mgo.Start).Children(func() {
			for _, dir := range skillDirs() {
				mgo.Text(c, dir).Font(mtheme.MonoFont).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			}
		})
		if minput.Button(c, "重新扫描", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
			cw.loadSkills()
		}
	})
}

func (cw *ChatWindow) skillsList(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(340).Shrink(0).Gap(u * 2).Children(func() {
		in := minput.TextInput(c, &cw.skills.query, minput.TextInputOptions{Label: "搜索", Placeholder: "搜索 Skill"})
		in.Element.FillWidth()
		items := cw.filteredSkills()
		if len(items) == 0 {
			mgo.Text(c, "没有匹配的 Skill").TextColor(k.TextMuted)
			return
		}
		mgo.List(c, &cw.skills.listState, len(items), func(i int) {
			s := items[i]
			selected := cw.skills.sel == s.ID
			enabled := cw.skillEnabled(s.ID)
			b := mgo.ButtonBase(c).Key(s.ID).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
				Padding(u, u*2).Radius(mtheme.ControlRadius)
			if selected {
				b.Background(k.Selection)
			} else if b.Hovered() {
				b.Background(k.SurfaceHover)
			}
			b.Children(func() {
				mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
					dotColor := k.TextMuted
					if enabled {
						dotColor = k.Success
					}
					mgo.Icon(c, icons.Must("circle")).Size(6, 6).TextColor(dotColor).Shrink(0)
					mgo.Column(c).Gap(u / 2).MinWidth(0).Grow(1).Children(func() {
						mgo.Text(c, s.Name).FontWeight(600).SingleLine()
						if s.Description != "" {
							mgo.Text(c, s.Description).TextColor(k.TextMuted).
								FontSize(mcore.FontSize(c, mtheme.CaptionSize)).MaxLines(1)
						}
					})
				})
			})
			if b.Clicked() {
				cw.skills.sel = s.ID
			}
		}).Grow(1).MinHeight(0)
	})
}

func (cw *ChatWindow) filteredSkills() []skill {
	q := strings.ToLower(strings.TrimSpace(cw.skills.query))
	if q == "" {
		return cw.skills.list
	}
	var out []skill
	for _, s := range cw.skills.list {
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
		for i := range cw.skills.list {
			if cw.skills.list[i].ID == cw.skills.sel {
				s = &cw.skills.list[i]
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
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			agentTag(c, skillSourceLabel(s.Source))
			mgo.Text(c, s.Path).Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).
				SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		})
		dir := filepath.Dir(s.Path)
		if cw.skills.filesFor != s.ID {
			cw.skills.files = listSkillFiles(dir)
			cw.skills.filesFor = s.ID
			cw.skills.fileSel = "SKILL.md"
			cw.skills.fileBodyOf = ""
		}
		if cw.skills.fileSel == "" {
			cw.skills.fileSel = "SKILL.md"
		}
		if len(cw.skills.files) > 0 {
			mgo.Scroll(c).MaxHeight(180).Children(func() {
				cw.skillFileTree(c, dir)
			})
		}
		if cw.skills.fileBodyOf != cw.skills.fileSel {
			data, err := os.ReadFile(filepath.Join(dir, cw.skills.fileSel))
			if err != nil {
				cw.skills.fileBody = "读取失败：" + err.Error()
			} else {
				cw.skills.fileBody = string(data)
			}
			cw.skills.fileBodyOf = cw.skills.fileSel
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if strings.HasSuffix(strings.ToLower(cw.skills.fileSel), ".md") {
				mchat.MarkdownView(c, cw.skills.fileBody)
			} else {
				mgo.Text(c, cw.skills.fileBody).Font(mtheme.MonoFont).FontSize(caption)
			}
		})
	})
}

func (cw *ChatWindow) skillFileTree(c *mgo.Context, dir string) {
	select_ := func(f string) {
		cw.skills.fileSel = f
		cw.skills.fileBodyOf = ""
	}
	skillTreeInto(c, dir, "", cw.skills.files, cw.skills.fileSel, select_)
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
	if !cw.skills.loaded {
		cw.loadSkills()
	}
	var out []string
	for _, n := range names {
		for i := range cw.skills.list {
			if cw.skills.list[i].Name == n {
				out = append(out, cw.skills.list[i].ID)
			}
		}
	}
	return out
}

func (cw *ChatWindow) skillEnabled(id string) bool {
	return containsStr(cw.cfg.SkillsEnabled, id)
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
	cw.cfg.SkillsEnabled = setTool(cw.cfg.SkillsEnabled, id, on)
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
		for i := range cw.skills.list {
			if cw.skills.list[i].ID == id && cw.skills.list[i].Body != "" {
				parts = append(parts, "## Skill: "+cw.skills.list[i].Name+"\n\n"+cw.skills.list[i].Body)
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
			agentSearch(c, &cw.skills.menuQuery, "搜索 Skill").Height(mcore.Density(c).ControlHeight() - 6)
			q := strings.ToLower(strings.TrimSpace(cw.skills.menuQuery))
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
func (cw *ChatWindow) skillByID(id string) *skill {
	if !cw.skills.loaded {
		cw.loadSkills()
	}
	for i := range cw.skills.list {
		if cw.skills.list[i].ID == id {
			return &cw.skills.list[i]
		}
	}
	return nil
}

type skillsState struct {
	list      []skill
	loaded    bool
	sel       string
	query     string
	listState mgo.ListState

	files      []string
	filesFor   string
	fileSel    string
	fileBody   string
	fileBodyOf string
	menuQuery  string
}

func newSkillsState() skillsState { return skillsState{} }
