package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"chatgo/internal/config"
	"chatgo/internal/webui"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

const defaultWebAddr = "127.0.0.1:8422"

type webServiceState struct {
	mu  sync.Mutex
	srv *webui.Server
	url string
	err string
}

type webStatusInfo struct {
	Label    string
	Address  string
	Err      string
	Running  bool
	Disabled bool
}

// startWebService serves the browser UI from the same process as the desktop
// app, so the agent and conversations stay local. Its status is drawn in the
// app title bar and can be restarted from Settings → Web.
func (cw *ChatWindow) startWebService() {
	if !cw.cfg.WebEnabled {
		cw.invalidate()
		return
	}
	cw.web.mu.Lock()
	if cw.web.srv != nil {
		cw.web.mu.Unlock()
		return
	}
	cw.web.err = ""
	cw.web.mu.Unlock()
	cw.invalidate()

	webDir, err := findWebAssets()
	if err != nil {
		cw.setWebServiceError(err)
		return
	}
	srv, err := webui.NewServerWithRunner(cw.cfg, cw.convManager, webDir, cw.WebRuntime())
	if err != nil {
		cw.setWebServiceError(err)
		return
	}
	addr, err := srv.Start(defaultWebAddr)
	if err != nil {
		addr, err = srv.Start("127.0.0.1:0")
	}
	if err != nil {
		_ = srv.Close()
		cw.setWebServiceError(fmt.Errorf("启动 Web 服务失败: %w", err))
		return
	}

	cw.web.mu.Lock()
	cw.web.srv = srv
	cw.web.url = addr
	cw.web.err = ""
	cw.web.mu.Unlock()
	cw.invalidate()
}

// stopWebService shuts the server down when the app quits or when Settings
// asks for a restart.
func (cw *ChatWindow) stopWebService() {
	cw.web.mu.Lock()
	srv := cw.web.srv
	cw.web.srv = nil
	cw.web.url = ""
	cw.web.err = ""
	cw.web.mu.Unlock()

	if srv != nil {
		_ = srv.Close()
	}
}

func (cw *ChatWindow) restartWebService() {
	if !cw.cfg.WebEnabled {
		return
	}
	cw.stopWebService()
	cw.startWebService()
}

// setWebEnabled persists the startup preference and starts or stops the
// service immediately, so the switch in Settings is the whole control.
func (cw *ChatWindow) setWebEnabled(enabled bool) {
	cw.cfg.WebEnabled = enabled
	_ = config.SaveConfig(cw.cfg)
	if enabled {
		cw.startWebService()
	} else {
		cw.stopWebService()
	}
	cw.invalidate()
}

func (cw *ChatWindow) setWebServiceError(err error) {
	cw.web.mu.Lock()
	cw.web.err = err.Error()
	cw.web.url = ""
	cw.web.mu.Unlock()
	cw.invalidate()
}

func (cw *ChatWindow) webInfo() webStatusInfo {
	if !cw.cfg.WebEnabled {
		return webStatusInfo{Label: "Web 已禁用", Disabled: true}
	}
	cw.web.mu.Lock()
	defer cw.web.mu.Unlock()
	info := webStatusInfo{Address: cw.web.url, Err: cw.web.err}
	switch {
	case cw.web.err != "":
		info.Label = "Web 启动失败"
	case cw.web.url != "":
		info.Label = "Web 运行中"
		info.Running = true
	default:
		info.Label = "Web 启动中"
	}
	return info
}

// webNavItem is the left-rail status icon, placed above the theme switch.
// Its tooltip names the address; clicking opens the browser, or Settings → Web
// when the service is not running.
func (cw *ChatWindow) webNavItem(c *mgo.Context) {
	k := mcore.Tokens(c)
	info := cw.webInfo()
	color := k.TextMuted
	switch {
	case info.Running:
		color = k.Success
	case info.Err != "":
		color = k.Danger
	}
	tip := info.Label
	if info.Address != "" {
		tip += "\n" + info.Address
	} else if info.Err != "" {
		tip += "\n" + info.Err
	}
	b := mgo.ButtonBase(c).Size(navItemSize, navItemSize).Shrink(0).
		Radius(mtheme.ControlRadius).Label(tip).TextColor(k.TextMuted)
	if b.Hovered() {
		b.Background(k.SurfaceHover).TextColor(k.Text)
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must("globe")).Size(20, 20)
		mgo.Box(c).Absolute().Right(8).Bottom(8).Size(7, 7).Radius(3.5).
			Background(color).Border(mtheme.BorderWidth, k.Surface)
	})
	mcore.TooltipEnd(c, b, tip)
	if b.Clicked() {
		cw.openWebService()
	}
}

func (cw *ChatWindow) openWebService() {
	info := cw.webInfo()
	if info.Address == "" {
		cw.showMainWindow()
		cw.settings.tab = settingsWebTab
		cw.goConfigure(sectionSettings)
		if info.Err != "" {
			cw.toastErrorDetail("Web 服务不可用", info.Err)
		}
		return
	}
	if err := mygo.Shell.OpenExternal(info.Address); err != nil {
		cw.toastErrorDetail("打开 Web 界面失败", err.Error())
	}
}

// findWebAssets looks for the wasm client in the packaged resources first,
// then in the working directory for `go run` and development builds.
func findWebAssets() (string, error) {
	var candidates []string
	if dir, err := mygo.App.Path(mygo.PathResources); err == nil {
		candidates = append(candidates, filepath.Join(dir, "web"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "web"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web"))
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "app.wasm")); err != nil {
				return "", fmt.Errorf("缺少 %s，请先运行 ./web/build.sh", filepath.Join(dir, "app.wasm"))
			}
			return dir, nil
		}
	}
	return "", fmt.Errorf("未找到 web/index.html（已查找 %s）", strings.Join(candidates, "、"))
}
