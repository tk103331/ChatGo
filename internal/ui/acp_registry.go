package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgo/internal/acpregistry"
	"chatgo/internal/config"

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
// the registry's pinned package and environment, then saves.
func (cw *ChatWindow) addACPRegistryAgent(a acpregistry.Agent) {
	command, args, env, ok := a.Command()
	if !ok {
		cw.toast("「"+a.Name+"」仅提供二进制发行，暂不支持自动添加", mcore.SeverityWarning)
		return
	}
	name := uniqueACPName(cw, a.Name)
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, config.ACPConnection{
		Name: name, Command: command, Args: args, Env: env,
		RegistryID: a.ID,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已添加 "+a.Name+"（v"+a.Version+"）", mcore.SeveritySuccess)
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
		Description: "官方注册表收录的 ACP Agent（agentclientprotocol.com/registry），" +
			"添加后以 npx/uvx 运行固定版本。",
		Width:     680,
		MaxHeight: 560,
		Actions: func() {
			if kit.Button(c, "刷新", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("refresh-cw"), Disabled: cw.acpReg.loading,
				Loading: cw.acpReg.loading,
			}).Clicked() {
				cw.acpReg.loading = true
				cw.acpReg.err = ""
				go cw.fetchACPRegistry(true)
			}
			if kit.Button(c, "关闭", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.acpReg.open = false
			}
		},
	}, func() {
		mgo.Column(c).FillWidth().MinWidth(0).Gap(u * 2).Children(func() {
			in := kit.SearchInput(c, &cw.acpReg.query, kit.SearchInputOptions{
				Placeholder: "搜索 Agent", Label: "搜索 Agent",
			})
			in.Element.FillWidth()
			if cw.acpReg.err != "" {
				mgo.Text(c, cw.acpReg.err).FontSize(small).TextColor(k.Danger).MaxLines(2)
			}
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

// acpRegistryRow is one entry: the letter mark, the name with its version
// and license, a one-line description, and the action on the right —
// installable, installed, or binary-only.
func (cw *ChatWindow) acpRegistryRow(c *mgo.Context, a acpregistry.Agent) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		cw.acpLeading(c, a.Name, 24)
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
		mgo.Row(c).Shrink(0).Children(func() {
			switch {
			case cw.acpRegistryAdded(a):
				mgo.Text(c, "已添加").FontSize(small).TextColor(k.TextMuted)
			case a.SupportsRuntime():
				if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
					cw.addACPRegistryAgent(a)
				}
			default:
				mgo.Text(c, "仅二进制发行").FontSize(small).TextColor(k.TextMuted).
					Tooltip("该 Agent 未提供 npx/uvx 发行方式，暂不支持自动添加")
			}
		})
	})
}
