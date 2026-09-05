package chart

import (
	"fmt"
	"strings"
	"time"

	"github.com/Goonie-Software/steamdb/internal/steam"
)

// Period selects how much history to show.
type Period string

const (
	Period24h Period = "24h"
	Period7d  Period = "7d"
	Period30d Period = "30d"
	Period90d Period = "90d"
	Period1y  Period = "1y"
	PeriodAll Period = "all"
)

func ParsePeriod(s string) (Period, error) {
	p := Period(strings.ToLower(strings.TrimSpace(s)))
	switch p {
	case Period24h, Period7d, Period30d, Period90d, Period1y, PeriodAll:
		return p, nil
	default:
		return "", fmt.Errorf("invalid period %q (use 24h, 7d, 30d, 90d, 1y, all)", s)
	}
}

func (p Period) Duration() time.Duration {
	switch p {
	case Period24h:
		return 24 * time.Hour
	case Period7d:
		return 7 * 24 * time.Hour
	case Period30d:
		return 30 * 24 * time.Hour
	case Period90d:
		return 90 * 24 * time.Hour
	case Period1y:
		return 365 * 24 * time.Hour
	default:
		return 0
	}
}

// FilterByPeriod keeps points within the selected window (ending at the latest sample).
// Zero-count samples are dropped — SteamCharts occasionally emits gap placeholders.
func FilterByPeriod(points []steam.Point, period Period) []steam.Point {
	if len(points) == 0 {
		return points
	}

	end := points[len(points)-1].Time
	var start time.Time
	if period != PeriodAll {
		start = end.Add(-period.Duration())
	}

	out := make([]steam.Point, 0, len(points))
	for _, p := range points {
		if period != PeriodAll && p.Time.Before(start) {
			continue
		}
		if p.Count <= 0 {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return points[len(points)-1:]
	}
	return out
}

// Resample maps points onto exactly width columns (average down, interpolate up).
func Resample(points []steam.Point, width int) []steam.Point {
	if width <= 0 || len(points) == 0 {
		return points
	}
	if len(points) == width {
		return points
	}
	if len(points) > width {
		out := make([]steam.Point, 0, width)
		for i := 0; i < width; i++ {
			start := i * len(points) / width
			end := (i + 1) * len(points) / width
			if end <= start {
				end = start + 1
			}
			sum := 0
			for _, p := range points[start:end] {
				sum += p.Count
			}
			mid := points[start+(end-start)/2]
			out = append(out, steam.Point{
				Time:  mid.Time,
				Count: sum / (end - start),
			})
		}
		return out
	}

	// Upsample: linear interpolate counts across columns.
	out := make([]steam.Point, width)
	last := len(points) - 1
	for i := 0; i < width; i++ {
		pos := float64(i) * float64(last) / float64(width-1)
		i0 := int(pos)
		i1 := i0 + 1
		if i1 > last {
			i1 = last
		}
		frac := pos - float64(i0)
		c0, c1 := points[i0].Count, points[i1].Count
		count := int(float64(c0)*(1-frac) + float64(c1)*frac)
		t0, t1 := points[i0].Time, points[i1].Time
		ts := t0.Add(time.Duration(float64(t1.Sub(t0)) * frac))
		out[i] = steam.Point{Time: ts, Count: count}
	}
	return out
}

// Options control ASCII chart rendering.
type Options struct {
	Width      int
	Height     int
	Title      string
	Color      bool
	Theme      Theme
	Current    int // live player count; <0 to omit
	ShowLegend bool
}

// Render draws a btop-style boxed graph of concurrent players.
func Render(points []steam.Point, opts Options) string {
	if len(points) == 0 {
		return "no data"
	}
	if opts.Width < 24 {
		opts.Width = 24
	}
	if opts.Height < 4 {
		opts.Height = 4
	}
	th := opts.Theme
	if th == (Theme{}) {
		th = DefaultTheme()
	}
	color := opts.Color

	sampled := Resample(points, opts.Width)
	minV, maxV := sampled[0].Count, sampled[0].Count
	sum := 0
	for _, p := range sampled {
		sum += p.Count
		if p.Count < minV {
			minV = p.Count
		}
		if p.Count > maxV {
			maxV = p.Count
		}
	}
	if maxV == minV {
		maxV = minV + 1
	}
	avgV := sum / len(sampled)
	last := points[len(points)-1]
	graphW := len(sampled)

	labelW := len(formatCount(maxV))
	if w := len(formatCount(minV)); w > labelW {
		labelW = w
	}
	if w := len(formatCount(avgV)); w > labelW {
		labelW = w
	}

	// Layout: │<label><gap>│<graph>│  → inner = label + gap + 1 + graph
	const gap = 1
	innerW := labelW + gap + 1 + graphW

	title := opts.Title
	if title == "" {
		title = "players"
	}
	// Grow box if title needs more room
	titleNeed := 2 + visibleLen(title) + 2 // "─ title ─" minimum
	if innerW < titleNeed {
		extra := titleNeed - innerW
		graphW += extra
		sampled = Resample(points, graphW)
		innerW = labelW + gap + 1 + graphW
	}

	var b strings.Builder
	b.WriteString(hline(th, color, "top", title, innerW))
	b.WriteByte('\n')

	if opts.Current >= 0 {
		live := paint(color, th.Hi, "● ") +
			paint(color, th.Text, formatCount(opts.Current)) +
			paint(color, th.Inactive, " playing now")
		b.WriteString(row(th, color, padVisible(live, innerW), innerW))
		b.WriteByte('\n')
		b.WriteString(hline(th, color, "mid", "", innerW))
		b.WriteByte('\n')
	}

	for rowIdx := opts.Height - 1; rowIdx >= 0; rowIdx-- {
		var label string
		switch rowIdx {
		case opts.Height - 1:
			label = formatCount(maxV)
		case 0:
			label = formatCount(minV)
		case opts.Height / 2:
			label = formatCount(avgV)
		}
		labelCol := paint(color, th.Inactive, fmt.Sprintf("%*s", labelW, label))
		sep := paint(color, th.Box, "│")

		var graph strings.Builder
		for _, p := range sampled {
			frac := float64(p.Count-minV) / float64(maxV-minV)
			level := frac * float64(opts.Height)
			rowTop := float64(rowIdx + 1)
			rowBot := float64(rowIdx)

			switch {
			case level >= rowTop:
				intensity := rowTop / float64(opts.Height)
				cell := "⣿"
				if color {
					graph.WriteString(th.Gradient(intensity).ANSI())
					graph.WriteString(cell)
					graph.WriteString(reset)
				} else {
					graph.WriteString(cell)
				}
			case level > rowBot:
				partial := level - rowBot
				cell := braillePartial(partial)
				intensity := level / float64(opts.Height)
				if color {
					graph.WriteString(th.Gradient(intensity).ANSI())
					graph.WriteString(cell)
					graph.WriteString(reset)
				} else {
					graph.WriteString(cell)
				}
			default:
				if color {
					graph.WriteString(th.Meter.ANSI())
					graph.WriteString("⠀")
					graph.WriteString(reset)
				} else {
					graph.WriteString("⠀")
				}
			}
		}

		content := labelCol + strings.Repeat(" ", gap) + sep + graph.String()
		b.WriteString(row(th, color, content, innerW))
		b.WriteByte('\n')
	}

	b.WriteString(hline(th, color, "mid", "", innerW))
	b.WriteByte('\n')

	startLabel := sampled[0].Time.UTC().Format("01-02 15:04")
	endLabel := sampled[len(sampled)-1].Time.UTC().Format("01-02 15:04")
	axisPad := graphW - len(startLabel) - len(endLabel)
	if axisPad < 1 {
		axisPad = 1
	}
	axis := strings.Repeat(" ", labelW+gap+1) + startLabel + strings.Repeat(" ", axisPad) + endLabel
	if visibleLen(axis) > innerW {
		axis = startLabel + " → " + endLabel
	}
	b.WriteString(row(th, color, padVisible(paint(color, th.Inactive, axis), innerW), innerW))
	b.WriteByte('\n')

	cur := last.Count
	if opts.Current >= 0 {
		cur = opts.Current
	}
	stats := buildStats(th, color, minV, avgV, maxV, cur)
	b.WriteString(row(th, color, padVisible(stats, innerW), innerW))
	b.WriteByte('\n')

	if opts.ShowLegend && color {
		legend := paint(true, th.Inactive, "low ") + gradientLegend(th, min(24, graphW/2)) + paint(true, th.Inactive, " high")
		b.WriteString(row(th, color, padVisible(legend, innerW), innerW))
		b.WriteByte('\n')
	}

	b.WriteString(hline(th, color, "bot", "", innerW))
	b.WriteByte('\n')
	return b.String()
}

func buildStats(th Theme, color bool, minV, avgV, maxV, cur int) string {
	if !color {
		return fmt.Sprintf("min %s   avg %s   max %s   now %s",
			formatCount(minV), formatCount(avgV), formatCount(maxV), formatCount(cur))
	}
	return paint(true, th.Inactive, "min ") + paint(true, th.Start, formatCount(minV)) +
		paint(true, th.Inactive, "   avg ") + paint(true, th.Mid, formatCount(avgV)) +
		paint(true, th.Inactive, "   max ") + paint(true, th.End, formatCount(maxV)) +
		paint(true, th.Inactive, "   now ") + paint(true, th.Hi, formatCount(cur))
}

func braillePartial(frac float64) string {
	switch {
	case frac >= 0.875:
		return "⣿"
	case frac >= 0.625:
		return "⣷"
	case frac >= 0.375:
		return "⣧"
	case frac >= 0.125:
		return "⣇"
	default:
		return "⡀"
	}
}

func gradientLegend(th Theme, width int) string {
	if width < 4 {
		width = 4
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		t := float64(i) / float64(width-1)
		b.WriteString(th.Gradient(t).ANSI())
		b.WriteString("⣿")
	}
	b.WriteString(reset)
	return b.String()
}

// hline draws a box horizontal: top/mid/bot with optional title in top.
func hline(th Theme, color bool, kind, title string, innerW int) string {
	var left, right, fill string
	switch kind {
	case "top":
		left, right, fill = "╭", "╮", "─"
	case "bot":
		left, right, fill = "╰", "╯", "─"
	default:
		left, right, fill = "├", "┤", "─"
	}

	if kind == "top" && title != "" {
		// ╭─ title ──────╮
		bodyPlain := "─ " + title + " "
		remain := innerW - visibleLen(bodyPlain)
		if remain < 1 {
			remain = 1
		}
		body := paint(color, th.Box, "─ ") +
			paint(color, th.Title, title) +
			paint(color, th.Box, " "+strings.Repeat(fill, remain))
		for visibleLen(body) < innerW {
			body += paint(color, th.Box, fill)
		}
		return paint(color, th.Box, left) + body + paint(color, th.Box, right)
	}

	return paint(color, th.Box, left+strings.Repeat(fill, innerW)+right)
}

func row(th Theme, color bool, content string, innerW int) string {
	content = padVisible(content, innerW)
	// If still over (wide glyphs), hard-trim plain
	if visibleLen(content) > innerW {
		content = padVisible(stripANSI(content), innerW)
	}
	return paint(color, th.Box, "│") + content + paint(color, th.Box, "│")
}

func padVisible(s string, width int) string {
	n := visibleLen(s)
	if n == width {
		return s
	}
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	// Trim while respecting ANSI
	var b strings.Builder
	inEsc := false
	count := 0
	for _, r := range s {
		if inEsc {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		if count >= width {
			break
		}
		b.WriteRune(r)
		count++
	}
	if strings.Contains(s, "\x1b[") {
		b.WriteString(reset)
	}
	return b.String()
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatCount(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ",")
	}
	if neg {
		return "-" + s
	}
	return s
}

// Downsample is kept as an alias for callers/tests.
func Downsample(points []steam.Point, width int) []steam.Point {
	return Resample(points, width)
}
