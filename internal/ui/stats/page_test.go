package stats

import (
	"testing"
	"time"

	"chatgo/pkg/models"
)

func TestComputeAggregates(t *testing.T) {
	p := New()
	now := time.Now()
	convs := []models.Conversation{
		{ID: "c1", Title: "A", CreatedAt: now,
			Messages: []models.Message{
				{ID: "u1", Role: "user", Timestamp: now},
				{ID: "a1", Role: "assistant", Timestamp: now},
				{ID: "u2", Role: "user", Timestamp: now},
				{ID: "a2", Role: "assistant", Timestamp: now},
			},
			Turns: []models.Turn{
				{ID: "t1", UserMsgID: "u1", ReplyMsgID: "a1", Model: "gpt-4", Input: 10, Output: 5, Total: 15, StartedAt: now},
				{ID: "t2", UserMsgID: "u2", ReplyMsgID: "a2", Model: "gpt-4", Input: 20, Output: 5, Total: 25, StartedAt: now},
			}},
		{ID: "t1", Title: "Task", CreatedAt: now, TaskID: "x",
			Messages: []models.Message{{ID: "a3", Role: "assistant", Timestamp: now}},
			Turns:    []models.Turn{{ID: "t3", UserMsgID: "u3", ReplyMsgID: "a3", Model: "gpt-4", Input: 1, Output: 1, Total: 2, StartedAt: now}}},
	}
	s := p.compute(convs)
	if s.sessions != 2 || s.messages != 5 || s.total != 42 || s.input != 31 || s.output != 11 {
		t.Fatalf("aggregate %+v", s)
	}
	if len(s.models) != 1 || s.models[0].name != "gpt-4" || s.models[0].value != 42 {
		t.Fatalf("models %+v", s.models)
	}
	if n := len(s.days); n != Range {
		t.Fatalf("days %d", n)
	}
	last := s.days[len(s.days)-1]
	if last.sessions != 2 || last.total != 42 {
		t.Fatalf("today bucket %+v", last)
	}
}

func TestComputeCountsModelsPerTurn(t *testing.T) {
	p := New()
	now := time.Now()
	convs := []models.Conversation{
		{ID: "c1", Title: "a", CreatedAt: now, Messages: []models.Message{
			{ID: "u1", Role: "user", Timestamp: now},
			{ID: "a1", Role: "assistant", Timestamp: now},
			{ID: "u2", Role: "user", Timestamp: now},
			{ID: "a2", Role: "assistant", Timestamp: now},
		}, Turns: []models.Turn{
			{ID: "t1", UserMsgID: "u1", ReplyMsgID: "a1", Model: "gpt-4", Total: 100, StartedAt: now},
			{ID: "t2", UserMsgID: "u2", ReplyMsgID: "a2", Model: "gpt-4o", Total: 300, StartedAt: now},
		}},
	}
	s := p.compute(convs)
	if len(s.models) != 2 {
		t.Fatalf("models %+v", s.models)
	}
	for _, m := range s.models {
		want := 100
		if m.name == "gpt-4o" {
			want = 300
		}
		if m.value != want {
			t.Fatalf("model %s = %d, want %d", m.name, m.value, want)
		}
	}
}
