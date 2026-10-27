package ui

import (
	"fmt"
	"sort"
	"time"

	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// statsRange is the number of days shown by the daily charts.
const statsRange = 30

// dayUsage is the tokens and sessions recorded for one calendar day.
type dayUsage struct {
	day      time.Time
	input    int
	output   int
	total    int
	sessions int
	messages int
}

// statsSummary aggregates every stored conversation.
type statsSummary struct {
	sessions   int
	messages   int
	input      int
	output     int
	total      int
	activeDays int
	days       []dayUsage
	models     []statCount
	tools      int
}

// statCount is one labelled share of the total.
type statCount struct {
	name  string
	value int
}

// statsCache memoizes the aggregate until a conversation changes.
type statsCache struct {
	built   bool
	convN   int
	msgN    int
	stamp   time.Time
	summary statsSummary
}

// computeStats walks every conversation once and buckets usage by day.
func (cw *ChatWindow) computeStats() statsSummary {
	all := make([]models.Conversation, 0, len(cw.convs)+len(cw.taskConvs))
	all = append(all, cw.convs...)
	all = append(all, cw.taskConvs...)
	if len(all) == 0 {
		return statsSummary{}
	}
	sum := cw.statsCache
	msgN := 0
	var latest time.Time
	for _, conv := range all {
		msgN += len(conv.Messages)
		if last := convLast(conv); last.After(latest) {
			latest = last
		}
	}
	if sum.built && sum.convN == len(all) && sum.msgN == msgN && sum.stamp.Equal(latest) {
		return sum.summary
	}

	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	days := make([]dayUsage, statsRange)
	for i := range days {
		days[i].day = today.AddDate(0, 0, i-statsRange+1)
	}
	index := make(map[string]int, len(days))
	for i := range days {
		index[days[i].day.Format("2006-01-02")] = i
	}

	modelUsage := map[string]int{}
	var out statsSummary
	out.sessions = len(all)
	active := map[string]bool{}

	for _, conv := range all {
		if len(conv.Turns) > 0 {
			out.messages += len(conv.Messages)
			for _, t := range conv.Turns {
				if name := t.Model; name != "" {
					modelUsage[name] += t.Total
				}
				out.input += t.Input
				out.output += t.Output
				out.total += t.Total
				ts := t.StartedAt
				if ts.IsZero() {
					ts = conv.CreatedAt
				}
				key := ts.In(loc).Format("2006-01-02")
				active[key] = true
				if i, ok := index[key]; ok {
					days[i].input += t.Input
					days[i].output += t.Output
					days[i].total += t.Total
					days[i].messages++
				}
			}
			continue
		}
		for _, msg := range conv.Messages {
			out.messages++
			out.tools += len(msg.ToolCalls)
		}
	}
	// Sessions count once per day, by their last activity.
	for _, conv := range all {
		key := convLast(conv).In(loc).Format("2006-01-02")
		if i, ok := index[key]; ok {
			days[i].sessions++
		}
	}
	out.days = days
	out.activeDays = len(active)
	for name, n := range modelUsage {
		out.models = append(out.models, statCount{name: name, value: n})
	}
	sort.Slice(out.models, func(i, j int) bool {
		if out.models[i].value != out.models[j].value {
			return out.models[i].value > out.models[j].value
		}
		return out.models[i].name < out.models[j].name
	})
	if len(out.models) > 6 {
		out.models = out.models[:6]
	}

	cw.statsCache = statsCache{built: true, convN: len(all), msgN: msgN, stamp: latest, summary: out}
	return out
}

// statsPage shows usage statistics and daily charts.
func (cw *ChatWindow) statsPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	sum := cw.computeStats()

	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Column(c).Grow(1).Gap(u / 2).Children(func() {
				mgo.Text(c, "统计").
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Text(c, fmt.Sprintf("近 %d 天的使用概览", statsRange)).TextColor(k.TextMuted).FontSize(caption)
			})
			if kit.Button(c, "刷新", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("refresh-cw")}).Clicked() {
				cw.statsCache = statsCache{}
				cw.invalidate()
			}
		})

		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().AlignItems(mgo.Stretch).Children(func() {
				mgo.Column(c).FillWidth().MaxWidth(980).AlignSelf(mgo.Center).Gap(u * 3).Children(func() {
					cw.statsCards(c, sum)
					cw.statsTokenChart(c, sum)
					mgo.Row(c).FillWidth().AlignItems(mgo.Stretch).Gap(u * 3).Children(func() {
						cw.statsSessionChart(c, sum)
						cw.statsModelChart(c, sum)
					})
				})
			})
		})
	})
}

// statsCards shows the headline totals as a row of metric cards.
func (cw *ChatWindow) statsCards(c *mgo.Context, sum statsSummary) {
	u := mcore.Density(c).Unit()
	cards := []struct {
		label string
		value string
		hint  string
	}{
		{"会话数", fmt.Sprintf("%d", sum.sessions), fmt.Sprintf("%d 条消息", sum.messages)},
		{"总 Token", formatCount(sum.total), fmt.Sprintf("↑%s ↓%s", formatCount(sum.input), formatCount(sum.output))},
		{"活跃天数", fmt.Sprintf("%d", sum.activeDays), fmt.Sprintf("共 %d 天记录", sum.activeDays)},
		{"工具调用", fmt.Sprintf("%d", sum.tools), "累计调用次数"},
	}
	mgo.Row(c).FillWidth().Gap(u * 2).Children(func() {
		for _, card := range cards {
			cw.statsCard(c, card.label, card.value, card.hint)
		}
	})
}

// statsCard renders one metric tile.
func (cw *ChatWindow) statsCard(c *mgo.Context, label, value, hint string) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Column(c).Grow(1).MinWidth(0).Background(k.Surface).Radius(mtheme.ControlRadius).
		BorderWidth(mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u / 2).Children(func() {
		mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).SingleLine()
		mgo.Text(c, value).FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).SingleLine()
		mgo.Text(c, hint).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).SingleLine()
	})
}

// statsTokenChart draws daily input/output token consumption.
func (cw *ChatWindow) statsTokenChart(c *mgo.Context, sum statsSummary) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	labels := make([]string, len(sum.days))
	in := make([]float64, len(sum.days))
	out := make([]float64, len(sum.days))
	for i, d := range sum.days {
		labels[i] = d.day.Format("01-02")
		in[i] = float64(d.input)
		out[i] = float64(d.output)
	}
	mgo.Column(c).FillWidth().Background(k.Surface).Radius(mtheme.ControlRadius).
		BorderWidth(mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u).Children(func() {
		cw.statsSectionTitle(c, "每日 Token 消耗", "输入 / 输出堆叠")
		kit.BarChart(c, labels, []kit.ChartSeries{
			{Name: "输入", Values: in},
			{Name: "输出", Values: out},
		}, kit.BarChartOptions{
			Frame:   kit.ChartFrame{Label: "每日 Token 消耗", Height: 220},
			Stacked: true,
			Format:  func(v float64) string { return formatCount(int(v)) },
		})
	})
}

// statsSessionChart draws daily session counts as a line chart.
func (cw *ChatWindow) statsSessionChart(c *mgo.Context, sum statsSummary) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	labels := make([]string, len(sum.days))
	vals := make([]float64, len(sum.days))
	for i, d := range sum.days {
		labels[i] = d.day.Format("01-02")
		vals[i] = float64(d.sessions)
	}
	mgo.Column(c).Grow(1).MinWidth(0).Background(k.Surface).Radius(mtheme.ControlRadius).
		BorderWidth(mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u).Children(func() {
		cw.statsSectionTitle(c, "每日会话数", "按最后活动日统计")
		kit.BarChart(c, labels, []kit.ChartSeries{{Name: "会话", Values: vals}}, kit.BarChartOptions{
			Frame:  kit.ChartFrame{Label: "每日会话数", Height: 180},
			Format: func(v float64) string { return fmt.Sprintf("%.0f", v) },
		})
	})
}

// statsModelChart lists the most used models and their session counts.
func (cw *ChatWindow) statsModelChart(c *mgo.Context, sum statsSummary) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Grow(1).MinWidth(0).Background(k.Surface).Radius(mtheme.ControlRadius).
		BorderWidth(mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u).Children(func() {
		cw.statsSectionTitle(c, "模型 Token 分布", "各模型消耗的 Token")
		if len(sum.models) == 0 {
			mgo.Text(c, "暂无数据").FontSize(caption).TextColor(k.TextMuted)
			return
		}
		var top int
		for _, m := range sum.models {
			top = max(top, m.value)
		}
		// A model that ran but consumed no tokens still appears here, with a
		// zero value; its bar must not divide by zero, which would hand the
		// layout a NaN width and blank the whole page.
		if top == 0 {
			top = 1
		}
		for _, m := range sum.models {
			frac := float32(m.value) / float32(top) * 100
			mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
				mgo.Text(c, m.name).FontSize(caption).Width(140).SingleLine()
				mgo.Row(c).Grow(1).Height(8).Radius(4).Background(k.SurfaceHover).Children(func() {
					mgo.Row(c).FillHeight().WidthPercent(frac).Background(k.Accent).Radius(4)
				})
				mgo.Text(c, formatCount(m.value)).FontSize(caption).TextColor(k.TextMuted).
					Width(64).TextAlign(mgo.End).SingleLine()
			})
		}
	})
}

// statsSectionTitle renders a small section heading inside a card.
func (cw *ChatWindow) statsSectionTitle(c *mgo.Context, title, hint string) {
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Justify(mgo.SpaceBetween).Children(func() {
		mgo.Text(c, title).FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).SingleLine()
		mgo.Text(c, hint).FontSize(caption).TextColor(k.TextMuted).SingleLine()
	})
}

// formatCount renders a token count with thousands separators and k/M suffixes.
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
