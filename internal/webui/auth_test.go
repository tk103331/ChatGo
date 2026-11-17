package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	a2aclient "chatgo/internal/a2a"
	"chatgo/internal/config"
)

func getWithHeaders(t *testing.T, ts *httptest.Server, path string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

func TestWebTokenProtectsAPI(t *testing.T) {
	const token = "test-token"
	s := protocolServerWithToken(t, &protocolRunner{}, token)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, data := getWithHeaders(t, ts, "/api/meta", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token status %d: %s", resp.StatusCode, data)
	}
	if !strings.Contains(string(data), "访问令牌") {
		t.Fatalf("no token body: %s", data)
	}
	if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Fatalf("WWW-Authenticate = %q", got)
	}

	resp, data = getWithHeaders(t, ts, "/api/meta", map[string]string{
		"Authorization": "Bearer " + token,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bearer status %d: %s", resp.StatusCode, data)
	}
	resp, data = getWithHeaders(t, ts, "/api/meta", map[string]string{
		webTokenHeader: token,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("header status %d: %s", resp.StatusCode, data)
	}
}

func TestWebTokenLoginFlow(t *testing.T) {
	const token = "test-token"
	s := protocolServerWithToken(t, &protocolRunner{}, token)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, data := getWithHeaders(t, ts, "/", nil)
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(data), `name="token"`) {
		t.Fatalf("login page status %d: %s", resp.StatusCode, data)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("login content type = %q", got)
	}
	if !strings.Contains(string(data), "width: 100%") {
		t.Fatalf("login page CSS was not formatted correctly: %s", data)
	}

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := noRedirect.PostForm(ts.URL+"/login", url.Values{
		"token": {token},
		"next":  {"/"},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login status %d: %s", resp.StatusCode, data)
	}
	cookies := resp.Cookies()
	if len(cookies) == 0 || cookies[0].Name != webTokenCookie || cookies[0].Value != token {
		t.Fatalf("login cookies = %#v", cookies)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/meta", nil)
	req.AddCookie(&http.Cookie{Name: webTokenCookie, Value: token})
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatalf("cookie request: %v", err)
	}
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cookie status %d: %s", resp.StatusCode, data)
	}

	resp, err = noRedirect.PostForm(ts.URL+"/login", url.Values{
		"token": {"wrong"},
		"next":  {"/"},
	})
	if err != nil {
		t.Fatalf("bad login: %v", err)
	}
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(data), "令牌不正确") {
		t.Fatalf("bad login status %d: %s", resp.StatusCode, data)
	}
}

func TestWebTokenQueryLoginAndAPIRejection(t *testing.T) {
	const token = "test-token"
	s := protocolServerWithToken(t, &protocolRunner{}, token)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := noRedirect.Get(ts.URL + "/?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatalf("query login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("query login status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if len(resp.Cookies()) == 0 || resp.Cookies()[0].Name != webTokenCookie {
		t.Fatalf("query login cookies = %#v", resp.Cookies())
	}

	resp, data := getWithHeaders(t, ts, "/api/meta?token="+url.QueryEscape(token), nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api query token status %d: %s", resp.StatusCode, data)
	}
}

func TestWebTokenProtectsMCP(t *testing.T) {
	const token = "test-token"
	s := protocolServerWithToken(t, &protocolRunner{}, token)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test","version":"1.0"},"capabilities":{}}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("mcp without token: %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mcp without token status %d: %s", resp.StatusCode, data)
	}

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatalf("mcp with token: %v", err)
	}
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(data), `"serverInfo"`) {
		t.Fatalf("mcp with token status %d: %s", resp.StatusCode, data)
	}
}

func TestWebTokenA2AClient(t *testing.T) {
	const token = "test-token"
	s := protocolServerWithToken(t, &protocolRunner{chunks: []string{"authorized"}}, token)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if _, err := a2aclient.New(t.Context(), config.A2AConnection{Name: "no-token", URL: ts.URL}); err == nil {
		t.Fatal("a2a card discovery without a token should fail")
	}
	cli, err := a2aclient.New(t.Context(), config.A2AConnection{
		Name: "local", URL: ts.URL, APIKey: token,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	res, err := cli.Chat(t.Context(), a2aclient.ChatRequest{Text: "你好"}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if res.Text != "authorized" {
		t.Fatalf("text = %q", res.Text)
	}
}

func TestWebTokenGeneratedWhenUnset(t *testing.T) {
	first := protocolServerWithToken(t, &protocolRunner{}, "")
	second := protocolServerWithToken(t, &protocolRunner{}, "")

	if first.Token() == "" || second.Token() == "" {
		t.Fatal("an unset token must still produce one")
	}
	if !first.TokenIsEphemeral() || !second.TokenIsEphemeral() {
		t.Fatal("a generated token must be marked ephemeral")
	}
	if first.Token() == second.Token() {
		t.Fatalf("two servers got the same generated token %q", first.Token())
	}

	ts := httptest.NewServer(first.Handler())
	defer ts.Close()

	resp, data := getWithHeaders(t, ts, "/api/meta", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("generated token did not protect the API: %d: %s", resp.StatusCode, data)
	}
	resp, data = getWithHeaders(t, ts, "/api/meta", map[string]string{
		"Authorization": "Bearer " + first.Token(),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generated token status %d: %s", resp.StatusCode, data)
	}
}

func TestConfiguredWebTokenIsLongLived(t *testing.T) {
	const token = "saved-token"
	s := protocolServerWithToken(t, &protocolRunner{}, token)
	if s.Token() != token {
		t.Fatalf("token = %q, want the configured value", s.Token())
	}
	if s.TokenIsEphemeral() {
		t.Fatal("a configured token must not be marked ephemeral")
	}
}
