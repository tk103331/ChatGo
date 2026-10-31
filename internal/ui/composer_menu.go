package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	acp "github.com/coder/acp-go-sdk"
)

func (cw *ChatWindow) chatMenus(c *mgo.Context, in kit.InputResult) {
	if cw.isACP() {
		if _, ok := kit.SlashCommandMenu(c, in, &cw.draft, cw.acpCommands()).Chosen(); ok {
			in.Input.Focus()
			cw.invalidate()
		}
	} else {
		cmds, tokens := cw.slashSkills()
		if cmd, ok := kit.SlashCommandMenu(c, in, &cw.draft, cmds).Chosen(); ok {
			cw.selectSkill(tokens[cmd.Name])
			in.Input.Focus()
			return
		}
	}
	if it, ok := kit.ContextMentionMenu(c, in, &cw.draft, cw.contextItems(), kit.ContextMentionOptions{}).Chosen(); ok {
		cw.insertContextMention(it)
		in.Input.Focus()
	}
}

func (cw *ChatWindow) acpCommands() []kit.SlashCommand {
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
	out := make([]kit.SlashCommand, 0, len(cmds))
	for _, c := range cmds {
		if c.Name == "" || strings.ContainsAny(c.Name, " \t\n") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Name), "skill:") {
			continue
		}
		out = append(out, kit.SlashCommand{Name: c.Name, Description: c.Description})
	}
	return out
}

func (cw *ChatWindow) slashSkills() ([]kit.SlashCommand, map[string]string) {
	if cw.cur == nil {
		return nil, nil
	}
	ids := cw.slashSkillIDs()
	cmds := make([]kit.SlashCommand, 0, len(ids))
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
		cmds = append(cmds, kit.SlashCommand{Name: token, Description: s.Description})
	}
	return cmds, tokens
}

func (cw *ChatWindow) slashSkillIDs() []string {
	if !cw.skills.loaded {
		cw.loadSkills()
	}
	a, ok := cw.currentAgentDef()
	if !ok {
		out := make([]string, 0, len(cw.skills.list))
		for _, s := range cw.skills.list {
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

// contextButton opens the reference picker from the composer's leading +:
// external files and directories, plus an open Webview when allowed.
func (cw *ChatWindow) contextButton(c *mgo.Context, allowWebview bool) {
	b := kit.IconButton(c, icons.Must("plus"), "添加引用", kit.IconButtonOptions{
		Variant: kit.Ghost,
	})
	open := mgo.Local(b, "chatgo.contextbutton", func() bool { return false })
	mode := mgo.Local(b, "chatgo.contextmode", func() string { return "" })
	if b.Clicked() {
		*open = !*open
		if *open {
			*mode = ""
		}
	}
	title := "添加引用"
	if allowWebview && *mode == "webview" {
		title = "选择 Webview"
	}
	kit.Popover(c, b, open, kit.PopoverOptions{
		Title: title, Width: 360, Up: true,
	}, func() {
		u := mcore.Density(c).Unit()
		k := mcore.Tokens(c)
		caption := mcore.FontSize(c, mtheme.CaptionSize)
		row := func(icon, label, detail string, onClick func()) {
			b := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
				Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).
				Radius(mtheme.ControlRadius).Label(label)
			if b.Hovered() {
				b.Background(k.SurfaceHover)
			}
			b.Children(func() {
				mgo.Icon(c, icons.Must(icon)).Size(14, 14).
					TextColor(k.TextMuted).Shrink(0)
				mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
					mgo.Text(c, label).SingleLine().Ellipsis("…")
					if detail != "" {
						mgo.Text(c, detail).SingleLine().Ellipsis("…").
							FontSize(caption).TextColor(k.TextMuted)
					}
				})
			})
			if b.Clicked() {
				onClick()
			}
		}

		if allowWebview && *mode == "webview" {
			row("chevron-left", "返回", "", func() { *mode = "" })
			for _, tab := range cw.centerTabsOfKind(centerWeb) {
				if tab.Web == nil {
					continue
				}
				web := tab.Web
				row("globe", "webview:"+web.ID, web.Title+" · "+web.URL, func() {
					cw.insertContextMention(kit.ContextItem{
						ID: "webview:" + web.ID, Label: "webview:" + web.ID,
						Kind: kit.ContextWeb,
					})
					*open = false
				})
			}
			return
		}

		row("file", "外部文件", "从系统文件浏览器选择文件", func() {
			*open = false
			cw.pickExternalFiles()
		})
		row("folder", "外部目录", "从系统文件浏览器选择目录", func() {
			*open = false
			cw.pickExternalDirectory()
		})
		if allowWebview && len(cw.centerTabsOfKind(centerWeb)) > 0 {
			row("globe", "Webview", "引用当前打开的网页", func() { *mode = "webview" })
		}
	})
}

// pickExternalFiles opens the system file browser and adds every chosen file.
func (cw *ChatWindow) pickExternalFiles() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: w, Title: "选择外部文件", Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		cw.postMain(func() {
			for _, path := range paths {
				cw.addExternalContext(path)
			}
		})
	}()
}

// pickExternalDirectory opens the system directory browser and adds it.
func (cw *ChatWindow) pickExternalDirectory() {
	cw.pickDirectory("选择外部目录", func(path string) { cw.addExternalContext(path) })
}

func (cw *ChatWindow) addExternalContext(path string) {
	label := cw.workspaceRel(path)
	kind := contextKindOf(filepath.Base(path))
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		kind = kit.ContextDirectory
	}
	cw.insertContextMention(kit.ContextItem{ID: label, Label: label, Kind: kind})
}

func (cw *ChatWindow) contextItems() []kit.ContextItem {
	return cw.contextItemsMatching(mentionQuery(cw.draft))
}

func (cw *ChatWindow) contextItemsMatching(query string) []kit.ContextItem {
	root := cw.fileRoot()
	if cw.mentionRoot != root {
		cw.mentionRoot = root
		cw.mentionFiles = listWorkspaceFiles(root)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return cw.mentionFiles
	}
	out := make([]kit.ContextItem, 0, len(cw.mentionFiles))
	for _, it := range cw.mentionFiles {
		if strings.Contains(strings.ToLower(it.Label), q) {
			out = append(out, it)
		}
	}
	return out
}

func (cw *ChatWindow) insertContextMention(it kit.ContextItem) {
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

func listWorkspaceFiles(root string) []kit.ContextItem {
	var out []kit.ContextItem
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			out = append(out, kit.ContextItem{ID: rel, Label: rel, Kind: kit.ContextDirectory})
			if len(out) >= 300 {
				return filepath.SkipAll
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
		rel = filepath.ToSlash(rel)
		out = append(out, kit.ContextItem{
			ID: rel, Label: rel, Kind: contextKindOf(name),
		})
		if len(out) >= 300 {
			return filepath.SkipAll
		}
		return nil
	})
	return out
}

func contextKindOf(name string) kit.ContextKind {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return kit.ContextImage
	case ".md", ".markdown", ".txt", ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".csv", ".json", ".yaml", ".yml", ".html", ".htm":
		return kit.ContextDoc
	default:
		return kit.ContextFile
	}
}
