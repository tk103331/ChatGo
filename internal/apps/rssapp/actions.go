package rssapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
)

// feedTitle returns a feed's display title by id.
func feedTitle(feeds []Feed, id string) string {
	for _, f := range feeds {
		if f.ID == id {
			if f.Title != "" {
				return f.Title
			}
			return f.URL
		}
	}
	return "未知订阅"
}

// itemKey returns a stable list key for an article.
func itemKey(it Item) string {
	if it.GUID != "" {
		return it.FeedID + "|" + it.GUID
	}
	if it.Link != "" {
		return it.FeedID + "|" + it.Link
	}
	return it.FeedID + "|" + it.Title
}

// formatDate renders an article date.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("01-02")
	}
	return t.Format("2006-01-02")
}

// cycleFeed switches the selected feed: all, then each feed in turn.
func (cl *Client) cycleFeed() {
	cl.mu.Lock()
	if cl.st.feedSel == "" {
		if len(cl.st.feeds) == 0 {
			cl.mu.Unlock()
			return
		}
		cl.st.feedSel = cl.st.feeds[0].ID
	} else {
		idx := -1
		for i, f := range cl.st.feeds {
			if f.ID == cl.st.feedSel {
				idx = i
				break
			}
		}
		if idx >= 0 && idx+1 < len(cl.st.feeds) {
			cl.st.feedSel = cl.st.feeds[idx+1].ID
		} else {
			cl.st.feedSel = ""
		}
	}
	cl.st.list = data.ListState[string]{}
	cl.st.picked = -1
	filter := cl.st.feedSel
	all := cl.st.items
	cl.mu.Unlock()
	cl.applyFilter(all, filter)
}

// applyFilter sets the visible items for a feed filter.
func (cl *Client) applyFilter(all []Item, filter string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if filter == "" {
		cl.st.items = all
	} else {
		var out []Item
		for _, it := range all {
			if it.FeedID == filter {
				out = append(out, it)
			}
		}
		cl.st.items = out
	}
	cl.st.picked = -1
	cl.st.list = data.ListState[string]{}
	cl.host.Invalidate()
}

// refresh fetches every feed concurrently.
func (cl *Client) refresh() {
	cl.mu.Lock()
	if cl.st.loading {
		cl.mu.Unlock()
		return
	}
	feeds := append([]Feed(nil), cl.st.feeds...)
	filter := cl.st.feedSel
	cl.st.loading = true
	cl.st.err = ""
	cl.mu.Unlock()
	if len(feeds) == 0 {
		cl.mu.Lock()
		cl.st.loading = false
		cl.mu.Unlock()
		cl.host.Invalidate()
		return
	}
	go func() {
		var all []Item
		var failures []string
		for _, f := range feeds {
			title, items, err := readFeed(f.URL)
			if err != nil {
				failures = append(failures, f.Title)
				continue
			}
			if title != "" && f.Title == "" || f.Title == f.URL {
				cl.mu.Lock()
				for i := range cl.st.feeds {
					if cl.st.feeds[i].ID == f.ID {
						cl.st.feeds[i].Title = title
					}
				}
				cl.mu.Unlock()
			}
			for i := range items {
				items[i].FeedID = f.ID
			}
			all = append(all, items...)
		}
		sortItems(all)
		cl.mu.Lock()
		cl.st.loading = false
		_ = saveStore(cl.st.feeds)
		if len(failures) > 0 {
			cl.st.err = "部分源刷新失败: " + strings.Join(failures, ", ")
		}
		cl.mu.Unlock()
		cl.applyFilter(all, filter)
	}()
}

// sortItems sorts articles newest first.
func sortItems(items []Item) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Date.After(items[j-1].Date); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// pickItem selects an article by list key.
func (cl *Client) pickItem(key string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for i, it := range cl.st.items {
		if itemKey(it) == key {
			cl.st.picked = i
			return
		}
	}
}

// openArticle opens the selected article's link.
func (cl *Client) openArticle() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.st.picked < 0 || cl.st.picked >= len(cl.st.items) {
		return
	}
	if link := cl.st.items[cl.st.picked].Link; link != "" {
		cl.host.OpenFile(link)
	}
}

// importOPML asks for an OPML file and merges its feeds.
func (cl *Client) importOPML() {
	path := cl.host.OpenFileDialog(cl.host.Root())
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		cl.setErr(err)
		return
	}
	feeds, err := parseOPML(data)
	if err != nil {
		cl.setErr(err)
		return
	}
	if len(feeds) == 0 {
		cl.setErr(fmt.Errorf("OPML 中没有订阅源"))
		return
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
	if added == 0 {
		cl.host.Toast("没有新的订阅源", mcore.SeverityInfo)
		return
	}
	cl.host.Toast(fmt.Sprintf("已导入 %d 个订阅源", added), mcore.SeveritySuccess)
	cl.refresh()
}

// exportOPML writes the feeds to a chosen file.
func (cl *Client) exportOPML() {
	cl.mu.Lock()
	feeds := append([]Feed(nil), cl.st.feeds...)
	cl.mu.Unlock()
	if len(feeds) == 0 {
		cl.setErr(fmt.Errorf("没有可导出的订阅源"))
		return
	}
	path := cl.host.SaveFileDialog("subscriptions.opml")
	if path == "" {
		return
	}
	if !strings.HasSuffix(strings.ToLower(path), ".opml") {
		path += ".opml"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		cl.setErr(err)
		return
	}
	if err := os.WriteFile(path, buildOPML(feeds), 0o644); err != nil {
		cl.setErr(err)
		return
	}
	cl.host.Toast("已导出 "+path, mcore.SeveritySuccess)
}

// confirmRemoveFeed removes a feed after confirmation.
func (cl *Client) confirmRemoveFeed(id string) {
	cl.mu.Lock()
	title := feedTitle(cl.st.feeds, id)
	cl.mu.Unlock()
	cl.host.Confirm("删除订阅源", "确定删除 "+title+" 吗？", func() {
		cl.mu.Lock()
		out := cl.st.feeds[:0]
		for _, f := range cl.st.feeds {
			if f.ID != id {
				out = append(out, f)
			}
		}
		cl.st.feeds = out
		if cl.st.feedSel == id {
			cl.st.feedSel = ""
		}
		var items []Item
		for _, it := range cl.st.items {
			if it.FeedID != id {
				items = append(items, it)
			}
		}
		cl.st.items = items
		cl.st.list = data.ListState[string]{}
		cl.st.picked = -1
		_ = saveStore(cl.st.feeds)
		cl.mu.Unlock()
		cl.host.Toast("已删除 "+title, mcore.SeveritySuccess)
		cl.host.Invalidate()
	})
}
