package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

var tokenStore = mygo.Secrets.Service("chatgo.mobile")

func tokenKey(desktopID string) string { return "token-" + desktopID }

func loadToken(desktopID string) string {
	if desktopID == "" {
		return ""
	}
	if data, err := tokenStore.Get(tokenKey(desktopID)); err == nil {
		return string(data)
	}
	return loadTokenFile(desktopID)
}

func saveToken(desktopID, token string) {
	if desktopID == "" || token == "" {
		return
	}
	if err := tokenStore.Set(tokenKey(desktopID), []byte(token)); err == nil {
		return
	}
	saveTokenFile(desktopID, token)
}

func deleteToken(desktopID string) {
	if desktopID == "" {
		return
	}
	_ = tokenStore.Delete(tokenKey(desktopID))
	fallback := tokenFile()
	data, err := os.ReadFile(fallback)
	if err != nil {
		return
	}
	var tokens map[string]string
	if json.Unmarshal(data, &tokens) != nil {
		return
	}
	delete(tokens, desktopID)
	data, _ = json.Marshal(tokens)
	_ = os.WriteFile(fallback, data, 0o600)
}

func tokenFile() string {
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && dir != "" {
		return filepath.Join(dir, "tokens.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".chatgo", "mobile", "tokens.json")
}

func loadTokenFile(desktopID string) string {
	data, err := os.ReadFile(tokenFile())
	if err != nil {
		return ""
	}
	var tokens map[string]string
	if json.Unmarshal(data, &tokens) != nil {
		return ""
	}
	return tokens[desktopID]
}

func saveTokenFile(desktopID, token string) {
	path := tokenFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	var tokens map[string]string
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &tokens)
	}
	if tokens == nil {
		tokens = map[string]string{}
	}
	tokens[desktopID] = token
	data, _ := json.Marshal(tokens)
	_ = os.WriteFile(path, data, 0o600)
}

type deepLink struct {
	kind    string
	relay   string
	desktop string
	code    string
	session string
	room    string
	task    string
}

func parseDeepLink(raw string) (deepLink, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return deepLink{}, err
	}
	if u.Scheme == "https" || u.Scheme == "http" {
		if strings.TrimSuffix(u.Path, "/") != "/pair" {
			return deepLink{}, os.ErrNotExist
		}
		q := u.Query()
		if u.Fragment != "" {
			if f, err := url.ParseQuery(u.Fragment); err == nil {
				for k, v := range f {
					q[k] = v
				}
			}
		}
		return deepLink{
			kind:    "pair",
			relay:   q.Get("r"),
			desktop: q.Get("d"),
			code:    q.Get("c"),
		}, nil
	}
	kind := u.Host
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	id := ""
	if len(parts) > 0 {
		id = parts[0]
	}
	switch kind {
	case "pair":
		q := u.Query()
		return deepLink{
			kind:    "pair",
			relay:   q.Get("r"),
			desktop: q.Get("d"),
			code:    q.Get("c"),
		}, nil
	case "session":
		return deepLink{kind: "session", session: id}, nil
	case "room":
		return deepLink{kind: "room", room: id}, nil
	case "task":
		return deepLink{kind: "task", task: id}, nil
	}
	return deepLink{}, os.ErrNotExist
}

func e2eKeyName(desktopID string) string { return "e2e-" + desktopID }

func loadE2EKey(desktopID string) []byte {
	if desktopID == "" {
		return nil
	}
	if data, err := tokenStore.Get(e2eKeyName(desktopID)); err == nil && len(data) > 0 {
		return data
	}
	data, err := os.ReadFile(keyFile())
	if err != nil {
		return nil
	}
	var keys map[string][]byte
	if json.Unmarshal(data, &keys) != nil {
		return nil
	}
	return keys[desktopID]
}

func saveE2EKey(desktopID string, key []byte) {
	if desktopID == "" || len(key) == 0 {
		return
	}
	if err := tokenStore.Set(e2eKeyName(desktopID), key); err == nil {
		return
	}
	path := keyFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	var keys map[string][]byte
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &keys)
	}
	if keys == nil {
		keys = map[string][]byte{}
	}
	keys[desktopID] = key
	data, _ := json.Marshal(keys)
	_ = os.WriteFile(path, data, 0o600)
}

func deleteE2EKey(desktopID string) {
	if desktopID == "" {
		return
	}
	_ = tokenStore.Delete(e2eKeyName(desktopID))
	path := keyFile()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var keys map[string][]byte
	if json.Unmarshal(data, &keys) != nil {
		return
	}
	delete(keys, desktopID)
	data, _ = json.Marshal(keys)
	_ = os.WriteFile(path, data, 0o600)
}

func keyFile() string {
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && dir != "" {
		return filepath.Join(dir, "e2e-keys.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".chatgo", "mobile", "e2e-keys.json")
}
