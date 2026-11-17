package webui

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
)

const (
	webTokenCookie = "chatgo_web_token"
	webTokenHeader = "X-ChatGo-Token"
	webTokenMaxAge = 30 * 24 * 60 * 60
)

// authenticate protects the browser UI, its JSON API and the external agent
// endpoints. The server always has a token. Protocol clients send it as a
// Bearer header; the browser exchanges it for an HttpOnly cookie at /login.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/login" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions && isProtocolPath(path) {
			setProtocolCORS(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if s.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		if token := r.URL.Query().Get("token"); token != "" && s.tokenMatches(token) &&
			r.Method == http.MethodGet && !isAPIPath(path) && !isProtocolPath(path) {
			s.setTokenCookie(w, r)
			target := *r.URL
			query := target.Query()
			query.Del("token")
			target.RawQuery = query.Encode()
			redirect := target.Path
			if target.RawQuery != "" {
				redirect += "?" + target.RawQuery
			}
			http.Redirect(w, r, redirect, http.StatusFound)
			return
		}
		if isProtocolPath(path) {
			setProtocolCORS(w)
		}
		if isAPIPath(path) || isProtocolPath(path) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ChatGo"`)
			writeError(w, http.StatusUnauthorized, errors.New("需要访问令牌"))
			return
		}
		if path == "/favicon.ico" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.writeLoginPage(w, r.URL.Path, "", http.StatusUnauthorized)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if scheme, value, ok := strings.Cut(auth, " "); ok &&
			strings.EqualFold(scheme, "Bearer") && s.tokenMatches(strings.TrimSpace(value)) {
			return true
		}
	}
	if s.tokenMatches(strings.TrimSpace(r.Header.Get(webTokenHeader))) {
		return true
	}
	if cookie, err := r.Cookie(webTokenCookie); err == nil && s.tokenMatches(cookie.Value) {
		return true
	}
	return false
}

func (s *Server) tokenMatches(value string) bool {
	if s.token == "" || value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(s.token)) == 1
}

func (s *Server) setTokenCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     webTokenCookie,
		Value:    s.token,
		Path:     "/",
		MaxAge:   webTokenMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
	})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))
	if s.authorized(r) {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}
	s.writeLoginPage(w, next, "", http.StatusOK)
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		s.writeLoginPage(w, "/", "", http.StatusBadRequest)
		return
	}
	next := safeNext(r.PostFormValue("next"))
	if !s.tokenMatches(strings.TrimSpace(r.PostFormValue("token"))) {
		s.writeLoginPage(w, next, "令牌不正确", http.StatusUnauthorized)
		return
	}
	s.setTokenCookie(w, r)
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) writeLoginPage(w http.ResponseWriter, next, message string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	notice := ""
	if message != "" {
		notice = `<p class="error">` + html.EscapeString(message) + `</p>`
	}
	fmt.Fprintf(w, loginPageHTML, notice, html.EscapeString(safeNext(next)))
}

func safeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if next == "/login" || strings.HasPrefix(next, "/login?") {
		return "/"
	}
	return next
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func isProtocolPath(path string) bool {
	return path == "/mcp" || path == "/agui" || path == "/a2a" ||
		strings.HasPrefix(path, "/a2a/") || path == agentCardPath
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

const loginPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ChatGo · 访问令牌</title>
<style>
:root { color-scheme: light dark; }
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
       background: #f4f4f5; color: #18181b; font: 15px/1.5 system-ui, -apple-system, sans-serif; }
main { width: 420px; max-width: calc(100vw - 48px); padding: 32px; border-radius: 16px;
       background: #fff; box-shadow: 0 12px 40px rgba(0,0,0,.10); }
h1 { margin: 0 0 8px; font-size: 24px; }
.hint { margin: 0 0 20px; color: #71717a; }
.error { margin: 0 0 16px; padding: 10px 12px; border-radius: 8px; background: #fef2f2; color: #b91c1c; }
label { display: block; margin-bottom: 8px; font-size: 13px; color: #71717a; }
input[type=password] { box-sizing: border-box; width: 100%%; padding: 11px 12px; border-radius: 8px;
       border: 1px solid #d4d4d8; background: transparent; color: inherit; font: inherit; }
input[type=password]:focus { outline: 2px solid #2563eb; outline-offset: 1px; border-color: transparent; }
button { margin-top: 16px; width: 100%%; padding: 11px 12px; border: 0; border-radius: 8px;
       background: #18181b; color: #fff; font: inherit; font-weight: 600; cursor: pointer; }
@media (prefers-color-scheme: dark) {
  body { background: #18181b; color: #fafafa; }
  main { background: #27272a; box-shadow: none; }
  .hint, label { color: #a1a1aa; }
  input[type=password] { border-color: #52525b; }
  button { background: #fafafa; color: #18181b; }
}
</style>
</head>
<body>
<main>
<h1>ChatGo</h1>
<p class="hint">此 Web 服务始终要求访问令牌。请使用「设置 → Web → 当前有效令牌」，或在 webd 启动日志中查看。</p>
%s
<form method="post" action="/login">
<input type="hidden" name="next" value="%s">
<label for="token">访问令牌</label>
<input id="token" name="token" type="password" autocomplete="current-password" autofocus required>
<button type="submit">进入</button>
</form>
</main>
</body>
</html>
`
