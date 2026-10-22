package rssapp

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是 RSS 阅读助手。你可以通过 rss_feeds 列出订阅源，通过 rss_add 添加订阅源，通过 rss_import 导入 OPML，通过 rss_items 列出文章，通过 rss_read 读取文章正文。添加与导入前先向用户确认。"

// state is the client's retained model.
type state struct {
	feeds   []Feed
	feedSel string
	items   []Item
	picked  int
	loading bool
	err     string
	list    data.ListState[string]

	adding bool
	newURL string

	splitW float32
}

// Client is one RSS reader bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	st   *state
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns an RSS reader driving the given host.
func New(h Host) *Client {
	feeds := loadStore()
	cl := &Client{host: h, st: &state{feeds: feeds, picked: -1, splitW: 420}}
	return cl
}

// View renders the RSS reader.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	st := cl.snapshot()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			label := "全部订阅"
			if st.feedSel != "" {
				label = feedTitle(st.feeds, st.feedSel)
			}
			if minput.Button(c, label, minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("list")}).Clicked() {
				cl.cycleFeed()
			}
			mgo.Text(c, fmt.Sprintf("%d 个源 · %d 篇", len(st.feeds), len(st.items))).
				FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.refresh()
			}
			mgo.Box(c).Grow(1)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus"), Label: "添加源"}).Clicked() {
				cl.openAdd()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("download"), Label: "导入 OPML"}).Clicked() {
				cl.importOPML()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("upload"), Label: "导出 OPML"}).Clicked() {
				cl.exportOPML()
			}
			if st.feedSel != "" {
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("trash-2"), Label: "删除源"}).Clicked() {
					cl.confirmRemoveFeed(st.feedSel)
				}
			}
		})
		if st.err != "" {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mgo.Text(c, st.err).FontSize(caption).TextColor(k.Danger).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			})
		}
		if st.loading {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "刷新中…").FontSize(caption).TextColor(k.TextMuted)
			})
		}
		if len(st.feeds) == 0 {
			mgo.Column(c).Fill().Center().Children(func() {
				mgo.Text(c, "还没有订阅源。点击「添加源」或「导入 OPML」").FontSize(caption).TextColor(k.TextMuted)
			})
		} else {
			mgo.Split(c, &cl.st.splitW, func() {
				cl.listPane(c, st, k, u, caption)
			}, func() {
				cl.readPane(c, st, k, u, caption)
			}).Grow(1).MinHeight(0)
		}
	})
	cl.addDialog(c, st, k, u, caption)
}

// snapshot returns a copy of the state for rendering.
func (cl *Client) snapshot() *state {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cp := *cl.st
	return &cp
}

// listPane draws the article list.
func (cl *Client) listPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	entries := st.items
	v := data.List(c, &cl.st.list, len(entries), data.ListOptions[string]{
		Key:   func(i int) string { return itemKey(entries[i]) },
		Label: func(i int) string { return entries[i].Title },
		Empty: "没有文章，点击刷新",
	}, func(i int) {
		it := entries[i]
		mgo.Column(c).FillWidth().MinWidth(0).Gap(2).Children(func() {
			title := it.Title
			if strings.TrimSpace(title) == "" {
				title = "(无标题)"
			}
			mgo.Text(c, title).FontSize(caption).FontWeight(600).SingleLine().Ellipsis("…")
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, feedTitle(st.feeds, it.FeedID)).FontSize(caption).
					TextColor(k.AccentText).Shrink(0).SingleLine().Ellipsis("…")
				mgo.Text(c, formatDate(it.Date)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				mgo.Box(c).Grow(1)
			})
		})
	})
	v.Element.FillWidth().Grow(1).MinWidth(0).MinHeight(0)
	if v.Changed() {
		if key, ok := cl.st.list.Selected(); ok {
			cl.pickItem(key)
		}
	}
	if v.Submitted() {
		cl.openArticle()
	}
}

// readPane draws the selected article.
func (cl *Client) readPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		if st.picked < 0 || st.picked >= len(st.items) {
			mgo.Column(c).Fill().Center().Children(func() {
				mgo.Text(c, "选择一篇文章阅读").FontSize(caption).TextColor(k.TextMuted)
			})
			return
		}
		it := st.items[st.picked]
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			mgo.Text(c, feedTitle(st.feeds, it.FeedID)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			mgo.Box(c).Grow(1)
			if it.Link != "" {
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("globe"), Label: "打开原文"}).Clicked() {
					cl.host.OpenFile(it.Link)
				}
			}
		})
		mgo.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().Padding(u*2).Gap(u).Children(func() {
				mgo.Text(c, it.Title).FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(600)
				if !it.Date.IsZero() {
					mgo.Text(c, it.Date.Format("2006-01-02 15:04")).FontSize(caption).TextColor(k.TextMuted)
				}
				mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Background(k.Border)
				body := it.Content
				if strings.TrimSpace(body) == "" {
					body = it.Summary
				}
				body = stripHTML(body)
				if strings.TrimSpace(body) == "" {
					mgo.Text(c, "(无正文)").FontSize(caption).TextColor(k.TextMuted)
					return
				}
				mgo.Text(c, body).FontSize(caption).TextColor(k.Text)
			})
		})
	})
}

// addDialog shows the add-feed dialog.
func (cl *Client) addDialog(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	overlay.Dialog(c, &cl.st.adding, overlay.DialogOptions{
		Title:       "添加订阅源",
		Description: "输入 RSS 或 Atom 地址",
		Width:       520,
		Actions: func() {
			if minput.Button(c, "取消", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cl.closeAdd()
			}
			if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cl.submitAdd()
			}
		},
	}, func() {
		mgo.Column(c).FillWidth().Gap(u).Children(func() {
			minput.TextInput(c, &cl.st.newURL, minput.TextInputOptions{
				Placeholder: "https://example.com/feed.xml",
			}).Element.FillWidth()
			mgo.Text(c, "支持 RSS 2.0 与 Atom，也支持直接粘贴 OPML 中导出的地址").
				FontSize(caption).TextColor(k.TextMuted)
		})
	})
}

// openAdd shows the add dialog.
func (cl *Client) openAdd() {
	cl.mu.Lock()
	cl.st.adding = true
	cl.st.newURL = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// closeAdd hides the add dialog.
func (cl *Client) closeAdd() {
	cl.mu.Lock()
	cl.st.adding = false
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// submitAdd validates and stores the new feed.
func (cl *Client) submitAdd() {
	cl.mu.Lock()
	url := strings.TrimSpace(cl.st.newURL)
	cl.mu.Unlock()
	if url == "" {
		cl.setErr(fmt.Errorf("请输入订阅地址"))
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		cl.setErr(fmt.Errorf("地址需以 http:// 或 https:// 开头"))
		return
	}
	cl.mu.Lock()
	for _, f := range cl.st.feeds {
		if f.URL == url {
			cl.mu.Unlock()
			cl.setErr(fmt.Errorf("该订阅源已存在"))
			return
		}
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
	go func() {
		title, items, err := readFeed(url)
		if err != nil {
			cl.host.Post(func() { cl.setErr(fmt.Errorf("订阅源无效: %s", shortErr(err))) })
			return
		}
		cl.host.Post(func() {
			cl.mu.Lock()
			feed := Feed{ID: newID(), URL: url, Title: title}
			if feed.Title == "" {
				feed.Title = url
			}
			cl.st.feeds = append(cl.st.feeds, feed)
			cl.st.adding = false
			cl.st.items = mergeItems(cl.st.items, feed.ID, items)
			cl.st.err = ""
			_ = saveStore(cl.st.feeds)
			cl.mu.Unlock()
			cl.host.Toast("已添加 "+title, mcore.SeveritySuccess)
			cl.host.Invalidate()
		})
	}()
}

// setErr shows a one-line error.
func (cl *Client) setErr(err error) {
	cl.mu.Lock()
	cl.st.err = err.Error()
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// shortErr trims an error to one line.
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

// newID returns a short unique id.
func newID() string {
	return fmt.Sprintf("f%d", time.Now().UnixNano())
}

// mergeItems merges a feed's items into the cache, sorted newest first.
func mergeItems(existing []Item, feedID string, items []Item) []Item {
	out := existing[:0]
	for _, it := range existing {
		if it.FeedID != feedID {
			out = append(out, it)
		}
	}
	for _, it := range items {
		it.FeedID = feedID
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out
}
