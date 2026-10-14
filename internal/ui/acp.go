package ui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// acpHandshakeTimeout bounds Initialize and NewSession so a stuck CLI cannot
// hang the warmup goroutine forever.
const acpHandshakeTimeout = 60 * time.Second

//go:embed assets/claude.png
var claudeLogoPNG []byte

//go:embed assets/opencode.png
var opencodeLogoPNG []byte

var (
	logoOnce   sync.Once
	logoClaude *mgo.Bitmap
	logoOpen   *mgo.Bitmap
)

// acpLogo returns the official logo of a known ACP agent by name.
func acpLogo(name string) *mgo.Bitmap {
	logoOnce.Do(func() {
		logoClaude, _ = mgo.DecodeBitmap(claudeLogoPNG)
		logoOpen, _ = mgo.DecodeBitmap(opencodeLogoPNG)
	})
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "claude"):
		return logoClaude
	case strings.Contains(low, "opencode"):
		return logoOpen
	}
	return nil
}

type acpForm struct {
	editing  int
	name     string
	command  string
	args     string
	env      string
	cwd      string
	disabled bool
}

type acpCatalogEntry struct {
	Name        string
	Command     string
	Args        []string
	Description string
	Homepage    string
}

func acpCatalog() []acpCatalogEntry {
	return []acpCatalogEntry{
		{Name: "Claude Code", Command: "npx", Args: []string{"-y", "@zed-industries/claude-code-acp"}, Description: "Anthropic Claude Code", Homepage: "https://claude.com"},
		{Name: "Claude Agent SDK", Command: "npx", Args: []string{"-y", "@zed-industries/claude-agent-acp"}, Description: "Claude Agent SDK via Zed 适配器", Homepage: "https://github.com/zed-industries/claude-agent-acp"},
		{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}, Description: "OpenCode AI 编程助手", Homepage: "https://github.com/sst/opencode"},
		{Name: "Codex CLI", Command: "npx", Args: []string{"-y", "@agentclientprotocol/codex-acp"}, Description: "OpenAI Codex CLI", Homepage: "https://github.com/openai/codex"},
		{Name: "Cursor", Command: "agent", Args: []string{"acp"}, Description: "Cursor CLI Agent", Homepage: "https://cursor.com"},
		{Name: "Gemini CLI", Command: "gemini", Description: "Google Gemini CLI", Homepage: "https://github.com/google-gemini/gemini-cli"},
		{Name: "Kiro CLI", Command: "kiro-cli", Args: []string{"acp"}, Description: "AWS Kiro CLI", Homepage: "https://kiro.dev"},
		{Name: "GitHub Copilot", Command: "copilot", Description: "GitHub Copilot CLI", Homepage: "https://github.com/features/copilot"},
		{Name: "Goose", Command: "goose", Description: "Block Goose AI Agent", Homepage: "https://block.github.io/goose/"},
		{Name: "Qwen Code", Command: "qwen", Description: "阿里通义灵码 Qwen Code", Homepage: "https://github.com/QwenLM/qwen-code"},
		{Name: "Kimi CLI", Command: "kimi", Description: "月之暗面 Kimi CLI", Homepage: "https://github.com/MoonshotAI/kimi-cli"},
		{Name: "Cline", Command: "npx", Args: []string{"-y", "@cline/cline-acp-adapter"}, Description: "Cline AI 助手", Homepage: "https://cline.bot"},
		{Name: "Docker cagent", Command: "cagent", Description: "Docker cagent", Homepage: "https://github.com/docker/cagent"},
		{Name: "Factory Droid", Command: "droid", Description: "Factory Droid", Homepage: "https://factory.ai"},
		{Name: "Augment Code", Command: "augment", Args: []string{"acp"}, Description: "Augment Code CLI", Homepage: "https://docs.augmentcode.com/cli/acp"},
		{Name: "OpenHands", Command: "openhands", Args: []string{"acp"}, Description: "OpenHands AI Agent", Homepage: "https://docs.openhands.dev"},
		{Name: "Qoder CLI", Command: "qoder", Args: []string{"acp"}, Description: "Qoder CLI", Homepage: "https://docs.qoder.com/cli/acp"},
	}
}

func (cw *ChatWindow) hasACPConn(name string) bool {
	for _, a := range cw.cfg.ACPAgents {
		if strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

func (cw *ChatWindow) addACPFromCatalog(e acpCatalogEntry) {
	if cw.hasACPConn(e.Name) {
		cw.toast("已存在同名连接", mcore.SeverityInfo)
		return
	}
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, config.ACPConnection{
		Name: e.Name, Command: e.Command, Args: e.Args, Disabled: true,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.acpCatalogOpen = false
	cw.toast("已添加 "+e.Name+"，请先安装对应 CLI 再启用", mcore.SeveritySuccess)
}

func (cw *ChatWindow) acpCatalogDialog(c *mgo.Context) {
	if !cw.acpCatalogOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	moverlay.Dialog(c, &cw.acpCatalogOpen, moverlay.DialogOptions{
		Title: "从目录添加 Agent",
		Width: 560,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			in := minput.SearchInput(c, &cw.acpCatalogQuery, minput.SearchInputOptions{Placeholder: "搜索 Agent", Label: "搜索"})
			in.Element.FillWidth()
			q := strings.ToLower(strings.TrimSpace(cw.acpCatalogQuery))
			var items []acpCatalogEntry
			for _, e := range acpCatalog() {
				if q != "" && !strings.Contains(strings.ToLower(e.Name), q) && !strings.Contains(strings.ToLower(e.Description), q) {
					continue
				}
				items = append(items, e)
			}
			if len(items) == 0 {
				mgo.Text(c, "没有匹配的 Agent").TextColor(k.TextMuted)
				return
			}
			mgo.Scroll(c).MaxHeight(400).Children(func() {
				mgo.Column(c).Gap(u).Children(func() {
					for _, e := range items {
						e := e
						mgo.Row(c).Key(e.Name).FillWidth().Gap(u*2).AlignItems(mgo.Center).
							Padding(u, u*2).Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border).
							Children(func() {
								acpIcon(c, e.Name, 24, 24)
								mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
									mgo.Text(c, e.Name).FontWeight(600).SingleLine()
									mgo.Text(c, e.Command+" "+strings.Join(e.Args, " ")).
										Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
									if e.Description != "" {
										mgo.Text(c, e.Description).FontSize(caption).TextColor(k.TextMuted).MaxLines(1)
									}
								})
								if cw.hasACPConn(e.Name) {
									mgo.Text(c, "已添加").FontSize(caption).TextColor(k.Success).Shrink(0)
								} else if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
									cw.addACPFromCatalog(e)
								}
							})
					}
				})
			})
		})
	})
}

func (cw *ChatWindow) addNewACP() {
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, config.ACPConnection{
		Name: "New Connection", Disabled: true,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.loadACPForm(len(cw.cfg.ACPAgents) - 1)
}

func (cw *ChatWindow) acpCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	a := cw.cfg.ACPAgents[i]
	expanded := cw.acpForm.editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("acp:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				acpIcon(c, a.Name, 18, 18)
				mgo.Text(c, a.Name).Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := !a.Disabled
				if minput.Switch(c, &en, "", minput.SwitchOptions{}).Changed() {
					cw.cfg.ACPAgents[i].Disabled = !en
					cw.acpForm.disabled = !en
					_ = config.SaveConfig(cw.cfg)
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearACPForm()
					} else {
						cw.loadACPForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.acpFormFields(c)
		}
	})
}

func (cw *ChatWindow) acpFormFields(c *mgo.Context) {
	f := &cw.acpForm
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "连接名称"})
		minput.TextInput(c, &f.command, minput.TextInputOptions{Label: "命令", Placeholder: "如 claude-code-acp 或 gemini"})
		minput.TextArea(c, &f.args, minput.TextAreaOptions{Label: "参数", Placeholder: "每行一个参数", MinHeight: 60})
		minput.TextArea(c, &f.env, minput.TextAreaOptions{Label: "环境变量", Placeholder: "KEY=VALUE，每行一个", MinHeight: 60})
		minput.TextInput(c, &f.cwd, minput.TextInputOptions{Label: "工作目录", Placeholder: "留空使用当前目录"})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cw.saveACP()
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
				cw.deleteACP()
			}
		})
	})
}

func (cw *ChatWindow) loadACPForm(i int) {
	a := cw.cfg.ACPAgents[i]
	cw.acpForm = acpForm{
		editing: i, name: a.Name, command: a.Command,
		args: strings.Join(a.Args, "\n"), env: joinKV(a.Env), cwd: a.Cwd, disabled: a.Disabled,
	}
}

func (cw *ChatWindow) clearACPForm() {
	cw.acpForm = acpForm{editing: -1}
}

func (cw *ChatWindow) saveACP() {
	f := &cw.acpForm
	if strings.TrimSpace(f.name) == "" || strings.TrimSpace(f.command) == "" {
		cw.toast("名称与命令不能为空", mcore.SeverityError)
		return
	}
	conn := config.ACPConnection{
		Name: f.name, Command: f.command, Args: splitLines(f.args),
		Env: parseKV(f.env), Cwd: f.cwd, Disabled: f.disabled,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.ACPAgents) {
		cw.cfg.ACPAgents[f.editing] = conn
	} else {
		cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, conn)
		f.editing = len(cw.cfg.ACPAgents) - 1
	}
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存连接 "+conn.Name, mcore.SeveritySuccess)
}

func (cw *ChatWindow) deleteACP() {
	f := &cw.acpForm
	if f.editing < 0 || f.editing >= len(cw.cfg.ACPAgents) {
		return
	}
	idx := f.editing
	name := cw.cfg.ACPAgents[idx].Name
	cw.askConfirm("删除连接", "确定删除「"+name+"」？", func() {
		cw.acpCloseConn(name)
		cw.cfg.ACPAgents = append(cw.cfg.ACPAgents[:idx], cw.cfg.ACPAgents[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearACPForm()
		cw.toast("连接已删除", mcore.SeveritySuccess)
	})
}

type acpClient struct{ cw *ChatWindow }

var _ acp.Client = acpClient{}

type acpUpdate struct {
	kind   string
	id     string
	name   string
	text   string
	args   string
	result string
	status string
}

func parseACPUpdate(u acp.SessionUpdate) (acpUpdate, bool) {
	switch {
	case u.AgentMessageChunk != nil:
		if t := u.AgentMessageChunk.Content.Text; t != nil && t.Text != "" {
			return acpUpdate{kind: "text", text: t.Text}, true
		}
	case u.AgentThoughtChunk != nil:
		if t := u.AgentThoughtChunk.Content.Text; t != nil && t.Text != "" {
			return acpUpdate{kind: "reasoning", text: t.Text}, true
		}
	case u.ToolCall != nil:
		return acpUpdate{
			kind:   "tool",
			id:     string(u.ToolCall.ToolCallId),
			name:   u.ToolCall.Title,
			args:   acpArgsJSON(u.ToolCall.RawInput),
			result: acpContentText(u.ToolCall.Content),
			status: string(u.ToolCall.Status),
		}, true
	case u.ToolCallUpdate != nil:
		title := ""
		if u.ToolCallUpdate.Title != nil {
			title = *u.ToolCallUpdate.Title
		}
		status := ""
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		return acpUpdate{
			kind:   "tool",
			id:     string(u.ToolCallUpdate.ToolCallId),
			name:   title,
			args:   acpArgsJSON(u.ToolCallUpdate.RawInput),
			result: acpContentText(u.ToolCallUpdate.Content),
			status: status,
		}, true
	}
	return acpUpdate{}, false
}

func acpArgsJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := string(b)
	if s == "null" || s == "{}" || s == `""` {
		return ""
	}
	return s
}

func acpContentText(items []acp.ToolCallContent) string {
	var b strings.Builder
	for _, it := range items {
		if it.Content != nil {
			if t := it.Content.Content.Text; t != nil && t.Text != "" {
				b.WriteString(t.Text)
				b.WriteByte('\n')
			}
		}
		if it.Diff != nil {
			fmt.Fprintf(&b, "diff: %s\n", it.Diff.Path)
		}
	}
	return strings.TrimSpace(b.String())
}

func (a acpClient) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	sess := n.SessionId
	if m := n.Update.CurrentModeUpdate; m != nil {
		a.cw.postMain(func() { a.cw.acpSetModeState(sess, m.CurrentModeId) })
		return nil
	}
	if cfg := n.Update.ConfigOptionUpdate; cfg != nil {
		opts := cfg.ConfigOptions
		a.cw.postMain(func() { a.cw.acpSetConfigState(sess, opts) })
		return nil
	}
	if ac := n.Update.AvailableCommandsUpdate; ac != nil {
		cmds := ac.AvailableCommands
		a.cw.acpMu.Lock()
		if rt := a.cw.acpConvs[a.cw.acpConvBySess[sess]]; rt != nil {
			rt.commands = cmds
		}
		a.cw.acpMu.Unlock()
		a.cw.postMain(func() { a.cw.invalidate() })
		return nil
	}
	upd, ok := parseACPUpdate(n.Update)
	if !ok {
		return nil
	}
	if upd.kind == "text" {
		a.cw.acpMu.Lock()
		if b := a.cw.channelReply[sess]; b != nil {
			b.WriteString(upd.text)
		}
		a.cw.acpMu.Unlock()
	}
	a.cw.postMain(func() {
		a.cw.acpMu.Lock()
		convID := a.cw.acpConvBySess[sess]
		a.cw.acpMu.Unlock()
		if convID != "" {
			a.cw.acpApplyToConv(convID, upd)
		}
	})
	return nil
}

func (cw *ChatWindow) acpToolDuration(tc *models.ToolCall, id, status string) {
	if id == "" {
		return
	}
	switch status {
	case string(acp.ToolCallStatusPending), string(acp.ToolCallStatusInProgress):
		cw.acpMu.Lock()
		if _, ok := cw.toolStart[id]; !ok {
			cw.toolStart[id] = time.Now()
		}
		cw.acpMu.Unlock()
	case string(acp.ToolCallStatusCompleted), string(acp.ToolCallStatusFailed):
		cw.acpMu.Lock()
		start, ok := cw.toolStart[id]
		delete(cw.toolStart, id)
		cw.acpMu.Unlock()
		if ok {
			tc.DurationMs = time.Since(start).Milliseconds()
		}
	}
}

func (cw *ChatWindow) acpApplyToConv(convID string, upd acpUpdate) {
	cw.acpMu.Lock()
	conv := cw.acpConvObj[convID]
	cw.acpMu.Unlock()
	if conv == nil {
		return
	}
	n := len(conv.Messages)
	if n == 0 || conv.Messages[n-1].Role != "assistant" {
		conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Timestamp: time.Now()})
	}
	last := &conv.Messages[len(conv.Messages)-1]
	switch upd.kind {
	case "text":
		last.Content += upd.text
		appendSegmentText(last, "text", upd.text)
	case "reasoning":
		last.Reasoning += upd.text
		appendSegmentText(last, "reasoning", upd.text)
	case "tool":
		if upd.id != "" {
			for i := range last.ToolCalls {
				if last.ToolCalls[i].ID == upd.id {
					tc := &last.ToolCalls[i]
					if upd.name != "" {
						tc.Name = upd.name
					}
					if upd.args != "" {
						tc.Arguments = upd.args
					}
					if upd.result != "" {
						tc.Result = upd.result
					}
					if upd.status != "" {
						tc.Status = upd.status
					}
					if upd.status == string(acp.ToolCallStatusFailed) {
						tc.Error = "failed"
					}
					cw.acpToolDuration(tc, upd.id, upd.status)
					cw.acpSyncToolSegment(last, tc)
					cw.invalidate()
					return
				}
			}
		}
		if upd.name == "" {
			return
		}
		tc := models.ToolCall{ID: upd.id, Name: upd.name, Arguments: upd.args, Result: upd.result, Status: upd.status}
		if upd.status == string(acp.ToolCallStatusFailed) {
			tc.Error = "failed"
		}
		last.ToolCalls = append(last.ToolCalls, tc)
		idx := len(last.ToolCalls) - 1
		cw.acpToolDuration(&last.ToolCalls[idx], upd.id, upd.status)
		cw.acpSyncToolSegment(last, &last.ToolCalls[idx])
	}
	cw.invalidate()
}

// acpSyncToolSegment keeps the ordered segment view in step with a tool call so
// an ACP reply renders interleaved (thinking, tool, text, ...) like the ReAct
// path instead of one collapsed group of every call.
func (cw *ChatWindow) acpSyncToolSegment(m *models.Message, tc *models.ToolCall) {
	if tc.ID == "" {
		return
	}
	for i := range m.Segments {
		s := &m.Segments[i]
		if s.Kind == "tool" && s.ToolCall != nil && s.ToolCall.ID == tc.ID {
			cp := *tc
			s.ToolCall = &cp
			return
		}
	}
	cp := *tc
	m.Segments = append(m.Segments, models.Segment{Kind: "tool", ToolCall: &cp})
}

func (a acpClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	a.cw.acpMu.Lock()
	auto := a.cw.channelSess[p.SessionId]
	a.cw.acpMu.Unlock()
	if auto {
		if r, ok := acpAutoApprove(p); ok {
			return r, nil
		}
	}
	reply := make(chan acp.RequestPermissionResponse, 1)
	req := p
	a.cw.postMain(func() {
		a.cw.permissionReq = &req
		a.cw.permissionCh = reply
		a.cw.permissionShow = true
	})
	select {
	case r := <-reply:
		return r, nil
	case <-ctx.Done():
		return acpCancelled(), ctx.Err()
	}
}

func acpAutoApprove(p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, bool) {
	var id acp.PermissionOptionId
	for _, opt := range p.Options {
		if opt.Kind == acp.PermissionOptionKindAllowAlways {
			id = opt.OptionId
			break
		}
	}
	if id == "" {
		for _, opt := range p.Options {
			if opt.Kind == acp.PermissionOptionKindAllowOnce {
				id = opt.OptionId
				break
			}
		}
	}
	if id == "" {
		return acpCancelled(), false
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: id}},
	}, true
}

func (a acpClient) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, fmt.Errorf("filesystem access is not supported")
}

func (a acpClient) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, fmt.Errorf("filesystem access is not supported")
}

func (a acpClient) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("terminal access is not supported")
}

func acpCancelled() acp.RequestPermissionResponse {
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
	}
}

// acpPermissionDialog asks the user to allow or deny a tool call.
func (cw *ChatWindow) acpPermissionDialog(c *mgo.Context) {
	if cw.permissionReq == nil {
		return
	}
	req := cw.permissionReq
	title := "工具调用"
	if req.ToolCall.Title != nil {
		title = *req.ToolCall.Title
	}
	moverlay.Dialog(c, &cw.permissionShow, moverlay.DialogOptions{
		Title:       "Agent 请求权限",
		Description: title,
		Actions: func() {
			for _, opt := range req.Options {
				id := opt.OptionId
				if minput.Button(c, opt.Name, minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
					cw.resolvePermission(acp.RequestPermissionResponse{
						Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: id}},
					})
				}
			}
			if minput.Button(c, "拒绝", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
				cw.resolvePermission(acpCancelled())
			}
		},
	}, func() {
		mgo.Text(c, "该操作需要你的授权：").TextColor(mcore.Tokens(c).TextMuted)
	})
	if !cw.permissionShow && cw.permissionReq != nil {
		cw.resolvePermission(acpCancelled())
	}
}

func (cw *ChatWindow) resolvePermission(r acp.RequestPermissionResponse) {
	if cw.permissionCh != nil {
		cw.permissionCh <- r
	}
	cw.permissionReq = nil
	cw.permissionCh = nil
	cw.permissionShow = false
}

// acpConv is a conversation's live ACP session.
type acpConv struct {
	cmd       *exec.Cmd
	conn      *acp.ClientSideConnection
	sessionID acp.SessionId
	convID    string
	modes     []acp.SessionMode
	modeID    acp.SessionModeId
	configs   []acp.SessionConfigOption
	commands  []acp.AvailableCommand
}

// acpPersist stores a mode/config choice on the conversation so it survives a restart.
func (cw *ChatWindow) acpPersist(rt *acpConv, key, value string) {
	conv := cw.acpConvObj[rt.convID]
	if conv == nil {
		return
	}
	if key == "mode" {
		conv.ACPMode = value
	} else {
		if conv.ACPConfig == nil {
			conv.ACPConfig = map[string]string{}
		}
		conv.ACPConfig[key] = value
	}
	_ = cw.convManager.SaveConversation(conv)
}

func acpConfigId(opt acp.SessionConfigOption) acp.SessionConfigId {
	if opt.Select != nil {
		return opt.Select.Id
	}
	if opt.Boolean != nil {
		return opt.Boolean.Id
	}
	return ""
}

// acpRuntimeFor returns the live session of a conversation, if any.
func (cw *ChatWindow) acpRuntimeFor(convID string) *acpConv {
	cw.acpMu.Lock()
	defer cw.acpMu.Unlock()
	return cw.acpConvs[convID]
}

func (cw *ChatWindow) acpInitializing(convID string) bool {
	cw.acpMu.Lock()
	defer cw.acpMu.Unlock()
	return cw.acpStarting[convID] && cw.acpConvs[convID] == nil
}

func (cw *ChatWindow) acpSetMode(rt *acpConv, id acp.SessionModeId) {
	rt.modeID = id
	cw.acpPersist(rt, "mode", string(id))
	conn, sess := rt.conn, rt.sessionID
	go func() {
		_, _ = conn.SetSessionMode(context.Background(), acp.SetSessionModeRequest{SessionId: sess, ModeId: id})
	}()
}

func (cw *ChatWindow) acpSetConfigValue(rt *acpConv, id acp.SessionConfigId, value acp.SessionConfigValueId) {
	cw.acpPersist(rt, string(id), string(value))
	conn, sess := rt.conn, rt.sessionID
	go func() {
		_, _ = conn.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
			ValueId: &acp.SetSessionConfigOptionValueId{SessionId: sess, ConfigId: id, Value: value},
		})
	}()
}

func (cw *ChatWindow) acpSetConfigBool(rt *acpConv, id acp.SessionConfigId, value bool) {
	cw.acpPersist(rt, string(id), strconv.FormatBool(value))
	conn, sess := rt.conn, rt.sessionID
	go func() {
		_, _ = conn.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
			Boolean: &acp.SetSessionConfigOptionBoolean{SessionId: sess, ConfigId: id, Value: value},
		})
	}()
}

// acpSetModeState records a mode change reported by the agent.
func (cw *ChatWindow) acpSetModeState(sess acp.SessionId, id acp.SessionModeId) {
	cw.acpMu.Lock()
	convID := cw.acpConvBySess[sess]
	rt := cw.acpConvs[convID]
	cw.acpMu.Unlock()
	if rt != nil {
		rt.modeID = id
		cw.acpPersist(rt, "mode", string(id))
		cw.invalidate()
	}
}

// acpSetConfigState records config options reported by the agent.
func (cw *ChatWindow) acpSetConfigState(sess acp.SessionId, opts []acp.SessionConfigOption) {
	cw.acpMu.Lock()
	convID := cw.acpConvBySess[sess]
	rt := cw.acpConvs[convID]
	cw.acpMu.Unlock()
	if rt != nil {
		rt.configs = opts
		cw.invalidate()
	}
}

func acpSelectOptions(o acp.SessionConfigSelectOptions) []minput.SelectOption[string] {
	var out []minput.SelectOption[string]
	add := func(list []acp.SessionConfigSelectOption) {
		for _, it := range list {
			out = append(out, minput.SelectOption[string]{Value: string(it.Value), Label: it.Name})
		}
	}
	if o.Ungrouped != nil {
		add(*o.Ungrouped)
	}
	if o.Grouped != nil {
		for _, g := range *o.Grouped {
			add(g.Options)
		}
	}
	return out
}

func (cw *ChatWindow) findACPConn(name string) (config.ACPConnection, bool) {
	for _, a := range cw.cfg.ACPAgents {
		if a.Name == name {
			return a, true
		}
	}
	return config.ACPConnection{}, false
}

// acpApplyUsage records a turn's token usage when the adapter reports it; the
// ACP usage fields are unstable and most adapters leave them empty.
func (cw *ChatWindow) acpApplyUsage(msg *models.Message, resp acp.PromptResponse) {
	if resp.Usage == nil {
		return
	}
	msg.PromptTokens = resp.Usage.InputTokens
	msg.CompletionTokens = resp.Usage.OutputTokens
	msg.TotalTokens = resp.Usage.TotalTokens
}

// acpChatStart runs one turn of a conversation through its ACP coding CLI.
func (cw *ChatWindow) acpChatStart(conv *models.Conversation, connName string, idx int, text string) {
	conn, ok := cw.findACPConn(connName)
	if !ok {
		conv.Messages[idx].Content = "⚠️ 未找到 ACP 连接：" + connName
		cw.streaming = false
		return
	}
	cw.acpConvObj[conv.ID] = conv
	go func() {
		rt, err := cw.acpEnsureSession(conv, conn)
		if err != nil {
			cw.postMain(func() {
				conv.Messages[idx].Content = "⚠️ " + err.Error()
				cw.streaming = false
				_ = cw.convManager.SaveConversation(conv)
			})
			return
		}
		resp, perr := rt.conn.Prompt(context.Background(), acp.PromptRequest{
			SessionId: rt.sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
		})
		cw.postMain(func() {
			if perr != nil && conv.Messages[idx].Content == "" {
				conv.Messages[idx].Content = "⚠️ " + perr.Error()
			}
			cw.acpApplyUsage(&conv.Messages[idx], resp)
			cw.streaming = false
			conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
			cw.notifySessionComplete(conv, conv.Messages[idx].Content)
		})
	}()
}

func (cw *ChatWindow) acpEnsureSession(conv *models.Conversation, conn config.ACPConnection) (*acpConv, error) {
	cw.acpMu.Lock()
	if rt := cw.acpConvs[conv.ID]; rt != nil {
		cw.acpMu.Unlock()
		return rt, nil
	}
	if cw.acpStarting[conv.ID] {
		cw.acpMu.Unlock()
		return nil, fmt.Errorf("ACP 会话正在建立")
	}
	cw.acpStarting[conv.ID] = true
	cw.acpMu.Unlock()
	cw.invalidate()
	defer func() {
		cw.acpMu.Lock()
		delete(cw.acpStarting, conv.ID)
		cw.acpMu.Unlock()
	}()

	cmd := exec.Command(conn.Command, conn.Args...)
	cwd := conv.Workspace
	if conn.Cwd != "" {
		cwd = conn.Cwd
	}
	if cwd != "" {
		cmd.Dir = cwd
	}
	env := os.Environ()
	for k, v := range conn.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	// Own process group so the whole tree (npx → node) dies together.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// stderr is drained into a bounded buffer: inheriting it can block or
	// write to an invalid descriptor in a GUI launch.
	var errBuf acpTailBuffer
	cmd.Stderr = &errBuf
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	rt := &acpConv{cmd: cmd, convID: conv.ID}
	go func() {
		_ = cmd.Wait()
		cw.acpReap(rt)
	}()
	clientConn := acp.NewClientSideConnection(acpClient{cw: cw}, stdin, stdout)
	rt.conn = clientConn
	fail := func(err error) (*acpConv, error) {
		cw.acpKillProcess(rt)
		if msg := errBuf.String(); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), acpHandshakeTimeout)
	defer cancel()
	if _, err := clientConn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{},
	}); err != nil {
		return fail(err)
	}
	resp, err := clientConn.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		return fail(err)
	}
	rt.sessionID = resp.SessionId
	rt.configs = resp.ConfigOptions
	if resp.Modes != nil {
		rt.modes = resp.Modes.AvailableModes
		rt.modeID = resp.Modes.CurrentModeId
	}
	cw.acpMu.Lock()
	cw.acpConvs[conv.ID] = rt
	cw.acpConvBySess[rt.sessionID] = conv.ID
	cw.acpMu.Unlock()
	cw.acpApplySaved(rt, conv)
	cw.invalidate()
	return rt, nil
}

// acpKillProcess kills the session's whole process group and reaps it.
func (cw *ChatWindow) acpKillProcess(rt *acpConv) {
	if rt == nil || rt.cmd == nil || rt.cmd.Process == nil {
		return
	}
	pid := rt.cmd.Process.Pid
	// Negative pid targets the group made in acpEnsureSession.
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = rt.cmd.Process.Kill()
}

// acpReap drops a session whose CLI exited so a later warmup restarts it.
func (cw *ChatWindow) acpReap(rt *acpConv) {
	cw.acpMu.Lock()
	if cur := cw.acpConvs[rt.convID]; cur == rt {
		delete(cw.acpConvs, rt.convID)
	}
	if rt.sessionID != "" {
		delete(cw.acpConvBySess, rt.sessionID)
	}
	cw.acpMu.Unlock()
	cw.invalidate()
}

// acpTailBuffer keeps the last cap bytes written, for diagnosing a CLI that
// fails at startup without unbounded memory.
type acpTailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

const acpTailCap = 4096

func (b *acpTailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > acpTailCap {
		b.buf = b.buf[len(b.buf)-acpTailCap:]
	}
	return len(p), nil
}

func (b *acpTailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.buf))
}

// acpApplySaved restores a conversation's saved mode and config choices.
func (cw *ChatWindow) acpApplySaved(rt *acpConv, conv *models.Conversation) {
	if conv.ACPMode != "" {
		for _, m := range rt.modes {
			if string(m.Id) == conv.ACPMode {
				rt.modeID = m.Id
				_, _ = rt.conn.SetSessionMode(context.Background(), acp.SetSessionModeRequest{SessionId: rt.sessionID, ModeId: m.Id})
				break
			}
		}
	}
	for _, opt := range rt.configs {
		id := acpConfigId(opt)
		v, ok := conv.ACPConfig[string(id)]
		if !ok {
			continue
		}
		switch {
		case opt.Select != nil:
			opt.Select.CurrentValue = acp.SessionConfigValueId(v)
			_, _ = rt.conn.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
				ValueId: &acp.SetSessionConfigOptionValueId{SessionId: rt.sessionID, ConfigId: id, Value: acp.SessionConfigValueId(v)},
			})
		case opt.Boolean != nil:
			b, err := strconv.ParseBool(v)
			if err != nil {
				continue
			}
			opt.Boolean.CurrentValue = b
			_, _ = rt.conn.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
				Boolean: &acp.SetSessionConfigOptionBoolean{SessionId: rt.sessionID, ConfigId: id, Value: b},
			})
		}
	}
}

// acpWarmup starts a conversation's ACP session so its model and config
// options load before the first message.
func (cw *ChatWindow) acpWarmup(conv *models.Conversation) {
	name := strings.TrimPrefix(conv.Agent, "acp:")
	conn, ok := cw.findACPConn(name)
	if !ok || cw.acpRuntimeFor(conv.ID) != nil {
		return
	}
	cw.acpConvObj[conv.ID] = conv
	go func() {
		if _, err := cw.acpEnsureSession(conv, conn); err != nil {
			cw.postMain(func() { cw.status = "ACP 会话启动失败：" + err.Error() })
		}
	}()
}

// acpCloseConn closes the sessions of conversations using a connection.
func (cw *ChatWindow) acpCloseConn(name string) {
	cw.acpMu.Lock()
	var ids []string
	for id := range cw.acpConvs {
		ids = append(ids, id)
	}
	cw.acpMu.Unlock()
	for _, id := range ids {
		if conv := cw.acpConvObj[id]; conv != nil && conv.Agent == "acp:"+name {
			cw.acpConvClose(id)
		}
	}
}

func (cw *ChatWindow) acpConvClose(convID string) {
	cw.acpMu.Lock()
	rt := cw.acpConvs[convID]
	delete(cw.acpConvs, convID)
	if rt != nil {
		delete(cw.acpConvBySess, rt.sessionID)
	}
	cw.acpMu.Unlock()
	delete(cw.acpConvObj, convID)
	if rt != nil {
		cw.acpKillProcess(rt)
	}
}
