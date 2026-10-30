package ui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// roomPassToken is what a member replies when it chooses not to speak.
//
// Silence needs an explicit, checkable form. An empty completion cannot carry
// the decision: it is indistinguishable from a stream that has not produced
// its first chunk yet, and providers disagree on whether an empty reply is
// even allowed.
const roomPassToken = "PASS"

// roomProtocol is the coordination contract appended to every member's system
// prompt.
//
// Shape-level rules only, and deliberately few. Enumerating scenarios ("in a
// counting task, post the highest posted number plus one") makes the prompt
// larger, makes the member worse at recognising the same shape elsewhere, and
// starts a slope where every observed misstep earns its own clause. When a
// member decides wrongly, fix the rule that already covers it or fix the
// scheduler — do not add a case.
const roomProtocol = `你在一个群聊里，成员包括用户和其他 AI 助手。每条别人的消息都带 [#序号 说话人] 前缀。

1. 用户可以不用 @ 就指名某个成员。看清他叫的是谁——如果不是你，就不要插嘴。
   "用户问了群里，总得有人答"只适用于明确问全体的消息。

2. 基于真实已发布的消息回复，不要基于"我排第几"或"猜别人会说什么"。
   对逐项推进的任务（计数、接龙、每人选一个），接已发布的最大项之后那一项。

3. 不要重复别人说过的内容。如果你想说的已经有人说了，就保持沉默。
   完成以任务的项数衡量，不是人头数：还有剩余项而有成员缺席时，在场的人接下一项。

4. 你有三种回应方式：
   - 正常回复：直接输出你要说的话。
   - 保持沉默：只输出 ` + roomPassToken + `，不要有任何其他内容。
   - 简短示意：只想表示看到了或同意，输出一个 emoji 即可。

5. 不需要你发言时，保持沉默是完全正确的，不要为了参与感而说话。

6. 你的工作完成后需要别人接手时，在回复里 @那个成员的名字，他会在你之后发言。
   也可以用 room_send 工具发消息交接；不确定找谁时不要乱 @。`

// sendRoomDraft posts the user's message and runs one turn of the room.
func (cw *ChatWindow) sendRoomDraft(r config.Room) {
	text := strings.TrimSpace(cw.room.draft)
	if text == "" || cw.room.running != "" {
		return
	}
	conv := cw.roomConv()
	if conv == nil {
		return
	}
	members := cw.roomSpeakers(r, conv, text)
	if len(members) == 0 {
		cw.toast("没有成员会回应这条消息", mcore.SeverityWarning)
		return
	}
	cw.closeDonePlan(conv)
	r.KBID = cw.ensureRoomKBID(r.ID)
	human := conv.NewHumanMessage(cw.newID(), text, time.Now())
	human.QuoteID = cw.room.quoteID
	conv.Messages = append(conv.Messages, human)
	conv.UpdatedAt = time.Now()
	cw.room.draft = ""
	cw.room.quoteID = ""
	_ = cw.convManager.SaveConversation(conv)
	go cw.roomArchiveMessage(r.KBID, human)
	cw.room.list.ScrollToEnd()
	cw.startRoomTurn(r, conv, members)
}

// roomSpeakers are the members that answer this turn, in roster order.
func (cw *ChatWindow) roomSpeakers(r config.Room, conv *models.Conversation, text string) []config.RoomMember {
	active := r.ActiveMembers()
	banned := roomBannedAuthor(conv, r.AllowSelfResponses)
	eligible := make([]config.RoomMember, 0, len(active))
	for _, m := range active {
		if m.Agent != banned {
			eligible = append(eligible, m)
		}
	}
	// An explicit @ is a direct address: the named members answer, whatever
	// the activation strategy would otherwise decide.
	if at := roomAtMentioned(eligible, text); len(at) > 0 {
		return at
	}
	switch r.EffectiveActivation() {
	case config.ActivationMention:
		return roomMentionedSpeakers(eligible, text)
	case config.ActivationNatural:
		return roomNaturalSpeakers(eligible, text, rand.IntN)
	case config.ActivationPooled:
		return roomPooledSpeaker(eligible, conv, rand.IntN)
	default:
		// ActivationMaster also lands here: every eligible member is a
		// candidate, and the moderator narrows the list inside the turn,
		// where its model call cannot block the UI.
		return eligible
	}
}

func roomMentionedSpeakers(members []config.RoomMember, text string) []config.RoomMember {
	out := make([]config.RoomMember, 0, len(members))
	for _, m := range members {
		if roomMentions(text, roomMemberName(m)) {
			out = append(out, m)
		}
	}
	return out
}

// roomNaturalSpeakers picks the members named in the message, plus each other
// member by a talkativeness dice roll. A silent round falls back to one
// random member, because a message into a room deserves at least one answer.
func roomNaturalSpeakers(members []config.RoomMember, text string, roll func(int) int) []config.RoomMember {
	out := make([]config.RoomMember, 0, len(members))
	for _, m := range members {
		if roomMentions(text, roomMemberName(m)) || roll(100) < m.EffectiveTalkativeness() {
			out = append(out, m)
		}
	}
	if len(out) == 0 && len(members) > 0 {
		out = append(out, members[roll(len(members))])
	}
	return out
}

// roomPooledSpeaker picks one member per turn, preferring those who have not
// spoken since the user's previous message, so the room rotates through its
// roster instead of letting one member dominate.
func roomPooledSpeaker(members []config.RoomMember, conv *models.Conversation, roll func(int) int) []config.RoomMember {
	if len(members) == 0 {
		return nil
	}
	spoken := roomSpokenSinceHuman(conv)
	fresh := make([]config.RoomMember, 0, len(members))
	for _, m := range members {
		if !spoken[m.Agent] {
			fresh = append(fresh, m)
		}
	}
	pool := fresh
	if len(pool) == 0 {
		pool = members
	}
	return []config.RoomMember{pool[roll(len(pool))]}
}

// roomSpokenSinceHuman is the set of agents that answered since the last
// human message. roomSpeakers runs before the new user message is appended,
// so this covers the previous round.
func roomSpokenSinceHuman(conv *models.Conversation) map[string]bool {
	out := map[string]bool{}
	if conv == nil {
		return out
	}
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		m := conv.Messages[i]
		if m.IsHuman() {
			break
		}
		if m.AuthorKind == models.AuthorKindAgent && m.AuthorID != "" {
			out[m.AuthorID] = true
		}
	}
	return out
}

// roomBannedAuthor is the member that may not speak this turn: whoever spoke
// last, when the room disallows consecutive turns. It keeps one member from
// answering itself in a loop.
func roomBannedAuthor(conv *models.Conversation, allowSelf bool) string {
	if allowSelf || conv == nil {
		return ""
	}
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		m := conv.Messages[i]
		if m.IsHuman() {
			return ""
		}
		if m.AuthorKind == models.AuthorKindAgent && m.AuthorID != "" {
			return m.AuthorID
		}
	}
	return ""
}

// roomMentions reports whether text names the member. Matching is on the
// whole name, case-insensitively, so "Anna" does not match "Annabelle".
func roomMentions(text, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	lower := strings.ToLower(text)
	target := strings.ToLower(name)
	for _, form := range []string{"@" + target, target} {
		idx := strings.Index(lower, form)
		if idx < 0 {
			continue
		}
		if roomWholeWordAt(lower, idx, len(form)) {
			return true
		}
	}
	return false
}

// roomWholeWordAt reports whether the run at idx stands on its own, so a name
// inside a longer word does not count as a mention.
func roomWholeWordAt(s string, idx, n int) bool {
	before := idx == 0 || !isNameRune(rune(s[idx-1]))
	end := idx + n
	after := end >= len(s) || !isNameRune(rune(s[end]))
	return before && after
}

func isNameRune(r rune) bool {
	return r == '_' || r == '-' ||
		(r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z')
}

// startRoomTurn runs the members one after another, each seeing what the ones
// before it just posted.
//
// Serial on purpose. Running them at once would need a freshness gate, a
// concurrency cap and spawn pacing to keep two members from answering the
// same state, and would put several streams on screen at once. Running them
// in order removes all of that and makes a member able to build on its
// predecessor instead of racing it. The cost is latency, which the roster's
// queue numbers make visible.
func (cw *ChatWindow) startRoomTurn(r config.Room, conv *models.Conversation, members []config.RoomMember) {
	cw.room.running = r.ID
	cw.room.passed = map[string]bool{}
	cw.room.queue = make([]string, 0, len(members))
	cw.room.turnOrder = make([]string, 0, len(members))
	cw.room.detailAgent = ""
	cw.room.detailTurnID = ""
	for _, m := range members {
		cw.room.queue = append(cw.room.queue, m.Agent)
		cw.room.turnOrder = append(cw.room.turnOrder, m.Agent)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cw.room.cancel = cancel
	cw.room.turnConv = conv
	if r.EffectiveActivation() == config.ActivationMaster {
		// The moderator client reads the configuration, which belongs to
		// the main thread, so it is resolved here and only called inside
		// the turn goroutine.
		cw.room.masterChat = cw.masterClient(r)
	} else {
		cw.room.masterChat = nil
	}
	go cw.runRoomTurn(ctx, r, conv, members)
}

// stopRoomTurn cancels the running turn; the member being generated keeps
// whatever text it has already produced.
func (cw *ChatWindow) stopRoomTurn() {
	if cw.room.cancel != nil {
		cw.room.cancel()
		cw.room.cancel = nil
	}
}

func (cw *ChatWindow) runRoomTurn(ctx context.Context, r config.Room, conv *models.Conversation, members []config.RoomMember) {
	if r.EffectiveActivation() == config.ActivationMaster {
		members = cw.masterNarrow(ctx, r.KBID, conv, members)
	}
	// The queue is handed along: a member's reply or room_send may @ other
	// members, who join the end of the turn. spoken caps every member at
	// one slot per turn, which is what makes a handoff chain terminate.
	spoken := map[string]bool{}
	pending := append([]config.RoomMember(nil), members...)
	for len(pending) > 0 {
		if ctx.Err() != nil {
			break
		}
		m := pending[0]
		pending = pending[1:]
		if spoken[m.Agent] {
			continue
		}
		spoken[m.Agent] = true
		cw.runRoomMember(ctx, r, conv, m)
		pending = append(pending, cw.takeRoomHandoff(r, spoken)...)
	}
	cw.postMain(func() {
		cw.room.running = ""
		cw.room.queue = nil
		cw.room.speaking = ""
		cw.room.cancel = nil
		cw.room.turnConv = nil
		cw.room.handoff = nil
		conv.UpdatedAt = time.Now()
		_ = cw.convManager.SaveConversation(conv)
		if cw.room.active == r.ID {
			cw.reloadRoomSessions(r.ID)
		}
		cw.invalidate()
	})
}

// takeRoomHandoff drains the agents @-mentioned since the last take and
// returns those who have not spoken this turn, queueing them on the roster.
func (cw *ChatWindow) takeRoomHandoff(r config.Room, spoken map[string]bool) []config.RoomMember {
	done := make(chan []string, 1)
	cw.postMain(func() {
		agents := cw.room.handoff
		cw.room.handoff = nil
		done <- agents
	})
	agents := <-done
	out := make([]config.RoomMember, 0, len(agents))
	added := map[string]bool{}
	for _, agent := range agents {
		if spoken[agent] || added[agent] {
			continue
		}
		for _, m := range r.ActiveMembers() {
			if m.Agent == agent {
				added[agent] = true
				out = append(out, m)
				break
			}
		}
	}
	if len(out) > 0 {
		cw.postMain(func() {
			for _, m := range out {
				cw.room.queue = append(cw.room.queue, m.Agent)
				cw.room.turnOrder = append(cw.room.turnOrder, m.Agent)
			}
			cw.invalidate()
		})
	}
	return out
}

// runRoomMember generates one member's reply and blocks until it is done, so
// the next member reads a conversation that already contains it.
func (cw *ChatWindow) runRoomMember(ctx context.Context, r config.Room, conv *models.Conversation, m config.RoomMember) {
	if strings.HasPrefix(m.Agent, "acp:") {
		cw.runRoomACPMember(ctx, r, conv, m)
		return
	}
	name := roomMemberName(m)
	client, react, groupClient, groupReact, err := cw.roomMemberClient(r, m)
	if err != nil {
		cw.postMain(func() { cw.roomAppendError(conv, m, name, err) })
		return
	}
	msgs := cw.buildRoomMessages(conv, m, name)

	ready := make(chan string, 1)
	cw.postMain(func() {
		reply := conv.NewAgentMessage(cw.newID(), m.Agent, name, time.Now())
		// A member answers the message that opened the turn, so its reply
		// quotes it: the thread stays readable when several members answer
		// different things in one turn.
		reply.QuoteID = roomTurnPromptID(conv)
		conv.Messages = append(conv.Messages, reply)
		cw.room.speaking = m.Agent
		cw.room.queue = roomDrop(cw.room.queue, m.Agent)
		cw.beginStream(conv.ID, reply.ID, nil)
		cw.room.list.ScrollToEnd()
		cw.invalidate()
		ready <- reply.ID
	})
	replyID := <-ready

	onChunk := func(chunk string) {
		cw.postMain(func() {
			if i := roomIndexOf(conv, replyID); i >= 0 {
				conv.Messages[i].Content += chunk
				appendSegmentText(&conv.Messages[i], "text", chunk)
			}
		})
	}
	onReasoning := func(chunk string) {
		cw.postMain(func() {
			if i := roomIndexOf(conv, replyID); i >= 0 {
				conv.Messages[i].Reasoning += chunk
				appendSegmentText(&conv.Messages[i], "reasoning", chunk)
			}
		})
	}
	onTool := func(info llm.ToolCallInfo) {
		cw.postMain(func() {
			if i := roomIndexOf(conv, replyID); i >= 0 {
				applyToolCall(&conv.Messages[i], info)
				appendSegmentTool(&conv.Messages[i], info)
			}
		})
	}

	var resp *llm.ChatResponse
	var runErr error
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

	done := make(chan struct{})
	cw.postMain(func() {
		defer close(done)
		cw.endStream(replyID)
		cw.room.speaking = ""
		i := roomIndexOf(conv, replyID)
		if i < 0 {
			return
		}
		msg := &conv.Messages[i]
		if runErr != nil && msg.Content == "" {
			msg.Content = "⚠️ " + runErr.Error()
		} else if resp != nil && msg.Content == "" {
			msg.Content = resp.Content
		}
		if resp != nil && msg.Reasoning == "" {
			msg.Reasoning = resp.Reasoning
		}
		if roomIsPass(msg.Content) {
			// A member that declined: drop the empty bubble and mark it on
			// the roster instead, so silence reads as a decision rather
			// than a blank reply.
			conv.Messages = append(conv.Messages[:i], conv.Messages[i+1:]...)
			cw.room.passed[m.Agent] = true
		} else if !strings.HasPrefix(msg.Content, "⚠️") {
			// A reply that @s another member is a handoff, the same as a
			// room_send: the named members join the turn after this one.
			for _, h := range roomAtMentioned(r.ActiveMembers(), msg.Content) {
				if h.Agent != m.Agent {
					cw.room.handoff = append(cw.room.handoff, h.Agent)
				}
			}
			archived := *msg
			go cw.roomArchiveMessage(r.KBID, archived)
		}
		_ = cw.convManager.SaveConversation(conv)
		cw.invalidate()
	})
	<-done
}

// roomIsPass reports a reply that declines to speak.
func roomIsPass(content string) bool {
	t := strings.TrimSpace(content)
	if t == "" {
		return true
	}
	return strings.EqualFold(strings.Trim(t, ".。!！"), roomPassToken)
}

func (cw *ChatWindow) roomAppendError(conv *models.Conversation, m config.RoomMember, name string, err error) {
	msg := conv.NewAgentMessage(cw.newID(), m.Agent, name, time.Now())
	msg.Content = "⚠️ " + err.Error()
	conv.Messages = append(conv.Messages, msg)
	cw.room.queue = roomDrop(cw.room.queue, m.Agent)
	_ = cw.convManager.SaveConversation(conv)
	cw.invalidate()
}

// roomMemberClient builds a member's own client, rooted at the room's
// directory so every member's file and command tools act on one project.
// Every member also carries the room_history tool, so it can recall archived
// discussion beyond its context window.
func (cw *ChatWindow) roomMemberClient(r config.Room, m config.RoomMember) (*llm.Client, *llm.ReactClient, *llm.GroupClient, *llm.GroupReactClient, error) {
	return cw.taskClientWithChannel(m.Agent, r.Workspace, nil, nil,
		cw.roomHistoryTool(r.KBID), cw.roomSendTool(r, m))
}

// buildRoomMessages renders the conversation from one member's point of view.
//
// The chat protocols carry only user, assistant and system roles, so several
// speakers cannot be represented directly: the member's own messages become
// assistant, everyone else's become user with a "[#seq name]:" prefix. The
// provider-specific `name` field is not used because only some providers
// support it and ChatGo speaks four protocols.
//
// Another member's reasoning is never included. It would cost tokens and
// invite the member to read a peer's thinking as its own.
func (cw *ChatWindow) buildRoomMessages(conv *models.Conversation, m config.RoomMember, name string) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, len(conv.Messages)+3)
	sys := cw.roomSystemPrompt(m, name)
	out = append(out, llm.ChatMessage{Role: "system", Content: sys})
	for _, msg := range conv.Messages {
		body := strings.TrimSpace(msg.Content)
		if body == "" || msg.AuthorKind == models.AuthorKindSystem {
			// A system notice, as the moderator's routing decision, is for
			// the human reading the log; injected into a member it would
			// read as someone speaking.
			continue
		}
		if msg.AuthorID == m.Agent && msg.AuthorKind == models.AuthorKindAgent {
			out = append(out, llm.ChatMessage{Role: "assistant", Content: body})
			continue
		}
		head := fmt.Sprintf("[#%d %s]", msg.Seq, roomSpeakerName(msg))
		if q, ok := roomFindMessage(conv, msg.QuoteID); ok {
			if ctx, ok := roomQuoteContext(q.Content, q.Reasoning); ok {
				head += fmt.Sprintf("（回复 #%d %s 的“%s”）", q.Seq, roomSpeakerName(q), ctx)
			} else {
				head += fmt.Sprintf("（回复 #%d %s）", q.Seq, roomSpeakerName(q))
			}
		}
		out = append(out, llm.ChatMessage{Role: "user", Content: head + ": " + body})
	}
	out = append(out, llm.ChatMessage{Role: "system", Content: roomNudge(name)})
	return out
}

// roomSystemPrompt is the member's own prompt followed by the protocol.
func (cw *ChatWindow) roomSystemPrompt(m config.RoomMember, name string) string {
	parts := make([]string, 0, 3)
	if own := strings.TrimSpace(cw.taskSystemPrompt(m.Agent)); own != "" {
		parts = append(parts, own)
	}
	parts = append(parts, "你在群聊里的身份是「"+name+"」。")
	parts = append(parts, roomProtocol)
	return strings.Join(parts, "\n\n")
}

// roomNudge closes the prompt by naming whose turn it is. It sits at the end,
// next to the point of generation, where a long history cannot dilute it.
func roomNudge(name string) string {
	return fmt.Sprintf("[现在轮到「%s」发言。只以这个身份说话。如果这轮不该你说，只输出 %s。]", name, roomPassToken)
}

func roomSpeakerName(msg models.Message) string {
	if n := strings.TrimSpace(msg.AuthorName); n != "" {
		return n
	}
	if msg.IsHuman() {
		return "用户"
	}
	return "助手"
}

func roomIndexOf(conv *models.Conversation, id string) int {
	for i := range conv.Messages {
		if conv.Messages[i].ID == id {
			return i
		}
	}
	return -1
}

// roomDrop returns the queue without agent, in a new slice: reusing the
// backing array would mutate the one the roster is drawing from.
func roomDrop(queue []string, agent string) []string {
	out := make([]string, 0, len(queue))
	for _, a := range queue {
		if a != agent {
			out = append(out, a)
		}
	}
	return out
}

// roomAtMentioned is the members named with an explicit @ in the text, in
// roster order. An @ is a direct address, so these members speak regardless
// of the room's activation strategy.
func roomAtMentioned(members []config.RoomMember, text string) []config.RoomMember {
	lower := strings.ToLower(text)
	out := make([]config.RoomMember, 0, len(members))
	for _, m := range members {
		name := strings.ToLower(strings.TrimSpace(roomMemberName(m)))
		if name == "" {
			continue
		}
		form := "@" + name
		idx := strings.Index(lower, form)
		if idx >= 0 && roomWholeWordAt(lower, idx, len(form)) {
			out = append(out, m)
		}
	}
	return out
}
