package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// searchHit is one line of a file matching the search query.
type searchHit struct {
	Path string
	Line int
	Text string
}

// codeSearchState is the Code Search tab: the workspace it searched, the
// query and the last results, searched in the background so the interface
// stays responsive.
type codeSearchState struct {
	// ws is the directory the results were searched in; the body drops them
	// the moment the workspace moves, so a tab in another project never
	// lists the previous one's matches.
	ws        string
	query     string
	results   []searchHit
	searching bool
}

// resetCodeSearch points the CodeSearch tab at ws, discarding the previous
// directory's query and results when it changed.
func (cw *ChatWindow) resetCodeSearch(ws string) {
	if cw.search.ws == ws {
		return
	}
	cw.search = codeSearchState{ws: ws}
}

// openCodeSearchTab opens the workspace search as a center tab, one at a time.
func (cw *ChatWindow) openCodeSearchTab(pane int) {
	if t := cw.focusCenterTab("codesearch"); t != nil {
		return
	}
	cw.openCenterTab(pane, &centerTab{
		ID: "codesearch", Kind: centerCodeSearch, Title: "CodeSearch",
	})
}

// codeSearchBody is the Code Search tab: a query box and the hits below it.
func (cw *ChatWindow) codeSearchBody(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cw.resetCodeSearch(cw.workspace)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			in := kit.TextInput(c, &cw.search.query, kit.TextInputOptions{
				Placeholder: "搜索工作区代码，回车搜索",
				Label:       "CodeSearch",
			})
			in.Element.Grow(1).MinWidth(0)
			if in.Submitted() {
				cw.runCodeSearch(strings.TrimSpace(cw.search.query))
			}
			btn := kit.Button(c, "搜索", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("search"),
				Disabled: cw.search.searching || strings.TrimSpace(cw.search.query) == "",
			})
			if btn.Clicked() {
				cw.runCodeSearch(strings.TrimSpace(cw.search.query))
			}
		})
		switch {
		case cw.search.searching:
			mgo.Row(c).Fill().Center().Gap(u).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "搜索中"})
				mgo.Text(c, "正在搜索…").TextColor(k.TextMuted).FontSize(caption)
			})
		case len(cw.search.results) == 0:
			mgo.Column(c).Fill().Center().Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("search")).Size(28, 28).TextColor(k.TextMuted)
				hint := "输入关键词搜索工作区代码"
				if cw.search.query != "" {
					hint = "没有找到匹配的内容"
				}
				mgo.Text(c, hint).TextColor(k.TextMuted).FontSize(caption)
			})
		default:
			mgo.Text(c, codeSearchSummary(cw.search.results)).
				FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				mgo.Column(c).FillWidth().Gap(2).Children(func() {
					for _, group := range groupSearchHits(cw.search.results) {
						cw.searchHitGroup(c, group)
					}
				})
			})
		}
	})
}

// codeSearchSummary describes the hits found.
func codeSearchSummary(hits []searchHit) string {
	files := map[string]bool{}
	for _, hit := range hits {
		files[hit.Path] = true
	}
	return itoa(len(hits)) + " 处匹配，" + itoa(len(files)) + " 个文件"
}

// fileHitGroup is a file's hits, kept in one collapsible row of the results.
type fileHitGroup struct {
	Path string
	Hits []searchHit
}

// groupSearchHits gathers hits by file, in the files' order.
func groupSearchHits(hits []searchHit) []fileHitGroup {
	var groups []fileHitGroup
	byPath := map[string]int{}
	for i := range hits {
		path := hits[i].Path
		g, ok := byPath[path]
		if !ok {
			g = len(groups)
			byPath[path] = g
			groups = append(groups, fileHitGroup{Path: path})
		}
		groups[g].Hits = append(groups[g].Hits, hits[i])
	}
	return groups
}

// searchHitGroup is one file's hits: the file opens and closes as a row, and
// each line below it opens the file at that line.
func (cw *ChatWindow) searchHitGroup(c *mgo.Context, group fileHitGroup) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	rel := cw.workspaceRel(group.Path)
	open := mgo.Local(c.Root(), "codesearch.open:"+group.Path, func() bool { return true })
	kit.Collapsible(c, open, kit.CollapsibleOptions{
		Title: rel,
		Trigger: func() {
			mgo.Text(c, rel).Font(mtheme.MonoFont).FontSize(caption).
				TextColor(k.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			mgo.Text(c, itoa(len(group.Hits))+" 处").FontSize(caption).
				TextColor(k.TextMuted).Shrink(0)
		},
	}, func() {
		mgo.Column(c).Margin(0, 0, 0, u*2).Gap(1).Children(func() {
			for i := range group.Hits {
				cw.searchHitLine(c, &group.Hits[i])
			}
		})
	})
}

// searchHitLine is one matching line of a file: its number and text, opening
// the file when double-clicked.
func (cw *ChatWindow) searchHitLine(c *mgo.Context, hit *searchHit) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	b := mgo.ButtonBase(c).Row().FillWidth().Padding(u/2, u).Gap(u).
		AlignItems(mgo.Start).Justify(mgo.Start).Radius(mtheme.ControlRadius).
		Label(hit.Path).Tooltip("双击打开文件")
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		// The gutter is sized from the font, not from the density unit: a
		// monospace digit is about 0.62em, so six digits need ~3.7em and a
		// unit-based width silently clips four-digit lines.
		mgo.Text(c, itoa(hit.Line)).Font(mtheme.MonoFont).FontSize(caption).
			TextColor(k.Accent).Shrink(0).Width(caption * 3.8).
			Justify(mgo.End).SingleLine()
		mgo.Text(c, strings.TrimSpace(hit.Text)).FontSize(caption).TextColor(k.TextMuted).
			SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
	})
	if b.DoubleClicked() {
		cw.openPreviewAt(hit.Path, hit.Line)
	}
}

// runCodeSearch searches the workspace for query in the background, capping
// the hits so a common word does not flood the list.
func (cw *ChatWindow) runCodeSearch(query string) {
	if query == "" {
		cw.search.results = nil
		return
	}
	cw.search.searching = true
	root := cw.fileRoot()
	want := cw.workspace
	cw.search.ws = want
	go func() {
		results := searchWorkspace(root, query)
		cw.postMain(func() {
			// The workspace may have moved while the search ran; a hit list
			// of a directory that is no longer active is dropped.
			if cw.search.ws != want || cw.workspace != want {
				return
			}
			cw.search.searching = false
			cw.search.results = results
			cw.invalidate()
		})
	}()
}

// maxSearchHits caps how many matches a search shows, keeping a common word
// from building a list no one can read.
const maxSearchHits = 500

// searchWorkspace walks root and matches query against the text files it
// finds, skipping build output and hidden directories.
func searchWorkspace(root, query string) []searchHit {
	q := strings.ToLower(query)
	var hits []searchHit
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if path != root && isSearchSkippedDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSearchableFile(info.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || bytesContainNull(data) {
			return nil
		}
		for n, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), q) {
				hits = append(hits, searchHit{Path: path, Line: n + 1, Text: line})
				if len(hits) >= maxSearchHits {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	return hits
}

// isSearchSkippedDir reports a directory a search should not walk into.
func isSearchSkippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".idea", ".vscode":
		return true
	}
	return false
}

// isSearchableFile reports whether a file is text worth searching. Known
// source extensions always count; everything else is checked for null bytes.
func isSearchableFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rs", ".java", ".c", ".h",
		".cpp", ".hpp", ".json", ".yaml", ".yml", ".toml", ".sh", ".bash", ".md",
		".markdown", ".sql", ".html", ".css", ".xml", ".txt", ".mod", ".sum":
		return true
	}
	return false
}

// bytesContainNull reports whether data holds a null byte, the mark of a
// binary file that would only garble a search.
func bytesContainNull(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
