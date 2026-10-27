package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A group message keeps its mention in the text; the prompt must not.
func TestStripDingTalkMention(t *testing.T) {
	cases := map[string]string{
		"@ChatGo 帮我看下这个报错":      "帮我看下这个报错",
		"  @Bot   two spaces  ": "two spaces",
		"@Bot":                  "",
		"no mention":            "no mention",
	}
	for in, want := range cases {
		if got := stripDingTalkMention(in); got != want {
			t.Errorf("stripDingTalkMention(%q) = %q, want %q", in, got, want)
		}
	}
}

// The session webhook is the only way to answer a DingTalk conversation, so a
// reply posts markdown there and surfaces the platform's error code.
func TestDingTalkSendToWebhook(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	d := NewDingTalk()
	if err := d.SendToWebhook(context.Background(), srv.URL, "**hi**"); err != nil {
		t.Fatal(err)
	}
	if gotBody["msgtype"] != "markdown" {
		t.Fatalf("msgtype is %v", gotBody["msgtype"])
	}
	md, _ := gotBody["markdown"].(map[string]any)
	if md["text"] != "**hi**" {
		t.Fatalf("markdown text is %v", md["text"])
	}
}

// Without a webhook there is nothing to address, so the send is refused with
// an explanation rather than silently dropped.
func TestDingTalkSendNeedsWebhook(t *testing.T) {
	if err := NewDingTalk().Send(context.Background(), "conv", "hi"); err == nil {
		t.Fatal("a reply without a session webhook must fail")
	}
	if err := NewDingTalk().SendToWebhook(context.Background(), "", "hi"); err == nil {
		t.Fatal("an empty webhook must fail")
	}
}

// Missing credentials are refused before connecting.
func TestDingTalkRequiresCredentials(t *testing.T) {
	err := NewDingTalk().Start(context.Background(), Config{Type: "dingtalk"}, func(Inbound) {})
	if err == nil {
		t.Fatal("a driver without credentials must refuse to start")
	}
}
