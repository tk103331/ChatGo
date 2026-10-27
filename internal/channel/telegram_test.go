package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A long poll delivers each text message once and advances the offset past
// it, so a restart does not replay old messages.
func TestTelegramPollsAndAdvancesOffset(t *testing.T) {
	var mu sync.Mutex
	offsets := []int{}
	updates := []string{
		`{"update_id":10,"message":{"message_id":1,"from":{"id":7},"chat":{"id":42},"text":" hello "}}`,
		`{"update_id":11,"message":{"message_id":2,"from":{"id":7},"chat":{"id":42},"text":""}}`,
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":99,"username":"chatgo_bot"}}`))
			return
		}
		var body struct {
			Offset int64 `json:"offset"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		offsets = append(offsets, int(body.Offset))
		n := calls
		calls++
		mu.Unlock()
		if n == 0 {
			_, _ = w.Write([]byte(`{"ok":true,"result":[` + strings.Join(updates, ",") + `]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	d := NewTelegram()
	got := make(chan Inbound, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx, Config{Type: "telegram", AppID: "tok", BaseURL: srv.URL}, func(in Inbound) { got <- in }); err != nil {
		t.Fatal(err)
	}

	select {
	case in := <-got:
		if in.Text != "hello" {
			t.Fatalf("text is %q, want trimmed %q", in.Text, "hello")
		}
		if in.ChatID != "42" || in.FromUser != "7" {
			t.Fatalf("chat=%q from=%q", in.ChatID, in.FromUser)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message delivered")
	}

	// The second poll must ask for update 12: both received updates are
	// acknowledged by the offset moving past them.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(offsets)
		last := 0
		if n > 0 {
			last = offsets[n-1]
		}
		mu.Unlock()
		if n >= 2 {
			if last != 12 {
				t.Fatalf("next offset is %d, want 12", last)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A missing token is reported before any network call.
func TestTelegramRequiresToken(t *testing.T) {
	err := NewTelegram().Start(context.Background(), Config{Type: "telegram"}, func(Inbound) {})
	if err == nil {
		t.Fatal("a driver without a token must refuse to start")
	}
}

// Telegram rejects a whole message when one entity is malformed, so a stray
// marker is escaped while real emphasis, code and links survive.
func TestTelegramMarkdownEscapesStrayMarkers(t *testing.T) {
	got := telegramMarkdown("a * b and **bold** and `code`")
	if strings.Contains(got, "a * b") {
		t.Fatalf("the stray asterisk was left unescaped: %q", got)
	}
	if !strings.Contains(got, "*bold*") {
		t.Fatalf("paired emphasis was escaped away: %q", got)
	}
	if !strings.Contains(got, "`code`") {
		t.Fatalf("inline code was escaped away: %q", got)
	}
	// A heading has no MarkdownV2 equivalent and is reduced to its text.
	if got := telegramMarkdown("# Title"); strings.Contains(got, "#") {
		t.Fatalf("a heading kept its marker: %q", got)
	}
}

// The registry knows every platform the app ships.
func TestDriverRegistryCoversPlatforms(t *testing.T) {
	for _, typ := range []string{"feishu", "wechat", "telegram", "dingtalk", "slack", "discord", "wecom"} {
		if NewDriver(typ) == nil {
			t.Errorf("no driver registered for %q", typ)
		}
	}
	if NewDriver("nope") != nil {
		t.Error("an unknown type must not build a driver")
	}
}

// A Chinese reply must survive the Markdown conversion byte for byte: the
// converter walks the text, and slicing it by byte instead of by rune would
// split every multi-byte character into mojibake.
func TestTelegramMarkdownKeepsNonASCII(t *testing.T) {
	cases := map[string]string{
		"你好世界":            "你好世界",
		"收到，我来看看。":        "收到，我来看看。",
		"**加粗中文** 和 `代码`": "*加粗中文* 和 `代码`",
		"中文里有 * 星号":       "中文里有 \\* 星号",
		"表情 😀 也要保留":       "表情 😀 也要保留",
	}
	for in, want := range cases {
		if got := telegramMarkdown(in); got != want {
			t.Errorf("telegramMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
}
