package kit

import (
	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// ChartSeries is one named run of values, one per category.
type ChartSeries struct {
	Name   string
	Values []float64
}

// ChartFrame is the shell around a plot.
type ChartFrame struct {
	// Label names the chart for assistive technology; required.
	Label string
	// Height is the plot height in DIPs; zero means 240.
	Height float32
	// Empty replaces the built-in "暂无数据" text.
	Empty string
}

// BarChartOptions configure a BarChart.
type BarChartOptions struct {
	Frame ChartFrame
	// Stacked piles series into one bar per category.
	Stacked bool
	// Format writes a value for the axis and the tooltips.
	Format func(v float64) string
}

// chartSeriesColors returns the colors of n series, derived from the theme so
// they follow the window's palette.
func chartSeriesColors(k theme.Tokens, n int) []mgo.Color {
	base := []mgo.Color{k.Accent, k.Info, k.Success, k.Warning, k.Ornament}
	out := make([]mgo.Color, n)
	for i := range out {
		c := base[i%len(base)]
		// Past the base colors, lighten each round so runs stay apart.
		if round := i / len(base); round > 0 {
			c = c.Mix(k.Surface, min(0.2*float32(round), 0.6))
		}
		out[i] = c
	}
	return out
}

// BarChart draws one bar per category, the series side by side or stacked.
func BarChart(c *mgo.Context, labels []string, series []ChartSeries, opts BarChartOptions) mgo.Element {
	if opts.Frame.Label == "" {
		panic("kit: BarChart needs a Frame Label")
	}
	k := uikit.Tokens(c)
	height := opts.Frame.Height
	if height == 0 {
		height = 240
	}
	format := opts.Format
	if format == nil {
		format = func(v float64) string { return chart.NewLinear(0, 1).Format(v) }
	}
	if len(labels) == 0 || len(series) == 0 {
		return chartEmpty(c, opts.Frame, height)
	}

	values := make([][]float64, len(series))
	for i, s := range series {
		values[i] = s.Values
	}
	colors := chartSeriesColors(k, len(series))

	hi := 0.0
	var stacked [][]chart.Segment
	if opts.Stacked {
		stacked = chart.Stack(values, chart.StackStandard)
		for _, seg := range stacked[len(stacked)-1] {
			hi = max(hi, seg.Hi)
		}
	} else {
		for _, v := range values {
			if _, h := chart.Extent(v...); h > hi {
				hi = h
			}
		}
	}
	if hi == 0 {
		hi = 1
	}

	x := chart.NewBand(labels...).Padding(0.3, 0.1)
	y := chart.NewLinear(0, hi).Nice(4)
	y.Format = format
	ticks := y.Ticks(4)
	// Labels thin out so they keep apart on a narrow plot.
	every := 1 + len(labels)/8
	var cats []chart.Tick
	for i, tick := range x.Ticks() {
		if i%every == 0 {
			cats = append(cats, tick)
		}
	}

	axes := chart.Axes{Left: ticks, Bottom: cats, Label: chartAxisLabel(c)}
	plot := chart.Plot(c, axes, func(area mgo.Element) {
		chartGrid(area, ticks, k.Border)
		for si := range series {
			for i := range labels {
				if i >= len(values[si]) {
					break
				}
				v := values[si][i]
				lo, high := 0.0, v
				band := x.Band(i)
				if opts.Stacked {
					seg := stacked[si][i]
					lo, high = seg.Lo, seg.Hi
				} else if len(series) > 1 {
					band = subBand(band, si, len(series))
				}
				chart.Rect(c.Key(si*len(labels)+i), band, y.Span(lo, high)).
					Radius(2).Background(colors[si]).
					Tooltip(series[si].Name + ": " + format(v)).
					Label(labels[i] + " " + series[si].Name)
			}
		}
	})
	plot.Label(opts.Frame.Label).Height(height).GapX(8).GapY(6)
	if len(series) > 1 {
		chartLegend(c, series, colors)
	}
	return plot
}

// subBand splits a category's band into n side-by-side slots and returns
// the i-th, so grouped series stand beside each other.
func subBand(b chart.Span, i, n int) chart.Span {
	w := (b.Hi - b.Lo) / float32(n)
	lo := b.Lo + w*float32(i)
	return chart.Span{Lo: lo, Hi: lo + w}
}

// chartEmpty stands in for a plot with no data.
func chartEmpty(c *mgo.Context, frame ChartFrame, height float32) mgo.Element {
	k := uikit.Tokens(c)
	text := frame.Empty
	if text == "" {
		text = "暂无数据"
	}
	return mgo.Column(c).FillWidth().Height(height).Center().
		Role(mgo.RoleImage).Label(frame.Label).Children(func() {
		mgo.Text(c, text).FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)
	})
}

// chartAxisLabel is the look of the labels of a chart's axes.
func chartAxisLabel(c *mgo.Context) func(chart.Side, chart.Tick) {
	k := uikit.Tokens(c)
	size := uikit.FontSize(c, theme.CaptionSize-1)
	return func(_ chart.Side, tick chart.Tick) {
		mgo.Text(c, tick.Label).NoWrap().FontSize(size).TextColor(k.TextMuted)
	}
}

// chartGrid paints a line across the plot at each tick of y.
func chartGrid(area mgo.Element, y []chart.Tick, color mgo.Color) {
	area.Draw(func(p *mgo.Painter, r mgo.Rect) {
		for _, tick := range y {
			p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, color)
		}
	})
}

// chartLegend shows the series' names by their colors.
func chartLegend(c *mgo.Context, series []ChartSeries, colors []mgo.Color) {
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	size := uikit.FontSize(c, theme.CaptionSize)
	mgo.Row(c).Gap(u * 3).Wrap().Children(func() {
		for i, s := range series {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Box(c).Size(10, 10).Radius(2).Background(colors[i])
				mgo.Text(c, s.Name).FontSize(size).TextColor(k.TextMuted)
			})
		}
	})
}
