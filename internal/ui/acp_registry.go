package ui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"chatgo/internal/acpregistry"
	"chatgo/internal/config"
	"chatgo/internal/netx"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	mgo "github.com/egoist/mygo/ui"
)

// acpRegState is the Registry dialog's state: the fetched index, its search
// field, and the once-per-launch refresh marker shared with the catalog's.
type acpRegState struct {
	open bool
	// checked starts the once-per-launch background refresh on the first
	// open, so opening the dialog twice does not fetch twice.
	checked bool
	// loading marks a fetch in flight, named in the dialog body.
	loading bool
	// err is the last fetch failure, shown with the cache's contents.
	err string
	// agents is what the dialog lists: the fetched index, or the disk cache
	// until the network answers.
	agents []acpregistry.Agent
	query  string
	// installing is the registry ID of the binary install in flight, so the
	// row's 安装 button can spin and a second click cannot start another.
	installing string
}

// acpRegistryIndexTimeout bounds one index fetch.
const acpRegistryIndexTimeout = 30 * time.Second

// acpRegistryCacheFile is where the fetched index is cached; the next launch
// starts from it instead of waiting for the network.
func acpRegistryCacheFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "acp-registry.json"), nil
}

// maybeRefreshACPRegistry is the once-per-launch background index refresh,
// started when the dialog first opens: show the cache right away, then swap
// in the live index when it arrives. A failure keeps the cache silently —
// the dialog reports it instead of going blank.
func (cw *ChatWindow) maybeRefreshACPRegistry() {
	st := &cw.acpReg
	if st.checked {
		return
	}
	st.checked = true
	loadACPRegistryCache(st)
	if len(st.agents) > 0 {
		// The cache already lists everything; the refresh can improve on it
		// without the spinner.
		go cw.fetchACPRegistry(false)
		return
	}
	st.loading = true
	go cw.fetchACPRegistry(true)
}

// loadACPRegistryCache fills the state from the disk cache of a previous
// launch, whatever it holds.
func loadACPRegistryCache(st *acpRegState) {
	path, err := acpRegistryCacheFile()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if reg, err := acpregistry.Parse(strings.NewReader(string(data))); err == nil {
		st.agents = reg.Agents
	}
}

// fetchACPRegistry pulls the live index and swaps it in, caching it to disk.
// With report set, a failure surfaces in the dialog.
func (cw *ChatWindow) fetchACPRegistry(report bool) {
	ctx, cancel := context.WithTimeout(context.Background(), acpRegistryIndexTimeout)
	defer cancel()
	reg, err := acpregistry.Fetch(ctx)
	cw.postMain(func() {
		st := &cw.acpReg
		st.loading = false
		if err != nil {
			if report {
				st.err = err.Error()
			}
			cw.invalidate()
			return
		}
		st.err = ""
		st.agents = reg.Agents
		// The whole index document is cached, so reading it back goes
		// through the same Parse as a network fetch.
		if path, err := acpRegistryCacheFile(); err == nil {
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			if data, err := json.MarshalIndent(reg, "", "  "); err == nil {
				_ = os.WriteFile(path, append(data, '\n'), 0o644)
			}
		}
		cw.invalidate()
	})
}

// acpRegistryAdded reports whether one of the configured connections already
// runs the entry: the registry ID matches, or the connection's npx/uvx
// package is the entry's own, whatever version each side pinned.
func (cw *ChatWindow) acpRegistryAdded(a acpregistry.Agent) bool {
	want := acpregistry.PkgName(versionPkgOf(a))
	if want == "" {
		// A binary-only entry can still be recognised by its recorded ID.
		for _, c := range cw.cfg.ACPAgents {
			if c.RegistryID == a.ID {
				return true
			}
		}
		return false
	}
	for _, c := range cw.cfg.ACPAgents {
		if c.RegistryID == a.ID {
			return true
		}
		if pkg := acpRuntimePackage(c); pkg != "" &&
			acpregistry.PkgName(pkg) == want {
			return true
		}
	}
	return false
}

// acpRuntimePackage is the package spec a connection's command runs: the
// first non-flag argument of an npx or uvx invocation, "" for anything else.
func acpRuntimePackage(c config.ACPConnection) string {
	fields := strings.Fields(strings.TrimSpace(c.Command))
	if len(fields) == 0 {
		return ""
	}
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(fields[0])), ".exe") {
	case "npx", "npx.cmd":
		return acpNpxPackage(c.Args)
	case "uvx", "uvx.exe":
		return firstNonEmptyArg(c.Args)
	}
	return ""
}

// versionPkgOf is the entry's own package spec, whatever runtime it picked.
func versionPkgOf(a acpregistry.Agent) string {
	if d := a.Distribution.Npx; d != nil && strings.TrimSpace(d.Package) != "" {
		return d.Package
	}
	if d := a.Distribution.Uvx; d != nil && strings.TrimSpace(d.Package) != "" {
		return d.Package
	}
	return ""
}

// firstNonEmptyArg is acpNpxPackage without npx's flag filtering: uvx takes
// the package first, flags after.
func firstNonEmptyArg(args []string) string {
	for _, a := range args {
		if a = strings.TrimSpace(a); a != "" {
			return a
		}
	}
	return ""
}

// addACPRegistryAgent appends the entry as a new connection, enabled, with
// the registry's pinned package and environment, then saves. The entry's
// brand icon is fetched to disk in the background and attached to the
// connection, so it shows in the ACP list and the agent picker afterwards.
func (cw *ChatWindow) addACPRegistryAgent(a acpregistry.Agent) {
	command, args, env, ok := a.Command()
	if !ok {
		cw.toast("「"+a.Name+"」仅提供二进制发行，请使用安装按钮下载", mcore.SeverityWarning)
		return
	}
	cw.appendRegistryConnection(a, config.ACPConnection{
		Command: command, Args: args, Env: env,
	})
}

// installACPRegistryBinary downloads the entry's binary archive for this
// platform — verify its digest, extract, resolve the executable — and adds
// the result as a connection. The download runs off the UI thread; the row
// reports the run and the outcome.
func (cw *ChatWindow) installACPRegistryBinary(a acpregistry.Agent) {
	if cw.acpReg.installing != "" {
		return
	}
	_, _, ok := a.BinaryTarget(runtime.GOOS, runtime.GOARCH)
	if !ok {
		cw.toast("「"+a.Name+"」没有当前平台（"+runtime.GOOS+"/"+runtime.GOARCH+"）的二进制发行", mcore.SeverityWarning)
		return
	}
	cw.acpReg.installing = a.ID
	cw.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), acpBinaryInstallTimeout)
		defer cancel()
		dest := acpRegistryAgentDir(a.ID, a.Version)
		bin, err := acpregistry.InstallBinary(ctx, a, runtime.GOOS, runtime.GOARCH, dest)
		cw.postMain(func() {
			cw.acpReg.installing = ""
			if err != nil {
				cw.toastErrorDetail("安装「"+a.Name+"」失败", err.Error())
				cw.invalidate()
				return
			}
			dist, _, _ := a.BinaryTarget(runtime.GOOS, runtime.GOARCH)
			cw.appendRegistryConnection(a, config.ACPConnection{
				Command: bin, Args: dist.Args, Env: dist.Env,
			})
		})
	}()
}

// acpBinaryInstallTimeout bounds one binary install: the download of a large
// CLI can take a while on a slow link.
const acpBinaryInstallTimeout = 30 * time.Minute

// acpRegistryAgentDir is where an entry's binary version installs, one
// directory per registry id and version.
func acpRegistryAgentDir(id, version string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".chatgo", "agents", id, version)
}

// appendRegistryConnection adds the entry as a named, enabled connection
// with its registry ID recorded, then attaches its brand icon in the
// background and saves.
func (cw *ChatWindow) appendRegistryConnection(a acpregistry.Agent, conn config.ACPConnection) {
	name := uniqueACPName(cw, a.Name)
	conn.Name = name
	conn.RegistryID = a.ID
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, conn)
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已添加 "+a.Name+"（v"+a.Version+"）", mcore.SeveritySuccess)
	if a.Icon == "" {
		return
	}
	go func() {
		cw.fetchACPRegistryIcon(a)
		acpRegIconMu.Lock()
		_, loaded := acpRegIconCache[a.Icon]
		acpRegIconMu.Unlock()
		if !loaded {
			return // the download failed; the connection keeps the fallback mark
		}
		path := acpRegistryIconFile(a.ID)
		if path == "" {
			return
		}
		cw.postMain(func() {
			for i := range cw.cfg.ACPAgents {
				if cw.cfg.ACPAgents[i].Name == name {
					cw.cfg.ACPAgents[i].Icon = path
					break
				}
			}
			_ = config.SaveConfig(cw.cfg)
			cw.invalidate()
		})
	}()
}

// uniqueACPName appends a counter when the name is taken, so two entries of
// the same agent can coexist instead of colliding.
func uniqueACPName(cw *ChatWindow, name string) string {
	if !cw.hasACPConn(name) {
		return name
	}
	for i := 2; ; i++ {
		candidate := name + " " + itoa(i)
		if !cw.hasACPConn(candidate) {
			return candidate
		}
	}
}

// The registry's brand icons, fetched from its CDN once per launch and
// cached on disk so an offline launch still shows them.
var (
	acpRegIconMu    sync.Mutex
	acpRegIconCache = map[string]*mgo.SVG{} // icon URL → parsed SVG
	acpRegIconTried = map[string]bool{}     // URLs a fetch was started for
)

// acpRegIconTimeout bounds one icon download; icons are a few kilobytes.
const acpRegIconTimeout = 15 * time.Second

// acpRegIconMaxBytes bounds one icon file; the registry ships 16x16 SVGs.
const acpRegIconMaxBytes = 1 << 20

// acpRegistryIconFile is the disk cache path of one entry's icon, keyed by
// its registry id.
func acpRegistryIconFile(id string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".chatgo", "icons", "acp-registry", id+".svg")
}

// acpRegistryIcon draws the entry's brand icon at size, starting a fetch
// when it has none yet. False reports that nothing was drawn, so the caller
// falls back to the bundled marks.
func (cw *ChatWindow) acpRegistryIcon(c *mgo.Context, a acpregistry.Agent, size float32) bool {
	if a.Icon == "" {
		return false
	}
	acpRegIconMu.Lock()
	svg, loaded := acpRegIconCache[a.Icon]
	tried := acpRegIconTried[a.Icon]
	acpRegIconMu.Unlock()
	if loaded {
		mgo.Image(c, svg).Size(size, size).Shrink(0)
		return true
	}
	if !tried {
		acpRegIconMu.Lock()
		acpRegIconTried[a.Icon] = true
		acpRegIconMu.Unlock()
		go cw.fetchACPRegistryIcon(a)
	}
	return false
}

// fetchACPRegistryIcon loads one icon: the disk cache first, then the CDN.
// A failure is final for this launch; the rows fall back to the bundled
// marks rather than retrying on every frame.
func (cw *ChatWindow) fetchACPRegistryIcon(a acpregistry.Agent) {
	store := func(data []byte) {
		svg, err := mgo.ParseSVG(data)
		if err != nil {
			return
		}
		acpRegIconMu.Lock()
		acpRegIconCache[a.Icon] = svg
		acpRegIconMu.Unlock()
		cw.invalidate()
	}
	if path := acpRegistryIconFile(a.ID); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			store(data)
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), acpRegIconTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Icon, nil)
	if err != nil {
		return
	}
	resp, err := netx.Default().Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, acpRegIconMaxBytes))
	if err != nil || len(data) == 0 {
		return
	}
	if path := acpRegistryIconFile(a.ID); path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, data, 0o644)
	}
	store(data)
}

// acpRegistryDialog lists the official registry's agents: search above,
// entries under, each with its version, license and an 添加 action. A
// binary-only entry is named but not installable; an entry already running
// as a connection reads as added.
func (cw *ChatWindow) acpRegistryDialog(c *mgo.Context) {
	if !cw.acpReg.open {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	kit.Dialog(c, &cw.acpReg.open, kit.DialogOptions{
		Title: "从 ACP Registry 添加",
		Width: 680,
		// The description, the search field and the refresh action pin above
		// the scrolling list, so a long list never buries them.
		Header: func() {
			mgo.Column(c).FillWidth().MinWidth(0).Shrink(0).Gap(u).Children(func() {
				mgo.Text(c, "官方注册表收录的 ACP Agent（agentclientprotocol.com/registry），"+
					"添加后以 npx/uvx 运行固定版本。").
					FontSize(small).TextColor(k.TextMuted)
				mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
					in := kit.SearchInput(c, &cw.acpReg.query, kit.SearchInputOptions{
						Placeholder: "搜索 Agent", Label: "搜索 Agent",
					})
					in.Element.Grow(1).MinWidth(0)
					if kit.Button(c, "刷新", kit.ButtonOptions{
						Variant: kit.Secondary, Icon: icons.Must("refresh-cw"),
						Disabled: cw.acpReg.loading, Loading: cw.acpReg.loading,
					}).Clicked() {
						cw.acpReg.loading = true
						cw.acpReg.err = ""
						go cw.fetchACPRegistry(true)
					}
				})
				if cw.acpReg.err != "" {
					mgo.Text(c, cw.acpReg.err).FontSize(small).TextColor(k.Danger).MaxLines(2)
				}
			})
		},
		MaxHeight: 560,
		Actions: func() {
			if kit.Button(c, "关闭", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.acpReg.open = false
			}
		},
	}, func() {
		if cw.acpReg.loading && len(cw.acpReg.agents) == 0 {
			mgo.Column(c).FillWidth().AlignItems(mgo.Center).Gap(u).Padding(u * 4).Children(func() {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
					mgo.Text(c, "正在获取 ACP Registry…").FontSize(small).TextColor(k.TextMuted)
				})
			})
			return
		}
		entries := cw.acpRegistryMatches()
		if len(entries) == 0 {
			mgo.Column(c).FillWidth().AlignItems(mgo.Center).Gap(u).Padding(u * 3).Children(func() {
				mgo.Icon(c, icons.Must("search")).Size(28, 28).TextColor(k.TextMuted)
				if cw.acpReg.query != "" {
					mgo.Text(c, "没有匹配的 Agent").FontSize(small).TextColor(k.Text)
				} else {
					mgo.Text(c, "没有可添加的 Agent").FontSize(small).TextColor(k.Text)
					mgo.Text(c, "请检查网络（或在设置 → 系统配置置代理）后点击刷新。").
						FontSize(small).TextColor(k.TextMuted)
				}
			})
			return
		}
		mgo.Scroll(c).FillWidth().MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().MinWidth(0).Shrink(0).Gap(u).Children(func() {
				for _, a := range entries {
					cw.acpRegistryRow(c, a)
				}
			})
		})
	})
}

// acpRegistryMatches filters the index by the search field, over the name,
// the id and the description.
func (cw *ChatWindow) acpRegistryMatches() []acpregistry.Agent {
	q := strings.ToLower(strings.TrimSpace(cw.acpReg.query))
	var out []acpregistry.Agent
	for _, a := range cw.acpReg.agents {
		if q == "" ||
			strings.Contains(strings.ToLower(a.Name), q) ||
			strings.Contains(strings.ToLower(a.ID), q) ||
			strings.Contains(strings.ToLower(a.Description), q) {
			out = append(out, a)
		}
	}
	return out
}

// acpRegistryRow is one entry: a clickable header row — the chevron, the
// brand icon, the name with its version and license, a one-line description
// — with the action to its right, outside the header so the two clicks never
// fight. Expanding shows the entry's details: the full description, the
// authors, the links and the distribution it runs on.
func (cw *ChatWindow) acpRegistryRow(c *mgo.Context, a acpregistry.Agent) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	open := mgo.Local(c.Root(), "acpReg:"+a.ID, func() bool { return false })
	mgo.Column(c).FillWidth().MinWidth(0).Gap(u / 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			head := mgo.ButtonBase(c).Grow(1).MinWidth(0).Gap(u).AlignItems(mgo.Center).
				Padding(u/2, u/2).Radius(mtheme.ControlRadius).
				Role(mgo.RoleGroup).Label(a.Name + " 详情")
			if head.Hovered() {
				head.Background(k.SurfaceHover)
			}
			head.Children(func() {
				name := "chevron-right"
				if *open {
					name = "chevron-down"
				}
				mgo.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
				if !cw.acpRegistryIcon(c, a, 24) {
					cw.acpLeading(c, a.Name, 24)
				}
				mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
					mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
						mgo.Text(c, a.Name).FontWeight(600).SingleLine().Shrink(0)
						mgo.Text(c, "v"+a.Version).FontSize(small).TextColor(k.TextMuted).SingleLine().Shrink(0)
						if a.License != "" {
							mgo.Text(c, a.License).FontSize(small).TextColor(k.TextMuted).SingleLine().Shrink(0)
						}
					})
					if a.Description != "" {
						mgo.Text(c, a.Description).FontSize(small).TextColor(k.TextMuted).
							MaxLines(1).MinWidth(0)
					}
				})
			})
			if head.Clicked() {
				*open = !*open
			}
			mgo.Row(c).Shrink(0).Children(func() {
				switch {
				case cw.acpReg.installing == a.ID:
					mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Children(func() {
						mcore.Spinner(c, mcore.SpinnerOptions{Size: 10})
						mgo.Text(c, "安装中").FontSize(small).TextColor(k.TextMuted)
					})
				case cw.acpRegistryAdded(a):
					mgo.Text(c, "已添加").FontSize(small).TextColor(k.TextMuted)
				case a.SupportsRuntime():
					if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
						cw.addACPRegistryAgent(a)
					}
				case a.SupportsBinary(runtime.GOOS, runtime.GOARCH):
					if kit.Button(c, "安装", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
						cw.installACPRegistryBinary(a)
					}
				default:
					mgo.Text(c, "仅二进制发行").FontSize(small).TextColor(k.TextMuted).
						Tooltip("该 Agent 未提供 npx/uvx 发行方式，也没有当前平台的二进制包")
				}
			})
		})
		if *open {
			cw.acpRegistryDetail(c, a)
		}
	})
}

// acpRegistryDetail is the expanded body of one entry: the description
// rendered as rich text, the authors, the links and the distributions it
// ships, on a framed surface so the details read as a panel of their own.
func (cw *ChatWindow) acpRegistryDetail(c *mgo.Context, a acpregistry.Agent) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mono := mtheme.MonoFont
	mgo.Box(c).FillWidth().MinWidth(0).Shrink(0).
		Radius(mtheme.ControlRadius).Background(k.Surface).
		Border(mtheme.BorderWidth, k.Border).
		Padding(u * 2).Children(func() {
		mgo.Column(c).FillWidth().MinWidth(0).Shrink(0).Gap(u).Children(func() {
			if a.Description != "" {
				kit.MarkdownView(c, a.Description)
			}
			if len(a.Authors) > 0 {
				cw.acpRegistryField(c, "作者", strings.Join(a.Authors, " · "), "")
			}
			if a.License != "" {
				cw.acpRegistryField(c, "许可证", a.License, a.LicenseURL)
			}
			if a.Repository != "" {
				cw.acpRegistryField(c, "仓库", shortLink(a.Repository), a.Repository)
			}
			if a.Website != "" {
				cw.acpRegistryField(c, "官网", shortLink(a.Website), a.Website)
			}
			if d := a.Distribution.Npx; d != nil && strings.TrimSpace(d.Package) != "" {
				cw.acpRegistryCommand(c, "npx", d.Package, d.Args, mono, small)
			}
			if d := a.Distribution.Uvx; d != nil && strings.TrimSpace(d.Package) != "" {
				cw.acpRegistryCommand(c, "uvx", d.Package, d.Args, mono, small)
			}
			if platforms := a.BinaryPlatforms(); len(platforms) > 0 {
				mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, "二进制").FontSize(small).TextColor(k.TextMuted).Shrink(0)
					mgo.Text(c, strings.Join(platforms, " · ")).FontSize(small).TextColor(k.Text).
						Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					if !cw.acpRegistryAdded(a) && cw.acpReg.installing == "" &&
						a.SupportsBinary(runtime.GOOS, runtime.GOARCH) {
						if kit.Button(c, "安装二进制", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
							cw.installACPRegistryBinary(a)
						}
					}
				})
			}
			mgo.Text(c, "Registry ID："+a.ID).FontSize(small).TextColor(k.TextMuted)
		})
	})
}

// acpRegistryField is one label-and-value line of the details, with an
// 打开 action when the value names a URL.
func (cw *ChatWindow) acpRegistryField(c *mgo.Context, label, value, url string) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, label).FontSize(small).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, value).FontSize(small).TextColor(k.Text).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if url != "" {
			if kit.Button(c, "打开", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
				c.OpenURL(url)
			}
		}
	})
}

// acpRegistryCommand shows the exact command a runtime runs: the package
// with its pinned version and the entry's own arguments, in mono, with the
// environment it sets below.
func (cw *ChatWindow) acpRegistryCommand(c *mgo.Context, runtime, pkg string, args []string, mono string, small float32) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	cmd := runtime + " " + pkg
	if runtime == "npx" {
		cmd = runtime + " -y " + pkg
	}
	for _, arg := range args {
		cmd += " " + arg
	}
	mgo.Column(c).FillWidth().MinWidth(0).Gap(u / 2).Children(func() {
		mgo.Text(c, cmd).Font(mono).FontSize(small).TextColor(k.Text).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(cmd)
	})
}

// shortLink trims a URL to how it reads in the details: no scheme, no
// trailing slash.
func shortLink(url string) string {
	s := strings.TrimPrefix(url, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimSuffix(s, "/")
}
