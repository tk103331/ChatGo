// Package browserapp is the Apps menu's web browser: a tabbed WebView with a
// URL bar, back/forward, reload, find and page text extraction for the Agent.
package browserapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是浏览器助手。你可以通过 browser 工具打开网页、读取页面文本与标题、执行 JS，帮助用户浏览与提取信息。"

const homeURL = "https://www.bing.com"

// page is one browser tab.
type page struct {
	id    string
	url   string
	title string
	view  *mygo.WebView
	err   string
}

// Client is one browser bound to a Host.
type Client struct {
	host Host
	tabs appkit.Tabs
	byID map[string]*page
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a browser driving the given host.
func New(h Host) *Client {
	return &Client{host: h, byID: map[string]*page{}}
}

// Title labels a tab with the page title or host.
func (cl *Client) Title(id string) string {
	p := cl.byID[id]
	if p == nil {
		return "新标签页"
	}
	if p.title != "" {
		return p.title
	}
	if p.url != "" {
		return p.url
	}
	return "新标签页"
}

// NewTab opens a browser tab, optionally at a URL.
func (cl *Client) NewTab(rawURL string) string {
	p := &page{id: cl.host.NewID(), url: normalize(rawURL)}
	cl.byID[p.id] = p
	cl.tabs.Add(p.id)
	if p.url != "" {
		go cl.load(p, p.url)
	}
	return p.id
}

// Close drops a tab and destroys its view.
func (cl *Client) Close(id string) {
	if p := cl.byID[id]; p != nil && p.view != nil && !p.view.IsDestroyed() {
		p.view.Destroy()
	}
	delete(cl.byID, id)
	for i, key := range cl.tabs.Items {
		if key == id {
			cl.tabs.Close(i)
			return
		}
	}
}

// Tabs exposes the tab list for an embedding tab bar.
func (cl *Client) Tabs() *appkit.Tabs { return &cl.tabs }

// View draws the active page.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	p := cl.byID[cl.tabs.Active]
	if p == nil {
		mgo.Column(c).Fill().Center().Gap(u * 2).Children(func() {
			mgo.Icon(c, icons.Must("globe")).Size(32, 32).TextColor(k.TextMuted)
			mgo.Text(c, "输入网址开始浏览").FontSize(caption).TextColor(k.TextMuted)
		})
		return
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if mcore.IconAction(c, icons.Must("chevron-left"), "后退").Clicked() && p.view != nil && !p.view.IsDestroyed() {
				p.view.Page().GoBack()
			}
			if mcore.IconAction(c, icons.Must("chevron-right"), "前进").Clicked() && p.view != nil && !p.view.IsDestroyed() {
				p.view.Page().GoForward()
			}
			if mcore.IconAction(c, icons.Must("refresh-cw"), "刷新").Clicked() && p.view != nil && !p.view.IsDestroyed() {
				p.view.Page().Reload()
			}
			in := mgo.TextInput(c, &p.url).Placeholder("输入网址，回车打开").Grow(1).MinWidth(0).Label("网址")
			if in.Submitted() {
				go cl.load(p, normalize(p.url))
			}
		})
		if p.err != "" {
			mgo.Text(c, p.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			if p.view == nil || p.view.IsDestroyed() {
				mgo.Text(c, "正在打开网页…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
				return
			}
			mgo.WebView(c, p.view).Fill().Radius(mtheme.ControlRadius)
		})
	})
}

// load creates the page's WebView on the UI thread and navigates.
func (cl *Client) load(p *page, url string) {
	if url == "" {
		return
	}
	cl.host.Post(func() {
		w := cl.host.NativeWindow()
		if w == nil {
			p.err = "窗口不可用"
			cl.host.Invalidate()
			return
		}
		if p.view == nil || p.view.IsDestroyed() {
			v, err := w.NewWebView(mygo.WebViewOptions{BackgroundColor: "light-dark(#ffffff, #1c1c1c)"})
			if err != nil {
				p.err = "创建 WebView 失败: " + err.Error()
				cl.host.Invalidate()
				return
			}
			p.view = v
			v.Page().OnDidNavigate(func(u string) {
				p.url = u
				cl.host.Invalidate()
				go func() {
					if t, err := evalString(v, `return document.title`); err == nil && t != "" {
						cl.host.Post(func() { p.title = t; cl.host.Invalidate() })
					}
				}()
			})
		}
		p.url, p.err = url, ""
		if err := p.view.Page().LoadURL(url); err != nil {
			p.err = "加载失败: " + err.Error()
		}
		cl.host.Invalidate()
	})
}

// normalize adds a scheme to a bare host.
func normalize(raw string) string {
	url := strings.TrimSpace(raw)
	if url == "" {
		return ""
	}
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	return url
}

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "browser",
			Desc: "浏览器操作：open 打开网址、text 读取页面文本、title 读取标题、html 读取 HTML、eval 执行 JS",
			Params: map[string]Param{
				"action": {Desc: "open、text、title、html 或 eval", Required: true},
				"url":    {Desc: "动作 open 时的网址"},
				"code":   {Desc: "动作 eval 时执行的 JavaScript"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.run(str(args, "action"), str(args, "url"), str(args, "code"))
			},
		},
	}
}

// run executes one browser tool action.
func (cl *Client) run(action, url, js string) (string, error) {
	switch action {
	case "open":
		if url == "" {
			return "", fmt.Errorf("缺少必填参数 url")
		}
		cl.NewTab(url)
		return "已打开 " + normalize(url), nil
	case "title", "text", "html", "eval":
		p := cl.byID[cl.tabs.Active]
		if p == nil || p.view == nil || p.view.IsDestroyed() {
			return "", fmt.Errorf("当前没有打开的网页")
		}
		var script string
		switch action {
		case "title":
			script = `return document.title`
		case "text":
			script = `return document.body ? document.body.innerText : ""`
		case "html":
			script = `return document.body ? document.body.innerHTML : ""`
		default:
			if strings.TrimSpace(js) == "" {
				return "", fmt.Errorf("缺少必填参数 code")
			}
			script = js
		}
		out, err := evalString(p.view, script)
		if err != nil {
			return "", err
		}
		return out, nil
	default:
		return "", fmt.Errorf("不支持的 action: %s", action)
	}
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// evalString runs expr in the view's page and returns its string result.
func evalString(v *mygo.WebView, expr string) (string, error) {
	if v == nil || v.IsDestroyed() {
		return "", fmt.Errorf("当前没有打开的网页")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	js := `(function(){try{var r=(function(){` + expr + `})();return JSON.stringify({result:r})}catch(e){return JSON.stringify({error:String(e)})}})()`
	r, err := v.Page().EvalContext(ctx, js)
	if err != nil {
		return "", fmt.Errorf("JS 执行失败: %w", err)
	}
	s, ok := r.(string)
	if !ok {
		return fmt.Sprint(r), nil
	}
	return s, nil
}
