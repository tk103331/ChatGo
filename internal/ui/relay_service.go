package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/hostinfo"
	"chatgo/internal/relay"
	"chatgo/internal/relayclient"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/internal/webui"
)

// relayServiceState owns the outbound relay link and the loopback webui the
// relay forwards into. It is independent of the optional browser Web service.
type relayServiceState struct {
	mu     sync.Mutex
	conn   *relayclient.Connector
	cancel context.CancelFunc
	srv    *webui.Server
	url    string
	err    string
}

type relayInfo struct {
	Enabled    bool
	URL        string
	DesktopID  string
	AllowPair  bool
	State      relayclient.State
	Err        string
	PairCode   string
	PairExpiry string
	Devices    []relayclient.Device
}

func (cw *ChatWindow) relayInfo() relayInfo {
	info := relayInfo{
		Enabled:   cw.cfg.Relay.Enabled,
		URL:       cw.cfg.Relay.URL,
		DesktopID: cw.cfg.Relay.DesktopID,
		AllowPair: cw.cfg.Relay.AllowPairing,
		State:     relayclient.StateDisconnected,
	}
	cw.relay.mu.Lock()
	conn := cw.relay.conn
	info.Err = cw.relay.err
	cw.relay.mu.Unlock()
	if conn != nil {
		info.State = conn.State()
		code, expires := conn.PairCode()
		info.PairCode = code
		if !expires.IsZero() {
			info.PairExpiry = expires.Format("15:04:05")
		}
		info.Devices = conn.Devices()
	}
	return info
}

// startRelayService connects the desktop to the configured relay. It is a
// no-op when relay is disabled or already running.
func (cw *ChatWindow) startRelayService() {
	if !cw.cfg.Relay.Enabled {
		return
	}
	cw.relay.mu.Lock()
	if cw.relay.conn != nil {
		cw.relay.mu.Unlock()
		return
	}
	cw.relay.err = ""
	cw.relay.mu.Unlock()
	cw.invalidate()

	if strings.TrimSpace(cw.cfg.Relay.URL) == "" {
		cw.setRelayError(fmt.Errorf("请填写中继地址，例如 wss://relay.example.com"))
		return
	}
	cw.ensureRelayIdentity()

	webDir, err := findWebAssets()
	if err != nil {
		cw.setRelayError(err)
		return
	}
	srv, err := webui.NewServerWithRunner(cw.cfg, cw.convManager, webDir, cw.WebRuntime())
	if err != nil {
		cw.setRelayError(err)
		return
	}
	addr, err := srv.Start("127.0.0.1:0")
	if err != nil {
		_ = srv.Close()
		cw.setRelayError(fmt.Errorf("启动中继内部服务失败: %w", err))
		return
	}

	conn := relayclient.New(relayclient.Config{
		RelayURL:     strings.TrimSpace(cw.cfg.Relay.URL),
		DesktopID:    cw.cfg.Relay.DesktopID,
		DesktopName:  hostinfo.Current().Name,
		Secret:       cw.cfg.Relay.DesktopSecret,
		LocalURL:     addr,
		LocalToken:   srv.Token(),
		AllowPairing: cw.cfg.Relay.AllowPairing,
		Capabilities: cw.WebRuntime().Capabilities(),
		OnState:      func(relayclient.State) { cw.postMain(cw.invalidate) },
		OnDevices: func(devices []relayclient.Device) {
			cw.postMain(func() {
				cw.saveRelayDevices(devices)
				cw.invalidate()
			})
		},
	})
	conn.SetDevices(relayDevices(cw.cfg.Relay.Devices))

	ctx, cancel := context.WithCancel(context.Background())
	cw.relay.mu.Lock()
	cw.relay.conn = conn
	cw.relay.cancel = cancel
	cw.relay.srv = srv
	cw.relay.url = addr
	cw.relay.mu.Unlock()
	cw.invalidate()
	go conn.Run(ctx)
}

func (cw *ChatWindow) stopRelayService() {
	cw.relay.mu.Lock()
	cancel := cw.relay.cancel
	srv := cw.relay.srv
	cw.relay.conn = nil
	cw.relay.cancel = nil
	cw.relay.srv = nil
	cw.relay.url = ""
	cw.relay.err = ""
	cw.relay.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if srv != nil {
		_ = srv.Close()
	}
	cw.invalidate()
}

func (cw *ChatWindow) restartRelayService() {
	cw.stopRelayService()
	cw.startRelayService()
}

func (cw *ChatWindow) setRelayEnabled(enabled bool) {
	cw.cfg.Relay.Enabled = enabled
	if enabled {
		cw.cfg.Relay.AllowPairing = true
		cw.ensureRelayIdentity()
	}
	_ = config.SaveConfig(cw.cfg)
	if enabled {
		cw.startRelayService()
	} else {
		cw.stopRelayService()
	}
	cw.invalidate()
}

func (cw *ChatWindow) setRelayURL(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == cw.cfg.Relay.URL {
		return
	}
	cw.cfg.Relay.URL = raw
	_ = config.SaveConfig(cw.cfg)
	if cw.cfg.Relay.Enabled {
		cw.restartRelayService()
	}
	cw.invalidate()
}

func (cw *ChatWindow) setRelayAllowPairing(allowed bool) {
	cw.cfg.Relay.AllowPairing = allowed
	_ = config.SaveConfig(cw.cfg)
	if cw.cfg.Relay.Enabled {
		cw.restartRelayService()
	}
	cw.invalidate()
}

func (cw *ChatWindow) newRelayPairCode() {
	info := cw.relayInfo()
	if info.State != relayclient.StateConnected {
		cw.toast("中继尚未连接，无法生成配对码", mcore.SeverityWarning)
		return
	}
	cw.relay.mu.Lock()
	conn := cw.relay.conn
	cw.relay.mu.Unlock()
	if conn == nil {
		return
	}
	conn.NewPairCode(0)
	cw.toast("配对码已生成，2 分钟内有效", mcore.SeveritySuccess)
	cw.invalidate()
}

// setRelayDeviceDisabled pauses a paired phone without forgetting it: the
// relay refuses its token until the switch is turned back on.
func (cw *ChatWindow) setRelayDeviceDisabled(id string, disabled bool) {
	cw.relay.mu.Lock()
	conn := cw.relay.conn
	cw.relay.mu.Unlock()
	if conn == nil {
		cw.toastErrorDetail("修改设备状态失败", "中继未连接")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := conn.SetDeviceDisabled(ctx, id, disabled); err != nil {
			cw.postMain(func() { cw.toastErrorDetail("修改设备状态失败", err.Error()) })
			return
		}
		message := "设备已启用"
		if disabled {
			message = "设备已停用"
		}
		cw.postMain(func() {
			cw.toast(message, mcore.SeveritySuccess)
			cw.invalidate()
		})
	}()
}

func (cw *ChatWindow) revokeRelayDevice(id string) {
	cw.relay.mu.Lock()
	conn := cw.relay.conn
	cw.relay.mu.Unlock()
	if conn == nil {
		return
	}
	go func() {
		if err := conn.Revoke(context.Background(), id); err != nil {
			cw.postMain(func() { cw.toastErrorDetail("吊销设备失败", err.Error()) })
			return
		}
		cw.postMain(func() {
			cw.toast("设备已吊销", mcore.SeveritySuccess)
			cw.invalidate()
		})
	}()
}

func (cw *ChatWindow) ensureRelayIdentity() {
	changed := false
	if strings.TrimSpace(cw.cfg.Relay.DesktopID) == "" {
		cw.cfg.Relay.DesktopID = relay.NewID("d_")
		changed = true
	}
	if strings.TrimSpace(cw.cfg.Relay.DesktopSecret) == "" {
		cw.cfg.Relay.DesktopSecret = relay.NewToken()
		changed = true
	}
	if changed {
		_ = config.SaveConfig(cw.cfg)
	}
}

func (cw *ChatWindow) saveRelayDevices(devices []relayclient.Device) {
	out := make([]config.RelayDevice, 0, len(devices))
	for _, d := range devices {
		entry := config.RelayDevice{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  d.Platform,
			CreatedAt: d.CreatedAt,
			Disabled:  d.Disabled,
		}
		if len(d.E2EKey) > 0 {
			entry.E2EKey = base64.StdEncoding.EncodeToString(d.E2EKey)
		}
		out = append(out, entry)
	}
	cw.cfg.Relay.Devices = out
	_ = config.SaveConfig(cw.cfg)
}

func relayDevices(in []config.RelayDevice) []relayclient.Device {
	out := make([]relayclient.Device, 0, len(in))
	for _, d := range in {
		device := relayclient.Device{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  d.Platform,
			CreatedAt: d.CreatedAt,
			Disabled:  d.Disabled,
		}
		if d.E2EKey != "" {
			if key, err := base64.StdEncoding.DecodeString(d.E2EKey); err == nil {
				device.E2EKey = key
			}
		}
		out = append(out, device)
	}
	return out
}

// relayNotify sends a metadata-only notification to the paired phones. It is
// safe to call from any goroutine; a disconnected relay drops it.
func (cw *ChatWindow) relayNotify(n relay.Notification) {
	if !cw.cfg.Relay.Enabled {
		return
	}
	cw.relay.mu.Lock()
	conn := cw.relay.conn
	cw.relay.mu.Unlock()
	if conn == nil {
		return
	}
	n.DesktopID = cw.cfg.Relay.DesktopID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := conn.Notify(ctx, n); err != nil {
			cw.relay.mu.Lock()
			cw.relay.err = "推送失败: " + err.Error()
			cw.relay.mu.Unlock()
		}
	}()
}

func (cw *ChatWindow) setRelayError(err error) {
	cw.relay.mu.Lock()
	cw.relay.err = err.Error()
	cw.relay.mu.Unlock()
	cw.invalidate()
}
