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
)

// acpState holds every piece of ACP runtime state, guarded by mu.
type acpState struct {
	mu sync.Mutex

	convs      map[string]*acpConv
	convBySess map[acp.SessionId]string
	convObj    map[string]*models.Conversation
	starting   map[string]*acpStart

	// greeting marks the sessions whose CLI has not been prompted yet. An
	// adapter such as PiAgent announces itself in that window — its name,
	// version and the skills and extensions it loaded — as plain agent text
	// before the user has said anything, which would otherwise become a
	// stray assistant message ahead of every real reply.
	greeting map[acp.SessionId]bool

	channelSess  map[acp.SessionId]bool
	channelReply map[acp.SessionId]*strings.Builder
	toolStart    map[string]time.Time

	// configs holds each connection's full session config options, warmed at
	// startup for the 新会话 page.
	// models caches the model list each ACP connection's runtime reports,
	// fetched with a throwaway session for the agent picker. modelErrs
	// remembers a failed probe so the picker shows the reason instead of
	// probing again every frame, and modelsBusy marks a running probe.
	models     map[string][]acpModelOption
	modelErrs  map[string]string
	modelsBusy map[string]bool
	configs    map[string][]acp.SessionConfigOption

	permissionReq  *acp.RequestPermissionRequest
	permissionCh   chan acp.RequestPermissionResponse
	permissionShow bool

	// webSink routes a conversation's ACP updates to an embedded web run, so
	// they land under the run's lock instead of on the desktop's main thread.
	webSink map[string]func(func(*models.Conversation))
	// webPerm routes a conversation's permission requests to the browser.
	webPerm map[string]func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
}

func newACPState() acpState {
	return acpState{
		convs:        map[string]*acpConv{},
		convBySess:   map[acp.SessionId]string{},
		convObj:      map[string]*models.Conversation{},
		starting:     map[string]*acpStart{},
		greeting:     map[acp.SessionId]bool{},
		channelSess:  map[acp.SessionId]bool{},
		channelReply: map[acp.SessionId]*strings.Builder{},
		toolStart:    map[string]time.Time{},
		webSink:      map[string]func(func(*models.Conversation)){},
		webPerm:      map[string]func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error){},
		models:       map[string][]acpModelOption{},
		modelErrs:    map[string]string{},
		modelsBusy:   map[string]bool{},
		configs:      map[string][]acp.SessionConfigOption{},
	}
}

// acpStart is one in-flight session start: waiters block on done, then see
// the same error the starter got.
type acpStart struct {
	done chan struct{}
	err  error
}

type acpConv struct {
	cmd       *exec.Cmd
	conn      *acp.ClientSideConnection
	sessionID acp.SessionId
	convID    string
	// key is the session slot: convID for a normal conversation, and a
	// per-member key for a room, where one conversation hosts several
	// coding CLIs at once.
	key      string
	modes    []acp.SessionMode
	modeID   acp.SessionModeId
	configs  []acp.SessionConfigOption
	commands []acp.AvailableCommand

	// batch holds the tool calls still running in the current batch and the
	// batch's titles; a batch ends when the map empties, which is the
	// checkpoint trigger for the "batch" level.
	batch        map[string]bool
	batchSummary []string
}

// acpPersist stores a mode/config choice on the conversation so it survives a restart.

func (cw *ChatWindow) acpPersist(rt *acpConv, key, value string) {
	conv := cw.acp.convObj[rt.key]
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

// acpCancelConv asks the coding CLI to stop the turn running in a
// conversation. The agent answers the prompt as cancelled, so the ordinary
// completion path saves whatever it had produced.
func (cw *ChatWindow) acpCancelConv(convID string) {
	rt := cw.acpRuntimeFor(convID)
	if rt == nil || rt.sessionID == "" || rt.conn == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = rt.conn.Cancel(ctx, acp.CancelNotification{SessionId: rt.sessionID})
	}()
}

func (cw *ChatWindow) acpInitializing(convID string) bool {
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	_, starting := cw.acp.starting[convID]
	return starting && cw.acp.convs[convID] == nil
}

// acpSetTimeout bounds one config request to the CLI; a hung adapter must
// not leave the control in "applied" state forever.
const acpSetTimeout = 15 * time.Second

// acpSetMode asks the CLI to switch the session's mode. The new mode shows
// on screen right away, but it is saved on the conversation only once the
// CLI accepts it; a refusal restores the previous mode and says why briefly.
func (cw *ChatWindow) acpSetMode(rt *acpConv, id, old acp.SessionModeId) {
	rt.modeID = id
	cw.invalidate()
	conn, sess := rt.conn, rt.sessionID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), acpSetTimeout)
		defer cancel()
		_, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{SessionId: sess, ModeId: id})
		cw.postMain(func() {
			if err != nil {
				if rt.modeID == id {
					rt.modeID = old
				}
				cw.toastErrorDetail("模式切换未生效", acpFirstLine(err.Error()))
				cw.invalidate()
				return
			}
			cw.acpPersist(rt, "mode", string(id))
		})
	}()
}

// acpSetConfigValue asks the CLI to move a select option to value. The
// runtime's own answer replaces the local copy, so the control shows what
// the CLI really holds; a refusal restores the old value.
func (cw *ChatWindow) acpSetConfigValue(rt *acpConv, id acp.SessionConfigId, value, old acp.SessionConfigValueId) {
	conn, sess := rt.conn, rt.sessionID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), acpSetTimeout)
		defer cancel()
		resp, err := conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
			ValueId: &acp.SetSessionConfigOptionValueId{SessionId: sess, ConfigId: id, Value: value},
		})
		cw.postMain(func() {
			if err != nil {
				cw.acpRestoreConfig(rt, string(id), string(old))
				cw.toastErrorDetail("配置未生效", acpFirstLine(err.Error()))
				cw.invalidate()
				return
			}
			if len(resp.ConfigOptions) > 0 {
				rt.configs = resp.ConfigOptions
			}
			cw.acpPersist(rt, string(id), string(value))
			cw.invalidate()
		})
	}()
}

// acpSetConfigBool is acpSetConfigValue for a boolean option.
func (cw *ChatWindow) acpSetConfigBool(rt *acpConv, id acp.SessionConfigId, value, old bool) {
	conn, sess := rt.conn, rt.sessionID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), acpSetTimeout)
		defer cancel()
		resp, err := conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
			Boolean: &acp.SetSessionConfigOptionBoolean{SessionId: sess, ConfigId: id, Value: value},
		})
		cw.postMain(func() {
			if err != nil {
				cw.acpRestoreConfig(rt, string(id), strconv.FormatBool(old))
				cw.toastErrorDetail("配置未生效", acpFirstLine(err.Error()))
				cw.invalidate()
				return
			}
			if len(resp.ConfigOptions) > 0 {
				rt.configs = resp.ConfigOptions
			}
			cw.acpPersist(rt, string(id), strconv.FormatBool(value))
			cw.invalidate()
		})
	}()
}

// acpRestoreConfig puts a select or boolean option back to value after the
// CLI refused a change, so the control never shows a choice the runtime
// rejected.
func (cw *ChatWindow) acpRestoreConfig(rt *acpConv, id, value string) {
	for _, opt := range rt.configs {
		switch {
		case opt.Select != nil && string(opt.Select.Id) == id:
			opt.Select.CurrentValue = acp.SessionConfigValueId(value)
		case opt.Boolean != nil && string(opt.Boolean.Id) == id:
			if b, err := strconv.ParseBool(value); err == nil {
				opt.Boolean.CurrentValue = b
			}
		}
	}
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
	replyID := conv.Messages[idx].ID
	conn, ok := cw.findACPConn(connName)
	if !ok {
		conv.Messages[idx].Content = "⚠️ 未找到 ACP 连接：" + connName
		cw.endStream(replyID)
		return
	}
	cw.acp.convObj[conv.ID] = conv
	go func() {
		rt, err := cw.acpEnsureSession(conv, conn)
		if err != nil {
			cw.postMain(func() {
				conv.Messages[idx].Content = "⚠️ " + err.Error()
				cw.endStream(replyID)
				_ = cw.convManager.SaveConversation(conv)
			})
			return
		}
		resp, perr := rt.conn.Prompt(context.Background(), acp.PromptRequest{
			SessionId: rt.sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
		})
		// The greeting window closes with the first prompt, so anything the
		// CLI sends from here on belongs to the reply.
		cw.acp.mu.Lock()
		delete(cw.acp.greeting, rt.sessionID)
		cw.acp.mu.Unlock()
		cw.postMain(func() {
			if perr != nil && conv.Messages[idx].Content == "" {
				conv.Messages[idx].Content = "⚠️ " + perr.Error()
			}
			cw.acpApplyUsage(conv, conv.Messages[idx].ID, resp)
			if t := turnOf(conv, conv.Messages[idx].ID); t != nil {
				t.Assistant = conv.Messages[idx].Content
				t.EndedAt = time.Now()
			}
			cw.endStream(replyID)
			conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
			cw.notifySessionComplete(conv, conv.Messages[idx].Content)
			// The turn is over: under "turn" this is the checkpoint; under
			// "batch" it flushes a batch no tool finished, so a reply that
			// hung on a tool never loses its changes.
			cw.acpFlushTurn(conv, conv.Messages[idx].ID)
		})
	}()
}

// acpFlushTurn runs when a turn ends. The "turn" level commits the whole
// reply as one checkpoint; the "batch" level only flushes a batch that a
// finished reply left running, so a hung tool still keeps its changes.
func (cw *ChatWindow) acpFlushTurn(conv *models.Conversation, msgID string) {
	if conv == nil || conv.Workspace == "" {
		return
	}
	if cw.cfg.AgentCheckpoint == "turn" {
		cw.autoCommitCheckpoint(conv, msgID, toolsOfTurn(conv, msgID))
		return
	}
	rt := cw.acpRuntimeFor(conv.ID)
	if rt == nil {
		return
	}
	cw.acp.mu.Lock()
	pending := len(rt.batch) > 0
	summary := append([]string(nil), rt.batchSummary...)
	rt.batch = map[string]bool{}
	rt.batchSummary = nil
	cw.acp.mu.Unlock()
	if pending && cw.cfg.AgentCheckpoint == "batch" {
		cw.autoCommitCheckpoint(conv, msgID, summary)
	}
}

// toolsOfTurn lists the tool titles of the reply with the given message ID.
func toolsOfTurn(conv *models.Conversation, msgID string) []string {
	var out []string
	for _, m := range conv.Messages {
		if m.ID != msgID {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.Name != "" {
				out = append(out, tc.Name)
			}
		}
		break
	}
	return out
}

func (cw *ChatWindow) acpEnsureSession(conv *models.Conversation, conn config.ACPConnection) (*acpConv, error) {
	return cw.acpEnsureSessionAt(conv, conn, conv.ID)
}

// acpEnsureSessionAt starts (or reuses) the session in one slot. A room
// gives each ACP member its own slot, so two coding CLIs in one conversation
// do not share a process.
func (cw *ChatWindow) acpEnsureSessionAt(conv *models.Conversation, conn config.ACPConnection, key string) (rt *acpConv, err error) {
	// A warm-up may already be starting this slot. A message sent in that
	// window waits for it instead of failing, which is what a user sees as
	// "the session is still starting" right after opening a conversation.
	for {
		cw.acp.mu.Lock()
		if rt := cw.acp.convs[key]; rt != nil {
			cw.acp.mu.Unlock()
			return rt, nil
		}
		if w, ok := cw.acp.starting[key]; ok {
			cw.acp.mu.Unlock()
			<-w.done
			if w.err != nil {
				return nil, w.err
			}
			continue
		}
		cw.acp.starting[key] = &acpStart{done: make(chan struct{})}
		cw.acp.mu.Unlock()
		break
	}
	cw.invalidate()
	defer func() {
		cw.acp.mu.Lock()
		if w, ok := cw.acp.starting[key]; ok {
			w.err = err
			close(w.done)
			delete(cw.acp.starting, key)
		}
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
	rt = &acpConv{cmd: cmd, convID: conv.ID, key: key, batch: map[string]bool{}}
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
	cw.acp.convs[key] = rt
	cw.acp.convBySess[rt.sessionID] = key
	// The session is fresh: anything the CLI sends before the first prompt is
	// a greeting, dropped until that prompt clears the mark.
	cw.acp.greeting[rt.sessionID] = true
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
	if cur := cw.acp.convs[rt.key]; cur == rt {
		delete(cw.acp.convs, rt.key)
	}
	if rt.sessionID != "" {
		delete(cw.acp.convBySess, rt.sessionID)
		delete(cw.acp.greeting, rt.sessionID)
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
			// A saved value the runtime no longer offers is dropped rather
			// than forced on it: the option keeps the CLI's own current
			// choice instead of a value it would reject.
			if !acpSelectHasValue(opt.Select, v) {
				continue
			}
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

// acpSelectHasValue reports whether the option still offers value.
func acpSelectHasValue(s *acp.SessionConfigOptionSelect, value string) bool {
	has := func(list []acp.SessionConfigSelectOption) bool {
		for _, it := range list {
			if string(it.Value) == value {
				return true
			}
		}
		return false
	}
	if s.Options.Ungrouped != nil && has(*s.Options.Ungrouped) {
		return true
	}
	if s.Options.Grouped != nil {
		for _, g := range *s.Options.Grouped {
			if has(g.Options) {
				return true
			}
		}
	}
	return false
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
			cw.postMain(func() {
				cw.toastErrorDetail("ACP 会话启动失败", err.Error())
			})
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

// acpShutdown kills every live session, so no coding CLI outlives the app.
// The quit hook calls it once; the reap goroutine then finds the slot already
// gone and does nothing.
func (cw *ChatWindow) acpShutdown() {
	cw.acp.mu.Lock()
	rts := make([]*acpConv, 0, len(cw.acp.convs))
	for key, rt := range cw.acp.convs {
		rts = append(rts, rt)
		delete(cw.acp.convs, key)
		delete(cw.acp.convBySess, rt.sessionID)
	}
	cw.acp.mu.Unlock()
	for _, rt := range rts {
		cw.acpKillProcess(rt)
	}
}

func (cw *ChatWindow) acpConvClose(convID string) {
	cw.acp.mu.Lock()
	// A room hosts one slot per ACP member, so close every slot of the
	// conversation rather than only the plain one.
	var doomed []*acpConv
	for key, rt := range cw.acp.convs {
		if rt.convID != convID {
			continue
		}
		doomed = append(doomed, rt)
		delete(cw.acp.convs, key)
		delete(cw.acp.convBySess, rt.sessionID)
		delete(cw.acp.convObj, key)
	}
	cw.acp.mu.Unlock()
	delete(cw.acp.convObj, convID)
	for _, rt := range doomed {
		cw.acpKillProcess(rt)
	}
}
