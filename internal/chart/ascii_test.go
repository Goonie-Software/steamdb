package chart

import (
	"strings"
	"testing"
	"time"

	"github.com/Goonie-Software/steamdb/internal/steam"
)

func TestParsePeriod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    Period
		wantErr bool
	}{
		{name: "24h", in: "24h", want: Period24h},
		{name: "7d upper", in: "7D", want: Period7d},
		{name: "30d trimmed", in: "  30d  ", want: Period30d},
		{name: "90d", in: "90d", want: Period90d},
		{name: "1y", in: "1y", want: Period1y},
		{name: "all", in: "all", want: PeriodAll},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "week", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePeriod(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePeriod(%q) error = nil, want error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePeriod(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePeriod(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPeriodDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		period Period
		want   time.Duration
	}{
		{Period24h, 24 * time.Hour},
		{Period7d, 7 * 24 * time.Hour},
		{Period30d, 30 * 24 * time.Hour},
		{Period90d, 90 * 24 * time.Hour},
		{Period1y, 365 * 24 * time.Hour},
		{PeriodAll, 0},
		{Period("nope"), 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			t.Parallel()
			if got := tt.period.Duration(); got != tt.want {
				t.Fatalf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterByPeriod(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	mk := func(hoursAgo int, count int) steam.Point {
		return steam.Point{Time: base.Add(-time.Duration(hoursAgo) * time.Hour), Count: count}
	}

	tests := []struct {
		name   string
		points []steam.Point
		period Period
		wantN  int
		wantFirstCount int
	}{
		{
			name:   "empty",
			points: nil,
			period: Period24h,
			wantN:  0,
		},
		{
			name:   "drops zeros",
			points: []steam.Point{mk(2, 0), mk(1, 100), mk(0, 200)},
			period: Period24h,
			wantN:  2,
			wantFirstCount: 100,
		},
		{
			name: "keeps window only",
			points: []steam.Point{
				mk(48, 50),
				mk(20, 80),
				mk(5, 90),
				mk(0, 100),
			},
			period: Period24h,
			wantN:  3,
			wantFirstCount: 80,
		},
		{
			name: "all keeps history",
			points: []steam.Point{
				mk(1000, 10),
				mk(0, 20),
			},
			period: PeriodAll,
			wantN:  2,
			wantFirstCount: 10,
		},
		{
			name: "all zeros falls back to last",
			points: []steam.Point{
				mk(1, 0),
				mk(0, 0),
			},
			period: Period24h,
			wantN:  1,
			wantFirstCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FilterByPeriod(tt.points, tt.period)
			if len(got) != tt.wantN {
				t.Fatalf("len = %d, want %d (%v)", len(got), tt.wantN, got)
			}
			if tt.wantN > 0 && got[0].Count != tt.wantFirstCount {
				t.Fatalf("first count = %d, want %d", got[0].Count, tt.wantFirstCount)
			}
		})
	}
}

func TestResample(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	points := make([]steam.Point, 10)
	for i := range points {
		points[i] = steam.Point{Time: base.Add(time.Duration(i) * time.Hour), Count: (i + 1) * 10}
	}

	tests := []struct {
		name  string
		in    []steam.Point
		width int
		wantN int
	}{
		{name: "empty", in: nil, width: 5, wantN: 0},
		{name: "width zero returns input", in: points, width: 0, wantN: 10},
		{name: "same width", in: points, width: 10, wantN: 10},
		{name: "downsample", in: points, width: 5, wantN: 5},
		{name: "upsample", in: points[:3], width: 9, wantN: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Resample(tt.in, tt.width)
			if len(got) != tt.wantN {
				t.Fatalf("len = %d, want %d", len(got), tt.wantN)
			}
		})
	}

	t.Run("downsample averages", func(t *testing.T) {
		t.Parallel()
		in := []steam.Point{
			{Count: 10}, {Count: 20}, {Count: 30}, {Count: 40},
		}
		got := Resample(in, 2)
		if len(got) != 2 {
			t.Fatalf("len = %d", len(got))
		}
		if got[0].Count != 15 || got[1].Count != 35 {
			t.Fatalf("got counts %d,%d want 15,35", got[0].Count, got[1].Count)
		}
	})

	t.Run("upsample endpoints", func(t *testing.T) {
		t.Parallel()
		in := []steam.Point{{Count: 0}, {Count: 100}}
		got := Resample(in, 5)
		if got[0].Count != 0 || got[len(got)-1].Count != 100 {
			t.Fatalf("endpoints = %d,%d", got[0].Count, got[len(got)-1].Count)
		}
	})
}

func TestFormatCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{12, "12"},
		{999, "999"},
		{1000, "1,000"},
		{1125424, "1,125,424"},
		{-1500, "-1,500"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := formatCount(tt.in); got != tt.want {
				t.Fatalf("formatCount(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBraillePartial(t *testing.T) {
	t.Parallel()
	tests := []struct {
		frac float64
		want string
	}{
		{0.05, "⡀"},
		{0.2, "⣇"},
		{0.5, "⣧"},
		{0.7, "⣷"},
		{1.0, "⣿"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := braillePartial(tt.frac); got != tt.want {
				t.Fatalf("braillePartial(%v) = %q, want %q", tt.frac, got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	points := make([]steam.Point, 48)
	for i := range points {
		points[i] = steam.Point{
			Time:  base.Add(time.Duration(i) * time.Hour),
			Count: 1000 + i*50,
		}
	}

	tests := []struct {
		name string
		opts Options
		want []string // substrings that must appear
	}{
		{
			name: "plain box",
			opts: Options{Width: 40, Height: 8, Title: "players", Color: false, Current: 3000},
			want: []string{"╭", "╮", "╰", "╯", "players", "playing now", "min ", "max ", "now "},
		},
		{
			name: "color legend",
			opts: Options{Width: 40, Height: 8, Title: "cpu", Color: true, ShowLegend: true, Current: 2000},
			want: []string{"\x1b[38;2;", "low ", "high", "cpu"},
		},
		{
			name: "empty data",
			opts: Options{Width: 20, Height: 5},
			want: []string{"no data"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := points
			if tt.name == "empty data" {
				in = nil
			}
			out := Render(in, tt.opts)
			for _, sub := range tt.want {
				if !strings.Contains(out, sub) {
					t.Fatalf("Render output missing %q\n%s", sub, out)
				}
			}
			if tt.name != "empty data" {
				lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
				if len(lines) < 5 {
					t.Fatalf("too few lines: %d", len(lines))
				}
				// box lines should share a common visible width
				w0 := visibleLen(lines[0])
				for i, line := range lines {
					if visibleLen(line) != w0 {
						t.Fatalf("line %d width %d != %d\n%q", i, visibleLen(line), w0, line)
					}
				}
			}
		})
	}
}

func makePoints(n int) []steam.Point {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]steam.Point, n)
	for i := range out {
		out[i] = steam.Point{
			Time:  base.Add(time.Duration(i) * time.Hour),
			Count: 10000 + (i%200)*37,
		}
	}
	return out
}

func BenchmarkResample(b *testing.B) {
	points := makePoints(10_000)
	cases := []struct {
		name  string
		width int
	}{
		{"down_64", 64},
		{"down_256", 256},
		{"up_64", 64},
	}
	for _, bc := range cases {
		in := points
		if bc.name == "up_64" {
			in = points[:32]
		}
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = Resample(in, bc.width)
			}
		})
	}
}

func BenchmarkFilterByPeriod(b *testing.B) {
	points := makePoints(5_000)
	periods := []Period{Period24h, Period7d, Period30d, PeriodAll}
	for _, p := range periods {
		b.Run(string(p), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = FilterByPeriod(points, p)
			}
		})
	}
}

func BenchmarkRender(b *testing.B) {
	points := makePoints(2_000)
	cases := []struct {
		name  string
		color bool
	}{
		{"plain", false},
		{"color", true},
	}
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			opts := Options{
				Width:      64,
				Height:     14,
				Title:      "concurrent players · 7d",
				Color:      bc.color,
				Theme:      DefaultTheme(),
				Current:    42000,
				ShowLegend: bc.color,
			}
			for b.Loop() {
				_ = Render(points, opts)
			}
		})
	}
}
