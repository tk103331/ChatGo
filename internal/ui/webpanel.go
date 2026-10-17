package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// toggleWebPanel shows or hides the WebView panel.
func (cw *ChatWindow) toggleWebPanel() {
	cw.webPanel = !cw.webPanel
	cw.invalidate()
}

// ensureWebView makes the window's web view once, on first use.
func (cw *ChatWindow) ensureWebView() {
	if cw.webView != nil && !cw.webView.IsDestroyed() {
		return
	}
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil {
		return
	}
	v, err := w.NewWebView(mygo.WebViewOptions{BackgroundColor: "light-dark(#ffffff, #1c1c1c)"})
	if err != nil {
		cw.fail("创建 WebView 失败: " + err.Error())
		return
	}
	cw.webView = v
	v.Page().OnDidNavigate(func(url string) {
		cw.webCurrent = url
		cw.webURL = url
		cw.invalidate()
	})
	v.Page().SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if strings.HasPrefix(req.URL, "http://") || strings.HasPrefix(req.URL, "https://") {
			_ = v.Page().LoadURL(req.URL)
			return nil
		}
		_ = mygo.Shell.OpenExternal(req.URL)
		return nil
	})
}

// webLoadURL normalizes and loads a URL, creating the web view on first use.
func (cw *ChatWindow) webLoadURL(raw string) {
	url := strings.TrimSpace(raw)
	if url == "" {
		return
	}
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	cw.ensureWebView()
	if cw.webView == nil || cw.webView.IsDestroyed() {
		return
	}
	cw.webCurrent = url
	cw.webURL = url
	if err := cw.webView.Page().LoadURL(url); err != nil {
		cw.fail("加载网址失败: " + err.Error())
	}
}

// webPanelView is the WebView column: a toolbar and the page.
func (cw *ChatWindow) webPanelView(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Background(k.Surface).
		BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if mcore.IconAction(c, icons.Must("chevron-left"), "后退").Clicked() {
				if cw.webView != nil && !cw.webView.IsDestroyed() {
					cw.webView.Page().GoBack()
				}
			}
			if mcore.IconAction(c, icons.Must("chevron-right"), "前进").Clicked() {
				if cw.webView != nil && !cw.webView.IsDestroyed() {
					cw.webView.Page().GoForward()
				}
			}
			if mcore.IconAction(c, icons.Must("refresh-cw"), "刷新").Clicked() {
				if cw.webView != nil && !cw.webView.IsDestroyed() {
					cw.webView.Page().Reload()
				}
			}
			in := mgo.TextInput(c, &cw.webURL).Placeholder("输入网址，回车打开").
				Grow(1).MinWidth(0).Label("网址")
			if in.Submitted() {
				cw.webLoadURL(cw.webURL)
			}
			if mcore.IconAction(c, icons.Must("x"), "关闭面板").Clicked() {
				cw.webPanel = false
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() { cw.webContent(c) })
	})
}

// webContent shows the page or the empty hint.
func (cw *ChatWindow) webContent(c *mgo.Context) {
	k := mcore.Tokens(c)
	if cw.webView == nil || cw.webView.IsDestroyed() || cw.webCurrent == "" {
		mgo.Column(c).Fill().Center().Gap(mcore.Density(c).Unit() * 2).Children(func() {
			mgo.Icon(c, icons.Must("globe")).Size(32, 32).TextColor(k.TextMuted)
			mgo.Text(c, "在上方输入网址打开网页").TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		})
		return
	}
	mgo.WebView(c, cw.webView).Grow(1).MinHeight(0).Radius(mtheme.ControlRadius)
}

// webviewTool runs a webview builtin tool action. Safe from any goroutine.
func (cw *ChatWindow) webviewTool(ctx context.Context, action, rawURL, selector, value, code string, timeout time.Duration) (string, error) {
	switch action {
	case "open":
		url := strings.TrimSpace(rawURL)
		if url == "" {
			return "", fmt.Errorf("缺少必填参数 url")
		}
		if !strings.Contains(url, "://") {
			url = "https://" + url
		}
		cw.postMain(func() {
			cw.webPanel = true
			cw.webLoadURL(url)
			cw.invalidate()
		})
		return cw.webWaitLoad(ctx, timeout)
	case "url":
		v := cw.webView
		if v == nil || v.IsDestroyed() {
			return "", fmt.Errorf("WebView 未打开任何页面，请先用 open 操作打开网址")
		}
		return v.Page().URL(), nil
	case "title":
		return cw.webviewEvalJS(ctx, "", `return document.title`)
	case "content":
		if strings.TrimSpace(selector) == "" {
			return cw.webviewEvalJS(ctx, "", `return document.body ? document.body.innerText : ""`)
		}
		return cw.webviewEvalJS(ctx, selector, `return el.innerText`)
	case "html":
		if strings.TrimSpace(selector) == "" {
			return cw.webviewEvalJS(ctx, "", `return document.body ? document.body.innerHTML : ""`)
		}
		return cw.webviewEvalJS(ctx, selector, `return el.innerHTML`)
	case "click":
		if strings.TrimSpace(selector) == "" {
			return "", fmt.Errorf("缺少必填参数 selector")
		}
		return cw.webviewEvalJS(ctx, selector, `el.click(); return "已点击"`)
	case "fill":
		if strings.TrimSpace(selector) == "" {
			return "", fmt.Errorf("缺少必填参数 selector")
		}
		val, _ := json.Marshal(value)
		return cw.webviewEvalJS(ctx, selector, `el.value = `+string(val)+`; el.dispatchEvent(new Event("input", {bubbles:true})); el.dispatchEvent(new Event("change", {bubbles:true})); return "已填写"`)
	case "submit":
		if strings.TrimSpace(selector) == "" {
			return "", fmt.Errorf("缺少必填参数 selector")
		}
		return cw.webviewEvalJS(ctx, selector, `el.submit(); return "已提交"`)
	case "eval":
		if strings.TrimSpace(code) == "" {
			return "", fmt.Errorf("缺少必填参数 code")
		}
		v := cw.webView
		if v == nil || v.IsDestroyed() {
			return "", fmt.Errorf("WebView 未打开任何页面，请先用 open 操作打开网址")
		}
		r, err := v.Page().EvalContext(ctx, code)
		if err != nil {
			return "", fmt.Errorf("JS 执行失败: %w", err)
		}
		return webviewFormatResult(r), nil
	}
	return "", fmt.Errorf("未知 action %q（应为 open/url/title/content/html/click/fill/submit/eval）", action)
}

// webviewEvalJS evaluates a JS expression, optionally scoped to the element
// matching selector (as el). The result is JSON-wrapped to distinguish
// errors from values.
func (cw *ChatWindow) webviewEvalJS(ctx context.Context, selector, expr string) (string, error) {
	v := cw.webView
	if v == nil || v.IsDestroyed() {
		return "", fmt.Errorf("WebView 未打开任何页面，请先用 open 操作打开网址")
	}
	var js string
	if strings.TrimSpace(selector) == "" {
		js = `(function(){try{var r=(function(){` + expr + `})();return JSON.stringify({result:r})}catch(e){return JSON.stringify({error:String(e)})}})()`
	} else {
		sel, _ := json.Marshal(selector)
		js = `(function(){try{var el=document.querySelector(` + string(sel) + `);if(!el){return JSON.stringify({error:"element not found"})}var r=(function(){` + expr + `})();return JSON.stringify({result:r})}catch(e){return JSON.stringify({error:String(e)})}})()`
	}
	r, err := v.Page().EvalContext(ctx, js)
	if err != nil {
		return "", fmt.Errorf("JS 执行失败: %w", err)
	}
	s, ok := r.(string)
	if !ok {
		return webviewFormatResult(r), nil
	}
	var out struct {
		Result any    `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return s, nil
	}
	if out.Error != "" {
		if out.Error == "element not found" {
			return "", fmt.Errorf("元素未找到（选择器 %s）", selector)
		}
		return "", fmt.Errorf("JS 执行失败: %s", out.Error)
	}
	return webviewFormatResult(out.Result), nil
}

// webviewFormatResult renders an Eval result as a string.
func webviewFormatResult(r any) string {
	if r == nil {
		return "(null)"
	}
	switch v := r.(type) {
	case string:
		return v
	case bool:
		return fmt.Sprint(v)
	default:
		b, err := json.Marshal(r)
		if err != nil {
			return fmt.Sprint(r)
		}
		return string(b)
	}
}

// webWaitLoad polls until the page finishes loading or the timeout expires.
func (cw *ChatWindow) webWaitLoad(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	time.Sleep(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		v := cw.webView
		if v == nil || v.IsDestroyed() {
			return "", fmt.Errorf("WebView 不可用")
		}
		if !v.Page().IsLoading() {
			url := v.Page().URL()
			if url == "" || url == "about:blank" {
				continue
			}
			title, _ := v.Page().EvalContext(ctx, "document.title")
			return fmt.Sprintf("页面已加载: %s\n标题: %s", url, fmt.Sprint(title)), nil
		}
	}
	return "", fmt.Errorf("页面加载超时（%s）", timeout)
}
