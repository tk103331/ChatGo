package rssapp

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// storeFile is the on-disk feed list.
type storeFile struct {
	Feeds []Feed `json:"feeds"`
}

// storePath returns ~/.chatgo/rss.json.
func storePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "rss.json"), nil
}

// loadStore reads the saved feeds.
func loadStore() []Feed {
	path, err := storePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var st storeFile
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	return st.Feeds
}

// saveStore writes the feed list.
func saveStore(feeds []Feed) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(storeFile{Feeds: feeds}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// opmlDoc is the OPML import/export shape.
type opmlDoc struct {
	XMLName xml.Name  `xml:"opml"`
	Version string    `xml:"version,attr"`
	Title   string    `xml:"head>title"`
	Body    opmlBody  `xml:"body"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	HTMLURL  string        `xml:"htmlUrl,attr"`
	Children []opmlOutline `xml:"outline"`
}

// name returns the outline's display label.
func (o opmlOutline) name() string {
	if o.Text != "" {
		return o.Text
	}
	if o.Title != "" {
		return o.Title
	}
	return o.XMLURL
}

// parseOPML extracts feeds from OPML bytes, walking nested outlines.
func parseOPML(data []byte) ([]Feed, error) {
	var doc opmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("解析 OPML 失败: %w", err)
	}
	var out []Feed
	var walk func(list []opmlOutline)
	walk = func(list []opmlOutline) {
		for _, o := range list {
			if u := strings.TrimSpace(o.XMLURL); u != "" {
				out = append(out, Feed{URL: u, Title: o.name()})
			}
			if len(o.Children) > 0 {
				walk(o.Children)
			}
		}
	}
	walk(doc.Body.Outlines)
	if len(out) == 0 && doc.XMLName.Local != "opml" {
		return nil, fmt.Errorf("不是有效的 OPML 文件")
	}
	return out, nil
}

// buildOPML renders feeds as an OPML document.
func buildOPML(feeds []Feed) []byte {
	doc := opmlDoc{Version: "2.0", Title: "ChatGo RSS"}
	for _, f := range feeds {
		doc.Body.Outlines = append(doc.Body.Outlines, opmlOutline{
			Text: f.Title, Title: f.Title, Type: "rss", XMLURL: f.URL,
		})
	}
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil
	}
	return append([]byte(xml.Header), data...)
}
