package ui

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatgo/internal/channel"
	"chatgo/internal/config"
	"chatgo/internal/plugin"
	mcore "chatgo/internal/ui/uikit"
)

// Plugin channels are registered under "plugin:<pluginId>:<channelType>" so a
// private IM type cannot collide with a built-in platform.
const pluginChannelPrefix = "plugin:"

var (
	pluginChannelMu      sync.RWMutex
	pluginChannelSpecs   = map[string]plugin.ChannelContribution{}
	pluginChannelHost    *ChatWindow
	pluginChannelWebhook struct {
		mu       sync.Mutex
		server   *http.Server
		addr     string
		handlers map[string]http.HandlerFunc
	}
)

// pluginChannelKey is the channel type stored in config for a plugin channel.
func pluginChannelKey(pluginID, typ string) string {
	return pluginChannelPrefix + pluginID + ":" + typ
}

// parsePluginChannelKey splits a stored plugin channel type.
func parsePluginChannelKey(key string) (pluginID, typ string, ok bool) {
	rest, found := strings.CutPrefix(key, pluginChannelPrefix)
	if !found {
		return "", "", false
	}
	i := strings.Index(rest, ":")
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// syncPluginChannelTypes publishes the enabled plugins' channel types so the
// channel registry, the settings form and the driver factory see them. It must
// run after any plugin load, unload or reload.
func (cw *ChatWindow) syncPluginChannelTypes() {
	pluginChannelHost = cw
	next := map[string]plugin.ChannelContribution{}
	if cw.plugins != nil {
		for _, c := range cw.plugins.Channels() {
			key := pluginChannelKey(c.PluginID, c.Spec.Type)
			next[key] = c
			if channel.NewDriver(key) == nil {
				channel.RegisterDriver(key, func() channel.Driver {
					return &pluginChannelDriver{typ: key}
				})
			}
		}
	}
	pluginChannelMu.Lock()
	pluginChannelSpecs = next
	pluginChannelMu.Unlock()
}

// pluginChannelSpec looks up a registered plugin channel type.
func pluginChannelSpec(key string) (plugin.ChannelContribution, bool) {
	pluginChannelMu.RLock()
	defer pluginChannelMu.RUnlock()
	c, ok := pluginChannelSpecs[key]
	return c, ok
}

// pluginChannelDriver implements channel.Driver over a plugin's verify/send
// hooks. Receiving uses a loopback webhook; poll mode is not wired yet.
type pluginChannelDriver struct {
	typ string

	mu    sync.Mutex
	cfg   channel.Config
	state channel.State
	note  string
	hits  []time.Time
}

func (d *pluginChannelDriver) Type() string { return d.typ }

func (d *pluginChannelDriver) setState(s channel.State, note string) {
	d.mu.Lock()
	d.state, d.note = s, note
	d.mu.Unlock()
}

func (d *pluginChannelDriver) Status() (channel.State, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state, d.note
}

func (d *pluginChannelDriver) Start(ctx context.Context, cfg channel.Config, onMessage func(channel.Inbound)) error {
	pid, typ, ok := parsePluginChannelKey(d.typ)
	if !ok {
		return fmt.Errorf("插件渠道类型非法: %s", d.typ)
	}
	spec, ok := pluginChannelSpec(d.typ)
	if !ok {
		return fmt.Errorf("插件渠道未注册: %s", d.typ)
	}
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
	if spec.Spec.Mode == "poll" || spec.Spec.Poll != "" {
		if spec.Spec.Poll == "" {
			err := fmt.Errorf("插件渠道缺少 poll")
			d.setState(channel.StateError, err.Error())
			return err
		}
		interval := time.Duration(spec.Spec.PollSeconds) * time.Second
		if interval <= 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(cfg.Option("poll_seconds"))); err == nil {
				interval = time.Duration(n) * time.Second
			}
		}
		if interval < 2*time.Second {
			interval = 10 * time.Second
		}
		if interval > 300*time.Second {
			interval = 300 * time.Second
		}
		d.setState(channel.StateConnected, "轮询每 "+interval.String())
		go d.pollLoop(ctx, pid, typ, interval, onMessage)
		return nil
	}
	if spec.Spec.Verify == "" {
		return fmt.Errorf("插件渠道缺少 verify，不能使用 webhook 模式")
	}

	if err := ensurePluginChannelServer(); err != nil {
		d.setState(channel.StateError, err.Error())
		return err
	}
	token := strings.TrimSpace(cfg.Option("webhook_token"))
	if token == "" {
		token = strings.TrimSpace(cfg.AppSecret)
	}
	if token == "" {
		err := fmt.Errorf("缺少 webhook_token")
		d.setState(channel.StateError, err.Error())
		return err
	}
	registerPluginChannelHandler(cfg.ID, func(w http.ResponseWriter, r *http.Request) {
		d.handleWebhook(w, r, pid, typ, spec, token, onMessage)
	})
	d.setState(channel.StateConnected, pluginChannelWebhookURL(cfg.ID, token))
	// The webhook lives until the process ends; the context only marks the
	// channel as no longer accepting new messages.
	go func() {
		<-ctx.Done()
		d.setState(channel.StateStopped, "")
	}()
	return nil
}

func (d *pluginChannelDriver) Send(ctx context.Context, target, markdown string) error {
	pid, typ, ok := parsePluginChannelKey(d.typ)
	if !ok || pluginChannelHost == nil {
		return fmt.Errorf("插件渠道未初始化")
	}
	d.mu.Lock()
	cfg := d.cfg
	d.mu.Unlock()
	if err := pluginChannelHost.sendPluginChannelWithCreds(pid, typ, channelCreds(cfg), target, markdown); err != nil {
		d.setState(channel.StateError, err.Error())
		return err
	}
	return nil
}

// pollLoop asks the plugin for new messages on an interval until the channel
// stops. A failing poll keeps the previous note instead of disabling the
// channel, so a temporary upstream error recovers by itself.
func (d *pluginChannelDriver) pollLoop(ctx context.Context, pluginID, typ string, interval time.Duration, onMessage func(channel.Inbound)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.setState(channel.StateStopped, "")
			return
		case <-ticker.C:
			d.mu.Lock()
			creds := channelCreds(d.cfg)
			d.mu.Unlock()
			callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			msgs, err := pluginChannelHost.plugins.PollChannel(callCtx, pluginID, typ, creds)
			cancel()
			if err != nil {
				d.setState(channel.StateError, err.Error())
				continue
			}
			d.setState(channel.StateConnected, "")
			for _, msg := range msgs {
				onMessage(channel.Inbound{
					ChatID: msg.ChatID, FromUser: msg.FromUser,
					Text: msg.Text, ReplyToken: msg.ReplyToken,
				})
			}
		}
	}
}

// hit applies the per-channel inbound rate limit (60 per minute).
func (d *pluginChannelDriver) hit(now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	cutoff := now.Add(-time.Minute)
	kept := d.hits[:0]
	for _, t := range d.hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	d.hits = kept
	if len(d.hits) >= 60 {
		return false
	}
	d.hits = append(d.hits, now)
	return true
}

func (d *pluginChannelDriver) handleWebhook(w http.ResponseWriter, r *http.Request, pluginID, typ string, spec plugin.ChannelContribution, token string, onMessage func(channel.Inbound)) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	got := strings.TrimSpace(r.URL.Query().Get("token"))
	if got == "" {
		got = strings.TrimSpace(r.Header.Get("X-Chatgo-Token"))
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if pluginChannelHost == nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	d.mu.Lock()
	cfg := d.cfg
	d.mu.Unlock()
	if ch := pluginChannelHost.channelByID(cfg.ID); ch == nil || !ch.Enabled {
		http.Error(w, "channel disabled", http.StatusServiceUnavailable)
		return
	}
	if !d.hit(time.Now()) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	headers := map[string]string{}
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	in, err := pluginChannelHost.plugins.VerifyChannel(ctx, pluginID, typ, channelCreds(cfg), plugin.ChannelRequest{
		Method: r.Method, Path: r.URL.Path, Headers: headers, Body: string(body),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}
	onMessage(channel.Inbound{
		ChatID: in.ChatID, FromUser: in.FromUser, Text: in.Text, ReplyToken: in.ReplyToken,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ensurePluginChannelServer starts the loopback webhook listener once.
func ensurePluginChannelServer() error {
	pluginChannelWebhook.mu.Lock()
	defer pluginChannelWebhook.mu.Unlock()
	if pluginChannelWebhook.server != nil {
		return nil
	}
	if pluginChannelWebhook.handlers == nil {
		pluginChannelWebhook.handlers = map[string]http.HandlerFunc{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/chatgo/plugin-channel/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/chatgo/plugin-channel/")
		id = strings.Trim(id, "/")
		pluginChannelWebhook.mu.Lock()
		h := pluginChannelWebhook.handlers[id]
		pluginChannelWebhook.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("启动插件渠道监听失败: %w", err)
	}
	pluginChannelWebhook.addr = ln.Addr().String()
	pluginChannelWebhook.server = &http.Server{Handler: mux}
	go func(s *http.Server) {
		_ = s.Serve(ln)
	}(pluginChannelWebhook.server)
	return nil
}

func registerPluginChannelHandler(channelID string, h http.HandlerFunc) {
	pluginChannelWebhook.mu.Lock()
	defer pluginChannelWebhook.mu.Unlock()
	if pluginChannelWebhook.handlers == nil {
		pluginChannelWebhook.handlers = map[string]http.HandlerFunc{}
	}
	pluginChannelWebhook.handlers[channelID] = h
}

// pluginChannelWebhookURL is the URL an external platform should POST to.
func pluginChannelWebhookURL(channelID, token string) string {
	pluginChannelWebhook.mu.Lock()
	addr := pluginChannelWebhook.addr
	pluginChannelWebhook.mu.Unlock()
	if addr == "" {
		return ""
	}
	return "http://" + addr + "/chatgo/plugin-channel/" + channelID + "?token=" + token
}

// channelCreds flattens a running channel's credentials for a plugin hook.
func channelCreds(cfg channel.Config) map[string]string {
	out := map[string]string{
		"app_id": cfg.AppID, "app_secret": cfg.AppSecret, "base_url": cfg.BaseURL,
	}
	for k, v := range cfg.Options {
		out[k] = v
	}
	return out
}

// channelCredsFromConfig is channelCreds over the stored channel.
func channelCredsFromConfig(ch *config.Channel) map[string]string {
	out := map[string]string{
		"app_id": ch.AppID, "app_secret": ch.AppSecret, "base_url": ch.BaseURL,
	}
	for k, v := range ch.Options {
		out[k] = v
	}
	return out
}

// sendPluginChannelWithCreds delivers text through one plugin channel.
func (cw *ChatWindow) sendPluginChannelWithCreds(pluginID, typ string, creds map[string]string, target, text string) error {
	if cw.plugins == nil {
		return fmt.Errorf("插件系统未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return cw.plugins.SendChannel(ctx, pluginID, typ, creds, target, text)
}

// sendPluginChannel delivers text through a saved plugin channel.
func (cw *ChatWindow) sendPluginChannel(ch *config.Channel, target, text string) error {
	pid, typ, ok := parsePluginChannelKey(ch.Type)
	if !ok {
		return fmt.Errorf("不是插件渠道: %s", ch.Type)
	}
	return cw.sendPluginChannelWithCreds(pid, typ, channelCredsFromConfig(ch), target, text)
}

// addPluginChannel creates a disabled channel for one plugin type, with a
// webhook token, and opens its settings.
func (cw *ChatWindow) addPluginChannel(c plugin.ChannelContribution) {
	token := cw.newID()
	ch := config.Channel{
		ID: cw.newID(), Type: pluginChannelType(c), Name: c.Spec.Name,
		Enabled: false, Options: map[string]string{"webhook_token": token},
	}
	config.EnsureChannelTools(&ch)
	cw.cfg.Channels = append(cw.cfg.Channels, ch)
	_ = config.SaveConfig(cw.cfg)
	cw.settings.tab = 3
	cw.section = sectionSettings
	cw.toast("已添加渠道 "+c.Spec.Name+"，请填写凭证并启用", mcore.SeveritySuccess)
	cw.invalidate()
}

// pluginChannelType is the stored type of one plugin channel.
func pluginChannelType(c plugin.ChannelContribution) string {
	return pluginChannelKey(c.PluginID, c.Spec.Type)
}
