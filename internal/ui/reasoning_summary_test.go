package ui

import (
	"strings"
	"testing"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	"chatgo/pkg/models"
)

// The fold's summary is the thinking's newest non-empty line, kept short so
// the title stays one line.
func TestReasoningSummaryIsNewestLine(t *testing.T) {
	if got := reasoningSummary(""); got != "" {
		t.Fatalf("empty thinking has no summary, got %q", got)
	}
	if got := reasoningSummary("先看看这段代码\n\n  然后决定下一步  \n"); got != "然后决定下一步" {
		t.Fatalf("the newest non-empty line should be the summary, got %q", got)
	}
	long := strings.Repeat("长", 60)
	if got := reasoningSummary(long); len([]rune(got)) != 49 || !strings.HasSuffix(got, "…") {
		t.Fatalf("a long line should be truncated to 48 runes plus the mark, got %d runes", len([]rune(got)))
	}
}

// reasoningDurationFixture is a chat window whose conversation holds one
// assistant reply with segments, so a thinking block can be timed.
func reasoningDurationFixture(t *testing.T, turn models.Turn, segs []models.Segment) *ChatWindow {
	t.Helper()
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t",
		Turns:    []models.Turn{turn},
		Messages: []models.Message{{ID: "a1", Role: "assistant", Segments: segs}},
	}
	cw.convs = []models.Conversation{*cw.cur}
	return cw
}

// A thinking block runs from its own start to the next segment's, to the
// turn's end, or to now while it still streams; segments from before start
// times existed show no time rather than a made-up one.
func TestReasoningDurationTimesTheBlock(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)

	// The next segment's start bounds the thinking.
	cw := reasoningDurationFixture(t,
		models.Turn{ReplyMsgID: "a1"},
		[]models.Segment{
			{Kind: "reasoning", At: start},
			{Kind: "text", At: start.Add(4 * time.Second)},
		})
	got := cw.reasoningDuration(cw.cur.Messages[0], segmentRound{segments: cw.cur.Messages[0].Segments}, 0, false)
	if want := kit.AgentDuration(4 * time.Second); got != want {
		t.Fatalf("the block between two segments should read %q, got %q", want, got)
	}

	// The message's last block ends with the turn.
	cw = reasoningDurationFixture(t,
		models.Turn{ReplyMsgID: "a1", StartedAt: start, EndedAt: start.Add(7 * time.Second)},
		[]models.Segment{
			{Kind: "reasoning", At: start.Add(5 * time.Second)},
		})
	got = cw.reasoningDuration(cw.cur.Messages[0], segmentRound{segments: cw.cur.Messages[0].Segments}, 0, false)
	if want := kit.AgentDuration(2 * time.Second); got != want {
		t.Fatalf("the trailing block should run to the turn's end (%q), got %q", want, got)
	}

	// While it streams the clock is still running, so the time is present.
	cw = reasoningDurationFixture(t,
		models.Turn{ReplyMsgID: "a1", StartedAt: start},
		[]models.Segment{{Kind: "reasoning", At: time.Now().Add(-1 * time.Second)}})
	if got := cw.reasoningDuration(cw.cur.Messages[0], segmentRound{segments: cw.cur.Messages[0].Segments}, 0, true); got == "" {
		t.Fatal("a live block should show the time elapsed so far")
	}

	// A segment without a start time shows none.
	cw = reasoningDurationFixture(t,
		models.Turn{ReplyMsgID: "a1"},
		[]models.Segment{
			{Kind: "reasoning", Text: "old thinking"},
			{Kind: "text", Text: "答案"},
		})
	if got := cw.reasoningDuration(cw.cur.Messages[0], segmentRound{segments: cw.cur.Messages[0].Segments}, 0, false); got != "" {
		t.Fatalf("a segment from before start times should show no time, got %q", got)
	}
}

// A completed thinking block folds itself away — the fold shows its summary
// and time without the body — while the block being written stays expanded.
func TestReasoningBlockCollapsesWhenDone(t *testing.T) {
	start := time.Now().Add(-5 * time.Second)
	cw := reasoningDurationFixture(t,
		models.Turn{ID: "t1", ReplyMsgID: "a1", StartedAt: start, EndedAt: start.Add(5 * time.Second)},
		[]models.Segment{
			{Kind: "reasoning", Text: "先分析问题\n决定直接回答", At: start},
			{Kind: "text", Text: "这是答案", At: start.Add(5 * time.Second)},
		})
	cw.focusConvTab(cw.cur)

	tr := mgo.NewTester(cw.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	if err := tr.Click("执行过程"); err != nil {
		t.Fatalf("open the process fold: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	// Completed: the title with its summary shows, but the thinking's body
	// does not.
	for _, want := range []string{"思考过程", "决定直接回答", "5.0 s"} {
		if !tr.HasText(want) {
			t.Fatalf("the folded title should carry %q; texts: %v", want, tr.Texts())
		}
	}
	if tr.HasText("先分析问题") {
		t.Fatalf("a completed thinking block should be collapsed; texts: %v", tr.Texts())
	}

	// Streaming: the block being written stays open so the thinking can be
	// followed live.
	streaming := reasoningDurationFixture(t,
		models.Turn{ID: "t1", ReplyMsgID: "a1", StartedAt: time.Now().Add(-2 * time.Second)},
		[]models.Segment{{Kind: "reasoning", Text: "正在推理的第一行", At: time.Now().Add(-2 * time.Second)}})
	streaming.beginStream("c1", "a1", nil)
	streaming.focusConvTab(streaming.cur)
	tr = mgo.NewTester(streaming.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	if !tr.HasText("正在推理的第一行") {
		t.Fatalf("the streaming thinking block should be open; texts: %v", tr.Texts())
	}
	// The stream ends and the block folds itself; the round's own fold
	// closes with it, so only the outer title remains.
	streaming.endStream("a1")
	for range 3 {
		tr.Frame()
	}
	if tr.HasText("正在推理的第一行") {
		t.Fatalf("the finished thinking block should have folded; texts: %v", tr.Texts())
	}
	if tr.HasText("思考过程") {
		t.Fatalf("the closed round should not render its inner fold; texts: %v", tr.Texts())
	}
	if !tr.HasText("执行过程") {
		t.Fatalf("the round fold's title should remain; texts: %v", tr.Texts())
	}
}

// The chat page shows a thinking block's summary and its time beside the
// fold's title, so the fold answers "what did it think" without opening.
func TestReasoningBlockShowsSummaryAndTime(t *testing.T) {
	start := time.Now().Add(-5 * time.Second)
	cw := reasoningDurationFixture(t,
		models.Turn{ID: "t1", ReplyMsgID: "a1", StartedAt: start, EndedAt: start.Add(5 * time.Second)},
		[]models.Segment{
			{Kind: "reasoning", Text: "先看看这段代码\n最后决定用简单方案", At: start},
			{Kind: "text", Text: "这是答案", At: start.Add(5 * time.Second)},
		})
	cw.focusConvTab(cw.cur)

	tr := mgo.NewTester(cw.view, 1100, 760)
	for range 5 {
		tr.Frame()
	}
	// The process fold is closed by default; open it to reach the thinking.
	if err := tr.Click("执行过程"); err != nil {
		t.Fatalf("open the process fold: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("思考过程") {
		t.Fatalf("the fold's title is missing; texts: %v", tr.Texts())
	}
	if !tr.HasText("最后决定用简单方案") {
		t.Fatalf("the summary line is missing; texts: %v", tr.Texts())
	}
	if !tr.HasText("5.0 s") {
		t.Fatalf("the thinking time is missing; texts: %v", tr.Texts())
	}
}
