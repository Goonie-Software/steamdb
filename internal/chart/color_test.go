package chart

import (
	"testing"

	"github.com/Goonie-Software/steamdb/internal/steam"
)

func TestGradient(t *testing.T) {
	t.Parallel()
	th := DefaultTheme()
	tests := []struct {
		name string
		v    float64
		want RGB
	}{
		{name: "start", v: 0, want: th.Start},
		{name: "mid", v: 0.5, want: th.Mid},
		{name: "end", v: 1, want: th.End},
		{name: "clamp low", v: -1, want: th.Start},
		{name: "clamp high", v: 2, want: th.End},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := th.Gradient(tt.v)
			if got != tt.want {
				t.Fatalf("Gradient(%v) = %+v, want %+v", tt.v, got, tt.want)
			}
		})
	}

	t.Run("between start and mid", func(t *testing.T) {
		t.Parallel()
		got := th.Gradient(0.25)
		if got == th.Start || got == th.Mid || got == th.End {
			t.Fatalf("expected interpolated color, got %+v", got)
		}
	})
}

func TestVisibleLen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "plain", in: "hello", want: 5},
		{name: "empty", in: "", want: 0},
		{name: "ansi", in: "\x1b[38;2;255;0;0mhi\x1b[0m", want: 2},
		{name: "braille", in: "⣿⣿", want: 2},
		{name: "mixed", in: "a\x1b[1mb\x1b[0mc", want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := visibleLen(tt.in); got != tt.want {
				t.Fatalf("visibleLen(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestPaint(t *testing.T) {
	t.Parallel()
	c := RGB{255, 0, 0}
	tests := []struct {
		name    string
		enabled bool
		s       string
		want    string
	}{
		{name: "disabled", enabled: false, s: "x", want: "x"},
		{name: "empty", enabled: true, s: "", want: ""},
		{name: "enabled", enabled: true, s: "x", want: c.ANSI() + "x" + reset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := paint(tt.enabled, c, tt.s); got != tt.want {
				t.Fatalf("paint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColorEnabled(t *testing.T) {
	tests := []struct {
		name     string
		forceOff bool
		env      map[string]string
		want     bool
	}{
		{name: "forced off", forceOff: true, want: false},
		{name: "NO_COLOR", forceOff: false, env: map[string]string{"NO_COLOR": "1"}, want: false},
		{name: "TERM dumb", forceOff: false, env: map[string]string{"TERM": "dumb", "NO_COLOR": ""}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := ColorEnabled(tt.forceOff); got != tt.want {
				t.Fatalf("ColorEnabled(%v) = %v, want %v", tt.forceOff, got, tt.want)
			}
		})
	}
}

func TestDownsampleAlias(t *testing.T) {
	t.Parallel()
	in := []steam.Point{{Count: 10}, {Count: 20}, {Count: 30}, {Count: 40}}
	got := Downsample(in, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestRGBANSI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		c    RGB
		want string
	}{
		{RGB{0, 0, 0}, "\x1b[38;2;0;0;0m"},
		{RGB{80, 250, 123}, "\x1b[38;2;80;250;123m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.c.ANSI(); got != tt.want {
				t.Fatalf("ANSI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkGradient(b *testing.B) {
	th := DefaultTheme()
	vals := []float64{0, 0.25, 0.5, 0.75, 1}
	b.ReportAllocs()
	for b.Loop() {
		for _, v := range vals {
			_ = th.Gradient(v)
		}
	}
}

func BenchmarkVisibleLen(b *testing.B) {
	s := "\x1b[38;2;80;250;123m" + stringsRepeat("⣿", 64) + reset
	b.ReportAllocs()
	for b.Loop() {
		_ = visibleLen(s)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
