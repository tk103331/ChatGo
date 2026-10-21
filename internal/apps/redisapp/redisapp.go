package redisapp

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是 Redis 客户端助手。你可以通过 redis_connect 连接 Redis（默认 localhost:6379），通过 redis_keys 按模式扫描键，通过 redis_get 读取任意键的值（自动按类型渲染 string/list/set/zset/hash/stream），通过 redis_ttl 查看键的过期时间，通过 redis_type 查看键类型，通过 redis_del 删除键，通过 redis_cmd 执行任意 Redis 命令，通过 redis_info 查看服务器信息。删除键、清库（flushdb）等破坏性操作前先向用户确认。"

// Pages of the Redis client.
const (
	pageKeys = iota
	pageConsole
	pageServer
)

// state is the client's retained model.
type state struct {
	page      int
	conn      Conn
	connected bool

	pattern string
	keys    []KeyMeta
	picked  int
	value   ValueView
	valueKey string

	cmdLine string
	cmdOut  string

	infoText string

	loading bool
	err     string

	dbText string

	splitKeyW float32
	lastScan time.Time
}

// Client is one Redis client bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	st   *state
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a Redis client driving the given host.
func New(h Host) *Client {
	return &Client{host: h, st: &state{
		conn:      Conn{Host: "localhost", Port: 6379, DB: 0},
		pattern:   "*",
		picked:    -1,
		splitKeyW: 320,
	}}
}

// View renders the Redis client: page tabs above a content area.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	st := cl.snapshot()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			dotCol := k.TextMuted
			if st.connected {
				dotCol = k.Success
			}
			mgo.Box(c).Width(8).Height(8).Shrink(0).Radius(4).Background(dotCol)
			mgo.Text(c, st.conn.Addr()+" DB"+strconv.Itoa(st.conn.DB)).FontSize(caption).
				TextColor(k.Text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			pages := []struct {
				id    int
				label string
				icon  string
			}{
				{pageKeys, "键", "key-round"},
				{pageConsole, "命令", "terminal"},
				{pageServer, "服务器", "info"},
			}
			for _, p := range pages {
				p := p
				variant := minput.Ghost
				if st.page == p.id {
					variant = minput.Secondary
				}
				if minput.Button(c, "", minput.ButtonOptions{Variant: variant, Icon: icons.Must(p.icon), Label: p.label}).Clicked() {
					cl.setPage(p.id)
				}
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("key-round"), Label: "连接"}).Clicked() {
				cl.connect()
			}
			if st.connected {
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("x"), Label: "断开"}).Clicked() {
					cl.disconnect()
				}
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.refresh()
			}
		})
		if st.err != "" {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mgo.Text(c, st.err).FontSize(caption).TextColor(k.Danger).Grow(1).SingleLine().Ellipsis("…")
				if mcore.IconAction(c, icons.Must("x"), "清除错误").Clicked() {
					cl.mu.Lock()
					cl.st.err = ""
					cl.mu.Unlock()
					cl.host.Invalidate()
				}
			})
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			switch st.page {
			case pageKeys:
				cl.keysView(c, st, k, u, caption)
			case pageConsole:
				cl.consoleView(c, st, k, u, caption)
			case pageServer:
				cl.serverView(c, st, k, u, caption)
			}
		})
	})
}

// snapshot returns a copy of the state for rendering.
func (cl *Client) snapshot() *state {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cp := *cl.st
	return &cp
}

// setPage switches the active page and loads its data.
func (cl *Client) setPage(p int) {
	cl.mu.Lock()
	cl.st.page = p
	cl.mu.Unlock()
	cl.refresh()
}

// refresh reloads the active page's data.
func (cl *Client) refresh() {
	cl.mu.Lock()
	p := cl.st.page
	connected := cl.st.connected
	cl.mu.Unlock()
	if !connected {
		cl.mu.Lock()
		cl.st.err = ""
		cl.mu.Unlock()
		cl.host.Invalidate()
		return
	}
	switch p {
	case pageKeys:
		cl.loadKeys()
	case pageConsole:
	case pageServer:
		cl.loadInfo()
	}
}

// connect prompts for a connection string and connects.
func (cl *Client) connect() {
	cl.mu.Lock()
	addr := cl.st.conn.Addr()
	cl.mu.Unlock()
	cl.host.Prompt("连接 Redis", "地址 (host:port)", addr, func(s string) {
		s = strings.TrimSpace(s)
		host, port := "localhost", 6379
		if i := strings.LastIndex(s, ":"); i >= 0 {
			host = s[:i]
			if p, err := strconv.Atoi(s[i+1:]); err == nil {
				port = p
			}
		} else if s != "" {
			host = s
		}
		conn := Conn{Host: host, Port: port, DB: 0}
		go func() {
			if err := ping(conn); err != nil {
				cl.host.Post(func() {
					cl.mu.Lock()
					cl.st.err = "连接失败: " + shortErr(err)
					cl.mu.Unlock()
					cl.host.Invalidate()
				})
				return
			}
			cl.host.Post(func() {
				cl.mu.Lock()
				cl.st.conn = conn
				cl.st.connected = true
				cl.st.err = ""
				cl.mu.Unlock()
				cl.host.Toast("已连接 "+conn.Addr(), mcore.SeveritySuccess)
				cl.loadKeys()
			})
		}()
	})
}

// disconnect drops the current connection.
func (cl *Client) disconnect() {
	cl.mu.Lock()
	cl.st.connected = false
	cl.st.keys = nil
	cl.st.value = ValueView{}
	cl.st.valueKey = ""
	cl.st.err = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// loadKeys scans the key list in a background goroutine.
func (cl *Client) loadKeys() {
	cl.mu.Lock()
	if cl.st.loading {
		cl.mu.Unlock()
		return
	}
	cl.st.loading = true
	cl.st.err = ""
	conn := cl.st.conn
	pattern := cl.st.pattern
	cl.mu.Unlock()
	go func() {
		keys, err := scanKeys(conn, pattern, 200)
		var metas []KeyMeta
		if err == nil {
			metas, err = keyMetas(conn, keys)
		}
		cl.mu.Lock()
		cl.st.loading = false
		cl.st.lastScan = time.Now()
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.keys = metas
			if cl.st.picked >= len(metas) {
				cl.st.picked = -1
			}
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// loadValue reads the picked key's value.
func (cl *Client) loadValue(key string) {
	cl.mu.Lock()
	conn := cl.st.conn
	cl.mu.Unlock()
	go func() {
		v, err := getValue(conn, key, 2000)
		cl.mu.Lock()
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.value = v
			cl.st.valueKey = key
			cl.st.err = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// loadInfo fetches server info.
func (cl *Client) loadInfo() {
	cl.mu.Lock()
	conn := cl.st.conn
	cl.mu.Unlock()
	go func() {
		text, err := info(conn, "all")
		cl.mu.Lock()
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.infoText = text
			cl.st.err = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// deletePicked deletes the currently picked key.
func (cl *Client) deletePicked() {
	cl.mu.Lock()
	key := cl.st.valueKey
	conn := cl.st.conn
	cl.mu.Unlock()
	if key == "" {
		return
	}
	cl.host.Confirm("删除键", "确定删除键 "+key+" 吗？", func() {
		go func() {
			n, err := deleteKey(conn, key)
			cl.host.Post(func() {
				if err != nil {
					cl.mu.Lock()
					cl.st.err = "删除失败: " + shortErr(err)
					cl.mu.Unlock()
				} else {
					cl.mu.Lock()
					cl.st.value = ValueView{}
					cl.st.valueKey = ""
					cl.mu.Unlock()
					cl.host.Toast(fmt.Sprintf("已删除 %s（%d 个键）", key, n), mcore.SeveritySuccess)
				}
				cl.host.Invalidate()
			})
			cl.loadKeys()
		}()
	})
}

// shortErr trims an error to one short line.
func shortErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return msg
}

// keysView draws the key browser: pattern input, key list and value panel.
func (cl *Client) keysView(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	if !st.connected {
		cl.emptyView(c, "未连接。点击右上角「连接」并输入 host:port。", k, caption)
		return
	}
	mgo.Split(c, &cl.st.splitKeyW, func() {
		cl.keyList(c, st, k, u, caption)
	}, func() {
		cl.valuePanel(c, st, k, u, caption)
	}).Fill()
}

// keyList draws the pattern input and key list.
func (cl *Client) keyList(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "模式").FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			minput.TextInput(c, &cl.st.pattern, minput.TextInputOptions{Placeholder: "*"}).Element.Grow(1).MinWidth(0)
			if mcore.IconAction(c, icons.Must("search"), "扫描").Clicked() {
				cl.loadKeys()
			}
		})
		if st.loading {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "扫描键中…").FontSize(caption).TextColor(k.TextMuted)
			})
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
			if len(st.keys) == 0 {
				mgo.Text(c, "(没有匹配的键)").FontSize(caption).TextColor(k.TextMuted).Padding(u, u*2)
				return
			}
			mgo.Column(c).FillWidth().Padding(u, u*2).Gap(2).Children(func() {
				for i, km := range st.keys {
					i, km := i, km
					sel := i == cl.snapshot().picked
					b := mgo.ButtonBase(c).Column().Key("key:"+km.Key).
						FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).
						Radius(mtheme.ControlRadius).Padding(u, u).Children(func() {
						typeCol := keyTypeColor(k, km.Type)
						mgo.Text(c, km.Type).FontSize(caption).TextColor(typeCol).Shrink(0).Width(48)
						mgo.Text(c, km.Key).FontSize(caption).TextColor(k.Text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						mgo.Text(c, fmtTTL(km.TTL)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
						if mcore.IconAction(c, icons.Must("trash-2"), "删除键").Clicked() {
							cl.pickKey(i)
							cl.deletePicked()
						}
					})
					if sel {
						b.Background(k.Selection)
					} else if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					if b.Clicked() {
						cl.pickKey(i)
					}
				}
			})
		})
	})
}

// valuePanel draws the picked key's value.
func (cl *Client) valuePanel(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			name := "(选择左侧键查看)"
			if st.valueKey != "" {
				name = st.valueKey
			}
			mgo.Text(c, name).FontSize(caption).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if st.valueKey != "" {
				mgo.Text(c, st.value.Type).FontSize(caption).TextColor(keyTypeColor(k, st.value.Type)).Shrink(0)
				if mcore.IconAction(c, icons.Must("refresh-cw"), "重新读取").Clicked() {
					cl.loadValue(st.valueKey)
				}
				if mcore.IconAction(c, icons.Must("trash-2"), "删除键").Clicked() {
					cl.deletePicked()
				}
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Padding(u, u*2).Children(func() {
			if st.valueKey == "" {
				mgo.Column(c).Fill().Center().Children(func() {
					mgo.Text(c, "选择左侧键查看值").FontSize(caption).TextColor(k.TextMuted)
				})
				return
			}
			mgo.Text(c, st.value.Text).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text)
		})
	})
}

// consoleView draws the command console.
func (cl *Client) consoleView(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	if !st.connected {
		cl.emptyView(c, "未连接。先点击「连接」。", k, caption)
		return
	}
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "命令").FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			t := minput.TextInput(c, &cl.st.cmdLine, minput.TextInputOptions{Placeholder: "如 GET key 或 INFO all"})
			t.Element.Grow(1).MinWidth(0)
			if t.Submitted() {
				cl.runCommand()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("terminal"), Label: "执行"}).Clicked() {
				cl.runCommand()
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Padding(u, u*2).Children(func() {
			if st.cmdOut == "" {
				mgo.Text(c, "(输入命令执行，输出显示在此)").FontSize(caption).TextColor(k.TextMuted)
				return
			}
			mgo.Text(c, st.cmdOut).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text)
		})
	})
}

// runCommand executes the console's command line.
func (cl *Client) runCommand() {
	cl.mu.Lock()
	conn := cl.st.conn
	line := cl.st.cmdLine
	cl.mu.Unlock()
	go func() {
		out, err := runCmd(conn, line)
		cl.mu.Lock()
		if err != nil {
			cl.st.cmdOut = "error: " + err.Error()
		} else {
			cl.st.cmdOut = out
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// serverView draws the server info.
func (cl *Client) serverView(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	if !st.connected {
		cl.emptyView(c, "未连接。先点击「连接」。", k, caption)
		return
	}
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u*2, u*3).Gap(u*2).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "服务器信息").FontWeight(600).FontSize(caption).Grow(1)
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
					cl.loadInfo()
				}
			})
			if st.infoText != "" {
				mgo.Box(c).FillWidth().Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u*2).Children(func() {
					mgo.Text(c, st.infoText).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text)
				})
			}
		})
	})
}

// emptyView draws a centered placeholder.
func (cl *Client) emptyView(c *mgo.Context, msg string, k mtheme.Tokens, caption float32) {
	mgo.Column(c).Fill().Center().Children(func() {
		mgo.Text(c, msg).FontSize(caption).TextColor(k.TextMuted)
	})
}

// pickKey selects a key and loads its value.
func (cl *Client) pickKey(i int) {
	cl.mu.Lock()
	cl.st.picked = i
	if i >= 0 && i < len(cl.st.keys) {
		key := cl.st.keys[i].Key
		cl.mu.Unlock()
		cl.loadValue(key)
		return
	}
	cl.mu.Unlock()
}

// keyTypeColor maps a redis type to a token color.
func keyTypeColor(k mtheme.Tokens, typ string) mgo.Color {
	switch typ {
	case "string":
		return k.Success
	case "list":
		return k.Accent
	case "set":
		return k.Warning
	case "zset":
		return k.Info
	case "hash":
		return k.TextMuted
	}
	return k.TextMuted
}

// fmtTTL renders a TTL duration.
func fmtTTL(d time.Duration) string {
	switch {
	case d < 0:
		return "∞"
	case d < time.Second:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
