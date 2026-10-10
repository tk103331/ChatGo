package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "github.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

type acpForm struct {
	editing int
	name    string
	command string
	args    string
	env     string
	cwd     string
}

func (cw *ChatWindow) acpRow(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	a := cw.cfg.ACPAgents[i]
	b := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Radius(mtheme.ControlRadius)
	if cw.acpForm.editing == i {
		b.Background(k.Selection)
	}
	b.Children(func() {
		mgo.Text(c, a.Name).SingleLine()
	})
	if b.Clicked() {
		cw.loadACPForm(i)
	}
}

func (cw *ChatWindow) acpFormView(c *mgo.Context) {
	f := &cw.acpForm
	u := mcore.Density(c).Unit()
	mgo.Column(c).Gap(u).Children(func() {
		minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "连接名称"})
		minput.TextInput(c, &f.command, minput.TextInputOptions{Label: "命令", Placeholder: "如 claude-code-acp 或 gemini"})
		minput.TextArea(c, &f.args, minput.TextAreaOptions{Label: "参数", Placeholder: "每行一个参数", MinHeight: 60})
		minput.TextArea(c, &f.env, minput.TextAreaOptions{Label: "环境变量", Placeholder: "KEY=VALUE，每行一个", MinHeight: 60})
		minput.TextInput(c, &f.cwd, minput.TextInputOptions{Label: "工作目录", Placeholder: "留空使用当前目录"})
		mgo.Row(c).Gap(u).Wrap().Children(func() {
			if minput.Button(c, "新增", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cw.clearACPForm()
			}
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cw.saveACP()
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.editing < 0}).Clicked() {
				cw.deleteACP()
			}
		})
	})
}

func (cw *ChatWindow) loadACPForm(i int) {
	a := cw.cfg.ACPAgents[i]
	cw.acpForm = acpForm{
		editing: i, name: a.Name, command: a.Command,
		args: strings.Join(a.Args, "\n"), env: joinKV(a.Env), cwd: a.Cwd,
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
		Env: parseKV(f.env), Cwd: f.cwd,
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
	kind string
	id   string
	name string
	text string
}

func parseACPUpdate(u acp.SessionUpdate) (acpUpdate, bool) {
	switch {
	case u.AgentMessageChunk != nil:
		if t := u.AgentMessageChunk.Content.Text; t != nil && t.Text != "" {
			return acpUpdate{kind: "text", text: t.Text}, true
		}
	case u.AgentThoughtChunk != nil:
		if t := u.AgentThoughtChunk.Content.Text; t != nil && t.Text != "" {
			return acpUpdate{kind: "text", text: t.Text}, true
		}
	case u.ToolCall != nil:
		return acpUpdate{kind: "tool", id: string(u.ToolCall.ToolCallId), name: u.ToolCall.Title, text: string(u.ToolCall.Status)}, true
	case u.ToolCallUpdate != nil:
		title, status := "", ""
		if u.ToolCallUpdate.Title != nil {
			title = *u.ToolCallUpdate.Title
		}
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		return acpUpdate{kind: "tool", id: string(u.ToolCallUpdate.ToolCallId), name: title, text: status}, true
	}
	return acpUpdate{}, false
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
	upd, ok := parseACPUpdate(n.Update)
	if !ok {
		return nil
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

func (cw *ChatWindow) acpApplyToConv(convID string, upd acpUpdate) {
	conv := cw.acpConvObj[convID]
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
	case "tool":
		if upd.id != "" {
			for i := range last.ToolCalls {
				if last.ToolCalls[i].ID == upd.id {
					if upd.text != "" {
						last.ToolCalls[i].Result = upd.text
					}
					cw.invalidate()
					return
				}
			}
		}
		if upd.name == "" {
			return
		}
		last.ToolCalls = append(last.ToolCalls, models.ToolCall{ID: upd.id, Name: upd.name, Result: upd.text})
	}
	cw.invalidate()
}

func (a acpClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
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
		_, perr := rt.conn.Prompt(context.Background(), acp.PromptRequest{
			SessionId: rt.sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
		})
		cw.postMain(func() {
			if perr != nil && conv.Messages[idx].Content == "" {
				conv.Messages[idx].Content = "⚠️ " + perr.Error()
			}
			cw.streaming = false
			conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
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
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	clientConn := acp.NewClientSideConnection(acpClient{cw: cw}, stdin, stdout)
	rt := &acpConv{cmd: cmd, conn: clientConn, convID: conv.ID}
	ctx := context.Background()
	if _, err := clientConn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{},
	}); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	resp, err := clientConn.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
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
	if rt != nil && rt.cmd != nil && rt.cmd.Process != nil {
		_ = rt.cmd.Process.Kill()
	}
}
