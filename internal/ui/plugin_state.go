package ui

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/llm"
	"chatgo/internal/plugin"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// pluginUIState is the plugin settings page's own state plus the small caches
// the composer uses for plugin context providers.
type pluginUIState struct {
	selected    string
	installOpen bool
	installKind string
	installPath string
	installBusy bool
	installErr  string

	// filter is the category the list shows ("" = all), query the name
	// search, so the many built-in catalog plugins can be narrowed down.
	filter string
	query  string

	// installable holds the catalog entries not installed yet, its query
	// and the id of the entry a download is running for.
	installQuery    string
	installing      string
	catalogErr      string
	showInstallable bool

	settings    map[string]string
	settingsFor string
	list        mgo.ListState

	ctxMu      sync.Mutex
	ctxQuery   string
	ctxLoading bool
	ctxSeq     int
	ctxItems   []kit.ContextItem
	ctxTokens  map[string]pluginContextToken
	ctxCache   map[string]string
}

// pluginContextToken maps a visible mention token to the provider item it
// stands for.
type pluginContextToken struct {
	PluginID   string
	ProviderID string
	ItemID     string
}

// middlewares returns the plugin model middleware chain for client builders.
func (cw *ChatWindow) middlewares() []llm.Middleware {
	if cw.plugins == nil {
		return nil
	}
	return cw.plugins.Middlewares()
}

// toolResultHook lets plugins rewrite tool results before the model sees them.
func (cw *ChatWindow) toolResultHook() llm.ToolResultHook {
	if cw.plugins == nil {
		return nil
	}
	return func(ctx context.Context, name, arguments, result string) string {
		return cw.plugins.ApplyToolResult(ctx, name, arguments, result)
	}
}

// emit sends an event to subscribed plugins. It is a no-op without plugins.
func (cw *ChatWindow) emit(name string, payload map[string]any) {
	if cw.plugins != nil {
		cw.plugins.Emit(name, payload)
	}
}

// shorten trims a string for an event payload.
func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// pluginSlashCommands lists the enabled plugins' slash commands.
func (cw *ChatWindow) pluginSlashCommands() []kit.SlashCommand {
	if cw.plugins == nil {
		return nil
	}
	cmds := cw.plugins.Commands()
	out := make([]kit.SlashCommand, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, kit.SlashCommand{Name: c.Token, Description: c.Spec.Description})
	}
	return out
}

// findPluginCommand resolves a command token to its contribution.
func (cw *ChatWindow) findPluginCommand(token string) (plugin.CommandContribution, bool) {
	if cw.plugins == nil {
		return plugin.CommandContribution{}, false
	}
	for _, c := range cw.plugins.Commands() {
		if c.Token == token {
			return c, true
		}
	}
	return plugin.CommandContribution{}, false
}

// runPluginCommand executes a slash command and applies its result to the
// draft. It never sends automatically.
func (cw *ChatWindow) runPluginCommand(c plugin.CommandContribution, args string) {
	convID := ""
	if cw.cur != nil {
		convID = cw.cur.ID
	}
	selection := cw.draft
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := cw.plugins.RunCommand(ctx, c.PluginID, c.Spec.Name, args, selection, convID)
		cw.postMain(func() {
			if err != nil {
				cw.toastErrorDetail("命令执行失败", err.Error())
				return
			}
			switch {
			case res.Draft != "":
				cw.draft = res.Draft
			case res.Message != "":
				cw.draft = res.Message
			}
			cw.invalidate()
		})
	}()
}

// pluginContextItems triggers a provider search when the query changed and
// returns the results cached for the current query.
func (cw *ChatWindow) pluginContextItems(query string) ([]kit.ContextItem, bool) {
	st := &cw.pluginUI
	st.ctxMu.Lock()
	defer st.ctxMu.Unlock()
	if st.ctxQuery != query {
		st.ctxQuery = query
		st.ctxItems = nil
		st.ctxLoading = false
		st.ctxSeq++
		st.ctxTokens = map[string]pluginContextToken{}
		if cw.plugins != nil && strings.TrimSpace(query) != "" && len(cw.plugins.Contexts()) > 0 {
			st.ctxLoading = true
			seq := st.ctxSeq
			convID := ""
			if cw.cur != nil {
				convID = cw.cur.ID
			}
			go cw.searchPluginContext(query, convID, seq)
		}
	}
	return append([]kit.ContextItem(nil), st.ctxItems...), st.ctxLoading
}

// searchPluginContext queries every provider in parallel.
func (cw *ChatWindow) searchPluginContext(query, convID string, seq int) {
	type result struct {
		items []kit.ContextItem
		token map[string]pluginContextToken
	}
	ch := make(chan result, 1)
	ch <- func() result {
		var items []kit.ContextItem
		tokens := map[string]pluginContextToken{}
		for _, c := range cw.plugins.Contexts() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			found, err := cw.plugins.SearchContext(ctx, c.PluginID, c.Spec.ID, query, convID)
			cancel()
			if err != nil {
				continue
			}
			for _, it := range found {
				token := c.PluginID + "." + c.Spec.ID + "/" + sanitizeToken(it.ID)
				detail := it.Detail
				if detail == "" {
					detail = c.Spec.Label
				}
				items = append(items, kit.ContextItem{
					ID: token, Label: token, Detail: detail, Kind: kit.ContextDoc,
				})
				tokens[token] = pluginContextToken{PluginID: c.PluginID, ProviderID: c.Spec.ID, ItemID: it.ID}
			}
		}
		return result{items: items, token: tokens}
	}()
	res := <-ch
	st := &cw.pluginUI
	st.ctxMu.Lock()
	defer st.ctxMu.Unlock()
	if st.ctxSeq != seq {
		return
	}
	st.ctxItems = res.items
	if st.ctxTokens == nil {
		st.ctxTokens = map[string]pluginContextToken{}
	}
	for k, v := range res.token {
		st.ctxTokens[k] = v
	}
	st.ctxLoading = false
	cw.postMain(func() { cw.invalidate() })
}

func sanitizeToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "\n", "-")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// resolvePluginToken starts fetching a chosen context item's body so sending
// stays instant.
func (cw *ChatWindow) resolvePluginToken(token string) {
	st := &cw.pluginUI
	st.ctxMu.Lock()
	ref, ok := st.ctxTokens[token]
	st.ctxMu.Unlock()
	if !ok {
		return
	}
	convID := ""
	if cw.cur != nil {
		convID = cw.cur.ID
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		body, err := cw.plugins.ResolveContext(ctx, ref.PluginID, ref.ProviderID, ref.ItemID, convID)
		if err != nil {
			return
		}
		st.ctxMu.Lock()
		if st.ctxCache == nil {
			st.ctxCache = map[string]string{}
		}
		st.ctxCache[token] = body
		st.ctxMu.Unlock()
		cw.postMain(func() { cw.invalidate() })
	}()
}

// expandPluginContext appends the bodies of chosen plugin mentions to the
// outgoing message.
func (cw *ChatWindow) expandPluginContext(text string) string {
	st := &cw.pluginUI
	st.ctxMu.Lock()
	cache := make(map[string]string, len(st.ctxCache))
	for k, v := range st.ctxCache {
		cache[k] = v
	}
	st.ctxMu.Unlock()
	if len(cache) == 0 {
		return text
	}
	var parts []string
	total := 0
	for _, field := range strings.Fields(text) {
		token := strings.TrimPrefix(field, "@")
		token = strings.TrimRight(token, ",.;:!?，。；：！？")
		body, ok := cache[token]
		if !ok || body == "" {
			continue
		}
		if total+len(body) > 64000 {
			break
		}
		total += len(body)
		parts = append(parts, "### "+token+"\n"+body)
	}
	if len(parts) == 0 {
		return text
	}
	return text + "\n\n## 引用上下文\n\n" + strings.Join(parts, "\n\n")
}

// pluginSettingsFor returns the edit buffer for one plugin, seeded from its
// saved values.
func (cw *ChatWindow) pluginSettingsFor(info plugin.Info) map[string]string {
	st := &cw.pluginUI
	if st.settingsFor != info.ID {
		st.settingsFor = info.ID
		st.settings = map[string]string{}
		for _, f := range info.Settings {
			v := info.Values[f.Key]
			if v == "" {
				v = f.Default
			}
			st.settings[f.Key] = v
		}
	}
	if st.settings == nil {
		st.settings = map[string]string{}
	}
	return st.settings
}

// togglePlugin enables or disables a plugin, asking for permissions first.
func (cw *ChatWindow) togglePlugin(id string) {
	info, ok := cw.plugins.Info(id)
	if !ok {
		return
	}
	if info.Status == plugin.StatusLoaded {
		if err := cw.plugins.SetEnabled(id, false, nil, nil); err != nil {
			cw.toastErrorDetail("禁用插件失败", err.Error())
			return
		}
		cw.afterPluginChange()
		return
	}
	if len(info.Missing) > 0 {
		perms := append(append([]string{}, info.Permissions...), info.Missing...)
		cw.askConfirmAs("授权插件「"+info.Name+"」", permissionText(info.Missing), "同意并启用", func() {
			cw.enablePlugin(id, perms, info.Values)
		})
		return
	}
	cw.enablePlugin(id, info.Permissions, info.Values)
}

// enablePlugin loads a plugin with the granted permissions.
func (cw *ChatWindow) enablePlugin(id string, permissions []string, settings map[string]string) {
	if err := cw.plugins.SetEnabled(id, true, permissions, settings); err != nil {
		cw.toastErrorDetail("启用插件失败", err.Error())
		return
	}
	cw.afterPluginChange()
	cw.toast("插件已启用", mcore.SeveritySuccess)
}

// savePluginSettings persists the edit buffer, reloading a running plugin.
func (cw *ChatWindow) savePluginSettings(info plugin.Info) {
	values := make(map[string]string, len(cw.pluginUI.settings))
	for k, v := range cw.pluginUI.settings {
		values[k] = v
	}
	enabled := info.Status == plugin.StatusLoaded
	if err := cw.plugins.SetEnabled(info.ID, enabled, info.Permissions, values); err != nil {
		cw.toastErrorDetail("保存插件设置失败", err.Error())
		return
	}
	cw.afterPluginChange()
	cw.toast("插件设置已保存", mcore.SeveritySuccess)
}

// afterPluginChange rebuilds everything plugin contributions feed.
func (cw *ChatWindow) afterPluginChange() {
	cw.syncPluginChannelTypes()
	cw.startChannels()
	cw.toolCache, cw.toolCacheKey = nil, ""
	cw.skills.loaded = false
	cw.skills.list = nil
	cw.colorTheme = mtheme.ByName(cw.cfg.ColorTheme)
	cw.clientDirty.Store(true)
	cw.invalidate()
}

// uninstallPlugin removes a plugin and its contributions.
func (cw *ChatWindow) uninstallPlugin(info plugin.Info) {
	cw.askConfirm("卸载插件「"+info.Name+"」", "将删除插件目录与配置项，插件数据目录默认保留。", func() {
		if err := cw.plugins.Uninstall(info.ID, false); err != nil {
			cw.toastErrorDetail("卸载失败", err.Error())
			return
		}
		cw.afterPluginChange()
		cw.toast("插件已卸载", mcore.SeveritySuccess)
	})
}

// installPluginFromUI installs from the path typed on the plugin page. The
// plugin starts disabled so the user reviews its permissions first.
func (cw *ChatWindow) installPluginFromUI() {
	st := &cw.pluginUI
	kind := st.installKind
	if kind == "" {
		kind = "dir"
	}
	src := strings.TrimSpace(st.installPath)
	if src == "" {
		st.installErr = "请填写路径或地址"
		return
	}
	st.installBusy = true
	st.installErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		opts := plugin.InstallOptions{Kind: kind, Enabled: false}
		if kind == "dir" || kind == "zip" {
			opts.Path = src
		} else {
			opts.URL = src
		}
		manifest, err := cw.plugins.Install(ctx, opts)
		cw.postMain(func() {
			st.installBusy = false
			if err != nil {
				st.installErr = err.Error()
				return
			}
			st.installOpen = false
			st.installPath = ""
			cw.pluginUI.selected = manifest.ID
			cw.toast("已安装 "+manifest.Name+"，请确认权限后启用", mcore.SeveritySuccess)
			cw.invalidate()
		})
	}()
}

// permissionText renders granted permissions for the confirmation dialog.
func permissionText(perms []string) string {
	var lines []string
	for _, p := range perms {
		switch {
		case p == "exec":
			lines = append(lines, "• 执行本地命令（MCP / ACP）")
		case p == "events":
			lines = append(lines, "• 读取对话事件")
		case p == "context":
			lines = append(lines, "• 读取外部上下文")
		case strings.HasPrefix(p, "http:"):
			lines = append(lines, "• 访问网络 "+strings.TrimPrefix(p, "http:"))
		case strings.HasPrefix(p, "env:"):
			lines = append(lines, "• 读取环境变量 "+strings.TrimPrefix(p, "env:"))
		case p == "store":
			lines = append(lines, "• 读写自身数据目录")
		default:
			lines = append(lines, "• "+p)
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// pluginKindLabel names an install source.
func pluginKindLabel(kind string) string {
	switch kind {
	case "zip":
		return "ZIP"
	case "git":
		return "Git"
	case "npm":
		return "npm"
	default:
		return "目录"
	}
}
