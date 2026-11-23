// The "新会话" tab: one place to choose an agent, a project, a worktree (or
// create one) and a branch, then type a message that creates the
// conversation. The sidebar's new-workspace, new-worktree and new-conversation
// entries all open this tab, so starting a session follows the same shape
// wherever it is asked for.
package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// createNewWorktree is the worktree option value that asks for a new tree,
// after which the branch picker appears. It is not a path, so it can never
// collide with one.
const createNewWorktree = "\x00new"

// createState is the "新会话" tab's choices: the agent, the project, and how
// the conversation's directory is chosen — an existing worktree of the
// project, or a new one whose branch is also chosen here.
type createState struct {
	agent string
	ws    string
	// wt is the chosen worktree path; ws itself when the main tree is
	// picked, and createNewWorktree when "新建 worktree…" is.
	wt string
	// branch names the tree to create when wt is createNewWorktree.
	branch string
	// newBranch creates the branch instead of checking out an existing one.
	newBranch bool
	// acpConfig holds the ACP session's option choices made here: a config id
	// to the value picked for it. They ride the new conversation, so its
	// session starts with them.
	acpConfig map[string]string
	// pickerW is the width the choice pickers take this frame, measured from
	// the row's last frame so every picker shares the row however narrow
	// the pane is. Zero until createSelectors first draws.
	pickerW float32
}

// openCreateTab opens the "新会话" tab in the focused pane, at most one at a
// time.
func (cw *ChatWindow) openCreateTab() {
	if cw.focusCenterTab("create") != nil {
		return
	}
	if cw.create.ws == "" && cw.workspace != "" {
		cw.create.ws = cw.workspace
	}
	cw.openCenterTab(cw.center.focus, &centerTab{
		ID: "create", Kind: centerCreate, Title: "新会话",
	})
}

// openCreateTabIn is openCreateTab with the project preselected, as the
// sidebar's project header asks for it.
func (cw *ChatWindow) openCreateTabIn(ws string) {
	if ws != "" {
		cw.create.ws = ws
		// The tab picks the new conversation's directory, so the window moves
		// to it as well: the tab must never list another project's tabs next
		// to the sessions it is about to start here.
		cw.pointAtWorkspace(ws)
	}
	cw.openCreateTab()
}

// createIsACP reports whether the chosen agent is an ACP coding CLI, whose
// footer shows the connection's static config instead of the model and tool
// pickers.
func (cw *ChatWindow) createIsACP() bool {
	return strings.HasPrefix(cw.create.agent, "acp:")
}

// createIsExternalAgent reports whether the chosen agent is a remote A2A or
// AG-UI agent, whose own runtime owns the model and tools.
func (cw *ChatWindow) createIsExternalAgent() bool {
	return strings.HasPrefix(cw.create.agent, "a2a:") || strings.HasPrefix(cw.create.agent, "agui:")
}

// viewportCenter is a Scroll that centers its content when the viewport is
// taller than it and scrolls when it is shorter. A scroll hands its content
// the content's own height, so the column's minimum height follows the
// scroll's measured viewport height from the previous frame: that gives the
// Justify(Center) a measure to distribute in, and the content still grows
// past it on a short window.
func viewportCenter(c *mgo.Context, key string, children func()) {
	scroll := mgo.Scroll(c).Fill()
	h := mgo.Local(scroll, "chatgo:viewport-center:"+key, func() float32 { return 0 })
	scroll.Children(func() {
		mgo.Column(c).MinHeight(*h).FillWidth().Justify(mgo.Center).
			AlignItems(mgo.Center).Children(children)
	})
	if b := scroll.Bounds(); b.H > 0 {
		*h = b.H
	}
}

// createBody is the "新会话" tab: a large greeting and the same composer as
// a conversation, with Agent and project/worktree controls underneath.
// The block is centered in both directions and width-capped, and it scrolls
// on a window too short to hold it.
func (cw *ChatWindow) createBody(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	viewportCenter(c, "create", func() {
		mgo.Column(c).FillWidth().MaxWidth(createWidth).AlignSelf(mgo.Center).
			AlignItems(mgo.Start).Justify(mgo.Center).
			Padding(u*6, u*3).Gap(u * 2).Children(func() {
			mgo.Text(c, "在 ChatGo 里做点什么？").FontSize(mcore.FontSize(c, mtheme.H1Size)).
				FontWeight(650).Shrink(0).MaxLines(2)
			mgo.Text(c, "或直接开始，不选项目").FontSize(caption).
				TextColor(k.TextMuted).Shrink(0)
			cw.createComposer(c)
			cw.createChipRow(c)
		})
	})
}

// createChipRow is the new-session page's immersive picker row: the Agent,
// the project and the worktree as borderless chips under the composer, with
// where the session runs on the far right.
func (cw *ChatWindow) createChipRow(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Shrink(0).Gap(u / 2).AlignItems(mgo.Center).Wrap().Children(func() {
		cw.createAgentChip(c)
		cw.createWorkspaceChip(c)
		cw.createWorktreeChip(c)
		if cw.create.wt == createNewWorktree {
			cw.createBranchChip(c)
		}
		mgo.Spacer(c)
		mgo.Text(c, "桌面 Agent · 本地执行").FontSize(caption).TextColor(k.TextMuted).Shrink(0)
	})
}

// pickCreateAgent records an agent choice: another agent's options are not
// this one's, so the ACP choices made for the previous agent are dropped.
func (cw *ChatWindow) pickCreateAgent(ref string) {
	if ref == cw.create.agent {
		return
	}
	cw.create.agent = ref
	cw.create.acpConfig = nil
	cw.invalidate()
}

// createAgentChip is the immersive Agent chip of the new-session row.
func (cw *ChatWindow) createAgentChip(c *mgo.Context) {
	cw.agentPickerChip(c, &cw.create.agent, cw.pickCreateAgent)
}

// createSelectors lays out the agent, project, worktree and branch pickers as
// one wrapping row. The home page reuses it, so starting a session asks for
// the same things in the same place wherever it begins.
//
// The pickers share the row: each takes the quarter (or third, before a
// branch is asked for) of the width the previous frame measured, capped at
// createPickerWidth, so all of them stay on one row however narrow the pane
// is and never spill the branch picker onto a second line.
func (cw *ChatWindow) createSelectors(c *mgo.Context, scope string) {
	u := mcore.Density(c).Unit()
	gap := u * 2
	count := 3
	if cw.create.wt == createNewWorktree {
		count = 4
	}
	row := mgo.Row(c).FillWidth().Shrink(0).Gap(gap).AlignItems(mgo.Start).Wrap()
	// Every picker is a sibling of the same row, aligned at the top so their
	// labels line up however tall a picker grows.
	rowW := mgo.Local(row, "chatgo:create:selectors:roww", func() float32 { return 0 })
	if *rowW > 0 {
		cw.create.pickerW = min((*rowW-gap*float32(count-1))/float32(count), createPickerWidth)
	} else {
		cw.create.pickerW = createPickerWidth
	}
	row.Children(func() {
		cw.createAgentSelect(c, scope)
		cw.createWorkspaceSelect(c, scope)
		cw.createWorktreeSelect(c, scope)
		if cw.create.wt == createNewWorktree {
			cw.createBranchSelect(c, scope)
		}
	})
	if b := row.Bounds(); b.W > 0 {
		*rowW = b.W
	}
}

// pickerWidth is the width this frame's pickers take; the first frame falls
// back to the cap.
func (cw *ChatWindow) pickerWidth() float32 {
	if cw.create.pickerW > 0 {
		return cw.create.pickerW
	}
	return createPickerWidth
}

// createWidth caps the create block, so the composer keeps a readable
// measure on a wide pane.
const createWidth float32 = 820

// createPickerWidth caps a choice picker: wide enough for a full name on a
// roomy pane, while createSelectors shrinks the pickers below it to share a
// narrower row.
const createPickerWidth float32 = 168

// createPickerIconRoom is the horizontal room the 项目 picker's folder
// button needs beside the select inside the same field.
const createPickerIconRoom float32 = 36

// createPickerHeight keeps the four pickers level with one another.
const createPickerHeight float32 = 32

// createAgentSelect chooses the agent that runs the new conversation: the
// picker that lists every assistant by Agent. Its
// trigger shares the variant, width and height of the pickers beside it.
func (cw *ChatWindow) createAgentSelect(c *mgo.Context, scope string) {
	w := cw.pickerWidth()
	kit.FieldColumn(c, "Agent", kit.FieldOptions{
		Width: w, Name: scope + " Agent",
	}, func() {
		cw.agentPickerButton(c, &cw.create.agent, cw.pickCreateAgent, w)
	})
}

func (cw *ChatWindow) projectWorkspaces() []string {
	seen := map[string]bool{}
	var out []string
	for _, ws := range cw.workspaces() {
		cw.ensureConvGit(ws)
		main := cw.convGitGroup(ws)
		if main == "" || seen[main] {
			continue
		}
		seen[main] = true
		out = append(out, main)
	}
	return out
}

// createWorkspaceSelect chooses the project, with a button beside it that
// picks a new directory. Picking another project drops the worktree chosen
// for the old one and defaults to the new project's main tree, so the tree
// below never belongs to a different project.
func (cw *ChatWindow) createWorkspaceSelect(c *mgo.Context, scope string) {
	u := mcore.Density(c).Unit()
	opts := cw.projectChoices()
	kit.FieldColumn(c, "项目", kit.FieldOptions{
		Width: cw.pickerWidth(), Name: scope + " 项目",
	}, func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			sel := kit.SearchSelect(c, &cw.create.ws, opts, kit.SearchSelectOptions[string]{
				Label: "项目", Placeholder: "选择项目",
				SearchPlaceholder: "搜索项目", Empty: "没有项目，可先新增",
				MinWidth: cw.pickerWidth() - createPickerIconRoom, MinHeight: createPickerHeight,
			})
			sel.Element.Justify(mgo.Start)
			if mcore.IconAction(c, icons.Must("folder"), "新增项目").Tooltip("新增项目").Clicked() {
				cw.pickCreateProject()
			}
			if sel.Changed() {
				cw.pickCreateWorkspace(cw.create.ws)
			}
		})
	})
}

// projectChoices lists the projects the new-session page offers: one entry
// per main tree, plus directories outside any repository as themselves.
func (cw *ChatWindow) projectChoices() []kit.SearchSelectOption[string] {
	projects := cw.projectWorkspaces()
	if p := cw.convGitProject(cw.create.ws); p != nil && p.Main != "" {
		cw.create.ws = p.Main
	}
	opts := make([]kit.SearchSelectOption[string], 0, len(projects))
	seen := map[string]bool{}
	for _, ws := range projects {
		seen[ws] = true
		name := filepath.Base(ws)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = ws
		}
		opts = append(opts, kit.SearchSelectOption[string]{Value: ws, Label: name})
	}
	// A directory outside any repository is its own entry, so choosing one
	// keeps the conversation in it instead of an endless "loading" and a
	// repeat probe every frame.
	for _, ws := range cw.workspaces() {
		if seen[ws] {
			continue
		}
		cw.ensureConvGit(ws)
		if !cw.convGitLoaded(ws) || cw.convGitProject(ws) != nil {
			continue
		}
		seen[ws] = true
		name := filepath.Base(ws)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = ws
		}
		opts = append(opts, kit.SearchSelectOption[string]{Value: ws, Label: name, Note: middleEllipsis(ws, worktreePathWidth)})
	}
	return opts
}

// pickCreateWorkspace makes ws the project and resets the worktree choice to
// the new project's main tree, so the pickers never disagree.
func (cw *ChatWindow) pickCreateWorkspace(ws string) {
	cw.create.wt, cw.create.branch, cw.create.newBranch = "", "", false
	cw.ensureConvGit(ws)
	if p := cw.convGitProject(ws); p != nil {
		cw.create.wt = p.Main
	}
}

// pickCreateProject asks for a directory and makes it the chosen project,
// exactly like picking it from the dropdown.
func (cw *ChatWindow) pickCreateProject() {
	cw.pickDirectory("选择项目", func(path string) {
		cw.addWorkspace(path)
		cw.create.ws = path
		cw.pickCreateWorkspace(path)
	})
}

// createWorkspaceChip is the immersive project chip of the new-session row:
// the project's name, with a button beside it that picks a new directory.
func (cw *ChatWindow) createWorkspaceChip(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	opts := cw.projectChoices()
	mgo.Row(c).Shrink(0).Gap(u / 2).AlignItems(mgo.Center).Children(func() {
		sel := kit.SearchSelect(c, &cw.create.ws, opts, kit.SearchSelectOptions[string]{
			Label: "项目", Placeholder: "选择项目",
			SearchPlaceholder: "搜索项目", Empty: "没有项目，可先新增",
			MinHeight: createPickerHeight, Ghost: true, Up: true,
			Icon: icons.Must("folder"), Foreground: mcore.Tokens(c).Text, Chevron: true,
		})
		sel.Element.MaxWidth(createPickerWidth * 1.6).Justify(mgo.Start)
		if mcore.IconAction(c, icons.Must("plus"), "新增项目").Tooltip("新增项目").Clicked() {
			cw.pickCreateProject()
		}
		if sel.Changed() {
			cw.pickCreateWorkspace(cw.create.ws)
		}
	})
}

// createWorktreeSelect chooses an existing worktree of the project, or the
// "新建 worktree" option that opens the branch picker. Each entry is only the
// branch, the main tree marked [主]: the project is already chosen above, and
// the branch is what tells the trees apart.
func (cw *ChatWindow) createWorktreeSelect(c *mgo.Context, scope string) {
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	k := mcore.Tokens(c)
	p := cw.convGitProject(cw.create.ws)
	if p == nil {
		if cw.create.ws == "" {
			return
		}
		// The summary may still be loading; ask for it and say so, so the
		// worktree selector appears as soon as it lands.
		cw.ensureConvGit(cw.create.ws)
		if !cw.convGitLoaded(cw.create.ws) {
			mgo.Text(c, "正在读取项目工作树…").FontSize(caption).TextColor(k.TextMuted)
			return
		}
		// Not a repository: the directory itself is where the session runs.
		kit.FieldColumn(c, "工作区", kit.FieldOptions{
			Width: cw.pickerWidth(), Name: scope + " 工作区",
		}, func() {
			b := kit.Button(c, filepath.Base(cw.create.ws), kit.ButtonOptions{
				Variant: kit.Secondary, Disabled: true,
			}).Tooltip("该目录不是 Git 仓库，会话将直接在其中运行")
			b.MaxWidth(cw.pickerWidth()).MinWidth(0)
		})
		return
	}
	// No worktree chosen yet means the main tree, whatever way the project
	// got here — typed in, picked from the dropdown, or defaulted on open.
	if cw.create.wt == "" {
		cw.create.wt = p.Main
	}
	opts := worktreeChoices(p)
	kit.FieldColumn(c, "工作树", kit.FieldOptions{
		Width: cw.pickerWidth(), Name: scope + " 工作树",
	}, func() {
		sel := kit.SearchSelect(c, &cw.create.wt, opts, kit.SearchSelectOptions[string]{
			Label: "工作树", Placeholder: "选择工作树",
			SearchPlaceholder: "搜索工作树", Empty: "此项目没有工作树",
			MinWidth: cw.pickerWidth(), MinHeight: createPickerHeight,
		})
		sel.Element.Justify(mgo.Start)
	})
}

// worktreeChoices lists the project's trees, the main one first and marked,
// with the "new worktree" entry that reveals the branch picker.
func worktreeChoices(p *Project) []kit.SearchSelectOption[string] {
	opts := make([]kit.SearchSelectOption[string], 0, len(p.Worktrees)+1)
	opts = append(opts, kit.SearchSelectOption[string]{Value: createNewWorktree, Label: "新建 worktree…"})
	for _, path := range p.Worktrees {
		label := p.TreeBranches[path]
		if label == "" {
			continue
		}
		if path == p.Main {
			label = "[主] " + label
		}
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: path, Label: label, Note: middleEllipsis(path, worktreePathWidth),
		})
	}
	return opts
}

// createWorktreeChip is the immersive worktree chip of the new-session row.
// It also covers a plain directory, which runs the session as itself.
func (cw *ChatWindow) createWorktreeChip(c *mgo.Context) {
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	k := mcore.Tokens(c)
	p := cw.convGitProject(cw.create.ws)
	if p == nil {
		if cw.create.ws == "" {
			return
		}
		cw.ensureConvGit(cw.create.ws)
		if !cw.convGitLoaded(cw.create.ws) {
			mgo.Text(c, "正在读取项目工作树…").FontSize(caption).TextColor(k.TextMuted)
			return
		}
		b := kit.Button(c, filepath.Base(cw.create.ws), kit.ButtonOptions{
			Variant: kit.Ghost, Icon: icons.Must("folder"), Disabled: true,
		}).Tooltip("该目录不是 Git 仓库，会话将直接在其中运行")
		b.MaxWidth(createPickerWidth * 1.6).MinWidth(0)
		return
	}
	if cw.create.wt == "" {
		cw.create.wt = p.Main
	}
	sel := kit.SearchSelect(c, &cw.create.wt, worktreeChoices(p), kit.SearchSelectOptions[string]{
		Label: "工作树", Placeholder: "选择工作树",
		SearchPlaceholder: "搜索工作树", Empty: "此项目没有工作树",
		MinHeight: createPickerHeight, Ghost: true, Up: true,
		Icon: icons.Must("git-branch"), Foreground: mcore.Tokens(c).Text, Chevron: true,
	})
	sel.Element.MaxWidth(createPickerWidth * 1.6).Justify(mgo.Start)
}

// "新建 worktree" option that opens the branch picker. Each entry is just the
// branch, the main tree marked [主], since the project is already chosen
// createBranchSelect chooses the branch of a new worktree, searchable and
// able to create a new branch, the same picker the worktree panel uses.
func (cw *ChatWindow) createBranchSelect(c *mgo.Context, scope string) {
	p := cw.convGitProject(cw.create.ws)
	var branches []kit.Branch
	if p != nil {
		branches = branchList(p.Branches)
	}
	kit.FieldColumn(c, "分支", kit.FieldOptions{
		Width: cw.pickerWidth(), Name: scope + " 分支",
	}, func() {
		sel := kit.BranchSelector(c, &cw.create.branch, branches,
			kit.BranchSelectorOptions{AllowCreate: true, Key: "create", MinHeight: createPickerHeight})
		sel.Element.MinWidth(cw.pickerWidth()).Justify(mgo.Start)
		if sel.Changed() {
			cw.create.newBranch = false
		}
		if name, ok := sel.Created(); ok {
			cw.create.branch = name
			cw.create.newBranch = true
		}
	})
}

// createBranchChip is the immersive branch chip of the new-session row, shown
// when the worktree choice asks for a new tree.
func (cw *ChatWindow) createBranchChip(c *mgo.Context) {
	p := cw.convGitProject(cw.create.ws)
	var branches []kit.Branch
	if p != nil {
		branches = branchList(p.Branches)
	}
	sel := kit.BranchSelector(c, &cw.create.branch, branches, kit.BranchSelectorOptions{
		AllowCreate: true, Key: "create", MinHeight: createPickerHeight, Ghost: true,
		Foreground: mcore.Tokens(c).Text, Chevron: true,
	})
	sel.Element.MaxWidth(createPickerWidth * 1.6).Justify(mgo.Start)
	if sel.Changed() {
		cw.create.newBranch = false
	}
	if name, ok := sel.Created(); ok {
		cw.create.branch = name
		cw.create.newBranch = true
	}
}

// createComposer is the central input; its footer holds the agent-specific
// controls, and Enter starts the conversation.
func (cw *ChatWindow) createComposer(c *mgo.Context) {
	cw.startComposer(c, "问任何问题，@ 文件，$ Skills，/ 命令", cw.startFromCreate)
}

// startComposer is the composer shared by the "新会话" tab and the home page:
// the agent's model and tool controls sit in its footer and Enter runs start.
func (cw *ChatWindow) startComposer(c *mgo.Context, placeholder string, start func()) {
	in := composer(c, &cw.draft, composerOptions{
		Placeholder: placeholder,
		MaxHeight:   180,
		Tools: func() {
			if cw.createIsACP() {
				cw.contextButton(c, false)
				cw.createACPControls(c)
				return
			}
			if cw.createIsExternalAgent() {
				return
			}
			cw.contextButton(c, true)
			cw.modelSelector(c)
			cw.toolsMenu(c)
		},
		Actions: func() {
			r := kit.SendButton(c, kit.SendButtonOptions{Disabled: strings.TrimSpace(cw.draft) == ""})
			if r.Sent() {
				start()
			}
		},
	})
	// Keep the new-session/home composer centered and readable on wide panes.
	in.Element.FillWidth().MaxWidth(createWidth).AlignSelf(mgo.Center).Shrink(0)
	if in.Submitted() {
		start()
	}
}

// createACPControls shows the ACP agent's model and the options its runtime
// offers in the composer's row, through the same shared row a conversation
// uses. The choices wait in the page's state until the session starts.
func (cw *ChatWindow) createACPControls(c *mgo.Context) {
	name := strings.TrimPrefix(cw.create.agent, "acp:")
	if name == cw.create.agent || name == "" {
		return
	}
	opts := cw.acpConfigs(name)
	if len(opts) == 0 {
		// Keep the composer clean while the runtime is unavailable. Failed ACP
		// agents are marked in the Agent picker instead of rendering an error
		// inside the message input.
		return
	}
	items := cw.createACPItems(opts)
	kitItems := make([]kit.ConfigItem, len(items))
	for i, it := range items {
		kitItems[i] = kit.ConfigItem{Name: it.label, Kind: acpOptionKind(it), Rank: it.rank}
	}
	scope := "create:" + name
	kit.ConfigRow(c, kitItems, scope, func(i int, allowButtons bool) {
		cw.acpOptionControl(c, items[i], scope, allowButtons, acpChoiceSink{
			selectValue: func(id acp.SessionConfigId, value, _ acp.SessionConfigValueId) {
				cw.createACPSet(string(id), string(value))
			},
			boolValue: func(id acp.SessionConfigId, value, _ bool) {
				cw.createACPSet(string(id), strconv.FormatBool(value))
			},
		})
	})
}

// createACPItems builds the create page's ACP controls from the warm-up
// cache: the runtime's own options, with the choices made here applied.
func (cw *ChatWindow) createACPItems(opts []acp.SessionConfigOption) []acpConfigItem {
	copies := make([]acp.SessionConfigOption, 0, len(opts))
	for _, opt := range opts {
		o := opt
		switch {
		case o.Select != nil:
			s := *o.Select
			o.Select = &s
			if v, ok := cw.create.acpConfig[string(s.Id)]; ok {
				s.CurrentValue = acp.SessionConfigValueId(v)
			}
		case o.Boolean != nil:
			b := *o.Boolean
			o.Boolean = &b
			if v, ok := cw.create.acpConfig[string(b.Id)]; ok {
				b.CurrentValue = v == "true"
			}
		}
		copies = append(copies, o)
	}
	return acpConfigItemsFromOptions(copies)
}

// createACPSet records an ACP option's choice for the new session.
func (cw *ChatWindow) createACPSet(id, value string) {
	if cw.create.acpConfig == nil {
		cw.create.acpConfig = map[string]string{}
	}
	cw.create.acpConfig[id] = value
}

// applyCreateACPConfig carries the ACP choices made on the 新会话 page into
// the new conversation, so its session starts with them.
func (cw *ChatWindow) applyCreateACPConfig() {
	if len(cw.create.acpConfig) == 0 || cw.cur == nil {
		return
	}
	if cw.cur.ACPConfig == nil {
		cw.cur.ACPConfig = map[string]string{}
	}
	for id, value := range cw.create.acpConfig {
		cw.cur.ACPConfig[id] = value
	}
	if cw.convManager != nil {
		_ = cw.convManager.SaveConversation(cw.cur)
	}
}

// acpConnectionOf looks up the ACP connection an agent id names.
func (cw *ChatWindow) acpConnectionOf(agent string) *config.ACPConnection {
	name := strings.TrimPrefix(agent, "acp:")
	for i := range cw.cfg.ACPAgents {
		if cw.cfg.ACPAgents[i].Name == name {
			return &cw.cfg.ACPAgents[i]
		}
	}
	return nil
}

// startFromCreate creates the conversation the tab has set up and sends the
// draft as its first message. A new worktree is created first, then the
// session runs in it.
func (cw *ChatWindow) startFromCreate() {
	if strings.TrimSpace(cw.draft) == "" {
		return
	}
	ws := cw.create.ws
	if ws == "" {
		cw.toast("请选择项目", mcore.SeverityError)
		return
	}
	if cw.create.wt == createNewWorktree {
		branch := strings.TrimSpace(cw.create.branch)
		if branch == "" {
			cw.toast("请填写分支名", mcore.SeverityError)
			return
		}
		cw.addWorktreeIn(ws, "", branch, cw.create.newBranch, func(path string) {
			cw.finishCreate(path)
		})
		return
	}
	wt := cw.create.wt
	if wt == "" {
		wt = ws
	}
	cw.finishCreate(wt)
}

// finishCreate closes the tab and starts the conversation in ws.
func (cw *ChatWindow) finishCreate(ws string) {
	if ws == "" {
		return
	}
	for _, pi := range cw.center.visiblePanes() {
		p := &cw.center.panes[pi]
		for i := range p.tabs {
			if p.tabs[i].Kind == centerCreate {
				cw.closeCenterTab(pi, i)
				break
			}
		}
	}
	cw.newConversationIn(ws, cw.create.agent)
	// The ACP choices made on this page ride the new conversation, so its
	// session starts with them.
	cw.applyCreateACPConfig()
	cw.sendDraft()
}

// worktreePathWidth is how many characters a worktree's path line keeps
// before its middle is elided.
const worktreePathWidth = 48

// middleEllipsis shortens s to at most width characters by replacing its
// middle with "…", keeping both ends. A path's ends carry the meaning — the
// repository root and the tree's own directory — so cutting the middle reads
// better than cutting the tail.
func middleEllipsis(s string, width int) string {
	r := []rune(s)
	if width <= 1 || len(r) <= width {
		return s
	}
	keep := width - 1
	head := (keep + 1) / 2
	tail := keep - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
