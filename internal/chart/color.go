package chart

import (
	"fmt"
	"math"
	"os"
)

// Theme mirrors btop's default-ish CPU/process graph palette.
type Theme struct {
	Box      RGB // border
	Title    RGB // panel title
	Text     RGB // primary text
	Inactive RGB // axis / muted labels
	Hi       RGB // highlighted values
	// Graph gradient: low → mid → high (btop cpu/temp style)
	Start RGB
	Mid   RGB
	End   RGB
	Meter RGB // empty graph background tint
}

// DefaultTheme is close to btop's default green→yellow→red ramp.
func DefaultTheme() Theme {
	return Theme{
		Box:      RGB{108, 153, 187}, // soft steel blue
		Title:    RGB{224, 197, 85},  // gold
		Text:     RGB{204, 204, 204},
		Inactive: RGB{101, 115, 126},
		Hi:       RGB{141, 193, 73}, // green highlight
		Start:    RGB{80, 250, 123}, // green
		Mid:      RGB{241, 250, 140}, // yellow
		End:      RGB{255, 85, 85},   // red
		Meter:    RGB{40, 42, 54},
	}
}

type RGB struct{ R, G, B uint8 }

func (c RGB) ANSI() string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
}

const reset = "\x1b[0m"

func lerp(a, b uint8, t float64) uint8 {
	return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t))
}

func lerpRGB(a, b RGB, t float64) RGB {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return RGB{lerp(a.R, b.R, t), lerp(a.G, b.G, t), lerp(a.B, b.B, t)}
}

// Gradient returns a color along start→mid→end for t in [0,1].
func (t Theme) Gradient(v float64) RGB {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	if v < 0.5 {
		return lerpRGB(t.Start, t.Mid, v*2)
	}
	return lerpRGB(t.Mid, t.End, (v-0.5)*2)
}

// ColorEnabled reports whether ANSI colors should be used.
func ColorEnabled(forceOff bool) bool {
	if forceOff {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func paint(enabled bool, c RGB, s string) string {
	if !enabled || s == "" {
		return s
	}
	return c.ANSI() + s + reset
}

// visibleLen approximates printable width ignoring ANSI CSI sequences.
func visibleLen(s string) int {
	n := 0
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
		n++
	}
	return n
}
