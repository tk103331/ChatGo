package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A platform without a reaction API is simply skipped, so the reply path never
// has to know which platforms can acknowledge a message.
func TestReactorIsOptional(t *testing.T) {
	var plain Driver = NewDingTalk()
	if _, ok := plain.(Reactor); ok {
		t.Fatal("DingTalk has no reaction API and must not claim one")
	}
	var withReact Driver = NewFeishu()
	if _, ok := withReact.(Reactor); !ok {
		t.Fatal("Feishu must implement Reactor")
	}
}

// reactQuietly never propagates a failure: acknowledging a message must not be
// able to break the reply.
func TestReactQuietlySwallowsErrors(t *testing.T) {
	d := &failingReactor{}
	reactQuietly(context.Background(), d, Inbound{ReplyToken: "x"}, ReactionWorking)
	if !d.called {
		t.Fatal("the reactor was not called")
	}
}

type failingReactor struct{ called bool }

func (f *failingReactor) Type() string { return "failing" }
func (f *failingReactor) Start(context.Context, Config, func(Inbound)) error {
	return nil
}
func (f *failingReactor) Send(context.Context, string, string) error { return nil }
func (f *failingReactor) React(context.Context, Inbound, Reaction) error {
	f.called = true
	return context.Canceled
}

// Each platform maps the two states to its own emoji vocabulary.
func TestReactionEmojiPerPlatform(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"feishu working", feishuReactionEmoji(ReactionWorking), "Typing"},
		{"feishu done", feishuReactionEmoji(ReactionDone), "DONE"},
		{"slack working", slackReactionEmoji(ReactionWorking), "eyes"},
		{"slack done", slackReactionEmoji(ReactionDone), "white_check_mark"},
		{"discord working", discordReactionEmoji(ReactionWorking), "👀"},
		{"discord done", discordReactionEmoji(ReactionDone), "✅"},
		{"telegram working", telegramReactionEmoji(ReactionWorking), "👀"},
		{"telegram done", telegramReactionEmoji(ReactionDone), "👍"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: emoji is %q, want %q", c.name, c.got, c.want)
		}
	}
}

// Telegram reacts through setMessageReaction, addressing the chat and the
// message the reply token names.
func TestTelegramReacts(t *testing.T) {
	var method string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = strings.TrimPrefix(r.URL.Path, "/bottok/")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	d := NewTelegram()
	d.token = "tok"
	d.baseURL = srv.URL
	if err := d.React(context.Background(), Inbound{ChatID: "42", ReplyToken: "7"}, ReactionDone); err != nil {
		t.Fatal(err)
	}
	if method != "setMessageReaction" {
		t.Fatalf("called %q", method)
	}
	if body["chat_id"] != float64(42) || body["message_id"] != float64(7) {
		t.Fatalf("body addressed %v", body)
	}
}
