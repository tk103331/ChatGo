// Package rssapp is the Apps menu's RSS reader: feed management, OPML
// import/export and article reading with Agent tools.
package rssapp

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Feed is one subscribed source.
type Feed struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Item is one article.
type Item struct {
	FeedID  string
	Title   string
	Link    string
	Summary string
	Content string
	Date    time.Time
	GUID    string
}

// rssDoc is an RSS 2.0/0.9x document.
type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Content     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// atomDoc is an Atom feed document.
type atomDoc struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Links   []atomLink `xml:"link"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
	Updated string     `xml:"updated"`
	ID      string     `xml:"id"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// httpClient fetches feeds with a bounded timeout.
var httpClient = &http.Client{Timeout: 20 * time.Second}

// readFeed fetches and parses a feed URL.
func readFeed(url string) (string, []Item, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "ChatGo RSS/1.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", nil, err
	}
	return parseFeed(data)
}

// parseFeed decodes RSS or Atom bytes into a title and items.
func parseFeed(data []byte) (string, []Item, error) {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		return "", nil, fmt.Errorf("不支持的 JSON Feed")
	}
	var rss rssDoc
	if err := xml.Unmarshal(data, &rss); err == nil && rss.XMLName.Local == "rss" {
		return rss.Channel.Title, rssItems(rss.Channel.Items), nil
	}
	var atom atomDoc
	if err := xml.Unmarshal(data, &atom); err == nil && atom.XMLName.Local == "feed" {
		return atom.Title, atomItems(atom.Entries), nil
	}
	return "", nil, fmt.Errorf("无法识别的订阅源格式")
}

// rssItems converts RSS items.
func rssItems(items []rssItem) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			content = it.Description
		}
		out = append(out, Item{
			Title:   cleanText(it.Title),
			Link:    strings.TrimSpace(it.Link),
			Summary: cleanText(it.Description),
			Content: content,
			Date:    parseDate(it.PubDate),
			GUID:    strings.TrimSpace(it.GUID),
		})
	}
	return out
}

// atomItems converts Atom entries.
func atomItems(entries []atomEntry) []Item {
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		link := ""
		for _, l := range e.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		content := strings.TrimSpace(e.Content)
		if content == "" {
			content = e.Summary
		}
		out = append(out, Item{
			Title:   cleanText(e.Title),
			Link:    link,
			Summary: cleanText(e.Summary),
			Content: content,
			Date:    parseDate(e.Updated),
			GUID:    strings.TrimSpace(e.ID),
		})
	}
	return out
}

// parseDate tries the common feed date layouts.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822,
		time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05",
		"Mon, 2 Jan 2006 15:04:05 -0700",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// cleanText trims whitespace and collapses newlines in a title.
func cleanText(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}

// stripHTML removes tags for list previews.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteByte(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
