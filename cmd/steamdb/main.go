package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Goonie-Software/steamdb/internal/chart"
	"github.com/Goonie-Software/steamdb/internal/steam"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("steamdb", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	appID := fs.Int("appid", 0, "Steam AppID (skip name search)")
	periodStr := fs.String("period", "7d", "chart window: 24h, 7d, 30d, 90d, 1y, all")
	width := fs.Int("width", 64, "chart width in columns")
	height := fs.Int("height", 14, "chart height in rows")
	limit := fs.Int("limit", 8, "max search results to show")
	pick := fs.Int("pick", 0, "1-based search result to use (non-interactive)")
	listOnly := fs.Bool("list", false, "only list search matches, do not chart")
	noColor := fs.Bool("no-color", false, "disable ANSI colors (also respects NO_COLOR)")
	noLegend := fs.Bool("no-legend", false, "hide color gradient legend")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `steamdb — query Steam player charts by game name

Usage:
  steamdb [flags] <game name>
  steamdb -appid 730 -period 24h

Flags:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  steamdb "counter-strike 2"
  steamdb elden ring -period 30d
  steamdb -appid 570 -period 24h
  steamdb "dota" -list
`)
	}

	if err := fs.Parse(reorderArgs(os.Args[1:])); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))

	period, err := chart.ParsePeriod(*periodStr)
	if err != nil {
		return err
	}

	client := steam.NewClient()

	var game steam.Game
	switch {
	case *appID > 0:
		game = steam.Game{ID: *appID, Name: fmt.Sprintf("App %d", *appID)}
		if name, err := client.AppName(*appID); err == nil && name != "" {
			game.Name = name
		}
	case query == "":
		fs.Usage()
		return fmt.Errorf("provide a game name or -appid")
	default:
		games, err := client.SearchGames(query, *limit)
		if err != nil {
			return err
		}
		if len(games) == 0 {
			return fmt.Errorf("no games found for %q", query)
		}

		printMatches(games)
		if *listOnly {
			return nil
		}

		idx := 0
		if *pick > 0 {
			if *pick > len(games) {
				return fmt.Errorf("-pick %d out of range (1-%d)", *pick, len(games))
			}
			idx = *pick - 1
		} else if len(games) > 1 && isInteractive() {
			idx, err = promptChoice(len(games))
			if err != nil {
				return err
			}
		}
		game = games[idx]
	}

	current, err := client.CurrentPlayers(game.ID)
	if err != nil {
		// Chart may still work; show warning.
		fmt.Fprintf(os.Stderr, "warning: live player count: %v\n", err)
		current = -1
	}

	points, err := client.ChartHistory(game.ID)
	if err != nil {
		return err
	}
	window := chart.FilterByPeriod(points, period)
	useColor := chart.ColorEnabled(*noColor)
	th := chart.DefaultTheme()

	fmt.Println()
	printHeader(game, period, len(window), useColor, th)

	title := fmt.Sprintf("concurrent players · %s", period)
	fmt.Print(chart.Render(window, chart.Options{
		Width:      *width,
		Height:     *height,
		Title:      title,
		Color:      useColor,
		Theme:      th,
		Current:    current,
		ShowLegend: !*noLegend && useColor,
	}))
	return nil
}

func printHeader(game steam.Game, period chart.Period, samples int, color bool, th chart.Theme) {
	name := game.Name
	meta := fmt.Sprintf("app %d  ·  %s  ·  %d samples", game.ID, period, samples)
	if !color {
		fmt.Printf("%s\n%s\n\n", name, meta)
		return
	}
	fmt.Printf("%s%s%s\n", th.Title.ANSI(), name, "\x1b[0m")
	fmt.Printf("%s%s%s\n\n", th.Inactive.ANSI(), meta, "\x1b[0m")
}

func printMatches(games []steam.Game) {
	th := chart.DefaultTheme()
	color := chart.ColorEnabled(false)
	fmt.Println(paintLine(color, th.Title, "Matches:"))
	for i, g := range games {
		idx := paintLine(color, th.Hi, fmt.Sprintf("%d)", i+1))
		name := paintLine(color, th.Text, g.Name)
		id := paintLine(color, th.Inactive, fmt.Sprintf("[appid %d]", g.ID))
		fmt.Printf("  %s %s  %s\n", idx, name, id)
	}
	fmt.Println()
}

func paintLine(enabled bool, c chart.RGB, s string) string {
	if !enabled {
		return s
	}
	return c.ANSI() + s + "\x1b[0m"
}

func promptChoice(n int) (int, error) {
	fmt.Printf("Select game [1-%d] (default 1): ", n)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(line)
	if err != nil || v < 1 || v > n {
		return 0, fmt.Errorf("invalid selection %q", line)
	}
	return v - 1, nil
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func formatCount(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		return "-" + out
	}
	return out
}

// reorderArgs moves flags before positional args so `steamdb elden ring -period 30d` works.
func reorderArgs(args []string) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positionals = append(positionals, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-") && a != "-":
			flags = append(flags, a)
			// boolean flags take no value; value flags may be -flag=value or -flag value
			if strings.Contains(a, "=") {
				continue
			}
			name := strings.TrimLeft(a, "-")
			switch name {
			case "list", "h", "help", "no-color", "no-legend":
				// boolean flags take no separate value
			default:
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					flags = append(flags, args[i])
				}
			}
		default:
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}
