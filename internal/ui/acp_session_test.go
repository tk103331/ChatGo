package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// TestACPPConfigRank checks the composer ordering: model first, then thought
// level, then mode, then anything else.
func TestACPPConfigRank(t *testing.T) {
	m := acp.SessionConfigOptionCategoryModel
	th := acp.SessionConfigOptionCategoryThoughtLevel
	mo := acp.SessionConfigOptionCategoryMode
	cases := []struct {
		cat  *acp.SessionConfigOptionCategory
		want int
	}{
		{&m, 0}, {&th, 1}, {&mo, 2}, {nil, 3},
	}
	for _, tc := range cases {
		if got := acpConfigRank(tc.cat); got != tc.want {
			t.Fatalf("category %v should rank %d, got %d", tc.cat, tc.want, got)
		}
	}
	if got := acpConfigName(0); got != "模型" {
		t.Fatalf("rank 0 should be 模型, got %q", got)
	}
}

// TestACPToolKind checks the adapter's tool category is read back from the
// call's metadata and named in Chinese, with unknown kinds degrading to a
// plain row.
func TestACPToolKind(t *testing.T) {
	tc := models.ToolCall{Metadata: map[string]interface{}{"acp_kind": "execute"}}
	if got := acpToolKind(tc); got != "execute" {
		t.Fatalf("kind should come from metadata, got %q", got)
	}
	if got := acpToolKindLabel("execute"); got != "执行" {
		t.Fatalf("execute should read as 执行, got %q", got)
	}
	if got := acpToolKindIcon("edit"); got != "pencil" {
		t.Fatalf("edit should use the pencil icon, got %q", got)
	}
	if got := acpToolKindLabel("unknown"); got != "" {
		t.Fatalf("an unknown kind should have no label, got %q", got)
	}
	if got := acpToolKindIcon("unknown"); got != "wrench" {
		t.Fatalf("an unknown kind should fall back to the wrench icon, got %q", got)
	}
	if got := acpToolKind(models.ToolCall{}); got != "" {
		t.Fatalf("a call without metadata has no kind, got %q", got)
	}
}

// TestACPShutdownClearsSessions checks the quit hook drops every slot, so a
// reap afterwards finds nothing and no CLI is left tracked.
func TestACPShutdownClearsSessions(t *testing.T) {
	cw := testWindow()
	rt1 := &acpConv{key: "c1", sessionID: "s1"}
	rt2 := &acpConv{key: "c2\x00acp:claude", sessionID: "s2"}
	cw.acp.convs["c1"] = rt1
	cw.acp.convs["c2\x00acp:claude"] = rt2
	cw.acp.convBySess["s1"] = "c1"
	cw.acp.convBySess["s2"] = "c2\x00acp:claude"

	cw.acpShutdown()

	if len(cw.acp.convs) != 0 {
		t.Fatalf("every session slot should be gone, got %d", len(cw.acp.convs))
	}
	if len(cw.acp.convBySess) != 0 {
		t.Fatalf("session ids should be dropped too, got %d", len(cw.acp.convBySess))
	}
}

// TestParseACPPlan checks an ACP plan update is carried through with its
// entries, priority and status intact.
func TestParseACPPlan(t *testing.T) {
	upd, ok := parseACPUpdate(acp.SessionUpdate{Plan: &acp.SessionUpdatePlan{
		Entries: []acp.PlanEntry{
			{Content: "读取配置", Priority: acp.PlanEntryPriorityHigh, Status: acp.PlanEntryStatusCompleted},
			{Content: "修改代码", Priority: acp.PlanEntryPriorityMedium, Status: acp.PlanEntryStatusInProgress},
		},
	}})
	if !ok || upd.kind != "plan" {
		t.Fatalf("a plan update should parse, got kind=%q ok=%v", upd.kind, ok)
	}
	if len(upd.plan) != 2 {
		t.Fatalf("both entries should carry, got %d", len(upd.plan))
	}
	if upd.plan[0].Content != "读取配置" || upd.plan[0].Status != models.PlanCompleted {
		t.Fatalf("entry 0 lost its content or status: %+v", upd.plan[0])
	}
	if upd.plan[1].Priority != "medium" || upd.plan[1].Status != models.PlanInProgress {
		t.Fatalf("entry 1 lost its priority or status: %+v", upd.plan[1])
	}
}

// TestACPPlanReplacesWholeList checks the replace-not-merge rule: an update
// carrying fewer entries drops the old ones.
func TestACPPlanReplacesWholeList(t *testing.T) {
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	conv := &models.Conversation{ID: "c1", Plan: []models.PlanEntry{
		{Content: "旧任务一", Status: models.PlanPending},
		{Content: "旧任务二", Status: models.PlanPending},
	}}
	cw.acp.convObj["c1"] = conv

	cw.acpApplyToConv("c1", acpUpdate{kind: "plan", plan: []models.PlanEntry{
		{Content: "唯一任务", Status: models.PlanInProgress},
	}})

	if len(conv.Plan) != 1 || conv.Plan[0].Content != "唯一任务" {
		t.Fatalf("the plan should be replaced whole, got %+v", conv.Plan)
	}
}

// TestUpdatePlanTool checks the built-in plan tool replaces the whole list,
// normalizes statuses, and drops empty entries.
func TestUpdatePlanTool(t *testing.T) {
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.dispatch = func(f func()) { f() }
	conv := &models.Conversation{ID: "c1", Plan: []models.PlanEntry{
		{Content: "旧任务", Status: models.PlanPending},
	}}

	run := cw.updatePlanTool(conv)
	out, err := run(context.Background(), `{"entries":[
		{"content":"读取配置","priority":"high","status":"completed"},
		{"content":"修改代码","status":"in_progress"},
		{"content":"  ","status":"pending"},
		{"content":"跑测试","status":"bogus"}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Plan) != 3 {
		t.Fatalf("empty entries should be dropped, got %+v", conv.Plan)
	}
	if conv.Plan[0].Status != models.PlanCompleted || conv.Plan[1].Status != models.PlanInProgress {
		t.Fatalf("statuses should carry through, got %+v", conv.Plan)
	}
	if conv.Plan[2].Status != models.PlanPending {
		t.Fatalf("an unknown status should fall back to pending, got %q", conv.Plan[2].Status)
	}
	if !strings.Contains(out, "1/3") {
		t.Fatalf("the result should count completed entries, got %q", out)
	}
}

// TestACPEnsureSessionWaitsForStart checks a message sent while the session
// is warming up waits for that start instead of failing, and sees the same
// error when the start fails.
func TestACPEnsureSessionWaitsForStart(t *testing.T) {
	cw := testWindow()
	const key = "c1"
	ch := make(chan struct{})
	cw.acp.starting[key] = &acpStart{done: ch}

	done := make(chan error, 1)
	go func() {
		_, err := cw.acpEnsureSessionAt(&models.Conversation{ID: "c1"},
			config.ACPConnection{Name: "x", Command: "/nonexistent"}, key)
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("the caller should block while the session is starting")
	case <-time.After(50 * time.Millisecond):
	}

	cw.acp.mu.Lock()
	w := cw.acp.starting[key]
	w.err = fmt.Errorf("boom")
	close(w.done)
	delete(cw.acp.starting, key)
	cw.acp.mu.Unlock()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("the waiter should see the starter's error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the waiter never resumed")
	}
}

// TestPlanPanelRenders checks a conversation's plan shows above the composer
// with its entries, so a plain agent's update_plan call is visible.
func TestPlanPanelRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t",
		Messages: []models.Message{{ID: "m1", Role: "assistant", Content: "ok"}},
		Plan: []models.PlanEntry{
			{Content: "读取配置", Status: models.PlanCompleted},
			{Content: "修改代码", Status: models.PlanInProgress, Priority: "high"},
		}}
	cw.convs = []models.Conversation{*cw.cur}
	cw.focusConvTab(cw.cur)

	tr := mgo.NewTester(cw.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	if _, ok := tr.Find("执行计划"); !ok {
		t.Fatalf("the plan panel should render; texts: %v", tr.Texts())
	}
	if _, ok := tr.Find("读取配置"); !ok {
		t.Fatalf("a plan entry should render; texts: %v", tr.Texts())
	}
}

// TestUpdatePlanToolRegistered checks the plan tool reaches an agent's tool
// list, so a model can actually call it.
func TestUpdatePlanToolRegistered(t *testing.T) {
	cw := testWindow()
	cw.cfg.BuiltinTools = []config.BuiltinTool{
		{Name: "update_plan", Type: "update_plan", Enabled: true},
	}
	found := false
	for _, id := range cw.selectedToolIDs() {
		if id == "builtin:update_plan" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("update_plan should be a selectable tool, got %v", cw.selectedToolIDs())
	}
	def, err := cw.createBuiltinToolDefinition("update_plan", t.TempDir())
	if err != nil {
		t.Fatalf("the tool definition should build, got %v", err)
	}
	if def.Name == "" || def.Handler == nil {
		t.Fatalf("the definition is incomplete: %+v", def)
	}
}

// TestUpdatePlanFallsBackToCurrentConv checks a tool built before a
// conversation existed still records the plan on the open one.
func TestUpdatePlanFallsBackToCurrentConv(t *testing.T) {
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.dispatch = func(f func()) { f() }
	cw.cur = &models.Conversation{ID: "c1"}

	run := cw.updatePlanTool(nil) // built with no conversation
	out, err := run(context.Background(),
		`{"entries":[{"content":"任务一","priority":"high","status":"pending"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cw.cur.Plan) != 1 || cw.cur.Plan[0].Content != "任务一" {
		t.Fatalf("the plan should land on the open conversation, got %+v (out=%q)", cw.cur.Plan, out)
	}
}

// TestAskQuestionTool checks the question tool opens a prompt and returns the
// user's answer as its result.
func TestAskQuestionTool(t *testing.T) {
	cw := testWindow()
	cw.dispatch = func(f func()) { f() }

	done := make(chan string, 1)
	go func() {
		out, err := cw.askQuestionTool()(context.Background(),
			`{"question":"选哪个方案?","options":["A","B"]}`)
		if err != nil {
			done <- "ERR: " + err.Error()
			return
		}
		done <- out
	}()

	deadline := time.Now().Add(2 * time.Second)
	for cw.askQ == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if cw.askQ == nil {
		t.Fatal("the tool should open a question")
	}
	if len(cw.askQ.items) != 1 || cw.askQ.items[0].question != "选哪个方案?" ||
		len(cw.askQ.items[0].options) != 2 {
		t.Fatalf("the question lost its text or options: %+v", cw.askQ.items)
	}
	cw.askQ.reply <- "A"

	select {
	case out := <-done:
		if out != "A" {
			t.Fatalf("the tool should return the answer, got %q", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the tool never returned")
	}
}

// TestAskQuestionToolRegistered checks the tool reaches an agent's list.
func TestAskQuestionToolRegistered(t *testing.T) {
	cw := testWindow()
	cw.cfg.BuiltinTools = []config.BuiltinTool{
		{Name: "ask_question", Type: "ask_question", Enabled: true},
	}
	found := false
	for _, id := range cw.selectedToolIDs() {
		if id == "builtin:ask_question" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ask_question should be selectable, got %v", cw.selectedToolIDs())
	}
	if _, err := cw.createBuiltinToolDefinition("ask_question", t.TempDir()); err != nil {
		t.Fatalf("the definition should build, got %v", err)
	}
}

// TestAskQuestionMultiple checks several questions parse into tabs and their
// answers format back into one result.
func TestAskQuestionMultiple(t *testing.T) {
	cw := testWindow()
	cw.dispatch = func(f func()) { f() }

	done := make(chan string, 1)
	go func() {
		out, err := cw.askQuestionTool()(context.Background(),
			`{"questions":[{"question":"选哪个?","options":["A","B"]},{"question":"为什么?"}]}`)
		if err != nil {
			done <- "ERR: " + err.Error()
			return
		}
		done <- out
	}()

	deadline := time.Now().Add(2 * time.Second)
	for cw.askQ == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if cw.askQ == nil || len(cw.askQ.items) != 2 {
		t.Fatalf("both questions should be kept, got %+v", cw.askQ)
	}
	cw.askQ.items[0].answer = "A"
	cw.askQ.items[1].answer = "因为更简单"
	cw.askQ.reply <- cw.askQ.result()

	select {
	case out := <-done:
		for _, want := range []string{"选哪个?", "A", "为什么?", "因为更简单"} {
			if !strings.Contains(out, want) {
				t.Fatalf("the result should carry %q, got %q", want, out)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the tool never returned")
	}
}

// TestClosePlan checks the panel's close control drops the plan, and that a
// finished plan is dropped automatically when a new message starts a job.
func TestClosePlan(t *testing.T) {
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	conv := &models.Conversation{ID: "c1", Plan: []models.PlanEntry{
		{Content: "任务", Status: models.PlanCompleted},
	}}

	cw.closeDonePlan(conv)
	if conv.Plan != nil {
		t.Fatalf("an all-done plan should be dropped on a new message, got %+v", conv.Plan)
	}

	conv.Plan = []models.PlanEntry{{Content: "任务", Status: models.PlanInProgress}}
	cw.closeDonePlan(conv)
	if len(conv.Plan) != 1 {
		t.Fatal("a plan with unfinished work must stay")
	}

	cw.closePlan(conv)
	if conv.Plan != nil {
		t.Fatalf("closing should clear the plan, got %+v", conv.Plan)
	}
}

// TestAskQuestionPanelRenders checks the question shows above the composer
// rather than in a modal dialog.
func TestAskQuestionPanelRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t",
		Messages: []models.Message{{ID: "m1", Role: "assistant", Content: "ok"}}}
	cw.convs = []models.Conversation{*cw.cur}
	cw.focusConvTab(cw.cur)
	cw.askQ = &askQuestion{
		items: []*askQuestionItem{{question: "选哪个方案?", options: []string{"A", "B"}}},
		reply: make(chan string, 1),
	}

	tr := mgo.NewTester(cw.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	if _, ok := tr.Find("Agent 提问"); !ok {
		t.Fatalf("the question panel should render; texts: %v", tr.Texts())
	}
	if _, ok := tr.Find("选哪个方案?"); !ok {
		t.Fatalf("the question text should render; texts: %v", tr.Texts())
	}
}

// TestAskQuestionToolHeadless checks a question fails fast when no window can
// render it, instead of blocking the tool call forever.
func TestAskQuestionToolHeadless(t *testing.T) {
	cw := &ChatWindow{}
	done := make(chan error, 1)
	go func() {
		_, err := cw.askQuestionTool()(context.Background(), `{"question":"选哪个?"}`)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a question with no UI should report an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a question with no UI must not block")
	}
}
