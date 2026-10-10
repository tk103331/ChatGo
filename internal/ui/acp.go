package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "github.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
	acp "github.com/coder/acp-go-sdk"
)

type acpForm struct {
	editing int
	name    string
	command string
	args    string
	env     string
	cwd     string
}

type acpEntry struct {
	role string
	text string
}

type acpRuntime struct {
	cmd       *exec.Cmd
	conn      *acp.ClientSideConnection
	sessionID acp.SessionId
	entries   []acpEntry
	status    string
	busy      bool
}

// acpPage manages ACP connections and drives a coding-CLI session.
func (cw *ChatWindow) acpPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "ACP 连接").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		mgo.Row(c).Fill().Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
			mgo.Column(c).Width(360).Shrink(0).Gap(u).Children(func() {
				mgo.List(c, &cw.acpList, len(cw.cfg.ACPAgents), func(i int) {
					cw.acpRow(c, i)
				}).MaxHeight(200)
				cw.acpFormView(c)
			})
			cw.acpSessionView(c)
		})
	})
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
		label := a.Name
		if cw.acp.cmd != nil && cw.acp.sessionID != "" && cw.acpName() == a.Name {
			label += " · 已连接"
		}
		mgo.Text(c, label).SingleLine()
	})
	if b.Clicked() {
		cw.loadACPForm(i)
	}
}

func (cw *ChatWindow) acpName() string {
	return cw.acpForm.name
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
			if minput.Button(c, "连接", minput.ButtonOptions{Variant: minput.Outline, Disabled: f.editing < 0}).Clicked() {
				cw.acpConnect()
			}
			if minput.Button(c, "断开", minput.ButtonOptions{Variant: minput.Outline, Disabled: cw.acp.cmd == nil}).Clicked() {
				cw.acpDisconnect()
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
		if cw.acpForm.editing == idx {
			cw.acpDisconnect()
		}
		cw.cfg.ACPAgents = append(cw.cfg.ACPAgents[:idx], cw.cfg.ACPAgents[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearACPForm()
		cw.toast("连接已删除", mcore.SeveritySuccess)
	})
}

func (cw *ChatWindow) acpSessionView(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		if cw.acp.cmd == nil {
			mgo.Text(c, "选择或新建一个连接，再点击「连接」启动本地 Coding CLI。").
				TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			return
		}
		mgo.Text(c, "会话状态："+cw.acp.status).TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Shrink(0)
		mgo.Scroll(c).Grow(1).MinHeight(0).Padding(u).Gap(u * 2).Children(func() {
			for _, e := range cw.acp.entries {
				acpEntryView(c, e)
			}
		})
		in := mchat.PromptComposer(c, &cw.acpInput, mchat.PromptComposerOptions{
			Placeholder: "输入指令，Enter 发送",
			Actions: func() {
				r := mchat.SendButton(c, mchat.SendButtonOptions{
					Streaming: cw.acp.busy,
					Disabled:  strings.TrimSpace(cw.acpInput) == "" && !cw.acp.busy,
				})
				if r.Sent() {
					cw.acpSend()
				}
			},
		})
		if in.Submitted() {
			cw.acpSend()
		}
	})
}

func acpEntryView(c *mgo.Context, e acpEntry) {
	k := mcore.Tokens(c)
	switch e.role {
	case "user":
		mchat.MessageBubble(c, mchat.MessageUser, mchat.MessageBubbleOptions{}, func() {
			mchat.MarkdownView(c, e.text)
		})
	case "assistant":
		mchat.MessageBubble(c, mchat.MessageAssistant, mchat.MessageBubbleOptions{}, func() {
			mchat.MarkdownView(c, e.text)
		})
	default:
		color := k.TextMuted
		if e.role == "error" {
			color = k.Danger
		}
		mgo.Text(c, e.text).TextColor(color).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	}
}

// acpConnect launches the selected CLI and opens an ACP session.
func (cw *ChatWindow) acpConnect() {
	conn, ok := cw.selectedACP()
	if !ok {
		return
	}
	cw.acpDisconnect()
	cmd := exec.Command(conn.Command, conn.Args...)
	if conn.Cwd != "" {
		cmd.Dir = conn.Cwd
	}
	env := os.Environ()
	for k, v := range conn.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cw.toast("启动失败："+err.Error(), mcore.SeverityError)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cw.toast("启动失败："+err.Error(), mcore.SeverityError)
		return
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cw.toast("启动失败："+err.Error(), mcore.SeverityError)
		return
	}
	conn2 := acp.NewClientSideConnection(acpClient{cw: cw}, stdin, stdout)
	cw.acp = acpRuntime{cmd: cmd, conn: conn2, status: "连接中…"}
	cwd := conn.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	go func() {
		ctx := context.Background()
		if _, err := conn2.Initialize(ctx, acp.InitializeRequest{
			ProtocolVersion:    acp.ProtocolVersionNumber,
			ClientCapabilities: acp.ClientCapabilities{},
		}); err != nil {
			cw.postMain(func() { cw.acp.status = "初始化失败：" + err.Error() })
			return
		}
		resp, err := conn2.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
		if err != nil {
			cw.postMain(func() { cw.acp.status = "新建会话失败：" + err.Error() })
			return
		}
		cw.postMain(func() {
			cw.acp.sessionID = resp.SessionId
			cw.acp.status = "已连接"
		})
	}()
}

func (cw *ChatWindow) acpDisconnect() {
	if cw.acp.cmd != nil && cw.acp.cmd.Process != nil {
		_ = cw.acp.cmd.Process.Kill()
	}
	wasName := cw.acpForm.name
	cw.acp = acpRuntime{}
	if wasName != "" {
		cw.toast("已断开连接", mcore.SeverityInfo)
	}
}

func (cw *ChatWindow) selectedACP() (config.ACPConnection, bool) {
	if cw.acpForm.editing >= 0 && cw.acpForm.editing < len(cw.cfg.ACPAgents) {
		return cw.cfg.ACPAgents[cw.acpForm.editing], true
	}
	return config.ACPConnection{}, false
}

// acpSend prompts the connected agent and streams its updates.
func (cw *ChatWindow) acpSend() {
	text := strings.TrimSpace(cw.acpInput)
	if text == "" || cw.acp.conn == nil || cw.acp.sessionID == "" || cw.acp.busy {
		return
	}
	cw.acpInput = ""
	cw.acp.entries = append(cw.acp.entries, acpEntry{role: "user", text: text})
	cw.acp.busy = true
	conn, session := cw.acp.conn, cw.acp.sessionID
	go func() {
		_, err := conn.Prompt(context.Background(), acp.PromptRequest{
			SessionId: session,
			Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
		})
		cw.postMain(func() {
			cw.acp.busy = false
			if err != nil {
				cw.acp.entries = append(cw.acp.entries, acpEntry{role: "error", text: err.Error()})
			}
		})
	}()
}

// acpClient implements acp.Client, relaying updates to the window.
type acpClient struct{ cw *ChatWindow }

var _ acp.Client = acpClient{}

func (a acpClient) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	a.onACPSessionUpdate(n)
	return nil
}

func (a acpClient) onACPSessionUpdate(n acp.SessionNotification) {
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		if t := u.AgentMessageChunk.Content.Text; t != nil && t.Text != "" {
			chunk := t.Text
			a.cw.postMain(func() { a.cw.acpAppendAssistant(chunk) })
		}
	case u.ToolCall != nil:
		line := fmt.Sprintf("🔧 %s（%s）", u.ToolCall.Title, u.ToolCall.Status)
		a.cw.postMain(func() { a.cw.acp.entries = append(a.cw.acp.entries, acpEntry{role: "tool", text: line}) })
	case u.ToolCallUpdate != nil:
		title, status := "", ""
		if u.ToolCallUpdate.Title != nil {
			title = *u.ToolCallUpdate.Title
		}
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		line := strings.TrimSpace(fmt.Sprintf("🔧 %s %s", title, status))
		if line != "🔧" {
			a.cw.postMain(func() { a.cw.acp.entries = append(a.cw.acp.entries, acpEntry{role: "tool", text: line}) })
		}
	}
}

func (cw *ChatWindow) acpAppendAssistant(chunk string) {
	n := len(cw.acp.entries)
	if n > 0 && cw.acp.entries[n-1].role == "assistant" {
		cw.acp.entries[n-1].text += chunk
		return
	}
	cw.acp.entries = append(cw.acp.entries, acpEntry{role: "assistant", text: chunk})
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
