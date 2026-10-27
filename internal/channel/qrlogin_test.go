package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The WeCom flow asks for a code, then polls until the platform reports the
// robot it created, and hands back its BotID and secret.
func TestWeComQRLoginPollsUntilSuccess(t *testing.T) {
	var generateQuery url.Values
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ai/qc/generate"):
			generateQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"errcode":0,"data":{"scode":"s-1","auth_url":"https://work.weixin.qq.com/ai/qc/gen?scode=s-1"}}`))
		case strings.HasSuffix(r.URL.Path, "/ai/qc/query_result"):
			polls++
			if r.URL.Query().Get("scode") != "s-1" {
				t.Errorf("polled with scode %q", r.URL.Query().Get("scode"))
			}
			if polls < 2 {
				_, _ = w.Write([]byte(`{"errcode":0,"data":{"status":"waiting"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":0,"data":{"status":"success","bot_info":{"botid":"BOT","secret":"SEC"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	useTestWeComQR(t, srv.URL)

	d := NewWeCom()
	login, err := d.BeginQRLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if login.URL == "" || login.Tip == "" {
		t.Fatalf("login is incomplete: %+v", login)
	}
	if generateQuery.Get("source") == "" || generateQuery.Get("plat") == "" {
		t.Fatalf("generate was called with %v", generateQuery)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	creds, err := login.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if creds.AppID != "BOT" || creds.AppSecret != "SEC" {
		t.Fatalf("credentials are %+v", creds)
	}
}

// The DingTalk flow runs init → begin → poll and carries the brand notice,
// because the page the platform opens is not branded for this app.
func TestDingTalkQRLoginRunsDeviceFlow(t *testing.T) {
	var paths []string
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/init"):
			if body["source"] == "" {
				t.Error("init carried no source")
			}
			_, _ = w.Write([]byte(`{"errcode":0,"nonce":"n-1"}`))
		case strings.HasSuffix(r.URL.Path, "/begin"):
			if body["nonce"] != "n-1" {
				t.Errorf("begin carried nonce %q", body["nonce"])
			}
			_, _ = w.Write([]byte(`{"errcode":0,"device_code":"d-1","verification_uri_complete":"https://open-dev.dingtalk.com/x?user_code=1","expires_in":300,"interval":2}`))
		case strings.HasSuffix(r.URL.Path, "/poll"):
			polls++
			if body["device_code"] != "d-1" {
				t.Errorf("poll carried device code %q", body["device_code"])
			}
			if polls < 2 {
				_, _ = w.Write([]byte(`{"errcode":0,"status":"WAITING"}`))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":0,"status":"SUCCESS","client_id":"CID","client_secret":"CSEC"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	useTestDingTalkRegistration(t, srv.URL)

	d := NewDingTalk()
	login, err := d.BeginQRLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if login.Notice == "" {
		t.Fatal("the DingTalk flow must warn about the page's branding")
	}
	if !strings.Contains(login.Notice, "OpenClaw") {
		t.Fatalf("the notice does not name the brand the user will see: %q", login.Notice)
	}
	if !strings.Contains(login.URL, "open-dev.dingtalk.com") {
		t.Fatalf("the login URL is %q", login.URL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	creds, err := login.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if creds.AppID != "CID" || creds.AppSecret != "CSEC" {
		t.Fatalf("credentials are %+v", creds)
	}
	if len(paths) < 3 {
		t.Fatalf("the flow made %d requests, want init, begin and poll", len(paths))
	}
}

// A denied DingTalk authorization ends the wait with the platform's reason
// instead of polling until the timeout.
func TestDingTalkQRLoginReportsDenial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/init"):
			_, _ = w.Write([]byte(`{"errcode":0,"nonce":"n"}`))
		case strings.HasSuffix(r.URL.Path, "/begin"):
			_, _ = w.Write([]byte(`{"errcode":0,"device_code":"d","verification_uri_complete":"https://x","interval":2}`))
		default:
			_, _ = w.Write([]byte(`{"errcode":0,"status":"FAIL","fail_reason":"用户拒绝"}`))
		}
	}))
	defer srv.Close()
	useTestDingTalkRegistration(t, srv.URL)

	login, err := NewDingTalk().BeginQRLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = login.Wait(ctx)
	if err == nil || !strings.Contains(err.Error(), "用户拒绝") {
		t.Fatalf("error is %v, want the platform's reason", err)
	}
}

// Slack has no code to scan; its quick setup is a manifest link that pre-fills
// every setting the channel needs.
func TestSlackManifestURLPrefillsSettings(t *testing.T) {
	link, err := SlackManifestURL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "https://api.slack.com/apps?new_app=1&manifest_json=") {
		t.Fatalf("link is %q", link)
	}
	encoded := strings.TrimPrefix(link, "https://api.slack.com/apps?new_app=1&manifest_json=")
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal([]byte(decoded), &manifest); err != nil {
		t.Fatalf("the manifest is not valid JSON: %v", err)
	}
	settings := manifest["settings"].(map[string]any)
	if settings["socket_mode_enabled"] != true {
		t.Error("Socket Mode is not enabled in the manifest")
	}
	events := settings["event_subscriptions"].(map[string]any)["bot_events"].([]any)
	if len(events) != 2 {
		t.Errorf("subscribed events are %v", events)
	}
	scopes := manifest["oauth_config"].(map[string]any)["scopes"].(map[string]any)["bot"].([]any)
	want := map[string]bool{"chat:write": false, "app_mentions:read": false, "im:history": false, "im:read": false}
	for _, sc := range scopes {
		if _, ok := want[sc.(string)]; ok {
			want[sc.(string)] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("the manifest is missing scope %q", name)
		}
	}
}

// Only the platforms with a code flow claim one.
func TestQRLoginerIsPlatformSpecific(t *testing.T) {
	if _, ok := any(NewWeCom()).(QRLoginer); !ok {
		t.Error("WeCom must offer a QR login")
	}
	if _, ok := any(NewDingTalk()).(QRLoginer); !ok {
		t.Error("DingTalk must offer a QR login")
	}
	if _, ok := any(NewTelegram()).(QRLoginer); ok {
		t.Error("Telegram has no code flow")
	}
	if _, ok := any(NewDiscord()).(QRLoginer); ok {
		t.Error("Discord has no code flow")
	}
}
