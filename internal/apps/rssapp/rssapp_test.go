package rssapp

import (
	"strings"
	"testing"
)

func TestParseFeedRSS(t *testing.T) {
	raw := `<?xml version="1.0"?><rss version="2.0"><channel>
<title>Example Feed</title>
<item><title>First</title><link>https://example.com/1</link><description>Hello &lt;b&gt;world&lt;/b&gt;</description><pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate><guid>g1</guid></item>
<item><title>Second</title><link>https://example.com/2</link><description>Two</description></item>
</channel></rss>`
	title, items, err := parseFeed([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if title != "Example Feed" {
		t.Errorf("title = %q", title)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Title != "First" || items[0].Link != "https://example.com/1" || items[0].GUID != "g1" {
		t.Errorf("item0 = %+v", items[0])
	}
	if items[0].Date.IsZero() {
		t.Error("item0 date not parsed")
	}
	if !strings.Contains(items[0].Summary, "Hello") {
		t.Errorf("summary = %q", items[0].Summary)
	}
}

func TestParseFeedAtom(t *testing.T) {
	raw := `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
<title>Atom Feed</title>
<entry><title>Entry A</title><link href="https://a.example/1" rel="alternate"/><summary>Summary A</summary><updated>2024-05-06T07:08:09Z</updated><id>id-a</id></entry>
</feed>`
	title, items, err := parseFeed([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if title != "Atom Feed" {
		t.Errorf("title = %q", title)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].Link != "https://a.example/1" || items[0].GUID != "id-a" {
		t.Errorf("item = %+v", items[0])
	}
	if items[0].Date.IsZero() {
		t.Error("atom date not parsed")
	}
}

func TestParseFeedUnknown(t *testing.T) {
	if _, _, err := parseFeed([]byte("<html><body>x</body></html>")); err == nil {
		t.Error("html should fail")
	}
	if _, _, err := parseFeed([]byte(`{"version":"https://jsonfeed.org/version/1"}`)); err == nil {
		t.Error("json feed should fail with a clear message")
	}
}

func TestParseOPML(t *testing.T) {
	raw := `<?xml version="1.0"?><opml version="2.0"><head><title>Subs</title></head><body>
<outline text="Tech">
  <outline text="Go Blog" type="rss" xmlUrl="https://blog.golang.org/feed.atom"/>
  <outline text="HN" type="rss" xmlUrl="https://news.ycombinator.com/rss"/>
</outline>
<outline text="No URL"/>
</body></opml>`
	feeds, err := parseOPML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 2 {
		t.Fatalf("feeds = %d, want 2", len(feeds))
	}
	if feeds[0].URL != "https://blog.golang.org/feed.atom" || feeds[0].Title != "Go Blog" {
		t.Errorf("feed0 = %+v", feeds[0])
	}
}

func TestBuildOPMLRoundTrip(t *testing.T) {
	feeds := []Feed{
		{Title: "A", URL: "https://a.example/rss"},
		{Title: "B", URL: "https://b.example/rss"},
	}
	data := buildOPML(feeds)
	got, err := parseOPML(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].URL != "https://b.example/rss" || got[1].Title != "B" {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestStripHTML(t *testing.T) {
	in := "<p>Hello <b>world</b></p><br/>line2"
	got := stripHTML(in)
	if strings.Contains(got, "<") || !strings.Contains(got, "Hello") || !strings.Contains(got, "line2") {
		t.Errorf("stripHTML = %q", got)
	}
}

func TestItemKey(t *testing.T) {
	it := Item{FeedID: "f1", GUID: "g1", Link: "https://x/1", Title: "t"}
	if got := itemKey(it); got != "f1|g1" {
		t.Errorf("itemKey = %q", got)
	}
	it.GUID = ""
	if got := itemKey(it); got != "f1|https://x/1" {
		t.Errorf("itemKey fallback = %q", got)
	}
}
