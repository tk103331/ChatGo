package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
)

func roomConvWith(msgs ...models.Message) *models.Conversation {
	conv := &models.Conversation{ID: "c1", RoomID: "r1"}
	conv.Messages = append(conv.Messages, msgs...)
	return conv
}

func humanMsg(conv *models.Conversation, text string) models.Message {
	return conv.NewHumanMessage("u"+text, text, time.Now())
}

func agentMsg(conv *models.Conversation, agent, name, text string) models.Message {
	m := conv.NewAgentMessage("a"+text, agent, name, time.Now())
	m.Content = text
	return m
}

// TestRoomBannedAuthorBlocksConsecutiveTurns guards the rule that the member
// who spoke last does not speak again on the next turn, which is what keeps
// one agent from answering itself in a loop.
func TestRoomBannedAuthorBlocksConsecutiveTurns(t *testing.T) {
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "hi"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "first"))

	if got := roomBannedAuthor(conv, false); got != "agent:A" {
		t.Fatalf("the last speaker should be banned, got %q", got)
	}
	if got := roomBannedAuthor(conv, true); got != "" {
		t.Fatalf("AllowSelfResponses must lift the ban, got %q", got)
	}
}

// TestRoomBannedAuthorResetsAfterHuman checks that a human message reopens
// the floor to everyone: the ban exists to stop agent self-loops, not to bar
// a member from answering the user twice in a row.
func TestRoomBannedAuthorResetsAfterHuman(t *testing.T) {
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "first"))
	conv.Messages = append(conv.Messages, humanMsg(conv, "and now?"))

	if got := roomBannedAuthor(conv, false); got != "" {
		t.Fatalf("a human message should clear the ban, got %q", got)
	}
}

// TestRoomSpeakersAllActivation checks the default: every member answers, in
// roster order, with muted members and the last speaker left out.
func TestRoomSpeakersAllActivation(t *testing.T) {
	cw := testWindow()
	r := config.Room{
		ID: "r1", Activation: config.ActivationAll,
		Members: []config.RoomMember{
			{Agent: "agent:A"},
			{Agent: "agent:B", Muted: true},
			{Agent: "agent:C"},
		},
	}
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:C", "C", "last"))

	got := cw.roomSpeakers(r, conv, "有什么想法")
	if len(got) != 1 || got[0].Agent != "agent:A" {
		t.Fatalf("muted B and last-speaker C should be skipped, got %+v", got)
	}
}

// TestRoomSpeakersMentionActivation checks that the mention strategy wakes
// only the members the message names.
func TestRoomSpeakersMentionActivation(t *testing.T) {
	cw := testWindow()
	r := config.Room{
		ID: "r1", Activation: config.ActivationMention,
		Members: []config.RoomMember{
			{Agent: "agent:架构师"},
			{Agent: "agent:测试"},
		},
	}
	conv := roomConvWith()

	got := cw.roomSpeakers(r, conv, "@架构师 这个方案可行吗")
	if len(got) != 1 || got[0].Agent != "agent:架构师" {
		t.Fatalf("only the mentioned member should speak, got %+v", got)
	}
}

// TestRoomMentionsWholeWord guards against a name matching inside a longer
// one, which would wake a member nobody addressed.
func TestRoomMentionsWholeWord(t *testing.T) {
	cases := []struct {
		text, name string
		want       bool
	}{
		{"@Anna 看一下", "Anna", true},
		{"Anna 看一下", "Anna", true},
		{"Annabelle 看一下", "Anna", false},
		{"ask Anna, please", "Anna", true},
		{"anna 小写也算", "Anna", true},
		{"没人被提到", "Anna", false},
		{"", "Anna", false},
		{"@架构师 来看", "架构师", true},
	}
	for _, tc := range cases {
		if got := roomMentions(tc.text, tc.name); got != tc.want {
			t.Errorf("roomMentions(%q, %q) = %v, want %v", tc.text, tc.name, got, tc.want)
		}
	}
}

// TestRoomIsPass checks the silence token, including the empty reply: a
// provider that returns nothing must read as silence, not as a blank bubble.
func TestRoomIsPass(t *testing.T) {
	for _, s := range []string{"PASS", "pass", " PASS ", "PASS.", "Pass!", ""} {
		if !roomIsPass(s) {
			t.Errorf("%q should count as silence", s)
		}
	}
	for _, s := range []string{"PASSED the test", "我觉得可以", "passing it along"} {
		if roomIsPass(s) {
			t.Errorf("%q should not count as silence", s)
		}
	}
}

// TestBuildRoomMessagesInjectsIdentity is the core of group chat: a member
// must see who said what, see its own messages as its own, and never see a
// peer's reasoning.
func TestBuildRoomMessagesInjectsIdentity(t *testing.T) {
	cw := testWindow()
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "大家怎么看"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "架构师", "我觉得可行"))
	peer := agentMsg(conv, "agent:B", "测试", "有风险")
	peer.Reasoning = "这是 B 的思考过程"
	conv.Messages = append(conv.Messages, peer)

	msgs := cw.buildRoomMessages(conv, config.RoomMember{Agent: "agent:A"}, "架构师")

	if msgs[0].Role != "system" {
		t.Fatalf("the first message should be the system prompt, got %q", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "架构师") {
		t.Error("the system prompt should name the member")
	}
	if !strings.Contains(msgs[0].Content, roomPassToken) {
		t.Error("the system prompt should offer the silence token")
	}

	last := msgs[len(msgs)-1]
	if last.Role != "system" || !strings.Contains(last.Content, "架构师") {
		t.Fatalf("the turn should close with a nudge naming the speaker, got %+v", last)
	}

	var self, peerLine string
	for _, m := range msgs[1 : len(msgs)-1] {
		if m.Role == "assistant" {
			self = m.Content
		}
		if strings.Contains(m.Content, "有风险") {
			peerLine = m.Content
		}
	}
	if self != "我觉得可行" {
		t.Errorf("the member's own message should be assistant with no prefix, got %q", self)
	}
	if !strings.Contains(peerLine, "测试") {
		t.Errorf("a peer's message should be prefixed with its name, got %q", peerLine)
	}
	if !strings.Contains(peerLine, "#") {
		t.Errorf("a peer's message should carry its sequence, got %q", peerLine)
	}
	for _, m := range msgs {
		if strings.Contains(m.Content, "这是 B 的思考过程") {
			t.Error("a peer's reasoning must never reach another member")
		}
	}
}

// TestBuildRoomMessagesSkipsEmpty checks that a reply still being streamed
// does not enter the next member's prompt as a blank line.
func TestBuildRoomMessagesSkipsEmpty(t *testing.T) {
	cw := testWindow()
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "问题"))
	conv.Messages = append(conv.Messages, conv.NewAgentMessage("pending", "agent:B", "B", time.Now()))

	msgs := cw.buildRoomMessages(conv, config.RoomMember{Agent: "agent:A"}, "A")
	for _, m := range msgs[1 : len(msgs)-1] {
		if strings.TrimSpace(m.Content) == "" {
			t.Error("an empty in-flight reply should be left out of the prompt")
		}
	}
}

// TestRoomDropKeepsOriginal guards the queue against aliasing: the roster
// draws from the slice the scheduler is editing, so dropping a member must
// not write through the shared backing array.
func TestRoomDropKeepsOriginal(t *testing.T) {
	queue := []string{"agent:A", "agent:B", "agent:C"}
	got := roomDrop(queue, "agent:B")

	if len(queue) != 3 || queue[1] != "agent:B" {
		t.Fatalf("the original queue must be untouched, got %v", queue)
	}
	if len(got) != 2 || got[0] != "agent:A" || got[1] != "agent:C" {
		t.Fatalf("roomDrop should remove just the one member, got %v", got)
	}
}

// TestRoomConversationStaysOutOfSessionList is the listing rule: a room's
// conversation belongs to its room page, never to the chat session list.
func TestRoomConversationStaysOutOfSessionList(t *testing.T) {
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := cm.CreateConversation("普通会话", "p", "m")
	if err != nil {
		t.Fatal(err)
	}
	room, err := cm.CreateConversation("群聊会话", "", "")
	if err != nil {
		t.Fatal(err)
	}
	room.RoomID = "r1"
	if err := cm.SaveConversation(room); err != nil {
		t.Fatal(err)
	}

	cw := testWindow()
	cw.convManager = cm
	cw.reloadConvs()

	for _, conv := range cw.convs {
		if conv.ID == room.ID {
			t.Fatal("a room conversation must not appear in the session list")
		}
	}
	found := false
	for _, conv := range cw.convs {
		if conv.ID == plain.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("a normal conversation should still be listed")
	}
}

// TestAssignSeqNeverReuses checks that regenerating, which truncates the
// message slice, cannot make a new message compare as older than the one it
// replaced.
func TestAssignSeqNeverReuses(t *testing.T) {
	conv := roomConvWith()
	first := humanMsg(conv, "one")
	second := agentMsg(conv, "agent:A", "A", "two")
	conv.Messages = append(conv.Messages, first, second)

	conv.Messages = conv.Messages[:1]
	replacement := agentMsg(conv, "agent:A", "A", "three")

	if replacement.Seq <= second.Seq {
		t.Fatalf("a replacement must outrank the message it replaces: %d <= %d",
			replacement.Seq, second.Seq)
	}
}

// TestLegacyMessagesKeepWorking checks the compatibility rule: a message
// stored before authorship existed still reads as the right kind.
func TestLegacyMessagesKeepWorking(t *testing.T) {
	legacyUser := models.Message{ID: "1", Role: "user", Content: "hi"}
	legacyReply := models.Message{ID: "2", Role: "assistant", Content: "hello"}

	if !legacyUser.IsHuman() {
		t.Error("a legacy user message should read as human")
	}
	if legacyReply.IsHuman() {
		t.Error("a legacy assistant message should not read as human")
	}
	if _, kind := legacyReply.Author(); kind != models.AuthorKindAgent {
		t.Errorf("a legacy assistant message should be an agent, got %q", kind)
	}
}

// TestRoomCandidatesExcludeACP guards the roster against ACP agents: they are
// coding CLIs that taskAgentSpec cannot resolve, so adding one would quietly
// answer with the built-in chat model instead of the CLI the user picked.
func TestRoomCandidatesExcludeACP(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{Name: "架构师"}}
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "claude-code"}}

	got := cw.roomCandidates(&roomForm{})

	for _, ref := range got {
		if strings.HasPrefix(ref, "acp:") {
			t.Errorf("an ACP agent must not be offered as a room member: %q", ref)
		}
	}
	if len(got) != 1 || got[0] != "agent:架构师" {
		t.Fatalf("custom agents should still be offered, got %v", got)
	}
}

// TestRoomConvIsStableAcrossCalls guards the room view against re-reading the
// conversation from disk on every frame. The view and the running turn must
// share one object: a fresh copy per frame would render none of the streamed
// chunks, because the turn appends them to the object it captured at the
// start, and would also read and unmarshal the file at the frame rate.
func TestRoomConvIsStableAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	cw := testWindow()
	cw.convManager = cm
	cw.cfg.Rooms = []config.Room{{ID: "r1", Name: "群", Members: []config.RoomMember{{Agent: "agent:A"}}}}

	cw.openRoom("r1")
	first := cw.roomConv()
	if first == nil {
		t.Fatal("openRoom should make the conversation available")
	}
	if cw.roomConv() != first {
		t.Fatal("roomConv must return the same object every call, not reload it")
	}

	// A chunk written the way a running turn writes it must be visible to
	// the next render.
	first.Messages = append(first.Messages, first.NewAgentMessage("m1", "agent:A", "A", time.Now()))
	first.Messages[len(first.Messages)-1].Content = "partial"
	if got := cw.roomConv(); len(got.Messages) != 1 || got.Messages[0].Content != "partial" {
		t.Fatal("a streamed chunk must be visible through roomConv")
	}
}

// TestSaveRoomRefreshesOpenConversation checks that repointing a room's
// directory while its page is open updates the conversation held there. The
// conversation carries a copy of the path, so without this the stale one
// would be written back on the next save.
func TestSaveRoomRefreshesOpenConversation(t *testing.T) {
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	cw := testWindow()
	cw.convManager = cm
	cw.cfg.Agents = []config.AgentDef{{Name: "A"}}
	cw.cfg.Rooms = []config.Room{{
		ID: "r1", Name: "群", Workspace: "/old",
		Members: []config.RoomMember{{Agent: "agent:A"}},
	}}

	cw.openRoom("r1")
	if cw.room.conv == nil {
		t.Fatal("openRoom should hold the conversation")
	}

	cw.loadRoomForm(0)
	cw.room.form.workspace = "/new"
	cw.saveRoom()

	if got := cw.cfg.Rooms[0].Workspace; got != "/new" {
		t.Fatalf("the room should keep the new directory, got %q", got)
	}
	if got := cw.room.conv.Workspace; got != "/new" {
		t.Fatalf("the open conversation should follow the room, got %q", got)
	}
}

// TestLeaveRoomReleasesConversation checks that going back to the list drops
// the held conversation, so re-entering a room reads it fresh instead of
// rendering whatever the last visit left behind.
func TestLeaveRoomReleasesConversation(t *testing.T) {
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	cw := testWindow()
	cw.convManager = cm
	cw.cfg.Rooms = []config.Room{{ID: "r1", Name: "群", Members: []config.RoomMember{{Agent: "agent:A"}}}}

	cw.openRoom("r1")
	cw.leaveRoom()

	if cw.room.active != "" || cw.room.conv != nil {
		t.Fatal("leaveRoom should clear both the active room and its conversation")
	}
	if cw.roomConv() != nil {
		t.Fatal("roomConv should report nothing while no room is open")
	}
}

func roomNames(members []config.RoomMember) []string {
	out := make([]string, 0, len(members))
	for _, m := range members {
		out = append(out, roomMemberName(m))
	}
	return out
}

// TestRoomNaturalSpeakers checks the natural activation: a member named in
// the message always speaks, the rest roll against their talkativeness.
func TestRoomNaturalSpeakers(t *testing.T) {
	members := []config.RoomMember{
		{Agent: "agent:A", Talkativeness: 10},
		{Agent: "agent:B", Talkativeness: 90},
		{Agent: "agent:C", Talkativeness: 10},
	}
	alwaysHigh := func(n int) int { return n - 1 }

	got := roomNaturalSpeakers(members, "A 你怎么看", alwaysHigh)
	if len(got) != 1 || got[0].Agent != "agent:A" {
		t.Fatalf("with every roll failing, only the mentioned member speaks, got %v", roomNames(got))
	}

	alwaysLow := func(int) int { return 0 }
	got = roomNaturalSpeakers(members, "大家怎么看", alwaysLow)
	if len(got) != 3 {
		t.Fatalf("with every roll passing, everyone speaks, got %v", roomNames(got))
	}
}

// TestRoomNaturalSpeakersFallback checks that a round where nobody is named
// and every roll fails still produces one speaker: a message into a room
// deserves at least one answer.
func TestRoomNaturalSpeakersFallback(t *testing.T) {
	members := []config.RoomMember{
		{Agent: "agent:A", Talkativeness: 10},
		{Agent: "agent:B", Talkativeness: 10},
	}
	rolls := []int{99, 99, 1}
	roll := func(int) int { v := rolls[0]; rolls = rolls[1:]; return v }

	got := roomNaturalSpeakers(members, "大家怎么看", roll)
	if len(got) != 1 || got[0].Agent != "agent:B" {
		t.Fatalf("a silent round should fall back to one rolled member, got %v", roomNames(got))
	}
}

// TestRoomPooledSpeaker checks the pooled activation: one member per turn,
// preferring those who have not spoken since the user's last message.
func TestRoomPooledSpeaker(t *testing.T) {
	members := []config.RoomMember{
		{Agent: "agent:A"},
		{Agent: "agent:B"},
	}
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "hi"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "me first"))

	first := func(int) int { return 0 }
	got := roomPooledSpeaker(members, conv, first)
	if len(got) != 1 || got[0].Agent != "agent:B" {
		t.Fatalf("the member who has not spoken should be preferred, got %v", roomNames(got))
	}

	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:B", "B", "me too"))
	got = roomPooledSpeaker(members, conv, first)
	if len(got) != 1 {
		t.Fatalf("with everyone spoken the pool resets to all members, got %v", roomNames(got))
	}
}

// TestEffectiveTalkativeness checks the zero value means the 50% default,
// so rooms saved before the field existed keep behaving the same.
func TestEffectiveTalkativeness(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 50}, {-5, 50}, {101, 50}, {1, 1}, {100, 100}, {30, 30},
	}
	for _, tc := range cases {
		m := config.RoomMember{Talkativeness: tc.in}
		if got := m.EffectiveTalkativeness(); got != tc.want {
			t.Fatalf("talkativeness %d should resolve to %d, got %d", tc.in, tc.want, got)
		}
	}
}

// TestRoomFinalTextOf checks the compact row shows a finished reply as its
// last text segment, with plain content as the pre-segment fallback.
func TestRoomFinalTextOf(t *testing.T) {
	msg := models.Message{Segments: []models.Segment{
		{Kind: "reasoning", Text: "think"},
		{Kind: "text", Text: "draft"},
		{Kind: "tool", ToolCall: &models.ToolCall{Name: "read_file"}},
		{Kind: "text", Text: "final answer"},
	}}
	if got := roomFinalTextOf(msg); got != "final answer" {
		t.Fatalf("the last text segment should win, got %q", got)
	}

	legacy := models.Message{Content: "plain"}
	if got := roomFinalTextOf(legacy); got != "plain" {
		t.Fatalf("a message without segments should fall back to content, got %q", got)
	}

	empty := models.Message{Segments: []models.Segment{{Kind: "text", Text: "  "}}}
	if got := roomFinalTextOf(empty); got != "" {
		t.Fatalf("blank segments and content should yield empty, got %q", got)
	}
}

// TestRoomStreamStatus checks the one-line status reports the newest visible
// activity of a generating reply.
func TestRoomStreamStatus(t *testing.T) {
	tool := models.Message{Segments: []models.Segment{
		{Kind: "text", Text: "looking"},
		{Kind: "tool", ToolCall: &models.ToolCall{Name: "run_command"}},
	}}
	if got := roomStreamStatus(tool); got != "工具执行：run_command" {
		t.Fatalf("a trailing tool call should name the tool, got %q", got)
	}

	text := models.Message{Segments: []models.Segment{
		{Kind: "reasoning", Text: "hmm"},
		{Kind: "text", Text: "first line\nsecond line\n"},
	}}
	if got := roomStreamStatus(text); got != "second line" {
		t.Fatalf("trailing text should show its newest line, got %q", got)
	}

	thinking := models.Message{Segments: []models.Segment{{Kind: "reasoning", Text: "step one\nstep two"}}}
	if got := roomStreamStatus(thinking); got != "思考中：step two" {
		t.Fatalf("reasoning should be labelled with its newest line, got %q", got)
	}

	if got := roomStreamStatus(models.Message{}); got != "生成中…" {
		t.Fatalf("no output yet should read as generating, got %q", got)
	}
}

// TestRoomTurnStart checks the detail panel covers the current turn only:
// from the last human message to the end.
func TestRoomTurnStart(t *testing.T) {
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "first"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "old reply"))
	conv.Messages = append(conv.Messages, humanMsg(conv, "second"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:B", "B", "new reply"))

	if got := roomTurnStart(conv); got != 2 {
		t.Fatalf("the turn should start at the last human message, got %d", got)
	}

	empty := roomConvWith()
	if got := roomTurnStart(empty); got != 0 {
		t.Fatalf("an empty conversation starts at 0, got %d", got)
	}

	agentsOnly := roomConvWith()
	agentsOnly.Messages = append(agentsOnly.Messages, agentMsg(agentsOnly, "agent:A", "A", "hi"))
	if got := roomTurnStart(agentsOnly); got != 0 {
		t.Fatalf("no human message means the whole history is one turn, got %d", got)
	}
}

// TestRoomGraphNodes checks the execution graph of a running turn: order
// from the scheduler, state from the runtime, replies matched by author.
func TestRoomGraphNodes(t *testing.T) {
	cw := testWindow()
	r := config.Room{ID: "r1", Members: []config.RoomMember{
		{Agent: "agent:A"}, {Agent: "agent:B"}, {Agent: "agent:C"}, {Agent: "agent:D"},
	}}
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "go"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "done reply"))

	cw.room.running = "r1"
	cw.room.turnOrder = []string{"agent:A", "agent:B", "agent:C", "agent:D"}
	cw.room.speaking = "agent:B"
	cw.room.queue = []string{"agent:C", "agent:D"}
	cw.room.passed = map[string]bool{}

	nodes := cw.roomGraphNodes(r, conv, roomTurnStart(conv), len(conv.Messages))
	if len(nodes) != 4 {
		t.Fatalf("every member of the turn should be a node, got %d", len(nodes))
	}
	wantStates := []string{roomNodeDone, roomNodeSpeaking, roomNodeQueued, roomNodeQueued}
	for i, want := range wantStates {
		if nodes[i].State != want {
			t.Fatalf("node %d should be %s, got %s", i, want, nodes[i].State)
		}
	}
	if nodes[0].MsgID == "" {
		t.Fatal("a finished member should link to its reply")
	}
	if nodes[1].MsgID != "" {
		t.Fatal("a member still speaking has no finished reply to link")
	}
}

// TestRoomGraphNodesFinishedTurn checks the graph of an idle room is rebuilt
// from the messages, so it survives losing the runtime state.
func TestRoomGraphNodesFinishedTurn(t *testing.T) {
	cw := testWindow()
	r := config.Room{ID: "r1", Members: []config.RoomMember{
		{Agent: "agent:A"}, {Agent: "agent:B"},
	}}
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "go"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:B", "B", "spoke first"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "spoke second"))

	nodes := cw.roomGraphNodes(r, conv, roomTurnStart(conv), len(conv.Messages))
	if len(nodes) != 2 {
		t.Fatalf("a finished turn shows who replied, got %d nodes", len(nodes))
	}
	if nodes[0].Agent != "agent:B" || nodes[1].Agent != "agent:A" {
		t.Fatalf("nodes should follow speaking order from the messages, got %v", nodes)
	}
	for _, n := range nodes {
		if n.State != roomNodeDone || n.MsgID == "" {
			t.Fatalf("a replied member should be done with its reply linked, got %+v", n)
		}
	}
}

// TestRoomTurnBounds checks the detail panel's anchoring: an anchor message
// selects its own turn, and no anchor means the newest turn.
func TestRoomTurnBounds(t *testing.T) {
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "first"))                     // 0
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "old reply")) // 1
	conv.Messages = append(conv.Messages, humanMsg(conv, "second"))                    // 2
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:B", "B", "new reply")) // 3

	if s, e := roomTurnBounds(conv, ""); s != 2 || e != 4 {
		t.Fatalf("no anchor should mean the newest turn, got [%d,%d)", s, e)
	}
	if s, e := roomTurnBounds(conv, conv.Messages[1].ID); s != 0 || e != 2 {
		t.Fatalf("an old reply should anchor its own turn, got [%d,%d)", s, e)
	}
	if s, e := roomTurnBounds(conv, conv.Messages[3].ID); s != 2 || e != 4 {
		t.Fatalf("a new reply should anchor the newest turn, got [%d,%d)", s, e)
	}
	if s, e := roomTurnBounds(conv, "missing"); s != 2 || e != 4 {
		t.Fatalf("an unknown anchor should fall back to the newest turn, got [%d,%d)", s, e)
	}
}

// TestRoomSessions checks the room's multi-session behavior: opening a room
// enters its newest session, a new session starts clean, and switching back
// to the turn's conversation hands back the same object so streamed chunks
// stay visible.
func TestRoomSessions(t *testing.T) {
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	cw := testWindow()
	cw.convManager = cm
	cw.cfg.Rooms = []config.Room{{ID: "r1", Name: "群", Members: []config.RoomMember{{Agent: "agent:A"}}}}

	cw.openRoom("r1")
	first := cw.room.conv
	if first == nil || len(cw.room.sessions) != 1 {
		t.Fatalf("opening a fresh room should create its first session, got %d", len(cw.room.sessions))
	}

	first.Messages = append(first.Messages, first.NewHumanMessage("m1", "question one", time.Now()))
	if err := cm.SaveConversation(first); err != nil {
		t.Fatal(err)
	}

	cw.newRoomSession()
	second := cw.room.conv
	if second == nil || second.ID == first.ID {
		t.Fatal("a new session should be a fresh conversation")
	}
	if len(second.Messages) != 0 {
		t.Fatal("a new session must start with a clean context")
	}
	if len(cw.room.sessions) != 2 {
		t.Fatalf("both sessions should be listed, got %d", len(cw.room.sessions))
	}

	cw.room.turnConv = first
	cw.switchRoomSession(first.ID)
	if cw.room.conv != first {
		t.Fatal("switching to the turn's conversation must reuse its object, not a disk copy")
	}

	first.Messages = append(first.Messages, first.NewHumanMessage("m2", "question two", time.Now()))
	if err := cm.SaveConversation(first); err != nil {
		t.Fatal(err)
	}

	cw.leaveRoom()
	cw.openRoom("r1")
	if cw.room.conv == nil || cw.room.conv.ID != first.ID {
		t.Fatalf("reopening the room should enter the newest session %q, got %+v", first.ID, cw.room.conv)
	}
}

// TestRoomSessionIsolation checks a session cannot be grafted into another
// room, and that labels come from the first question.
func TestRoomSessionIsolation(t *testing.T) {
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	cw := testWindow()
	cw.convManager = cm
	cw.cfg.Rooms = []config.Room{
		{ID: "r1", Name: "甲", Members: []config.RoomMember{{Agent: "agent:A"}}},
		{ID: "r2", Name: "乙", Members: []config.RoomMember{{Agent: "agent:A"}}},
	}

	cw.openRoom("r1")
	foreign := cw.room.conv.ID
	cw.leaveRoom()

	if _, err := cw.loadRoomSession("r2", foreign); err == nil {
		t.Fatal("another room's conversation must be refused")
	}

	conv := &models.Conversation{ID: "c9", RoomID: "r1"}
	conv.Messages = append(conv.Messages, conv.NewHumanMessage("m1", "  \n帮我评审方案\n其余", time.Now()))
	if got := roomSessionLabel(*conv); got != "帮我评审方案" {
		t.Fatalf("the label should be the first question's first line, got %q", got)
	}
}

// TestParseMasterPick checks the moderator reply parser: names in order,
// NONE as an explicit nobody, and noise falling back to indecision.
func TestParseMasterPick(t *testing.T) {
	members := []config.RoomMember{
		{Agent: "agent:架构师", Name: "架构师"},
		{Agent: "agent:产品", Name: "产品"},
		{Agent: "agent:测试", Name: "测试"},
	}

	picked, ok := parseMasterPick("产品，架构师", members)
	if !ok || len(picked) != 2 || picked[0].Agent != "agent:产品" || picked[1].Agent != "agent:架构师" {
		t.Fatalf("names should parse in the moderator's order, got %v ok=%v", picked, ok)
	}

	picked, ok = parseMasterPick("NONE", members)
	if !ok || len(picked) != 0 {
		t.Fatalf("NONE should be an explicit nobody, got %v ok=%v", picked, ok)
	}

	if _, ok = parseMasterPick("我觉得这个问题很有意思", members); ok {
		t.Fatal("prose naming nobody is not a decision")
	}
	if _, ok = parseMasterPick("", members); ok {
		t.Fatal("an empty reply is not a decision")
	}

	picked, ok = parseMasterPick("测试, 测试, 不存在的人", members)
	if !ok || len(picked) != 1 || picked[0].Agent != "agent:测试" {
		t.Fatalf("duplicates and unknown names should be dropped, got %v ok=%v", picked, ok)
	}
}

// TestMasterPickFallsBack checks a broken moderator degrades to everyone,
// never to silence.
func TestMasterPickFallsBack(t *testing.T) {
	members := []config.RoomMember{{Agent: "agent:A", Name: "A"}, {Agent: "agent:B", Name: "B"}}
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "hello"))

	if got, _ := masterPick(context.Background(), nil, conv, members, ""); len(got) != 2 {
		t.Fatalf("no client should fall back to everyone, got %v", got)
	}

	failing := func(context.Context, []llm.ChatMessage) (*llm.ChatResponse, error) {
		return nil, context.DeadlineExceeded
	}
	if got, _ := masterPick(context.Background(), failing, conv, members, ""); len(got) != 2 {
		t.Fatalf("a failing call should fall back to everyone, got %v", got)
	}

	noisy := func(context.Context, []llm.ChatMessage) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Content: "我无法决定"}, nil
	}
	if got, _ := masterPick(context.Background(), noisy, conv, members, ""); len(got) != 2 {
		t.Fatalf("an unparseable reply should fall back to everyone, got %v", got)
	}

	decisive := func(context.Context, []llm.ChatMessage) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Content: "B"}, nil
	}
	got, _ := masterPick(context.Background(), decisive, conv, members, "")
	if len(got) != 1 || got[0].Agent != "agent:B" {
		t.Fatalf("a clear pick should narrow the roster, got %v", got)
	}

	self := func(context.Context, []llm.ChatMessage) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Content: "SELF: 这个问题我直接答：42。"}, nil
	}
	got, reply := masterPick(context.Background(), self, conv, members, "")
	if len(got) != 0 || reply != "这个问题我直接答：42。" {
		t.Fatalf("a SELF reply should carry the moderator's own answer, got %v %q", got, reply)
	}

	bareSelf := func(context.Context, []llm.ChatMessage) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Content: "SELF:"}, nil
	}
	if got, reply := masterPick(context.Background(), bareSelf, conv, members, ""); reply != "" || len(got) != 2 {
		t.Fatalf("a bare SELF is not an answer and should fall back, got %v %q", got, reply)
	}
}

// TestMasterAssignText checks the moderator's assignment reads as a direct
// @-address of the picked members, which is also what wakes them.
func TestMasterAssignText(t *testing.T) {
	picked := []config.RoomMember{
		{Agent: "agent:架构师", Name: "架构师"},
		{Agent: "agent:测试", Name: "测试"},
	}
	got := masterAssignText(picked)
	if got != "@架构师 @测试 请处理这个问题。" {
		t.Fatalf("the assignment should @ the picked members, got %q", got)
	}
	if roomAtMentioned(picked, got); len(roomAtMentioned(picked, got)) != 2 {
		t.Fatalf("the assignment must itself parse as mentions of the picked members")
	}
	if got := masterAssignText(nil); got != "本轮无需成员发言。" {
		t.Fatalf("a pick of nobody should say so, got %q", got)
	}
}

// TestMasterTranscriptCarriesHistory checks the moderator's reading includes
// the archived history block when retrieval found something, and omits it
// cleanly when it did not.
func TestMasterTranscriptCarriesHistory(t *testing.T) {
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "上次说的方案是什么"))
	members := []config.RoomMember{{Agent: "agent:A", Name: "A"}}

	with := masterTranscript(conv, members, "[10-27 14:00] A #3: 方案是走消息队列")
	if !strings.Contains(with, "群聊历史归档") || !strings.Contains(with, "走消息队列") {
		t.Fatalf("history should be injected with its header, got %q", with)
	}

	without := masterTranscript(conv, members, "")
	if strings.Contains(without, "群聊历史归档") {
		t.Fatalf("no history should mean no header, got %q", without)
	}
}

// TestRoomLastHumanText checks the moderator's retrieval query is the newest
// human message.
func TestRoomLastHumanText(t *testing.T) {
	conv := roomConvWith()
	conv.Messages = append(conv.Messages, humanMsg(conv, "first"))
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "reply"))
	conv.Messages = append(conv.Messages, humanMsg(conv, "second"))
	if got := roomLastHumanText(conv); got != "second" {
		t.Fatalf("the newest human message should be the query, got %q", got)
	}
	if got := roomLastHumanText(roomConvWith()); got != "" {
		t.Fatalf("no human message means no query, got %q", got)
	}
}

// TestRoomMentionQuery checks the @-completion trigger: a mention being
// typed at the end offers candidates, a finished or embedded @ does not.
func TestRoomMentionQuery(t *testing.T) {
	cases := []struct {
		draft  string
		prefix string
		start  int
		active bool
	}{
		{"@", "", 0, true},
		{"你看 @架", "架", len("你看 "), true},
		{"@架构师 ", "", 0, false},
		{"mail a@b", "", 0, false},
		{"没有提及", "", 0, false},
		{"", "", 0, false},
	}
	for _, tc := range cases {
		prefix, start, active := roomMentionQuery(tc.draft)
		if active != tc.active || (active && (prefix != tc.prefix || start != tc.start)) {
			t.Fatalf("draft %q should give (%q,%d,%v), got (%q,%d,%v)",
				tc.draft, tc.prefix, tc.start, tc.active, prefix, start, active)
		}
	}
}

// TestRoomAtMentionedOverridesStrategy checks an explicit @ wakes exactly
// the named members, whatever the activation strategy would decide.
func TestRoomAtMentionedOverridesStrategy(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{Name: "架构师"}, {Name: "产品"}, {Name: "测试"}}
	r := config.Room{
		ID: "r1", Activation: config.ActivationPooled,
		Members: []config.RoomMember{
			{Agent: "agent:架构师"}, {Agent: "agent:产品"}, {Agent: "agent:测试"},
		},
	}
	conv := roomConvWith()

	got := cw.roomSpeakers(r, conv, "@架构师 @测试 你们看呢")
	if len(got) != 2 || got[0].Agent != "agent:架构师" || got[1].Agent != "agent:测试" {
		t.Fatalf("an @ should wake exactly the named members in roster order, got %v", roomNames(got))
	}

	members := []config.RoomMember{{Agent: "agent:A", Name: "A"}, {Agent: "agent:Annabelle", Name: "Annabelle"}}
	if got := roomAtMentioned(members, "@Annabelle 你好"); len(got) != 1 || got[0].Agent != "agent:Annabelle" {
		t.Fatalf("@ matching must be whole-word, got %v", roomNames(got))
	}
	if got := roomAtMentioned(members, "A 你好"); len(got) != 0 {
		t.Fatalf("a bare name without @ is not a direct address, got %v", roomNames(got))
	}
}

// TestTakeRoomHandoff checks the handoff drain: mentioned members join once,
// speakers of this turn and strangers never, so a handoff chain terminates.
func TestTakeRoomHandoff(t *testing.T) {
	cw := testWindow()
	cw.dispatch = func(f func()) { f() }
	r := config.Room{ID: "r1", Members: []config.RoomMember{
		{Agent: "agent:A"}, {Agent: "agent:B"}, {Agent: "agent:C"},
	}}

	cw.room.handoff = []string{"agent:B", "agent:B", "agent:外人", "agent:A"}
	spoken := map[string]bool{"agent:A": true}

	got := cw.takeRoomHandoff(r, spoken)
	if len(got) != 1 || got[0].Agent != "agent:B" {
		t.Fatalf("only the unspoken roster member should join, once, got %v", roomNames(got))
	}
	if cw.room.handoff != nil {
		t.Fatal("the drain should clear the handoff list")
	}
	if len(cw.room.queue) != 1 || cw.room.queue[0] != "agent:B" {
		t.Fatalf("the joined member should appear on the queue, got %v", cw.room.queue)
	}

	if got := cw.takeRoomHandoff(r, spoken); len(got) != 0 {
		t.Fatalf("an empty handoff list should add nobody, got %v", roomNames(got))
	}
}

// TestRoomQuoteInContext checks a quote reaches a member's context as an
// explicit "replying to" marker, so a room with several threads stays
// readable, and that a dangling quote is simply ignored.
func TestRoomQuoteInContext(t *testing.T) {
	cw := testWindow()
	conv := roomConvWith()
	ask := humanMsg(conv, "方案怎么选")
	conv.Messages = append(conv.Messages, ask)
	reply := agentMsg(conv, "agent:A", "A", "我选 B 方案")
	reply.QuoteID = ask.ID
	conv.Messages = append(conv.Messages, reply)
	dangling := agentMsg(conv, "agent:C", "C", "无主引用")
	dangling.QuoteID = "gone"
	conv.Messages = append(conv.Messages, dangling)

	msgs := cw.buildRoomMessages(conv, config.RoomMember{Agent: "agent:B"}, "B")
	var quoted, loose string
	for _, m := range msgs {
		if strings.Contains(m.Content, "我选 B 方案") {
			quoted = m.Content
		}
		if strings.Contains(m.Content, "无主引用") {
			loose = m.Content
		}
	}
	if !strings.Contains(quoted, "（回复 #1 用户") || !strings.Contains(quoted, "方案怎么选") {
		t.Fatalf("a quoting reply should name the message and its gist, got %q", quoted)
	}
	if strings.Contains(loose, "回复 #") {
		t.Fatalf("a dangling quote should be ignored, got %q", loose)
	}
}

// TestRoomQuoteContextFallback checks a quoted message with no plain text
// falls back to its reasoning's opening line.
func TestRoomQuoteContextFallback(t *testing.T) {
	if got, ok := roomQuoteContext("第一行\n第二行", ""); !ok || got != "第一行" {
		t.Fatalf("plain text should win, got %q %v", got, ok)
	}
	if got, ok := roomQuoteContext("", "想了两步\n然后接近答案"); !ok || got != "想了两步" {
		t.Fatalf("reasoning should backfill empty text, got %q %v", got, ok)
	}
	if _, ok := roomQuoteContext("", ""); ok {
		t.Fatal("neither text nor reasoning means no context")
	}
}

// TestRoomTurnPromptID checks a member's reply quotes the message that
// opened the turn.
func TestRoomTurnPromptID(t *testing.T) {
	conv := roomConvWith()
	first := humanMsg(conv, "one")
	conv.Messages = append(conv.Messages, first)
	conv.Messages = append(conv.Messages, agentMsg(conv, "agent:A", "A", "r1"))
	second := humanMsg(conv, "two")
	conv.Messages = append(conv.Messages, second)

	if got := roomTurnPromptID(conv); got != second.ID {
		t.Fatalf("the newest human message should be quoted, got %q", got)
	}
	if got := roomTurnPromptID(roomConvWith()); got != "" {
		t.Fatalf("an empty conversation quotes nothing, got %q", got)
	}
}

// TestRoomQuoteSummary checks the one-line preview of a quoted message.
func TestRoomQuoteSummary(t *testing.T) {
	msg := models.Message{Content: "  \n第一行\n第二行"}
	if got := roomQuoteSummary(msg); got != "第一行" {
		t.Fatalf("the preview should be the first non-blank line, got %q", got)
	}
	if got := roomQuoteSummary(models.Message{}); got != "（无文字内容）" {
		t.Fatalf("an empty message needs a placeholder, got %q", got)
	}
}
