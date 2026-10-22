package rssapp

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
)

// Param is one tool parameter.
type Param struct {
	Desc     string
	Required bool
}

// Tool is one operation exposed to the Agent.
type Tool struct {
	Name   string
	Desc   string
	Params map[string]Param
	Run    func(args map[string]any) (string, error)
}

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "rss_feeds",
			Desc: "列出已订阅的 RSS 源",
			Run:  func(map[string]any) (string, error) { return cl.toolFeeds() },
		},
		{
			Name: "rss_add",
			Desc: "添加一个 RSS/Atom 订阅源",
			Params: map[string]Param{
				"url": {Desc: "订阅地址", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolAdd(strArg(args, "url"))
			},
		},
		{
			Name: "rss_import",
			Desc: "从 OPML 文件路径导入订阅源",
			Params: map[string]Param{
				"path": {Desc: "OPML 文件路径", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolImport(strArg(args, "path"))
			},
		},
		{
			Name: "rss_items",
			Desc: "列出文章（可按源过滤，默认最近 50 篇）",
			Params: map[string]Param{
				"feed":  {Desc: "订阅源标题或 ID，留空为全部"},
				"limit": {Desc: "条数，默认 50"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolItems(strArg(args, "feed"), strArg(args, "limit"))
			},
		},
		{
			Name: "rss_read",
			Desc: "读取一篇文章的正文（按序号，来自 rss_items）",
			Params: map[string]Param{
				"index": {Desc: "文章序号（从 1 开始）", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRead(strArg(args, "index"))
			},
		},
	}
}

// toolFeeds lists subscribed feeds.
func (cl *Client) toolFeeds() (string, error) {
	cl.mu.Lock()
	feeds := append([]Feed(nil), cl.st.feeds...)
	cl.mu.Unlock()
	if len(feeds) == 0 {
		return "(没有订阅源)", nil
	}
	var b strings.Builder
	for _, f := range feeds {
		fmt.Fprintf(&b, "- %s  %s\n", f.Title, f.URL)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolAdd adds a feed.
func (cl *Client) toolAdd(url string) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", fmt.Errorf("缺少必填参数 url")
	}
	cl.mu.Lock()
	for _, f := range cl.st.feeds {
		if f.URL == url {
			cl.mu.Unlock()
			return "", fmt.Errorf("该订阅源已存在")
		}
	}
	cl.mu.Unlock()
	title, items, err := readFeed(url)
	if err != nil {
		return "", err
	}
	cl.mu.Lock()
	feed := Feed{ID: newID(), URL: url, Title: title}
	if feed.Title == "" {
		feed.Title = url
	}
	cl.st.feeds = append(cl.st.feeds, feed)
	_ = saveStore(cl.st.feeds)
	cl.mu.Unlock()
	cl.host.Post(func() {
		cl.applyFilter(cl.st.items, cl.st.feedSel)
		cl.host.Toast("已添加 "+feed.Title, mcore.SeveritySuccess)
	})
	return fmt.Sprintf("已添加 %s（%d 篇文章）", feed.Title, len(items)), nil
}

// toolImport imports feeds from an OPML file.
func (cl *Client) toolImport(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	feeds, err := parseOPML(data)
	if err != nil {
		return "", err
	}
	cl.mu.Lock()
	existing := map[string]bool{}
	for _, f := range cl.st.feeds {
		existing[f.URL] = true
	}
	added := 0
	for _, f := range feeds {
		if existing[f.URL] {
			continue
		}
		f.ID = newID()
		if f.Title == "" {
			f.Title = f.URL
		}
		cl.st.feeds = append(cl.st.feeds, f)
		existing[f.URL] = true
		added++
	}
	_ = saveStore(cl.st.feeds)
	cl.mu.Unlock()
	cl.host.Post(func() {
		cl.host.Toast(fmt.Sprintf("已导入 %d 个源", added), mcore.SeveritySuccess)
		cl.refresh()
	})
	return fmt.Sprintf("导入完成，新增 %d 个订阅源（共 %d 个）", added, len(feeds)), nil
}

// toolItems lists articles.
func (cl *Client) toolItems(feed, limit string) (string, error) {
	cl.mu.Lock()
	items := append([]Item(nil), cl.st.items...)
	feeds := append([]Feed(nil), cl.st.feeds...)
	cl.mu.Unlock()
	if feed != "" {
		var filtered []Item
		for _, it := range items {
			if feedTitle(feeds, it.FeedID) == feed || it.FeedID == feed {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	n := 50
	if v, err := strconv.Atoi(strings.TrimSpace(limit)); err == nil && v > 0 {
		n = v
	}
	if len(items) > n {
		items = items[:n]
	}
	if len(items) == 0 {
		cl.host.Post(func() { cl.refresh() })
		return "(没有文章，正在刷新)", nil
	}
	var b strings.Builder
	for i, it := range items {
		fmt.Fprintf(&b, "%d. [%s] %s  %s\n", i+1, feedTitle(feeds, it.FeedID), it.Title, formatDate(it.Date))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolRead returns one article's text.
func (cl *Client) toolRead(index string) (string, error) {
	n, err := strconv.Atoi(strings.TrimSpace(index))
	if err != nil || n < 1 {
		return "", fmt.Errorf("无效序号 %q", index)
	}
	cl.mu.Lock()
	if n > len(cl.st.items) {
		cl.mu.Unlock()
		return "", fmt.Errorf("序号超出范围（共 %d 篇）", len(cl.st.items))
	}
	it := cl.st.items[n-1]
	feeds := append([]Feed(nil), cl.st.feeds...)
	cl.st.picked = n - 1
	cl.st.list = data.ListState[string]{}
	cl.mu.Unlock()
	cl.host.Post(func() { cl.host.Invalidate() })
	body := stripHTML(it.Content)
	if strings.TrimSpace(body) == "" {
		body = stripHTML(it.Summary)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "标题: %s\n来源: %s\n时间: %s\n链接: %s\n\n%s",
		it.Title, feedTitle(feeds, it.FeedID), formatDate(it.Date), it.Link, body)
	return b.String(), nil
}

// strArg reads a string argument.
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if b, err := json.Marshal(v); err == nil {
			return strings.Trim(string(b), "\"")
		}
	}
	return ""
}
