package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Calcium-Ion/moejs"
)

// httpTimeout bounds one host HTTP request.
const httpTimeout = 20 * time.Second

// maxHTTPBody bounds a response body a plugin can read.
const maxHTTPBody = 1 << 20

// callContext is what the host injects into a hook's second argument.
type callContext struct {
	// Conversation is the id of the chat the call belongs to, when there is one.
	Conversation string
	// Extra carries per-hook fields such as a command's selection.
	Extra map[string]any
}

func logLine(id, level, msg string) {
	log.Printf("[plugin:%s] %s: %s", id, level, msg)
}

// ctxValue builds the ctx object a hook receives. Only granted capabilities
// are present: an unauthorised namespace does not exist, so calling it throws
// a TypeError in JavaScript.
func (in *InstanceState) ctxValue(c callContext) map[string]any {
	out := map[string]any{
		"plugin": map[string]any{
			"id": in.Manifest.ID, "name": in.Manifest.Name, "version": in.Manifest.Version,
		},
		"settings": in.settingsSnapshot(),
		"log": map[string]any{
			"debug": moejs.NativeFunc(in.logFn("debug")),
			"info":  moejs.NativeFunc(in.logFn("info")),
			"warn":  moejs.NativeFunc(in.logFn("warn")),
			"error": moejs.NativeFunc(in.logFn("error")),
		},
		"store": map[string]any{
			"get":    moejs.NativeFunc(in.storeGet),
			"set":    moejs.NativeFunc(in.storeSet),
			"delete": moejs.NativeFunc(in.storeDelete),
			"keys":   moejs.NativeFunc(in.storeKeys),
		},
	}
	if c.Conversation != "" {
		out["conversation"] = c.Conversation
	}
	if in.granted("http") {
		out["http"] = map[string]any{
			"get":     moejs.NativeFunc(in.httpFn(http.MethodGet)),
			"post":    moejs.NativeFunc(in.httpFn(http.MethodPost)),
			"put":     moejs.NativeFunc(in.httpFn(http.MethodPut)),
			"delete":  moejs.NativeFunc(in.httpFn(http.MethodDelete)),
			"request": moejs.NativeFunc(in.httpRequestFn),
		}
	}
	if in.granted("env") {
		out["env"] = map[string]any{"get": moejs.NativeFunc(in.envGet)}
	}
	for k, v := range c.Extra {
		out[k] = v
	}
	return out
}

// granted reports whether any permission of a family was granted.
func (in *InstanceState) granted(family string) bool {
	for _, p := range in.Ref.Permissions {
		if p == family || strings.HasPrefix(p, family+":") {
			return true
		}
	}
	return false
}

// grantHost reports whether the exact host was authorised for HTTP.
func (in *InstanceState) grantHost(host string) bool {
	for _, p := range in.Ref.Permissions {
		if p == "http:"+host {
			return true
		}
	}
	return false
}

func (in *InstanceState) settingsSnapshot() map[string]any {
	out := map[string]any{}
	for _, f := range in.Manifest.Settings {
		v := in.Ref.Settings[f.Key]
		if v == "" {
			v = f.Default
		}
		out[f.Key] = v
	}
	return out
}

func (in *InstanceState) logFn(level string) func(*moejs.Realm, moejs.Value, []moejs.Value) (moejs.Value, error) {
	return func(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		parts := make([]string, 0, len(args))
		for _, a := range args {
			if s, err := r.ToString(a); err == nil {
				parts = append(parts, s.GoString())
			}
		}
		logLine(in.Manifest.ID, level, strings.Join(parts, " "))
		return moejs.Undefined(), nil
	}
}

func (in *InstanceState) storeGet(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	key := argString(args, 0)
	in.storeMu.Lock()
	raw, ok := in.store[key]
	in.storeMu.Unlock()
	if !ok {
		return moejs.Undefined(), nil
	}
	// Copy through JSON so JavaScript never holds a map the host mutates.
	data, err := json.Marshal(raw)
	if err != nil {
		return moejs.Undefined(), err
	}
	var copied any
	if err := json.Unmarshal(data, &copied); err != nil {
		return moejs.Undefined(), err
	}
	return r.FromGo(copied)
}

func (in *InstanceState) storeSet(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	key := argString(args, 0)
	if key == "" {
		return moejs.Undefined(), fmt.Errorf("store.set 缺少 key")
	}
	var val any
	if i := 1; i < len(args) {
		v, err := r.ToGoStrict(args[i])
		if err != nil {
			return moejs.Undefined(), err
		}
		val = v
	}
	in.storeMu.Lock()
	in.store[key] = val
	snapshot := cloneMap(in.store)
	in.storeMu.Unlock()
	if err := in.saveStore(snapshot); err != nil {
		return moejs.Undefined(), err
	}
	return moejs.Undefined(), nil
}

func (in *InstanceState) storeDelete(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	key := argString(args, 0)
	in.storeMu.Lock()
	delete(in.store, key)
	snapshot := cloneMap(in.store)
	in.storeMu.Unlock()
	_ = in.saveStore(snapshot)
	return moejs.Undefined(), nil
}

func (in *InstanceState) storeKeys(r *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
	in.storeMu.Lock()
	keys := make([]string, 0, len(in.store))
	for k := range in.store {
		keys = append(keys, k)
	}
	in.storeMu.Unlock()
	sort.Strings(keys)
	return r.FromGo(keys)
}

func (in *InstanceState) envGet(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	key := argString(args, 0)
	if !in.Ref.PermissionGranted("env:" + key) {
		return moejs.Undefined(), fmt.Errorf("未授权读取环境变量 %s", key)
	}
	return moejs.String(os.Getenv(key)), nil
}

func (in *InstanceState) httpFn(method string) func(*moejs.Realm, moejs.Value, []moejs.Value) (moejs.Value, error) {
	return func(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		rawURL := argString(args, 0)
		opts, _ := argMap(r, args, 1)
		headers := map[string]string{}
		if h, ok := opts["headers"].(map[string]any); ok {
			for k, v := range h {
				headers[k] = fmt.Sprint(v)
			}
		}
		body := ""
		if b, ok := opts["body"].(string); ok {
			body = b
		}
		if raw, ok := opts["json"]; ok {
			data, err := json.Marshal(raw)
			if err != nil {
				return moejs.Undefined(), err
			}
			body = string(data)
			if _, ok := headers["Content-Type"]; !ok {
				headers["Content-Type"] = "application/json"
			}
		}
		res, err := in.doHTTP(method, rawURL, headers, body)
		if err != nil {
			return moejs.Undefined(), err
		}
		return r.FromGo(res)
	}
}

func (in *InstanceState) httpRequestFn(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
	method := strings.ToUpper(argString(args, 0))
	rawURL := argString(args, 1)
	opts, _ := argMap(r, args, 2)
	headers := map[string]string{}
	if h, ok := opts["headers"].(map[string]any); ok {
		for k, v := range h {
			headers[k] = fmt.Sprint(v)
		}
	}
	body, _ := opts["body"].(string)
	res, err := in.doHTTP(method, rawURL, headers, body)
	if err != nil {
		return moejs.Undefined(), err
	}
	return r.FromGo(res)
}

// doHTTP performs an authorised request and returns a JSON-friendly result.
func (in *InstanceState) doHTTP(method, rawURL string, headers map[string]string, body string) (map[string]any, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("URL 非法: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("只允许 http/https 请求")
	}
	if !in.grantHost(u.Host) {
		return nil, fmt.Errorf("未授权访问 %s（需在插件权限中声明 http:%s）", u.Host, u.Host)
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			if !in.grantHost(r.URL.Host) {
				return fmt.Errorf("重定向目标 %s 未授权", r.URL.Host)
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBody))
	out := map[string]any{
		"status": resp.StatusCode,
		"body":   string(data),
	}
	hdrs := map[string]string{}
	for k := range resp.Header {
		hdrs[k] = resp.Header.Get(k)
	}
	out["headers"] = hdrs
	var parsed any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err == nil {
		out["json"] = parsed
	}
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet(string(data)))
	}
	return out, nil
}

func argString(args []moejs.Value, i int) string {
	if i >= len(args) || !args[i].IsString() {
		return ""
	}
	return args[i].AsString().GoString()
}

func argMap(r *moejs.Realm, args []moejs.Value, i int) (map[string]any, error) {
	if i >= len(args) || !args[i].IsObject() {
		return map[string]any{}, nil
	}
	v, err := r.ToGoStrict(args[i])
	if err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func snippet(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// loadStore reads the plugin's state file into memory.
func (in *InstanceState) loadStore() {
	in.store = map[string]any{}
	data, err := os.ReadFile(filepath.Join(in.DataDir, "state.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &in.store)
}

// saveStore writes the state file atomically.
func (in *InstanceState) saveStore(snapshot map[string]any) error {
	if err := os.MkdirAll(in.DataDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(in.DataDir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(in.DataDir, "state.json"))
}
