package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// maxViewBytes bounds a declarative view envelope, so a plugin cannot make the
// UI parse an unbounded payload.
const maxViewBytes = 64 << 10

// viewSpec is one validated declarative view attached to a tool result.
type viewSpec struct {
	Type  string
	Title string
	Data  map[string]any
}

// parseView reads a {"$chatgo":{"v":1,"view":{...}}} envelope. Anything
// malformed or unsupported returns false, and the caller shows the raw text.
func parseView(result string) (*viewSpec, bool) {
	if len(result) > maxViewBytes {
		return nil, false
	}
	var env struct {
		Chatgo *struct {
			V    int             `json:"v"`
			View json.RawMessage `json:"view"`
		} `json:"$chatgo"`
	}
	if err := json.Unmarshal([]byte(result), &env); err != nil {
		return nil, false
	}
	if env.Chatgo == nil || env.Chatgo.V != 1 || len(env.Chatgo.View) == 0 {
		return nil, false
	}
	var data map[string]any
	if err := json.Unmarshal(env.Chatgo.View, &data); err != nil {
		return nil, false
	}
	typ, _ := data["type"].(string)
	switch typ {
	case "markdown", "table", "keyvalue", "list", "chart", "code", "diff", "links", "image":
	default:
		return nil, false
	}
	title, _ := data["title"].(string)
	return &viewSpec{Type: typ, Title: title, Data: data}, true
}

// renderView draws one declarative view with the theme's own components.
func (cw *ChatWindow) renderView(c *mgo.Context, v *viewSpec) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).FillWidth().Gap(u).Padding(u).Radius(mtheme.ControlRadius).
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Children(func() {
		if v.Title != "" {
			mgo.Text(c, v.Title).FontWeight(600).FontSize(caption).TextColor(k.TextMuted)
		}
		switch v.Type {
		case "markdown":
			kit.MarkdownView(c, stringField(v.Data, "text", 20000))
		case "table":
			cw.renderTableView(c, v.Data)
		case "keyvalue":
			cw.renderKeyValueView(c, v.Data)
		case "list":
			cw.renderListView(c, v.Data)
		case "chart":
			cw.renderChartView(c, v.Data)
		case "code":
			code := stringField(v.Data, "code", 20000)
			lang := stringField(v.Data, "language", 40)
			if code != "" {
				kit.CodeBlock(c, code, kit.CodeBlockOptions{Language: lang, Wrap: false})
			}
		case "diff":
			cw.renderDiffView(c, v.Data)
		case "links":
			cw.renderLinksView(c, v.Data)
		case "image":
			cw.renderImageView(c, v.Data)
		}
	})
}

func (cw *ChatWindow) renderTableView(c *mgo.Context, data map[string]any) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cols := sliceField(data, "columns")
	rows := sliceField(data, "rows")
	if len(cols) == 0 || len(cols) > 12 {
		mgo.Text(c, "表格列数非法").FontSize(caption).TextColor(k.Danger)
		return
	}
	labels := make([]string, 0, len(cols))
	for _, col := range cols {
		m, _ := col.(map[string]any)
		label, _ := m["label"].(string)
		if label == "" {
			label, _ = m["key"].(string)
		}
		labels = append(labels, truncate(label, 40))
	}
	if len(rows) > 500 {
		rows = rows[:500]
	}
	mgo.Scroll(c).FillWidth().Shrink(0).MaxHeight(caption * 1.6 * 16).Children(func() {
		mgo.Column(c).FillWidth().Gap(u / 2).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).Children(func() {
				for _, label := range labels {
					mgo.Text(c, label).Grow(1).MinWidth(0).SingleLine().
						FontWeight(600).FontSize(caption).TextColor(k.TextMuted)
				}
			})
			mgo.Divider(c)
			for _, row := range rows {
				cells, _ := row.([]any)
				mgo.Row(c).FillWidth().Gap(u).Children(func() {
					for i := range labels {
						cell := ""
						if i < len(cells) {
							cell = fmt.Sprint(cells[i])
						}
						mgo.Text(c, truncate(cell, 500)).Grow(1).MinWidth(0).FontSize(caption)
					}
				})
			}
		})
	})
}

func (cw *ChatWindow) renderKeyValueView(c *mgo.Context, data map[string]any) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	items := sliceField(data, "items")
	if len(items) > 100 {
		items = items[:100]
	}
	for _, item := range items {
		m, _ := item.(map[string]any)
		label, _ := m["label"].(string)
		value := fmt.Sprint(m["value"])
		mgo.Row(c).FillWidth().Gap(u).Children(func() {
			mgo.Text(c, truncate(label, 80)).Shrink(0).MinWidth(0).FontSize(caption).TextColor(k.TextMuted)
			mgo.Text(c, truncate(value, 2000)).Grow(1).MinWidth(0).FontSize(caption)
		})
	}
}

func (cw *ChatWindow) renderListView(c *mgo.Context, data map[string]any) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	items := sliceField(data, "items")
	if len(items) > 200 {
		items = items[:200]
	}
	for _, item := range items {
		m, _ := item.(map[string]any)
		title, _ := m["title"].(string)
		detail, _ := m["detail"].(string)
		badge, _ := m["badge"].(string)
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Icon(c, icons.Must("circle")).Size(6, 6).TextColor(k.Accent).Shrink(0)
			mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
				mgo.Text(c, truncate(title, 200)).FontSize(caption)
				if detail != "" {
					mgo.Text(c, truncate(detail, 500)).FontSize(caption).TextColor(k.TextMuted)
				}
			})
			if badge != "" {
				mgo.Text(c, truncate(badge, 40)).FontSize(caption).TextColor(k.Accent)
			}
		})
	}
}

func (cw *ChatWindow) renderChartView(c *mgo.Context, data map[string]any) {
	labels := make([]string, 0)
	for _, l := range sliceField(data, "labels") {
		labels = append(labels, truncate(fmt.Sprint(l), 40))
		if len(labels) >= 100 {
			break
		}
	}
	var series []kit.ChartSeries
	for _, s := range sliceField(data, "series") {
		m, _ := s.(map[string]any)
		name, _ := m["name"].(string)
		var values []float64
		for _, v := range sliceField(m, "values") {
			values = append(values, toFloat(v))
		}
		series = append(series, kit.ChartSeries{Name: name, Values: values})
		if len(series) >= 4 {
			break
		}
	}
	if len(labels) == 0 || len(series) == 0 {
		mgo.Text(c, "图表没有数据").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
		return
	}
	stacked, _ := data["stacked"].(bool)
	kit.BarChart(c, labels, series, kit.BarChartOptions{
		Frame: kit.ChartFrame{Label: "工具结果图表", Height: 200}, Stacked: stacked,
	})
}

func (cw *ChatWindow) renderDiffView(c *mgo.Context, data map[string]any) {
	path := stringField(data, "path", 300)
	before := stringField(data, "before", 20000)
	after := stringField(data, "after", 20000)
	if path != "" {
		mgo.Text(c, path).Font(mtheme.MonoFont).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
			TextColor(mcore.Tokens(c).TextMuted)
	}
	if before != "" {
		kit.CodeBlock(c, before, kit.CodeBlockOptions{Language: "diff"})
	}
	if after != "" {
		kit.CodeBlock(c, after, kit.CodeBlockOptions{Language: "diff"})
	}
}

func (cw *ChatWindow) renderLinksView(c *mgo.Context, data map[string]any) {
	var b strings.Builder
	for _, item := range sliceField(data, "items") {
		m, _ := item.(map[string]any)
		title, _ := m["title"].(string)
		link, _ := m["url"].(string)
		if !safeWebURL(link) {
			continue
		}
		if title == "" {
			title = link
		}
		fmt.Fprintf(&b, "- [%s](%s)\n", title, link)
	}
	if b.Len() > 0 {
		kit.MarkdownView(c, b.String())
	}
}

func (cw *ChatWindow) renderImageView(c *mgo.Context, data map[string]any) {
	src := stringField(data, "src", 2000)
	alt := stringField(data, "alt", 200)
	if !safeWebURL(src) {
		mgo.Text(c, "图片地址不受支持（仅 https）").
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).Danger)
		return
	}
	if alt == "" {
		alt = "图片"
	}
	kit.MarkdownView(c, "!["+alt+"]("+src+")")
}

// safeWebURL allows only https links, matching the plugin security model.
func safeWebURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Host != ""
}

func stringField(data map[string]any, key string, limit int) string {
	s, _ := data[key].(string)
	return truncate(s, limit)
}

func sliceField(data map[string]any, key string) []any {
	if data == nil {
		return nil
	}
	v, _ := data[key].([]any)
	return v
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}
