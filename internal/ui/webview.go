package ui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// webTab is one web view of the preview area. Its ID names it to the webview
// tool, so the model can pick which page it acts on.
type webTab struct {
	ID      string
	Title   string
	URL     string
	Address string
	view    *mygo.WebView
}

// newWebTab opens a web view tab for target and returns it.
func (cw *ChatWindow) newWebTab(target, id string) (*webTab, error) {
	if target == "" {
		target = webLandingURL
	}
	src, err := cw.webTarget(target)
	if err != nil {
		return nil, err
	}
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil {
		return nil, fmt.Errorf("窗口尚未就绪")
	}
	v, err := w.NewWebView(mygo.WebViewOptions{})
	if err != nil {
		return nil, fmt.Errorf("WebView 不可用：%w", err)
	}
	cw.webMu.Lock()
	if id == "" {
		cw.webSeq++
		id = "wv" + strconv.Itoa(cw.webSeq)
	}
	cw.webMu.Unlock()
	blank := src == webLandingURL
	title, address := displayTitle(src), displayURL(src)
	if blank {
		title, address = "新标签页", ""
	}
	tab := &webTab{ID: id, Title: title, URL: src, Address: address, view: v}
	cw.watchWebTab(tab)
	if !blank {
		cw.loadWebTarget(v, src)
	}
	return tab, nil
}

// watchWebTab keeps a tab's URL and title current as its page navigates.
func (cw *ChatWindow) watchWebTab(tab *webTab) {
	p := tab.view.Page()
	if p == nil {
		return
	}
	refresh := func() {
		if u, err := mygo.EvalAs[string](p, "location.href"); err == nil && u != "" {
			tab.URL = u
			if tab.Address == "" || sameTarget(tab.Address, displayURL(u)) {
				tab.Address = displayURL(u)
			}
		}
		if t, err := mygo.EvalAs[string](p, "document.title"); err == nil && t != "" {
			tab.Title = t
		}
	}
	p.OnDidNavigate(func(string) { refresh() })
	p.OnDidFinishLoad(refresh)
	p.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if !e.UserInitiated || !isExternalURL(e.URL) {
			return
		}
		e.PreventDefault()
		_ = mygo.Shell.OpenExternal(e.URL)
	})
}

// webTabByID returns the open web tab with id, or nil. An empty id takes the
// first open tab.
func (cw *ChatWindow) webTabByID(id string) *webTab {
	for _, t := range cw.centerTabsOfKind(centerWeb) {
		if t.Web == nil {
			continue
		}
		if id == "" || t.Web.ID == id {
			return t.Web
		}
	}
	return nil
}

// openWebPreview opens target in a web tab, focusing the tab already showing
// it.
func (cw *ChatWindow) openWebPreview(target string) {
	if _, err := cw.newWebPreview(target, ""); err != nil {
		cw.toast(err.Error(), mcore.SeverityError)
	}
}

// newWebPreview adds a web tab for target in the focused pane.
func (cw *ChatWindow) newWebPreview(target, id string) (string, error) {
	return cw.newWebTabIn(cw.center.focus, target, id)
}

// newWebTabIn adds a web tab for target to a pane and shows it, reusing the
// tab named id when it is already open.
func (cw *ChatWindow) newWebTabIn(pane int, target, id string) (string, error) {
	target = strings.TrimSpace(target)
	src := ""
	if target != "" {
		resolved, err := cw.webTarget(target)
		if err != nil {
			return "", err
		}
		src = resolved
	}
	if existing := cw.webTabByID(id); existing != nil && id != "" {
		if existing.URL != src {
			existing.URL, existing.Address = src, displayURL(src)
			cw.loadWebTarget(existing.view, src)
		}
		cw.selectWebTab(existing.ID)
		return existing.ID, nil
	}
	tab, err := cw.newWebTab(src, id)
	if err != nil {
		return "", err
	}
	cw.openCenterTab(pane, &centerTab{
		ID: "webview:" + tab.ID, Path: "webview:" + tab.ID,
		Title: tab.Title, Kind: centerWeb, Web: tab,
	})
	return tab.ID, nil
}

// selectWebTab brings the tab holding id to the front.
func (cw *ChatWindow) selectWebTab(id string) {
	cw.focusCenterTab("webview:" + id)
}

// loadWebTarget points a web view at src, reading file:// URLs as files.
func (cw *ChatWindow) loadWebTarget(v *mygo.WebView, src string) {
	p := v.Page()
	if p == nil {
		return
	}
	if strings.HasPrefix(src, "file://") {
		if path, err := url.PathUnescape(strings.TrimPrefix(src, "file://")); err == nil {
			_ = p.LoadFile(path)
		}
		return
	}
	if err := p.LoadURL(src); err != nil {
		cw.toastErrorDetail("加载失败", err.Error())
	}
}

// sameTarget reports whether typed already names the page open at current.
func sameTarget(typed, current string) bool {
	if current == "" {
		return false
	}
	return typed == current || typed == displayURL(current)
}

// webTarget resolves a preview target to a URL the web view loads.
func (cw *ChatWindow) webTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("缺少网址")
	}
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return target, nil
	}
	if strings.HasPrefix(lower, "file://") {
		return target, nil
	}
	if lower == webLandingURL {
		return webLandingURL, nil
	}
	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(cw.fileRoot(), path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		if !strings.Contains(target, ".") || strings.ContainsAny(target, "/ ") {
			return "", fmt.Errorf("找不到文件：%s", target)
		}
		return "https://" + target, nil
	}
	return "file://" + abs, nil
}

// isExternalURL reports whether u leaves the page the preview shows.
func isExternalURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

// displayTitle names a web tab after its host or file.
func displayTitle(src string) string {
	if u, err := url.Parse(src); err == nil && u.Host != "" {
		return u.Host
	}
	return filepath.Base(strings.TrimPrefix(src, "file://"))
}

// previewURL opens target in a web tab and blocks until it settles, for the
// webview tool. id names the tab, empty creates one.
func (cw *ChatWindow) previewURL(ctx context.Context, id, target string) (string, error) {
	openID, err := cw.newWebPreview(target, id)
	if err != nil {
		return "", err
	}
	tab := cw.webTabByID(openID)
	if tab == nil {
		return "", fmt.Errorf("WebView 不可用")
	}
	if err := waitPage(ctx, tab.view.Page()); err != nil {
		return "", err
	}
	title, _ := mygo.EvalAs[string](tab.view.Page(), "document.title")
	return fmt.Sprintf("已在 webview %s 中打开 %s（标题：%s）", tab.ID, tab.URL, title), nil
}

// evalInWebTab evaluates code in the named web tab, or the one showing.
func (cw *ChatWindow) evalInWebTab(ctx context.Context, id, code string) (any, error) {
	tab := cw.webTabByID(id)
	if tab == nil {
		return nil, fmt.Errorf("没有打开的 webview，请先用 action=open 打开页面")
	}
	p := tab.view.Page()
	if p == nil {
		return nil, fmt.Errorf("webview %s 不可用", tab.ID)
	}
	return p.EvalContext(ctx, code)
}

// waitPage waits until the page finishes loading or ctx ends.
func waitPage(ctx context.Context, p *mygo.Page) error {
	if p == nil {
		return fmt.Errorf("WebView 不可用")
	}
	done := make(chan struct{}, 1)
	off := p.OnDidFinishLoad(func() {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	defer off()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("页面加载超时")
	}
}

// openWebInBrowser opens the tab's page in the desktop browser.
func (cw *ChatWindow) openWebInBrowser(tab *webTab) {
	if tab == nil || tab.URL == "" {
		cw.toast("还没有打开网页", mcore.SeverityInfo)
		return
	}
	if err := mygo.Shell.OpenExternal(tab.URL); err != nil {
		cw.toast("打开失败："+err.Error(), mcore.SeverityError)
	}
}

// webToolbar is a web tab's address bar with back, forward and reload.
func (cw *ChatWindow) webToolbar(c *mgo.Context, tab *webTab) {
	u := mcore.Density(c).Unit()
	p := tab.view.Page()
	if p == nil {
		return
	}
	nav := func(run func()) {
		run()
		cw.syncWebAddress(tab)
		cw.invalidate()
	}
	mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Gap(u).Children(func() {
		if mcore.IconAction(c, icons.Must("chevron-left"), "后退").Clicked() {
			nav(p.GoBack)
		}
		if mcore.IconAction(c, icons.Must("chevron-right"), "前进").Clicked() {
			nav(p.GoForward)
		}
		if mcore.IconAction(c, icons.Must("refresh-cw"), "刷新").Clicked() {
			nav(p.Reload)
		}
		in := kit.TextInput(c, &tab.Address, kit.TextInputOptions{
			Placeholder: "输入网址或工作目录中的 HTML 路径",
			Label:       "网址",
		})
		in.Element.Grow(1).MinWidth(0)
		if in.Submitted() {
			typed := strings.TrimSpace(tab.Address)
			if typed != "" && !sameTarget(typed, tab.URL) {
				if src, err := cw.webTarget(typed); err == nil {
					tab.URL, tab.Address = src, displayURL(src)
					cw.loadWebTarget(tab.view, src)
				} else {
					cw.toast(err.Error(), mcore.SeverityError)
				}
			}
		}
		if mcore.IconAction(c, icons.Must("external-link"), "外部浏览器").Clicked() {
			cw.openWebInBrowser(tab)
		}
	})
}

// syncWebAddress copies the page's current address into the toolbar field.
func (cw *ChatWindow) syncWebAddress(tab *webTab) {
	p := tab.view.Page()
	if p == nil {
		return
	}
	if u, err := mygo.EvalAs[string](p, "location.href"); err == nil && u != "" {
		tab.Address = displayURL(u)
	}
}

// displayURL drops the file:// scheme so the address bar shows a path.
func displayURL(u string) string {
	if strings.HasPrefix(u, "file://") {
		if p, err := url.PathUnescape(strings.TrimPrefix(u, "file://")); err == nil {
			return p
		}
	}
	return u
}

// webLandingURL is the blank page a new web tab shows before it loads a URL.
const webLandingURL = "about:blank"

// webLanding is the hint a blank web tab shows: open a link or type one.
func (cw *ChatWindow) webLanding(c *mgo.Context, tab *webTab) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().Center().Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("globe")).Size(36, 36).TextColor(k.TextMuted)
		mgo.Text(c, "还没有打开网页").FontWeight(600)
		mgo.Text(c, "在上方输入网址，或点击聊天/文档里的链接在此打开").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Text(c, "网页标签标识："+tab.ID).FontSize(small).TextColor(k.TextMuted)
	})
}

// closeWebTab closes the named web tab, or the one showing, for the webview
// tool.
func (cw *ChatWindow) closeWebTab(id string) (string, error) {
	tab := cw.webTabByID(id)
	if tab == nil {
		return "", fmt.Errorf("没有找到 webview，请先用 action=list 查看已打开的标签")
	}
	for _, t := range cw.centerTabsOfKind(centerWeb) {
		if t.Web != tab {
			continue
		}
		pane, i := cw.paneHoldingTab(t)
		if pane < 0 {
			break
		}
		cw.closeCenterTab(pane, i)
		return "已关闭 webview " + tab.ID, nil
	}
	return "", fmt.Errorf("webview %s 已经关闭", tab.ID)
}

// listWebTabs describes the open web tabs for the webview tool.
func (cw *ChatWindow) listWebTabs() (string, error) {
	var lines []string
	for _, t := range cw.centerTabsOfKind(centerWeb) {
		if t.Web == nil {
			continue
		}
		url := t.Web.URL
		if url == webLandingURL {
			url = "(空白页)"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", t.Web.ID, t.Web.Title, url))
	}
	if len(lines) == 0 {
		return "没有打开的 webview", nil
	}
	return "标识\t标题\t网址\n" + strings.Join(lines, "\n"), nil
}
