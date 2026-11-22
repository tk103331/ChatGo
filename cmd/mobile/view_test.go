package main

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/mobileapi"
	"chatgo/pkg/models"
)

func testApp() *app {
	now := time.Now()
	conv := &models.Conversation{
		ID:    "c1",
		Title: "测试会话",
		Messages: []models.Message{
			{ID: "u1", Role: "user", Content: "你好", Timestamp: now},
			{ID: "r1", Role: "assistant", AuthorName: "ChatGo", Timestamp: now,
				Segments: []models.Segment{
					{Kind: "reasoning", Text: "先想一想"},
					{Kind: "tool", ToolCall: &models.ToolCall{Name: "search", Status: "completed"}},
					{Kind: "text", Text: "**答案**在这里"},
				}},
		},
	}
	return &app{
		servers:     []server{{ID: "d1", Name: "Test", RelayURL: "https://relay.example.com", DesktopID: "d1", Token: "t"}},
		server:      0,
		meta:        mobileapi.Meta{Ready: true, Provider: "Local", Model: "llama3"},
		sessions:    []mobileapi.SessionSummary{{ID: "c1", Title: "测试会话", Preview: "你好", UpdatedAt: now, Messages: 2}},
		rooms:       []mobileapi.Room{{ID: "r1", Name: "测试群", Members: []mobileapi.RoomMember{{Agent: "agent:A", Name: "A"}}}},
		tasks:       []mobileapi.Task{{ID: "t1", Name: "日报", Prompt: "总结今天", Enabled: true, Schedule: mobileapi.Schedule{Kind: "daily", Hour: 9}}},
		roomFormSel: map[string]bool{},
		conv:        conv,
		openConv:    "c1",
	}
}

func renderApp(t *testing.T, a *app, w, h int) {
	t.Helper()
	img := ui.Render(a.view, w, h, 1)
	if img == nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("render %dx%d failed: %v", w, h, img)
	}
}

func TestMobileViewRendersLists(t *testing.T) {
	a := testApp()
	for _, screen := range []screen{screenList, screenNew, screenRooms, screenRoom, screenTasks, screenTask, screenSettings} {
		a.screen = screen
		renderApp(t, a, 390, 844)
	}
}

func TestMobileViewRendersChat(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	renderApp(t, a, 390, 844)
}

func TestMobileViewRendersNewSession(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.screen = screenNew
	a.agents = []mobileapi.Agent{
		{Ref: "", Name: "Chat", Kind: "chat"},
		{Ref: "agent:test", Name: "test", Kind: "agent"},
	}
	a.meta.Models = []mobileapi.ModelChoice{
		{Provider: "deepseek", Model: "deepseek-chat"},
		{Provider: "deepseek", Model: "deepseek-flash"},
	}
	a.newAgent = "agent:test"
	a.newModel = "deepseek::deepseek-flash"
	renderApp(t, a, 390, 844)
}

func TestNewChatUsesComposer(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("新建会话"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(300 * time.Millisecond)
	if !tr.HasText("创建会话") {
		t.Fatalf("new chat composer missing; texts: %v", tr.Texts())
	}
}

func TestPairSheetOpensFromEmptyState(t *testing.T) {
	a := testApp()
	a.servers = nil
	a.screen = screenList
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(10 * time.Millisecond)
	if err := tr.Click("添加桌面"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(500 * time.Millisecond)
	if !tr.HasText("连接桌面") {
		t.Fatalf("pair sheet did not open; texts: %v", tr.Texts())
	}
}

func TestMobileViewRendersTask(t *testing.T) {
	a := testApp()
	a.screen = screenTask
	task := a.tasks[0]
	a.task = &task
	a.taskSess = []mobileapi.RoomSession{{ID: "s1", Label: "今天", Updated: time.Now()}}
	renderApp(t, a, 390, 844)
}

func TestMobileViewRendersPairSheet(t *testing.T) {
	a := testApp()
	a.pairOpen = true
	a.pairRelay = "wss://relay.example.com"
	a.pairDesktop = "d1"
	a.pairCode = "ABCDEFGH"
	renderApp(t, a, 390, 844)
}

// The execution process folds away when the turn ends, as on the desktop:
// the reply reads as its answer, with the trace one tap away — and a tool
// call appears once, not again after the answer.
func TestChatFoldHidesProcessUntilOpened(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if !tr.HasText("思考 1 次 · 工具调用 1 次") {
		t.Fatalf("process summary missing; texts: %v", tr.Texts())
	}
	if tr.HasText("search") {
		t.Fatalf("tool row should stay folded; texts: %v", tr.Texts())
	}
	if err := tr.Click("执行过程"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(50 * time.Millisecond)
	if n := countText(tr.Texts(), "search"); n != 1 {
		t.Fatalf("tool row rendered %d times, want 1; texts: %v", n, tr.Texts())
	}
}

// A healthy link needs no status line on every page.
func TestConnectedChatHasNoStatusStrip(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	a.connected, a.eventsConnected = true, true
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if tr.HasText("已连接") {
		t.Fatalf("status strip still shown; texts: %v", tr.Texts())
	}
}

func countText(texts []string, want string) int {
	n := 0
	for _, s := range texts {
		if s == want {
			n++
		}
	}
	return n
}

// The window's root already keeps content above the keyboard, so the composer
// must not add that inset again: it belongs right on the keyboard's edge, not
// lifted off it by its full height.
func TestComposerSitsAboveKeyboard(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	tr := ui.NewTester(a.view, 390, 844)
	tr.SetInsets(0, 0, 300, 0)
	tr.Frame()
	r, ok := tr.Find("发送")
	if !ok {
		t.Fatalf("send button missing; texts: %v", tr.Texts())
	}
	if r.Y+r.H > 844-300 {
		t.Fatalf("composer under the keyboard: %+v", r)
	}
	if r.Y < 400 {
		t.Fatalf("composer lifted off the keyboard: %+v", r)
	}
}

// Opening a conversation shows its newest message, not its first.
func TestChatOpensAtTheEnd(t *testing.T) {
	a := testApp()
	var msgs []models.Message
	for i := 0; i < 30; i++ {
		msgs = append(msgs,
			models.Message{ID: "u" + strconv.Itoa(i), Role: "user", Content: "问题 " + strconv.Itoa(i)},
			models.Message{ID: "r" + strconv.Itoa(i), Role: "assistant", AuthorName: "ChatGo", Content: "回答 " + strconv.Itoa(i)})
	}
	a.conv.Messages = msgs
	a.screen = screenChat
	tr := ui.NewTester(a.view, 390, 844)
	for range 3 {
		tr.Frame()
	}
	if a.chatScroll.MaxY <= 0 {
		t.Fatalf("conversation did not scroll: %+v", a.chatScroll)
	}
	if a.chatScroll.Y < a.chatScroll.MaxY {
		t.Fatalf("opened at %v of %v, want the end", a.chatScroll.Y, a.chatScroll.MaxY)
	}
}

// A reader who scrolled up keeps their place while the turn streams.
func TestChatDoesNotYankAScrolledUpReader(t *testing.T) {
	a := testApp()
	var msgs []models.Message
	for i := 0; i < 30; i++ {
		msgs = append(msgs,
			models.Message{ID: "u" + strconv.Itoa(i), Role: "user", Content: "问题 " + strconv.Itoa(i)},
			models.Message{ID: "r" + strconv.Itoa(i), Role: "assistant", AuthorName: "ChatGo", Content: "回答 " + strconv.Itoa(i)})
	}
	a.conv.Messages = msgs
	a.screen = screenChat
	tr := ui.NewTester(a.view, 390, 844)
	for range 3 {
		tr.Frame()
	}
	a.chatScroll.Y = 0
	a.running = true
	for range 3 {
		tr.Frame()
	}
	if a.chatScroll.Y != 0 {
		t.Fatalf("scrolled-up reader was moved to %v", a.chatScroll.Y)
	}
}

// The chat composer offers what the new-session page does: an attachment
// button and a model chip.
func TestChatComposerOffersAttachmentsAndModel(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	a.meta.Models = []mobileapi.ModelChoice{{Provider: "deepseek", Model: "deepseek-chat"}}
	a.chatModel = "deepseek::deepseek-chat"
	tr := ui.NewTester(a.view, 390, 844)
	tr.Frame()
	if _, ok := tr.AccessByRole(ui.RoleButton, "添加附件"); !ok {
		t.Fatalf("chat composer has no attachment button; texts: %v", tr.Texts())
	}
	if _, ok := tr.AccessByRole(ui.RoleButton, "模型 deepseek-chat"); !ok {
		t.Fatalf("chat composer has no model chip; texts: %v", tr.Texts())
	}
}

// A room's members bring their own models, so its composer offers none.
func TestRoomChatHasNoModelChip(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	a.roomID = "r1"
	a.meta.Models = []mobileapi.ModelChoice{{Provider: "deepseek", Model: "deepseek-chat"}}
	a.chatModel = "deepseek::deepseek-chat"
	tr := ui.NewTester(a.view, 390, 844)
	tr.Frame()
	if _, ok := tr.AccessByRole(ui.RoleButton, "模型 deepseek-chat"); ok {
		t.Fatal("a room chat should not pick a model")
	}
}

// Picking a model in the sheet changes the model the chat composer sends with.
func TestChatModelSheetPicksForTheChat(t *testing.T) {
	a := testApp()
	a.screen = screenChat
	a.meta.Models = []mobileapi.ModelChoice{
		{Provider: "deepseek", Model: "deepseek-chat"},
		{Provider: "openai", Model: "gpt-5"},
	}
	a.chatModel = "deepseek::deepseek-chat"
	a.newModel = "deepseek::deepseek-chat"
	a.modelFor = modelForChat
	a.modelOpen = true
	tr := ui.NewTester(a.view, 390, 844)
	for range 3 {
		tr.Frame()
	}
	tr.Advance(time.Second)
	tr.Frame()
	if err := tr.Click("gpt-5"); err != nil {
		t.Fatalf("model row not clickable: %v; texts: %v", err, tr.Texts())
	}
	if a.chatModel != "openai::gpt-5" {
		t.Fatalf("chat model = %q, want openai::gpt-5", a.chatModel)
	}
	if a.newModel != "deepseek::deepseek-chat" {
		t.Fatalf("the new-session choice changed to %q", a.newModel)
	}
}

// Opening a conversation points its composer at the model it last used.
func TestChatModelFollowsTheConversation(t *testing.T) {
	a := testApp()
	a.meta.Models = []mobileapi.ModelChoice{
		{Provider: "deepseek", Model: "deepseek-chat"},
		{Provider: "openai", Model: "gpt-5"},
	}
	a.newModel = "deepseek::deepseek-chat"
	a.syncChatModel(&models.Conversation{Provider: "openai", Model: "gpt-5"})
	if a.chatModel != "openai::gpt-5" {
		t.Fatalf("chat model = %q, want the conversation's", a.chatModel)
	}
}

// The agent sheet never lists the built-in chat twice.
func TestAgentChoicesListChatOnce(t *testing.T) {
	a := testApp()
	a.agents = []mobileapi.Agent{{Ref: "", Name: "Chat", Kind: "chat"}, {Ref: "agent:x", Name: "X", Kind: "acp"}}
	choices := a.agentChoices()
	chat := 0
	for _, item := range choices {
		if item.Ref == "" {
			chat++
		}
	}
	if chat != 1 || len(choices) != 2 {
		t.Fatalf("choices = %+v, want one built-in chat and one agent", choices)
	}
	a.agents = []mobileapi.Agent{{Ref: "agent:x", Name: "X", Kind: "acp"}}
	choices = a.agentChoices()
	if len(choices) != 2 || choices[0].Ref != "" {
		t.Fatalf("choices = %+v, want the built-in chat first", choices)
	}
}

// A preview shows words, not markdown syntax.
func TestSessionPreviewIsPlainText(t *testing.T) {
	in := "**Jujutsu (jj)** — see [the docs](https://example.com) and `jj`.\n- item one\n> quote\n```\ncode\n```"
	got := plainPreview(in)
	if strings.ContainsAny(got, "*`>[]") {
		t.Fatalf("preview kept markdown: %q", got)
	}
	if !strings.Contains(got, "Jujutsu (jj)") || !strings.Contains(got, "the docs") {
		t.Fatalf("preview lost words: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("preview kept line breaks: %q", got)
	}
}

// Sessions group under today, yesterday, this week and older.
func TestSessionGroupsByDay(t *testing.T) {
	now := time.Now()
	groups := sessionGroups([]mobileapi.SessionSummary{
		{ID: "a", UpdatedAt: now},
		{ID: "b", UpdatedAt: now.AddDate(0, 0, -1)},
		{ID: "c", UpdatedAt: now.AddDate(0, 0, -3)},
		{ID: "d", UpdatedAt: now.AddDate(0, 0, -30)},
	})
	var labels []string
	for _, g := range groups {
		labels = append(labels, g.label)
	}
	want := []string{"今天", "昨天", "本周", "更早"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Fatalf("groups = %v, want %v", labels, want)
	}
}

// A long preview stays two lines, so rows keep a compact pitch.
func TestSessionListRowsStayCompact(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = true, true
	a.screen = screenList
	a.sessions = []mobileapi.SessionSummary{
		{ID: "1", Title: "第一条", Preview: strings.Repeat("很长的预览内容 ", 40), UpdatedAt: time.Now()},
		{ID: "2", Title: "第二条", Preview: "短", UpdatedAt: time.Now().Add(-time.Minute)},
	}
	tr := ui.NewTester(a.view, 390, 844)
	for range 3 {
		tr.Frame()
	}
	first, ok := tr.Find("第一条")
	if !ok {
		t.Fatalf("first row missing; texts: %v", tr.Texts())
	}
	second, ok := tr.Find("第二条")
	if !ok {
		t.Fatalf("second row missing; texts: %v", tr.Texts())
	}
	if pitch := second.Y - first.Y; pitch > 100 {
		t.Fatalf("row pitch = %v, want a compact two-line row", pitch)
	}
	if first.Y > 200 {
		t.Fatalf("list starts at %v, want it right under the search field", first.Y)
	}
}

// The right edge stays short: the time today, the date otherwise.
func TestShortTimeKeepsRowsShort(t *testing.T) {
	now := time.Now()
	if got := shortTime(now); len(got) != 5 {
		t.Fatalf("today = %q, want HH:MM", got)
	}
	if got := shortTime(now.AddDate(0, 0, -3)); len(got) != 5 {
		t.Fatalf("this year = %q, want MM-DD", got)
	}
	if got := shortTime(now.AddDate(-1, 0, 0)); len(got) != 10 {
		t.Fatalf("older = %q, want YYYY-MM-DD", got)
	}
}

// A generated title drops its quote or heading marks, and a raw protocol
// line is not shown as a name.
func TestSessionTitleDropsMarks(t *testing.T) {
	cases := map[string]string{
		"> {\"type\":\"assistant\"}": "未命名",
		"# 修复登录":                     "修复登录",
		"  普通标题  ":                   "普通标题",
		"":                           "未命名",
		"[1, 2]":                     "[1, 2]",
	}
	for in, want := range cases {
		if got := sessionTitle(mobileapi.SessionSummary{Title: in}); got != want {
			t.Fatalf("title(%q) = %q, want %q", in, got, want)
		}
	}
}

// The 会话 tab opens the history, not a blank new conversation.
func TestChatTabOpensHistory(t *testing.T) {
	a := testApp()
	a.screen = screenSettings
	a.tab = tabSettings
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("会话"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(200 * time.Millisecond)
	if a.screen != screenList {
		t.Fatalf("screen = %v, want the list", a.screen)
	}
}

// The button floating over the list starts a conversation.
func TestListFloatingButtonStartsAChat(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.screen = screenList
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("新建会话"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(300 * time.Millisecond)
	if a.screen != screenNew {
		t.Fatalf("screen = %v, want the new-session page", a.screen)
	}
	if !tr.HasText("创建会话") {
		t.Fatalf("new chat composer missing; texts: %v", tr.Texts())
	}
}

// The search field narrows the list, and says so when nothing matches.
func TestSessionSearchNarrowsTheList(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.screen = screenList
	a.sessions = []mobileapi.SessionSummary{
		{ID: "1", Title: "修复登录", Preview: "改好了", UpdatedAt: time.Now()},
		{ID: "2", Title: "重构中继", Preview: "重连问题", UpdatedAt: time.Now()},
	}
	a.listQuery = "登录"
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if !tr.HasText("修复登录") {
		t.Fatalf("matching row missing; texts: %v", tr.Texts())
	}
	if tr.HasText("重构中继") {
		t.Fatalf("non-matching row still listed; texts: %v", tr.Texts())
	}
	a.listQuery = "没有这个"
	tr = ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if !tr.HasText("没有匹配的会话") {
		t.Fatalf("no-match state missing; texts: %v", tr.Texts())
	}
}

// An agent's configured icon is decoded and drawn in its row; junk is not.
func TestAgentIconDecodesAndFallsBack(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="#f00"/></svg>`)
	src := decodeIcon(svg)
	if src == nil {
		t.Fatal("svg icon not decoded")
	}
	if decodeIcon([]byte("not an image")) != nil {
		t.Fatal("junk decoded as an icon")
	}
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.agentIcons = map[string]ui.ImageSource{"acp:oc": src}
	a.sessions = []mobileapi.SessionSummary{
		{ID: "1", Title: "带图标", Agent: "acp:oc", UpdatedAt: time.Now()},
		{ID: "2", Title: "无图标", Agent: "acp:other", UpdatedAt: time.Now()},
	}
	a.screen = screenList
	renderApp(t, a, 390, 844)
}

// The 会话 tab also leaves the new-session page for the list.
func TestChatTabLeavesNewSessionForTheList(t *testing.T) {
	a := testApp()
	a.screen = screenNew
	a.tab = tabChat
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("会话"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(200 * time.Millisecond)
	if a.screen != screenList {
		t.Fatalf("screen = %v, want the list", a.screen)
	}
}

// The rooms list floats the button that starts a group, and the bar only
// refreshes.
func TestRoomFloatingButtonOpensTheForm(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = true, true
	a.screen = screenRooms
	a.rooms = []mobileapi.Room{{ID: "r1", Name: "讨论组", Members: []mobileapi.RoomMember{{Agent: "", Name: "Chat"}}}}
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("新建群聊"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(200 * time.Millisecond)
	if !a.roomFormOpen {
		t.Fatal("the group form did not open")
	}
}

// The task list floats the button that starts a task.
func TestTaskFloatingButtonOpensTheForm(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = true, true
	a.screen = screenTasks
	a.tasks = []mobileapi.Task{{ID: "t1", Name: "早报", Enabled: true}}
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("新建任务"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(200 * time.Millisecond)
	if !a.taskFormOpen {
		t.Fatal("the task form did not open")
	}
}

// The settings page says what the link is doing.
func TestSettingsShowsTheConnectionState(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = true, true
	a.screen = screenSettings
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if !tr.HasText("已连接") {
		t.Fatalf("connection state missing; texts: %v", tr.Texts())
	}
	if !tr.HasText("状态") {
		t.Fatalf("state row missing; texts: %v", tr.Texts())
	}
}

// The "移除" button inside a desktop row takes that desktop away and does
// not also switch to it.
func TestRemoveDesktopButtonOnlyRemoves(t *testing.T) {
	a := testApp()
	a.storageDir = t.TempDir()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.servers = []server{
		{ID: "d1", Name: "工作台式机", RelayURL: "https://relay.example.com", DesktopID: "d1", Token: "t"},
		{ID: "d2", Name: "家里那台", RelayURL: "https://relay.example.com", DesktopID: "d2", Token: "t"},
	}
	a.server = 0
	a.screen = screenSettings
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if err := tr.Click("移除"); err != nil {
		t.Fatal(err)
	}
	tr.Advance(200 * time.Millisecond)
	if len(a.servers) != 1 || a.servers[0].DesktopID != "d1" {
		t.Fatalf("servers = %+v, want the other desktop kept", a.servers)
	}
	if a.server != 0 {
		t.Fatalf("server = %d, want the current desktop", a.server)
	}
}

// Removing a desktop above the current one keeps the phone on the desktop it
// is talking to, without reconnecting.
func TestRemoveDesktopBeforeCurrentKeepsTheCurrent(t *testing.T) {
	a := testApp()
	a.storageDir = t.TempDir()
	a.servers = []server{
		{ID: "d1", DesktopID: "d1"},
		{ID: "d2", DesktopID: "d2"},
		{ID: "d3", DesktopID: "d3"},
	}
	a.server = 1
	a.removeServer(0)
	if len(a.servers) != 2 || a.servers[0].DesktopID != "d2" {
		t.Fatalf("servers = %+v, want d1 gone", a.servers)
	}
	if a.server != 0 {
		t.Fatalf("server = %d, want the same desktop at its new index", a.server)
	}
}

// The new-session page greets above its choices, the way the desktop does.
func TestNewSessionPageGreets(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = true, true
	a.screen = screenNew
	a.tab = tabChat
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if !tr.HasText("在 ChatGo 里做点什么？") {
		t.Fatalf("greeting missing; texts: %v", tr.Texts())
	}
}

// An author's name resolves to the configured agent, so the byline can carry
// the same mark as the lists.
func TestAgentRefForName(t *testing.T) {
	a := testApp()
	a.agents = []mobileapi.Agent{
		{Ref: "acp:OpenCode", Name: "OpenCode"},
		{Ref: "a2a:reviewer", Name: "审查员"},
	}
	if got := a.agentRefForName("OpenCode"); got != "acp:OpenCode" {
		t.Fatalf("agentRefForName(OpenCode) = %q", got)
	}
	if got := a.agentRefForName("opencode"); got != "acp:OpenCode" {
		t.Fatalf("agentRefForName(opencode) = %q", got)
	}
	if got := a.agentRefForName("别人"); got != "" {
		t.Fatalf("agentRefForName(别人) = %q", got)
	}
}

// A failed connection leaves a client behind, but settings should not offer
// to disconnect a link that is not there.
func TestSettingsHidesDisconnectWhenOffline(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = false, false
	a.screen = screenSettings
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if tr.HasText("断开连接") {
		t.Fatalf("disconnect offered while offline; texts: %v", tr.Texts())
	}
}

// Settings has its own state row, so the global strip does not repeat it.
func TestSettingsDoesNotRepeatTheStatusStrip(t *testing.T) {
	a := testApp()
	a.client = mobileapi.New("http://127.0.0.1:1", "t", "d1")
	a.connected, a.eventsConnected = false, false
	a.screen = screenSettings
	tr := ui.NewTester(a.view, 390, 844)
	tr.Advance(20 * time.Millisecond)
	if tr.HasText("Test · 未连接") {
		t.Fatalf("status strip repeated on settings; texts: %v", tr.Texts())
	}
}
