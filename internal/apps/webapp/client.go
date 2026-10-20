package webapp

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"
	"chatgo/internal/config"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// Client hosts one web app in a WebView and routes the Agent's tool calls to
// the page through the bridge.
type Client struct {
	host   Host
	app    App
	br     *bridge
	view   *mygo.WebView
	url    string
	err    string
	loaded bool
	notes  []Notification
	routes map[string]http.Handler

	local    localServer
	httpOnce sync.Once
	httpAddr string
	httpErr  error
}

// New returns a client for a discovered web app.
func New(h Host, a App) *Client {
	cl := &Client{host: h, app: a, br: newBridge(), routes: map[string]http.Handler{}}
	cl.br.onNote = cl.onNote
	if a.ID == "draw" {
		cl.Mount("/mcp", cl.demoMCP())
	}
	return cl
}

// App returns the app's manifest.
func (cl *Client) App() App { return cl.app }

// Scheme is the custom protocol serving this app's files.
func (cl *Client) Scheme() string { return "webapp-" + cl.app.ID }

// URL is the page URL of the app.
func (cl *Client) URL() string { return cl.Scheme() + "://localhost/" + cl.app.Entry }

// Register serves the app over its own protocol. Call before any window
// exists: custom schemes must be registered before pages use them.
func (cl *Client) Register() error {
	return mygo.Protocol.Handle(cl.Scheme(), newHandler(cl.app, cl.br, cl.routes))
}

// Notifications returns the messages the page sent, newest last.
func (cl *Client) Notifications() []Notification {
	return cl.notes
}

// onNote records a page notification and refreshes the view.
func (cl *Client) onNote(payload map[string]any) {
	n := parseNote(payload)
	if n.Message == "" {
		return
	}
	cl.host.Post(func() {
		cl.notes = append(cl.notes, n)
		if len(cl.notes) > 20 {
			cl.notes = cl.notes[len(cl.notes)-20:]
		}
		cl.host.Invalidate()
	})
}

// View draws the app: a toolbar and the WebView.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cl.ensureView()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			if mcore.IconAction(c, icons.Must("refresh-cw"), "重新加载").Clicked() {
				cl.reload()
			}
			if mcore.IconAction(c, icons.Must("external-link"), "在浏览器打开").Clicked() {
				_ = mygo.Shell.OpenExternal(cl.URL())
			}
			mgo.Text(c, cl.app.Name).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if n := len(cl.app.Tools); n > 0 {
				mgo.Text(c, fmt.Sprintf("%d 个工具", n)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			}
			if n := len(cl.app.WebMCP); n > 0 {
				mgo.Text(c, fmt.Sprintf("%d 个 MCP", n)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			}
			if n := len(cl.app.Skills); n > 0 {
				mgo.Text(c, fmt.Sprintf("%d 个技能", n)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			}
		})
		if cl.err != "" {
			mgo.Text(c, cl.err).FontSize(caption).TextColor(k.Danger).Padding(0, u*2)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Padding(0, u*2, u*2, u*2).Children(func() {
			if cl.view == nil || cl.view.IsDestroyed() {
				if n := len(cl.notes); n > 0 {
					mgo.Text(c, cl.notes[n-1].Message).FontSize(caption).TextColor(k.TextMuted)
				}
				return
			}
			mgo.WebView(c, cl.view).Fill().Radius(mtheme.ControlRadius)
		})
		if len(cl.notes) > 0 {
			last := cl.notes[len(cl.notes)-1]
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mgo.Text(c, last.Message).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
			})
		}
	})
}

// ensureView creates the WebView once the window exists.
func (cl *Client) ensureView() {
	if cl.view != nil && !cl.view.IsDestroyed() {
		return
	}
	w := cl.host.NativeWindow()
	if w == nil || cl.loaded {
		return
	}
	v, err := w.NewWebView(mygo.WebViewOptions{BackgroundColor: "light-dark(#ffffff, #1c1c1c)"})
	if err != nil {
		cl.err = "创建 WebView 失败: " + err.Error()
		return
	}
	cl.view = v
	cl.loaded = true
	v.Page().OnDidNavigate(func(u string) {
		cl.url = u
		cl.host.Invalidate()
	})
	if err := v.Page().LoadURL(cl.URL()); err != nil {
		cl.err = "加载应用失败: " + err.Error()
	}
}

// reload reopens the app's entry page.
func (cl *Client) reload() {
	if cl.view != nil && !cl.view.IsDestroyed() {
		_ = cl.view.Page().LoadURL(cl.URL())
	}
}

// Close destroys the WebView and stops the loopback server.
func (cl *Client) Close() {
	if cl.view != nil && !cl.view.IsDestroyed() {
		cl.view.Destroy()
	}
	cl.local.close()
}

// Tools returns the manifest's tools as Agent operations.
func (cl *Client) Tools() []Tool {
	out := make([]Tool, 0, len(cl.app.Tools))
	for _, spec := range cl.app.Tools {
		spec := spec
		out = append(out, Tool{
			Name:   spec.Name,
			Desc:   spec.Description,
			Params: spec.Params,
			Run: func(args map[string]any) (string, error) {
				return cl.call(spec.Name, args)
			},
		})
	}
	return out
}

// call asks the page to run one of its registered tools.
func (cl *Client) call(name string, args map[string]any) (string, error) {
	cl.host.Post(func() { cl.ensureView() })
	v := cl.view
	if v == nil || v.IsDestroyed() {
		return "", fmt.Errorf("应用 %s 尚未加载", cl.app.Name)
	}
	id, req := cl.br.begin(name, args)
	defer cl.br.finish(id)
	wrapped, err := callScript(id, name, args)
	if err != nil {
		return "", err
	}
	ctx, cancel := contextWithTimeout()
	defer cancel()
	if _, err := v.Page().EvalContext(ctx, wrapped); err != nil {
		return "", fmt.Errorf("调用页面工具失败: %w", err)
	}
	select {
	case res := <-req.done:
		return res.out, res.err
	case <-timeAfter():
		return "", fmt.Errorf("页面工具 %s 超时", name)
	}
}

// WebMCP returns the app's declared MCP endpoints.
func (cl *Client) WebMCP() []WebMCP { return cl.app.WebMCP }

// Mount serves a handler at an app-relative path, so an app can host a real
// backend such as an MCP endpoint next to its pages. Mount before Register.
func (cl *Client) Mount(path string, h http.Handler) {
	if h == nil {
		return
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	cl.routes[path] = h
}

// MCPURL resolves a WebMCP endpoint to an http URL an MCP client can reach.
// Relative paths map onto the app's loopback server.
func (cl *Client) MCPURL(m WebMCP) (string, error) {
	url := strings.TrimSpace(m.URL)
	if url == "" {
		return "", fmt.Errorf("MCP 端点缺少 url")
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url, nil
	}
	if !strings.HasPrefix(url, "/") {
		url = "/" + url
	}
	base, err := cl.ensureHTTP()
	if err != nil {
		return "", err
	}
	return base + url, nil
}

// MCPServers turns the app's WebMCP endpoints into MCP server configs, so a
// web app can be mounted in the same registry as any other MCP server.
func (cl *Client) MCPServers() []config.MCPServer {
	out := make([]config.MCPServer, 0, len(cl.app.WebMCP))
	for _, m := range cl.app.WebMCP {
		name := m.Name
		if name == "" {
			name = cl.app.ID
		}
		kind := config.MCPServerTypeStreamableHTTP
		if strings.EqualFold(m.Transport, "sse") {
			kind = config.MCPServerTypeSSE
		}
		url, err := cl.MCPURL(m)
		if err != nil {
			continue
		}
		out = append(out, config.MCPServer{
			Name:    "webapp:" + cl.app.ID + "/" + name,
			Type:    kind,
			Enabled: true,
			URL:     url,
		})
	}
	return out
}

// SkillPaths returns the app-relative SKILL.md folders it ships.
func (cl *Client) SkillPaths() []string {
	out := make([]string, 0, len(cl.app.Skills))
	for _, s := range cl.app.Skills {
		if s.Path != "" {
			out = append(out, s.Path)
			continue
		}
		out = append(out, s.Name)
	}
	return out
}

// HasTools reports whether the app exports any tool.
func (cl *Client) HasTools() bool { return len(cl.app.Tools) > 0 }
