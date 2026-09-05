package main

import (
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Goonie-Software/steamdb/internal/chart"
	"github.com/Goonie-Software/steamdb/internal/steam"
)

var stdioMu sync.Mutex

func TestReorderArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "flags first unchanged",
			in:   []string{"-period", "24h", "dota"},
			want: []string{"-period", "24h", "dota"},
		},
		{
			name: "flags after positionals",
			in:   []string{"elden", "ring", "-period", "30d"},
			want: []string{"-period", "30d", "elden", "ring"},
		},
		{
			name: "bool flag mid",
			in:   []string{"dota", "-list"},
			want: []string{"-list", "dota"},
		},
		{
			name: "equals form",
			in:   []string{"cs2", "-period=7d", "-pick=1"},
			want: []string{"-period=7d", "-pick=1", "cs2"},
		},
		{
			name: "double dash",
			in:   []string{"-period", "7d", "--", "-weird", "name"},
			want: []string{"-period", "7d", "-weird", "name"},
		},
		{
			name: "no-color and no-legend",
			in:   []string{"game", "-no-color", "-no-legend", "-width", "40"},
			want: []string{"-no-color", "-no-legend", "-width", "40", "game"},
		},
		{
			name: "empty",
			in:   nil,
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := reorderArgs(tt.in)
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("reorderArgs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatCountMain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1_000_000, "1,000,000"},
		{-4200, "-4,200"},
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

func BenchmarkReorderArgs(b *testing.B) {
	in := []string{"very", "long", "game", "name", "-period", "30d", "-width", "80", "-no-legend"}
	b.ReportAllocs()
	for b.Loop() {
		_ = reorderArgs(in)
	}
}

func BenchmarkFormatCountMain(b *testing.B) {
	vals := []int{0, 12, 999, 1000, 1_125_424, -1500}
	b.ReportAllocs()
	for b.Loop() {
		for _, v := range vals {
			_ = formatCount(v)
		}
	}
}

func TestPaintLine(t *testing.T) {
	t.Parallel()
	c := chart.RGB{1, 2, 3}
	tests := []struct {
		name    string
		enabled bool
		in      string
		want    string
	}{
		{name: "off", enabled: false, in: "hi", want: "hi"},
		{name: "on", enabled: true, in: "hi", want: c.ANSI() + "hi" + "\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := paintLine(tt.enabled, c, tt.in); got != tt.want {
				t.Fatalf("paintLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintHeader(t *testing.T) {
	game := steam.Game{ID: 730, Name: "Counter-Strike 2"}
	th := chart.DefaultTheme()

	tests := []struct {
		name  string
		color bool
		want  []string
	}{
		{name: "plain", color: false, want: []string{"Counter-Strike 2", "app 730", "7d", "12 samples"}},
		{name: "color", color: true, want: []string{th.Title.ANSI(), "Counter-Strike 2", th.Inactive.ANSI(), "app 730"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				printHeader(game, chart.Period7d, 12, tt.color, th)
			})
			for _, sub := range tt.want {
				if !strings.Contains(out, sub) {
					t.Fatalf("output missing %q\n%s", sub, out)
				}
			}
		})
	}
}

func TestPrintMatches(t *testing.T) {
	games := []steam.Game{
		{ID: 730, Name: "Counter-Strike 2"},
		{ID: 570, Name: "Dota 2"},
	}
	out := captureStdout(t, func() { printMatches(games) })
	for _, sub := range []string{"Matches:", "Counter-Strike 2", "Dota 2", "appid 730", "appid 570"} {
		if !strings.Contains(out, sub) {
			t.Fatalf("output missing %q\n%s", sub, out)
		}
	}
}

func TestPromptChoice(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		n       int
		want    int
		wantErr bool
	}{
		{name: "default empty", input: "\n", n: 3, want: 0},
		{name: "select 2", input: "2\n", n: 3, want: 1},
		{name: "out of range", input: "9\n", n: 3, wantErr: true},
		{name: "garbage", input: "nope\n", n: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStdin(t, tt.input, func() {
				got, err := promptChoice(tt.n)
				if tt.wantErr {
					if err == nil {
						t.Fatal("expected error")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.want {
					t.Fatalf("got %d, want %d", got, tt.want)
				}
			})
		})
	}
}

func TestIsInteractive(t *testing.T) {
	// Piped stdin in tests is typically not a char device.
	if isInteractive() {
		t.Skip("stdin is a terminal in this environment")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	stdioMu.Lock()
	defer stdioMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	stdioMu.Lock()
	defer stdioMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	fn()
}
