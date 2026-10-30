// The "新会话" tab: one place to choose an agent, a project, a worktree (or
// create one) and a branch, then type a message that creates the
// conversation. The sidebar's new-workspace, new-worktree and new-conversation
// entries all open this tab, so starting a session follows the same shape
// wherever it is asked for.
package ui

import (
	"path/filepath"
	"strings"

	"chatgo/internal/config"
	mgo "github.com/egoist/mygo/ui"

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

// createBody is the "新会话" tab: the choice rows, the composer in the
// middle, and the agent-specific controls under it. The block is centered in
// both directions and width-capped, the way the home page's composer is.
//
// It is laid out directly in the pane, not inside a Scroll: a scroll gives
// its content the content's own height, so Justify(Center) would have nothing
// to distribute and the block would sit against the top edge.
func (cw *ChatWindow) createBody(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Justify(mgo.Center).
		AlignItems(mgo.Center).Padding(u * 4).Children(func() {
		mgo.Column(c).FillWidth().MaxWidth(createWidth).AlignSelf(mgo.Center).
			Gap(u * 2).Children(func() {
			mgo.Text(c, "新会话").FontSize(mcore.FontSize(c, mtheme.H2Size)).
				FontWeight(600).Shrink(0)
			mgo.Text(c, "选择 Agent、项目与工作树，输入消息后回车创建会话。").FontSize(caption).
				TextColor(k.TextMuted).Shrink(0)
			// Every picker is a sibling of the same row, aligned at the top
			// so their labels line up however tall a picker grows.
			mgo.Row(c).FillWidth().Shrink(0).Gap(u * 2).AlignItems(mgo.Start).
				Wrap().Children(func() {
				cw.createAgentSelect(c)
				cw.createWorkspaceSelect(c)
				cw.createWorktreeSelect(c)
				if cw.create.wt == createNewWorktree {
					cw.createBranchSelect(c)
				}
			})
			cw.createComposer(c)
		})
	})
}

// createWidth caps the create block, so the composer keeps a readable
// measure on a wide pane.
const createWidth float32 = 760

// createPickerWidth keeps the agent, project and worktree pickers wide
// enough for a full name, rather than shrinking to their shortest value.
const createPickerWidth float32 = 180

// createPickerHeight keeps the four pickers level with one another.
const createPickerHeight float32 = 32

// createAgentSelect chooses the agent that runs the new conversation.
func (cw *ChatWindow) createAgentSelect(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	opts := make([]kit.SearchSelectOption[string], 0, len(cw.cfg.Agents)+len(cw.cfg.ACPAgents)+1)
	opts = append(opts, kit.SearchSelectOption[string]{
		Value: "", Label: chatAgentName, Leading: iconLeading(c, chatAgentIcon),
	})
	for _, a := range cw.cfg.Agents {
		name := a.Name
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: "agent:" + name, Label: name, Leading: cw.agentLeading(c, name),
		})
	}
	for _, a := range cw.cfg.ACPAgents {
		if a.Disabled {
			continue
		}
		name := a.Name
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: "acp:" + name, Label: name, Leading: logoLeading(c, name),
		})
	}
	mgo.Column(c).Shrink(0).Gap(u).Children(func() {
		mgo.Text(c, "Agent").FontSize(caption).TextColor(mcore.Tokens(c).TextMuted).
			Label("新会话 Agent")
		kit.SearchSelect(c, &cw.create.agent, opts, kit.SearchSelectOptions[string]{
			Label: "Agent", Placeholder: "选择 Agent", SearchPlaceholder: "搜索 Agent",
			MinWidth: createPickerWidth, MinHeight: createPickerHeight,
		})
	})
}

// createWorkspaceSelect chooses the project, with a button beside it that
// picks a new directory. Picking another project drops the worktree chosen
// for the old one and defaults to the new project's main tree, so the tree
// below never belongs to a different project.
func (cw *ChatWindow) createWorkspaceSelect(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	opts := make([]kit.SearchSelectOption[string], 0, len(cw.workspaces()))
	for _, ws := range cw.workspaces() {
		name := filepath.Base(ws)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = ws
		}
		opts = append(opts, kit.SearchSelectOption[string]{Value: ws, Label: name})
	}
	mgo.Column(c).Shrink(0).Gap(u).Children(func() {
		mgo.Text(c, "项目").FontSize(caption).TextColor(mcore.Tokens(c).TextMuted).
			Label("新会话 项目")
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			sel := kit.SearchSelect(c, &cw.create.ws, opts, kit.SearchSelectOptions[string]{
				Label: "项目", Placeholder: "选择项目",
				SearchPlaceholder: "搜索项目", Empty: "没有项目，可先新增",
				MinWidth: createPickerWidth, MinHeight: createPickerHeight,
			})
			if mcore.IconAction(c, icons.Must("folder"), "新增项目").Tooltip("新增项目").Clicked() {
				// The new directory becomes the chosen project, exactly like
				// picking it from the dropdown: the worktree resets to the
				// new project's main tree so the pickers never disagree.
				cw.pickDirectory("选择项目", func(path string) {
					cw.addWorkspace(path)
					cw.create.ws = path
					cw.create.wt, cw.create.branch, cw.create.newBranch = "", "", false
					cw.ensureConvGit(path)
					if p := cw.convGitProject(path); p != nil {
						cw.create.wt = p.Main
					}
				})
			}
			if sel.Changed() {
				cw.create.wt, cw.create.branch, cw.create.newBranch = "", "", false
				cw.ensureConvGit(cw.create.ws)
				if p := cw.convGitProject(cw.create.ws); p != nil {
					cw.create.wt = p.Main
				}
			}
		})
	})
}

// createWorktreeSelect chooses an existing worktree of the project, or the
// "新建 worktree" option that opens the branch picker. Each entry is only the
// branch, the main tree marked [主]: the project is already chosen above, and
// the branch is what tells the trees apart.
func (cw *ChatWindow) createWorktreeSelect(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	k := mcore.Tokens(c)
	p := cw.convGitProject(cw.create.ws)
	if p == nil {
		// The project's summary may still be loading; ask for it and say so,
		// so the worktree selector appears as soon as it lands.
		if cw.create.ws != "" {
			cw.ensureConvGit(cw.create.ws)
		}
		mgo.Text(c, "正在读取项目工作树…").FontSize(caption).TextColor(k.TextMuted)
		return
	}
	// No worktree chosen yet means the main tree, whatever way the project
	// got here — typed in, picked from the dropdown, or defaulted on open.
	if cw.create.wt == "" {
		cw.create.wt = p.Main
	}
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
	mgo.Column(c).Shrink(0).Gap(u).Children(func() {
		mgo.Text(c, "工作树").FontSize(caption).TextColor(k.TextMuted).
			Label("新会话 工作树")
		kit.SearchSelect(c, &cw.create.wt, opts, kit.SearchSelectOptions[string]{
			Label: "工作树", Placeholder: "选择工作树",
			SearchPlaceholder: "搜索工作树", Empty: "此项目没有工作树",
			MinWidth: createPickerWidth, MinHeight: createPickerHeight,
		})
	})
}

// "新建 worktree" option that opens the branch picker. Each entry is just the
// branch, the main tree marked [主], since the project is already chosen
// createBranchSelect chooses the branch of a new worktree, searchable and
// able to create a new branch, the same picker the worktree panel uses.
func (cw *ChatWindow) createBranchSelect(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	p := cw.convGitProject(cw.create.ws)
	var branches []kit.Branch
	if p != nil {
		branches = branchList(p.Branches)
	}
	mgo.Column(c).Shrink(0).Gap(u).Children(func() {
		mgo.Text(c, "分支").FontSize(caption).TextColor(mcore.Tokens(c).TextMuted).
			Label("新会话 分支")
		sel := kit.BranchSelector(c, &cw.create.branch, branches,
			kit.BranchSelectorOptions{AllowCreate: true, Key: "create", MinHeight: createPickerHeight})
		if sel.Changed() {
			cw.create.newBranch = false
		}
		if name, ok := sel.Created(); ok {
			cw.create.branch = name
			cw.create.newBranch = true
		}
	})
}

// createComposer is the central input; its footer holds the agent-specific
// controls, and Enter starts the conversation.
func (cw *ChatWindow) createComposer(c *mgo.Context) {
	in := composer(c, &cw.draft, composerOptions{
		Placeholder: "输入消息，Enter 创建会话并发送",
		MaxHeight:   180,
		Tools: func() {
			if cw.createIsACP() {
				cw.createACPInfo(c)
				return
			}
			cw.modelSelector(c)
			cw.toolMenu(c)
			cw.mcpMenu(c)
			cw.skillMenu(c)
		},
		Actions: func() {
			r := kit.SendButton(c, kit.SendButtonOptions{Disabled: strings.TrimSpace(cw.draft) == ""})
			if r.Sent() {
				cw.startFromCreate()
			}
		},
	})
	in.Element.FillWidth().Shrink(0)
	if in.Submitted() {
		cw.startFromCreate()
	}
}

// createACPInfo shows the chosen ACP connection's static config. The dynamic
// options a server offers only appear after the process starts, in the
// conversation's own footer.
func (cw *ChatWindow) createACPInfo(c *mgo.Context) {
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	conn := cw.acpConnectionOf(cw.create.agent)
	if conn == nil {
		return
	}
	cmd := strings.TrimSpace(conn.Command + " " + strings.Join(conn.Args, " "))
	if cmd == "" {
		return
	}
	mgo.Text(c, "命令："+cmd).Font(mtheme.MonoFont).FontSize(caption).
		TextColor(k.TextMuted).SingleLine().Ellipsis("…").MaxLines(1)
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
	cw.finishCreate(cw.create.wt)
}

// finishCreate closes the tab and starts the conversation in ws.
func (cw *ChatWindow) finishCreate(ws string) {
	if ws == "" {
		return
	}
	for pi := range cw.center.visiblePanes() {
		p := &cw.center.panes[pi]
		for i := range p.tabs {
			if p.tabs[i].Kind == centerCreate {
				cw.closeCenterTab(pi, i)
				break
			}
		}
	}
	cw.newConversationIn(ws, cw.create.agent)
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
