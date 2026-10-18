// Package skills renders the Skill browser page: a searchable list of
// discovered SKILL.md files on the left and a Markdown viewer with a file
// tree on the right. The State struct is owned by the host window so runtime
// lookups (skill prompts, menus) share the same loaded list.
package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"

	"gopkg.in/yaml.v3"
)

// Skill is one discovered SKILL.md on disk.
type Skill struct {
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

// State is the skill browser's mutable state, shared with the host window.
type State struct {
	List      []Skill
	Loaded    bool
	Sel       string
	Query     string
	ListState mgo.ListState

	Files      []string
	FilesFor   string
	FileSel    string
	FileBody   string
	FileBodyOf string
	MenuQuery  string
}

// Options supplies the page with host data and actions.
type Options struct {
	SkillEnabled    func(id string) bool
	SetSkillEnabled func(id string, on bool)
}

// Dirs returns the directories scanned for SKILL.md files.
func Dirs() []string {
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

// SourceLabel shortens a source path with ~ when under the home directory.
func SourceLabel(source string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(source, home) {
		return "~" + source[len(home):]
	}
	return source
}

// Load scans the skill directories, deduplicating by name.
func Load(state *State) {
	seenPath := map[string]bool{}
	seenName := map[string]bool{}
	var out []Skill
	for _, dir := range Dirs() {
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
	state.List = out
	state.Loaded = true
}

func parseSkill(path, source string, data []byte) Skill {
	s := Skill{ID: path, Path: path, Source: source, Name: filepath.Base(filepath.Dir(path))}
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

// View renders the skills browser page.
func View(c *mgo.Context, state *State, opts Options) {
	if !state.Loaded {
		Load(state)
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "Skill").Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			mgo.Text(c, "共 "+strconv.Itoa(len(state.List))+" 个").TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			if minput.Button(c, "重新扫描", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("refresh-cw")}).Clicked() {
				Load(state)
			}
		})
		if len(state.List) == 0 {
			emptyState(c)
			return
		}
		mgo.Row(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
			list(c, state, opts)
			viewer(c, state, opts)
		})
	})
}

func emptyState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("brain")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "未发现 Skill").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "将 SKILL.md 放入以下任意目录后重新扫描").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Column(c).Gap(u).AlignItems(mgo.Start).Children(func() {
			for _, dir := range Dirs() {
				mgo.Text(c, dir).Font(mtheme.MonoFont).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			}
		})
		if minput.Button(c, "重新扫描", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
			Load(&State{})
		}
	})
}

func list(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(340).Shrink(0).Gap(u * 2).Children(func() {
		in := minput.TextInput(c, &state.Query, minput.TextInputOptions{Label: "搜索", Placeholder: "搜索 Skill"})
		in.Element.FillWidth()
		items := filtered(state)
		if len(items) == 0 {
			mgo.Text(c, "没有匹配的 Skill").TextColor(k.TextMuted)
			return
		}
		mgo.List(c, &state.ListState, len(items), func(i int) {
			s := items[i]
			selected := state.Sel == s.ID
			enabled := opts.SkillEnabled(s.ID)
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
				state.Sel = s.ID
			}
		}).Grow(1).MinHeight(0)
	})
}

func filtered(state *State) []Skill {
	q := strings.ToLower(strings.TrimSpace(state.Query))
	if q == "" {
		return state.List
	}
	var out []Skill
	for _, s := range state.List {
		if strings.Contains(strings.ToLower(s.Name), q) || strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}

func viewer(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		var s *Skill
		for i := range state.List {
			if state.List[i].ID == state.Sel {
				s = &state.List[i]
			}
		}
		if s == nil {
			mgo.Text(c, "从左侧选择一个 Skill 查看详情").TextColor(k.TextMuted)
			return
		}
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, s.Name).Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).Grow(1).SingleLine()
			on := opts.SkillEnabled(s.ID)
			if minput.Switch(c, &on, "启用", minput.SwitchOptions{}).Changed() {
				opts.SetSkillEnabled(s.ID, on)
			}
		})
		if s.Description != "" {
			mgo.Text(c, s.Description).TextColor(k.TextMuted)
		}
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			tag(c, SourceLabel(s.Source))
			mgo.Text(c, s.Path).Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).
				SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		})
		dir := filepath.Dir(s.Path)
		if state.FilesFor != s.ID {
			state.Files = listFiles(dir)
			state.FilesFor = s.ID
			state.FileSel = "SKILL.md"
			state.FileBodyOf = ""
		}
		if state.FileSel == "" {
			state.FileSel = "SKILL.md"
		}
		if len(state.Files) > 0 {
			mgo.Scroll(c).MaxHeight(180).Children(func() {
				fileTree(c, dir, state)
			})
		}
		if state.FileBodyOf != state.FileSel {
			data, err := os.ReadFile(filepath.Join(dir, state.FileSel))
			if err != nil {
				state.FileBody = "读取失败：" + err.Error()
			} else {
				state.FileBody = string(data)
			}
			state.FileBodyOf = state.FileSel
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if strings.HasSuffix(strings.ToLower(state.FileSel), ".md") {
				mchat.MarkdownView(c, state.FileBody)
			} else {
				mgo.Text(c, state.FileBody).Font(mtheme.MonoFont).FontSize(caption)
			}
		})
	})
}

func tag(c *mgo.Context, label string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.AccentText).
		Padding(0, u*1.5).Radius(mtheme.ControlRadius).Border(mtheme.BorderWidth, k.Border).SingleLine()
}

func fileTree(c *mgo.Context, dir string, state *State) {
	onSelect := func(f string) {
		state.FileSel = f
		state.FileBodyOf = ""
	}
	treeInto(c, dir, "", state.Files, state.FileSel, onSelect)
}

// treeInto renders files of a directory as a tree of folders and files.
func treeInto(c *mgo.Context, dir, prefix string, files []string, sel string, onSelect func(string)) {
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
		fileButton(c, prefix+f, sel, onSelect)
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
				treeInto(c, dir, prefix+n+"/", sub[n], sel, onSelect)
			})
		})
	}
}

func fileButton(c *mgo.Context, rel, sel string, onSelect func(string)) {
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

// listFiles lists a skill directory's files, relative and sorted.
func listFiles(dir string) []string {
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
