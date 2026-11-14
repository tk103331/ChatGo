// The model list an ACP agent offers lives in its runtime: the CLI reports
// session config options of category "model" when a session starts. The
// agent picker shows that list, so ChatGo probes it with a throwaway session
// and caches the answer per connection.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	acp "github.com/coder/acp-go-sdk"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// acpModelOption is one model an ACP connection's runtime offers.
type acpModelOption struct {
	ID      string
	Name    string
	Current bool
}

// acpProbeTimeout bounds one probe; a CLI that cannot start in time answers
// with an error the picker shows instead of the list.
const acpProbeTimeout = 30 * time.Second

// acpProbeModels starts a throwaway session with the connection's CLI, reads
// the model options its config options expose, then kills the process group.
// Nothing here touches the app's state: the caller owns caching. The full
// config options travel back too, so the 新会话 page can offer them.
func acpProbeModels(cw *ChatWindow, conn config.ACPConnection) ([]acpModelOption, []acp.SessionConfigOption, error) {
	cwd := conn.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cmd := exec.Command(conn.Command, conn.Args...)
	cmd.Dir = cwd
	env := os.Environ()
	for k, v := range conn.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	cmd.SysProcAttr = acpSysProcAttr()
	var errBuf acpTailBuffer
	cmd.Stderr = &errBuf
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	// The waiter keeps the exit from becoming a zombie; the kill below ends
	// the whole group, npx wrappers included.
	go func() { _ = cmd.Wait() }()
	client := acp.NewClientSideConnection(acpClient{cw: cw}, stdin, stdout)
	fail := func(err error) ([]acpModelOption, []acp.SessionConfigOption, error) {
		acpKillProcessGroup(cmd.Process.Pid)
		_ = cmd.Process.Kill()
		if msg := errBuf.String(); msg != "" {
			return nil, nil, fmt.Errorf("%w: %s", err, acpFirstLine(msg))
		}
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), acpProbeTimeout)
	defer cancel()
	if _, err := client.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{},
		ClientInfo:         acpClientInfo,
	}); err != nil {
		return fail(err)
	}
	resp, err := client.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		return fail(err)
	}
	acpKillProcessGroup(cmd.Process.Pid)
	_ = cmd.Process.Kill()
	return acpModelsFromConfigs(resp.ConfigOptions), resp.ConfigOptions, nil
}

// acpModelsFromConfigs flattens the select options of the first config option
// categorised as a model; grouped option lists flatten in group order.
func acpModelsFromConfigs(configs []acp.SessionConfigOption) []acpModelOption {
	for _, opt := range configs {
		if opt.Select == nil || opt.Select.Category == nil {
			continue
		}
		if *opt.Select.Category != acp.SessionConfigOptionCategoryModel {
			continue
		}
		var raw []acp.SessionConfigSelectOption
		if ungrouped := opt.Select.Options.Ungrouped; ungrouped != nil {
			raw = *ungrouped
		}
		if grouped := opt.Select.Options.Grouped; grouped != nil {
			for _, group := range *grouped {
				raw = append(raw, group.Options...)
			}
		}
		if len(raw) == 0 {
			continue
		}
		out := make([]acpModelOption, 0, len(raw))
		for _, o := range raw {
			out = append(out, acpModelOption{
				ID: string(o.Value), Name: o.Name,
				Current: o.Value == opt.Select.CurrentValue,
			})
		}
		return out
	}
	return nil
}

// acpCacheProbe runs one probe and caches its outcome: the model list, the
// full config options, or the failure. It runs on its caller's goroutine.
func (cw *ChatWindow) acpCacheProbe(conn config.ACPConnection) {
	models, configs, err := acpProbeModels(cw, conn)
	cw.postMain(func() {
		cw.acp.mu.Lock()
		delete(cw.acp.modelsBusy, conn.Name)
		if err != nil {
			cw.acp.modelErrs[conn.Name] = acpFirstLine(err.Error())
		} else {
			cw.acp.models[conn.Name] = models
			cw.acp.configs[conn.Name] = configs
		}
		cw.acp.mu.Unlock()
		cw.invalidate()
	})
}

// acpStartupInit warms every enabled ACP connection once, one CLI at a time,
// so the 新会话 page can show model lists and config options right away
// instead of waiting on a per-connection probe.
func (cw *ChatWindow) acpStartupInit() {
	for i := range cw.cfg.ACPAgents {
		conn := cw.cfg.ACPAgents[i]
		if conn.Disabled {
			continue
		}
		cw.acp.mu.Lock()
		cached := cw.acp.models[conn.Name] != nil
		busy := cw.acp.modelsBusy[conn.Name]
		if !cached && !busy {
			cw.acp.modelsBusy[conn.Name] = true
		}
		cw.acp.mu.Unlock()
		if cached || busy {
			continue
		}
		cw.acpCacheProbe(conn)
	}
}

// acpWarmConnection re-probes one connection from scratch. Enabling an agent
// is an explicit ask, so a cached failure or stale list is refreshed rather
// than trusted; the probe runs in the background.
func (cw *ChatWindow) acpWarmConnection(name string) {
	cw.acp.mu.Lock()
	busy := cw.acp.modelsBusy[name]
	if !busy {
		cw.acp.modelsBusy[name] = true
		delete(cw.acp.modelErrs, name)
		delete(cw.acp.models, name)
		delete(cw.acp.configs, name)
	}
	cw.acp.mu.Unlock()
	if busy {
		return
	}
	conn := cw.acpConnectionOf("acp:" + name)
	if conn == nil {
		cw.acp.mu.Lock()
		delete(cw.acp.modelsBusy, name)
		cw.acp.mu.Unlock()
		return
	}
	go cw.acpCacheProbe(*conn)
}

// acpWarmRow shows the warm-up state of a connection in its settings form: a
// running probe, or the failure with a retry. A connection that never warmed
// up (disabled) stays quiet.
func (cw *ChatWindow) acpWarmRow(c *mgo.Context, conn config.ACPConnection) {
	if conn.Name == "" {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	errMsg := cw.acpModelError(conn.Name)
	busy := cw.acpModelsLoading(conn.Name)
	if !busy && errMsg == "" {
		return
	}
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		if busy {
			mgo.Icon(c, icons.Must("loader-circle")).Size(u*4, u*4).Shrink(0).
				TextColor(k.TextMuted)
			mgo.Text(c, "正在预热 "+conn.Name+"…").FontSize(caption).
				TextColor(k.TextMuted).Shrink(0)
			return
		}
		mgo.Icon(c, icons.Must("triangle-alert")).Size(u*4, u*4).Shrink(0).
			TextColor(k.Danger)
		mgo.Text(c, "预热失败："+errMsg).FontSize(caption).TextColor(k.Danger).
			Grow(1).MinWidth(0).MaxLines(2)
		if kit.Button(c, "重试", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
			cw.acpWarmConnection(conn.Name)
		}
	})
}

// acpConfigs returns the cached config options of a connection, nil before
// its first probe lands. The 新会话 page offers them as pickers.
func (cw *ChatWindow) acpConfigs(name string) []acp.SessionConfigOption {
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	return cw.acp.configs[name]
}

// acpModelError explains why a connection has no model list, empty when one
// is loading or loaded.
func (cw *ChatWindow) acpModelError(name string) string {
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	if _, busy := cw.acp.modelsBusy[name]; busy {
		return ""
	}
	return cw.acp.modelErrs[name]
}

// acpModelsLoading reports whether a probe is running.
func (cw *ChatWindow) acpModelsLoading(name string) bool {
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	_, busy := cw.acp.modelsBusy[name]
	return busy
}
