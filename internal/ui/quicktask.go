package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgo/internal/llm"
	"chatgo/pkg/models"

	mcore "chatgo/internal/ui/uikit"
)

// quickMaxRunning caps the background tasks the panel starts at once, so a
// burst of shortcuts cannot spawn a client (or an ACP process) per task.
const quickMaxRunning = 3

// markQuickRunning records that replyID belongs to a quick task.
func (cw *ChatWindow) markQuickRunning(replyID string) {
	if replyID == "" {
		return
	}
	if cw.quickReplyIDs == nil {
		cw.quickReplyIDs = map[string]bool{}
	}
	if cw.quickReplyIDs[replyID] {
		return
	}
	cw.quickReplyIDs[replyID] = true
	cw.quickRunning++
	cw.updateTrayMenu()
}

// unmarkQuickReply is called when a quick reply ends or is cancelled.
func (cw *ChatWindow) unmarkQuickReply(replyID string) {
	if cw.quickReplyIDs == nil || !cw.quickReplyIDs[replyID] {
		return
	}
	delete(cw.quickReplyIDs, replyID)
	if cw.quickRunning > 0 {
		cw.quickRunning--
	}
	cw.updateTrayMenu()
}

// quickTask is one agent run started from the quick panel. It keeps the
// conversation it writes into, so the panel can draw the reply while the
// main window shows something else.
type quickTask struct {
	Prompt string
	Agent  string
	// Model is the chosen model reference, or empty for the agent's first.
	Model     string
	Workspace string
	Conv      *models.Conversation
	// ReplyIdx is the index of the reply message in Conv.Messages.
	ReplyIdx int
	ReplyID  string
	cancel   context.CancelFunc
	// preparing is set while a worktree is being created; note says what
	// the panel should show meanwhile.
	preparing bool
	note      string
	// canceled keeps a task whose worktree was still being created from
	// starting after the user stopped it.
	canceled bool
}

// running reports whether the reply is still being generated.
func (t *quickTask) running(cw *ChatWindow) bool {
	if t == nil {
		return false
	}
	if t.preparing {
		return true
	}
	return t.ReplyID != "" && cw.streamingMsg(t.ReplyID)
}

// reply is the message the panel draws, if the conversation still has it.
func (t *quickTask) reply() *models.Message {
	if t == nil || t.Conv == nil || t.ReplyIdx < 0 || t.ReplyIdx >= len(t.Conv.Messages) {
		return nil
	}
	return &t.Conv.Messages[t.ReplyIdx]
}

// stop cancels the reply, when its client supports cancellation, or keeps a
// task still preparing its worktree from starting.
func (t *quickTask) stop(cw *ChatWindow) {
	if t == nil {
		return
	}
	if t.preparing {
		t.canceled = true
		t.preparing = false
		t.note = "已取消"
		cw.invalidate()
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	if t.Conv != nil {
		cw.cancelConvStreams(t.Conv.ID)
	}
	cw.invalidate()
}

// quickStartTask creates a conversation and runs its first turn. It runs on
// the main thread; the reply itself runs off it. With worktree set, an
// isolated working tree is created first.
func (cw *ChatWindow) quickStartTask(prompt, agent, workspace, model string, worktree bool) *quickTask {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	if cw.quickRunning >= quickMaxRunning {
		cw.toast(fmt.Sprintf("最多同时运行 %d 个快捷任务，请等一个完成", quickMaxRunning), mcore.SeverityError)
		return nil
	}
	if workspace == "" {
		workspace = cw.fileRoot()
	}
	task := &quickTask{Prompt: prompt, Agent: agent, Model: model, Workspace: workspace}

	if worktree {
		task.preparing = true
		task.note = "正在创建 worktree…"
		go func() {
			path, err := cw.quickWorktree(workspace)
			cw.postMain(func() {
				task.preparing = false
				if task.canceled {
					return
				}
				if err != nil {
					cw.quickTaskFail(task, err)
					return
				}
				task.Workspace = path
				task.note = ""
				cw.addWorkspace(path)
				cw.quickLaunchTask(task)
			})
		}()
		return task
	}

	cw.quickLaunchTask(task)
	return task
}

// quickLaunchTask writes the conversation and starts the turn.
func (cw *ChatWindow) quickLaunchTask(task *quickTask) {
	conv, idx, replyID, err := cw.quickConversation(task.Agent, task.Model, task.Workspace, task.Prompt)
	if err != nil {
		cw.quickTaskFail(task, err)
		return
	}
	task.Conv, task.ReplyIdx, task.ReplyID = conv, idx, replyID

	agent := task.Agent
	if strings.HasPrefix(agent, "acp:") {
		// ACP sessions are keyed by conversation, so this runs without the
		// main window having to show the conversation.
		cw.beginStream(conv.ID, replyID, nil)
		cw.markQuickRunning(replyID)
		cw.acpChatStart(conv, strings.TrimPrefix(agent, "acp:"), idx, task.Prompt)
		cw.invalidate()
		return
	}

	if cw.cfg.QuickPanel.IsBackground() {
		cw.quickRunBackground(task)
		return
	}

	cw.openConversation(conv)
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	cw.beginStream(conv.ID, replyID, cancel)
	cw.markQuickRunning(replyID)
	cw.invalidate()
	go cw.runStream(ctx, conv, idx, cw.chatMessages(conv, idx))
}

// quickWorktree creates an isolated working tree of repo for one task. It
// runs off the main thread, since git shells out.
func (cw *ChatWindow) quickWorktree(repo string) (string, error) {
	if out, err := gitRun(repo, "rev-parse", "--is-inside-work-tree"); err != nil {
		return "", fmt.Errorf("不是 git 仓库：%s", strings.TrimSpace(out))
	}
	branch := "quick-" + time.Now().Format("0102-150405")
	if containsStr(gitBranchNames(repo), branch) {
		branch += "-" + time.Now().Format("05")
	}
	path := defaultWorktreePath(repo, branch)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("目录已存在：%s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := gitRun(repo, "worktree", "add", "-b", branch, path)
	if err != nil {
		return "", fmt.Errorf("%s", worktreeError(out, err))
	}
	return path, nil
}

// quickConversation writes the user message and the empty reply, so the
// panel has something to draw before the first chunk arrives.
func (cw *ChatWindow) quickConversation(agent, modelRef, workspace, prompt string) (*models.Conversation, int, string, error) {
	provider, model := "", ""
	isACP := strings.HasPrefix(agent, "acp:")
	if !isACP {
		provider, model, _, _, _ = cw.taskAgentSpec(agent)
		if ref := strings.TrimSpace(modelRef); ref != "" {
			if p, m := splitModelRef(ref); p != "" {
				provider, model = p, m
			} else if m != "" {
				model = m
			}
		}
		if model == "" {
			if p, ok := cw.findProvider(provider); ok {
				if ms := p.ModelsList(); len(ms) > 0 {
					model = ms[0]
				}
			}
		}
	}
	conv, err := cw.convManager.CreateConversation(titleFromText(prompt), provider, model)
	if err != nil {
		return nil, 0, "", err
	}
	conv.Workspace = workspace
	conv.Agent = agent

	now := time.Now()
	user := conv.NewHumanMessage(cw.newID(), prompt, now)
	reply := conv.NewAgentMessage(cw.newID(), agent, authorNameOf(agent), now)
	conv.Messages = append(conv.Messages, user, reply)

	turnProvider, turnModel := provider, model
	if isACP {
		turnProvider, turnModel = strings.TrimPrefix(agent, "acp:"), ""
	}
	conv.Turns = append(conv.Turns, models.Turn{
		ID:         cw.newID(),
		UserMsgID:  user.ID,
		ReplyMsgID: reply.ID,
		User:       prompt,
		Provider:   turnProvider,
		Model:      turnModel,
		StartedAt:  now,
	})
	if err := cw.convManager.SaveConversation(conv); err != nil {
		return nil, 0, "", err
	}
	cw.reloadConvs()
	return conv, len(conv.Messages) - 1, reply.ID, nil
}

// quickRunBackground runs the reply with a client of its own. It cannot use
// runStream: that one writes through the window's current client, which
// belongs to whatever conversation the main window shows.
func (cw *ChatWindow) quickRunBackground(task *quickTask) {
	msgs := buildMessages(task.Conv, task.ReplyIdx)
	if sys := cw.taskSystemPrompt(task.Agent); sys != "" {
		msgs = append([]llm.ChatMessage{{Role: "system", Content: sys}}, msgs...)
	}
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	cw.beginStream(task.Conv.ID, task.ReplyID, cancel)
	cw.markQuickRunning(task.ReplyID)
	cw.invalidate()

	go func() {
		// Building the client lists MCP tools and can touch the network, so
		// it stays off the thread that draws the panel.
		client, react, groupClient, groupReact, err := cw.taskClientFor(task.Agent, task.Workspace, task.Model, task.Conv)
		if err != nil {
			cw.postMain(func() { cw.quickTaskFail(task, err) })
			return
		}

		onChunk := func(chunk string) {
			cw.postMain(func() {
				m := task.reply()
				if m == nil {
					return
				}
				m.Content += chunk
				appendSegmentText(m, "text", chunk)
				cw.invalidate()
			})
		}
		onReasoning := func(chunk string) {
			cw.postMain(func() {
				m := task.reply()
				if m == nil {
					return
				}
				m.Reasoning += chunk
				appendSegmentText(m, "reasoning", chunk)
				cw.invalidate()
			})
		}
		onTool := func(info llm.ToolCallInfo) {
			cw.postMain(func() {
				m := task.reply()
				if m == nil {
					return
				}
				applyToolCall(m, info)
				appendSegmentTool(m, info)
				cw.invalidate()
			})
		}

		var (
			resp   *llm.ChatResponse
			runErr error
		)
		switch {
		case groupReact != nil:
			resp, runErr = groupReact.Chat(ctx, msgs, onChunk, onTool, onReasoning)
		case react != nil:
			resp, runErr = react.Chat(ctx, msgs, onChunk, onTool, onReasoning)
		case groupClient != nil:
			resp, runErr = groupClient.Chat(ctx, msgs, onChunk, onReasoning)
		default:
			resp, runErr = client.Chat(ctx, msgs, onChunk, onReasoning)
		}

		cw.postMain(func() {
			m := task.reply()
			if m == nil {
				cw.endStream(task.ReplyID)
				return
			}
			switch {
			case runErr != nil && m.Content == "":
				m.Content = "⚠️ " + runErr.Error()
			case resp != nil && m.Content == "":
				m.Content = resp.Content
			}
			if resp != nil && m.Reasoning == "" {
				m.Reasoning = resp.Reasoning
			}
			if t := turnOf(task.Conv, m.ID); t != nil {
				t.Assistant = m.Content
				t.EndedAt = time.Now()
				if resp != nil && resp.TotalTokens > 0 {
					t.Input, t.Output, t.Total = resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens
				}
			}
			cw.endStream(m.ID)
			task.Conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(task.Conv)
			cw.reloadConvs()
			cw.generateTitle(task.Conv)
			cw.notifySessionComplete(task.Conv, m.Content)
			cw.invalidate()
		})
	}()
}

// quickTaskFail marks a task that could not start.
func (cw *ChatWindow) quickTaskFail(task *quickTask, err error) {
	if task != nil {
		task.preparing = false
		task.note = "⚠️ " + err.Error()
		if m := task.reply(); m != nil {
			m.Content = "⚠️ " + err.Error()
			_ = cw.convManager.SaveConversation(task.Conv)
		}
		cw.endStream(task.ReplyID)
	}
	cw.toastErrorDetail("任务失败", err.Error())
	cw.invalidate()
}

// quickTaskSummary is the one-line label the panel shows for a task.
func quickTaskSummary(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if r := []rune(prompt); len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return prompt
}

// quickTaskProgress is what the panel draws for a task: its reply text once
// there is any, otherwise the model's reasoning or the tool it is running. A
// panel that only showed the reply looked frozen while a reasoning model
// thought or an agent worked through tools.
func quickTaskProgress(task *quickTask) string {
	if task == nil {
		return ""
	}
	if task.preparing || task.Conv == nil {
		return task.note
	}
	m := task.reply()
	if m == nil {
		return "正在思考…"
	}
	if s := strings.TrimSpace(m.Content); s != "" {
		return m.Content
	}
	if s := strings.TrimSpace(m.Reasoning); s != "" {
		return s
	}
	if tc := lastQuickTool(m); tc != nil {
		return "正在使用工具 · " + tc.Name
	}
	return "正在思考…"
}

// lastQuickTool returns the newest tool call of a reply, if any.
func lastQuickTool(m *models.Message) *models.ToolCall {
	for i := len(m.Segments) - 1; i >= 0; i-- {
		if s := m.Segments[i]; s.Kind == "tool" && s.ToolCall != nil {
			return s.ToolCall
		}
	}
	if n := len(m.ToolCalls); n > 0 {
		return &m.ToolCalls[n-1]
	}
	return nil
}

// quickTaskStatus is the running/done label under the prompt.
func quickTaskStatus(cw *ChatWindow, task *quickTask) string {
	if task == nil {
		return ""
	}
	if task.preparing {
		return task.note
	}
	if task.running(cw) {
		if m := task.reply(); m != nil {
			if tc := lastQuickTool(m); tc != nil && tc.Result == "" && tc.Error == "" {
				return "正在调用工具 · " + tc.Name
			}
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Reasoning) != "" {
				return "正在思考…"
			}
		}
		return "正在执行…"
	}
	if task.Conv == nil {
		if task.note != "" {
			return task.note
		}
		return "已取消"
	}
	if m := task.reply(); m != nil && strings.HasPrefix(m.Content, "⚠️") {
		return "已失败"
	}
	return fmt.Sprintf("完成 · %s", task.Conv.Title)
}
