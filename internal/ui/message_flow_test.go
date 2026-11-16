package ui

import (
	"testing"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// processFlowFixture is a chat window holding one finished assistant reply
// whose turn thought twice and called two tools around an intermediate
// message, ending in the final answer.
func processFlowFixture(t *testing.T) *ChatWindow {
	t.Helper()
	start := time.Now().Add(-12 * time.Second)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t",
		Turns: []models.Turn{{ID: "t1", ReplyMsgID: "a1", StartedAt: start, EndedAt: start.Add(10 * time.Second)}},
		Messages: []models.Message{{ID: "a1", Role: "assistant", Segments: []models.Segment{
			{Kind: "reasoning", Text: "第一轮思考\n想好了第一步", At: start},
			{Kind: "tool", ToolCall: &models.ToolCall{ID: "tc1", Name: "read_file"}, At: start.Add(2 * time.Second)},
			{Kind: "text", Text: "中间说明", At: start.Add(4 * time.Second)},
			{Kind: "reasoning", Text: "第二轮思考\n想好了第二步", At: start.Add(5 * time.Second)},
			{Kind: "tool", ToolCall: &models.ToolCall{ID: "tc2", Name: "run_command"}, At: start.Add(7 * time.Second)},
			{Kind: "text", Text: "最终答案", At: start.Add(9 * time.Second)},
		}}},
	}
	cw.convs = []models.Conversation{*cw.cur}
	return cw
}

// A finished turn shows one process fold with the totals of the whole turn,
// and only the last message text beside it; opening the fold lists the
// timeline — thinking rows, tool rows and the intermediate message — with
// every thinking and tool row still collapsed.
func TestMessageProcessFoldAggregatesTheTurn(t *testing.T) {
	cw := processFlowFixture(t)
	cw.focusConvTab(cw.cur)

	tr := mgo.NewTester(cw.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	// Folded: the counts stand for the whole turn, and only the final text
	// reads as the reply.
	for _, want := range []string{"思考 2 次 · 工具调用 2 次", "最终答案", "用时 10.0 s"} {
		if !tr.HasText(want) {
			t.Fatalf("the folded header should show %q; texts: %v", want, tr.Texts())
		}
	}
	for _, hidden := range []string{"中间说明", "想好了第一步", "想好了第二步", "read_file", "run_command", "思考过程"} {
		if tr.HasText(hidden) {
			t.Fatalf("%q should be inside the closed fold; texts: %v", hidden, tr.Texts())
		}
	}

	// Opened: the timeline lists every entry in order, and each thinking
	// and tool row stays collapsed to its one line — the summary carries
	// the newest thought, the body stays hidden.
	if err := tr.Click("思考 2 次 · 工具调用 2 次"); err != nil {
		t.Fatalf("open the process fold: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	for _, want := range []string{"思考过程", "中间说明", "read_file", "run_command", "最终答案",
		"想好了第一步", "想好了第二步"} {
		if !tr.HasText(want) {
			t.Fatalf("the timeline should show %q; texts: %v", want, tr.Texts())
		}
	}
	for _, hidden := range []string{"第一轮思考", "第二轮思考"} {
		if tr.HasText(hidden) {
			t.Fatalf("the thinking body %q should stay folded; texts: %v", hidden, tr.Texts())
		}
	}
	// A thinking row opens its body on click.
	if err := tr.Click("思考过程"); err != nil {
		t.Fatalf("open a thinking row: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("第一轮思考") {
		t.Fatalf("the opened thinking body is missing; texts: %v", tr.Texts())
	}
	if tr.HasText("第二轮思考") {
		t.Fatalf("the second thinking row should still be collapsed; texts: %v", tr.Texts())
	}
}

// While the turn runs the process timeline opens on its own and lists the
// entries as they happen; when the stream ends it folds to the counts,
// leaving the answer as the only message body.
func TestMessageProcessFollowsTheStream(t *testing.T) {
	start := time.Now().Add(-4 * time.Second)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t",
		Turns: []models.Turn{{ID: "t1", ReplyMsgID: "a1", StartedAt: start}},
		Messages: []models.Message{{ID: "a1", Role: "assistant", Segments: []models.Segment{
			{Kind: "reasoning", Text: "边想边说\n最新一行", At: start},
			{Kind: "tool", ToolCall: &models.ToolCall{ID: "tc1", Name: "read_file"}, At: start.Add(2 * time.Second)},
		}}},
	}
	cw.convs = []models.Conversation{*cw.cur}
	cw.beginStream("c1", "a1", nil)
	cw.focusConvTab(cw.cur)

	tr := mgo.NewTester(cw.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	// Live: the timeline is open, listing the thinking row (collapsed, its
	// summary carrying the newest line) and the running tool row.
	if !tr.HasText("思考过程") {
		t.Fatalf("the live timeline should list the thinking row; texts: %v", tr.Texts())
	}
	if !tr.HasText("read_file") {
		t.Fatalf("the live timeline should list the tool row; texts: %v", tr.Texts())
	}
	if !tr.HasText("最新一行") {
		t.Fatalf("the live summary should carry the newest line; texts: %v", tr.Texts())
	}
	if tr.HasText("边想边说") {
		t.Fatalf("the live thinking body should stay folded; texts: %v", tr.Texts())
	}

	// The stream ends: the process folds to its counts, leaving no trace
	// rows on screen.
	cw.endStream("a1")
	for range 3 {
		tr.Frame()
	}
	if tr.HasText("思考过程") || tr.HasText("read_file") {
		t.Fatalf("the finished process should fold its rows away; texts: %v", tr.Texts())
	}
	if !tr.HasText("思考 1 次 · 工具调用 1 次") {
		t.Fatalf("the finished fold should show its counts; texts: %v", tr.Texts())
	}
}
