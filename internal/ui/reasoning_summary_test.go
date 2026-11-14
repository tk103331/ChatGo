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
