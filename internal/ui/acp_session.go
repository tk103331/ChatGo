package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
)

// acpState holds every piece of ACP runtime state, guarded by mu.
type acpState struct {
	mu sync.Mutex

	convs      map[string]*acpConv
	convBySess map[acp.SessionId]string
	convObj    map[string]*models.Conversation
	starting   map[string]bool

	channelSess  map[acp.SessionId]bool
	channelReply map[acp.SessionId]*strings.Builder
	toolStart    map[string]time.Time

	permissionReq  *acp.RequestPermissionRequest
	permissionCh   chan acp.RequestPermissionResponse
	permissionShow bool
}

func newACPState() acpState {
	return acpState{
		convs:        map[string]*acpConv{},
		convBySess:   map[acp.SessionId]string{},
		convObj:      map[string]*models.Conversation{},
		starting:     map[string]bool{},
		channelSess:  map[acp.SessionId]bool{},
		channelReply: map[acp.SessionId]*strings.Builder{},
		toolStart:    map[string]time.Time{},
	}
}

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
	conv := cw.acp.convObj[rt.convID]
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
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	return cw.acp.convs[convID]
}

func (cw *ChatWindow) acpInitializing(convID string) bool {
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	return cw.acp.starting[convID] && cw.acp.convs[convID] == nil
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
	cw.acp.mu.Lock()
	convID := cw.acp.convBySess[sess]
	rt := cw.acp.convs[convID]
	cw.acp.mu.Unlock()
	if rt != nil {
		rt.modeID = id
		cw.acpPersist(rt, "mode", string(id))
		cw.invalidate()
	}
}

// acpSetConfigState records config options reported by the agent.

func (cw *ChatWindow) acpSetConfigState(sess acp.SessionId, opts []acp.SessionConfigOption) {
	cw.acp.mu.Lock()
	convID := cw.acp.convBySess[sess]
	rt := cw.acp.convs[convID]
	cw.acp.mu.Unlock()
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

// acpConfigCoversModes reports whether the adapter's config options already
// carry a mode or thought-level selector. The ACP spec lets agents send both
// configOptions and the older modes field in parallel, telling clients that
// support configOptions to use them exclusively, so showing both duplicates
// the control (for example two "Autoreview" or two "Thinking" choices).
func acpConfigCoversModes(opts []acp.SessionConfigOption) bool {
	for _, o := range opts {
		var (
			name string
			cat  *acp.SessionConfigOptionCategory
		)
		if o.Select != nil {
			name, cat = o.Select.Name, o.Select.Category
		} else if o.Boolean != nil {
			name, cat = o.Boolean.Name, o.Boolean.Category
		}
		if cat != nil && (*cat == acp.SessionConfigOptionCategoryMode || *cat == acp.SessionConfigOptionCategoryThoughtLevel) {
			return true
		}
		low := strings.ToLower(name)
		if strings.Contains(low, "thinking") || strings.Contains(low, "thought") ||
			low == "mode" || strings.Contains(low, "approval") {
			return true
		}
	}
	return false
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

func (cw *ChatWindow) acpApplyUsage(conv *models.Conversation, replyID string, resp acp.PromptResponse) {
	if resp.Usage == nil {
		return
	}
	t := turnOf(conv, replyID)
	if t == nil {
		return
	}
	t.Input = resp.Usage.InputTokens
	t.Output = resp.Usage.OutputTokens
	t.Total = resp.Usage.TotalTokens
}

// acpChatStart runs one turn of a conversation through its ACP coding CLI.

func (cw *ChatWindow) acpChatStart(conv *models.Conversation, connName string, idx int, text string) {
	conn, ok := cw.findACPConn(connName)
	if !ok {
		conv.Messages[idx].Content = "⚠️ 未找到 ACP 连接：" + connName
		cw.streaming = false
		return
	}
	cw.acp.convObj[conv.ID] = conv
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
			cw.acpApplyUsage(conv, conv.Messages[idx].ID, resp)
			if t := turnOf(conv, conv.Messages[idx].ID); t != nil {
				t.Assistant = conv.Messages[idx].Content
				t.EndedAt = time.Now()
			}
			cw.streaming = false
			conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
			cw.notifySessionComplete(conv, conv.Messages[idx].Content)
		})
	}()
}

func (cw *ChatWindow) acpEnsureSession(conv *models.Conversation, conn config.ACPConnection) (*acpConv, error) {
	cw.acp.mu.Lock()
	if rt := cw.acp.convs[conv.ID]; rt != nil {
		cw.acp.mu.Unlock()
		return rt, nil
	}
	if cw.acp.starting[conv.ID] {
		cw.acp.mu.Unlock()
		return nil, fmt.Errorf("ACP 会话正在建立")
	}
	cw.acp.starting[conv.ID] = true
	cw.acp.mu.Unlock()
	cw.invalidate()
	defer func() {
		cw.acp.mu.Lock()
		delete(cw.acp.starting, conv.ID)
		cw.acp.mu.Unlock()
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
	cmd.SysProcAttr = acpSysProcAttr()
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
	cw.acp.mu.Lock()
	cw.acp.convs[conv.ID] = rt
	cw.acp.convBySess[rt.sessionID] = conv.ID
	cw.acp.mu.Unlock()
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
	acpKillProcessGroup(pid)
	_ = rt.cmd.Process.Kill()
}

// acpReap drops a session whose CLI exited so a later warmup restarts it.

func (cw *ChatWindow) acpReap(rt *acpConv) {
	cw.acp.mu.Lock()
	if cur := cw.acp.convs[rt.convID]; cur == rt {
		delete(cw.acp.convs, rt.convID)
	}
	if rt.sessionID != "" {
		delete(cw.acp.convBySess, rt.sessionID)
	}
	cw.acp.mu.Unlock()
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
	cw.acp.convObj[conv.ID] = conv
	go func() {
		if _, err := cw.acpEnsureSession(conv, conn); err != nil {
			cw.postMain(func() { cw.status = "ACP 会话启动失败：" + err.Error() })
		}
	}()
}

// acpCloseConn closes the sessions of conversations using a connection.

func (cw *ChatWindow) acpCloseConn(name string) {
	cw.acp.mu.Lock()
	var ids []string
	for id := range cw.acp.convs {
		ids = append(ids, id)
	}
	cw.acp.mu.Unlock()
	for _, id := range ids {
		if conv := cw.acp.convObj[id]; conv != nil && conv.Agent == "acp:"+name {
			cw.acpConvClose(id)
		}
	}
}

func (cw *ChatWindow) acpConvClose(convID string) {
	cw.acp.mu.Lock()
	rt := cw.acp.convs[convID]
	delete(cw.acp.convs, convID)
	if rt != nil {
		delete(cw.acp.convBySess, rt.sessionID)
	}
	cw.acp.mu.Unlock()
	delete(cw.acp.convObj, convID)
	if rt != nil {
		cw.acpKillProcess(rt)
	}
}
