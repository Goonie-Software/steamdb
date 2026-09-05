package steam

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultStoreSearchURL  = "https://store.steampowered.com/api/storesearch/"
	defaultPlayerCountURL  = "https://api.steampowered.com/ISteamUserStats/GetNumberOfCurrentPlayers/v1/"
	defaultChartDataURLFmt = "https://steamcharts.com/app/%d/chart-data.json"
	defaultAppDetailsFmt   = "https://store.steampowered.com/api/appdetails?appids=%d&filters=basic"
	defaultUserAgent       = "steamdb-cli/1.0 (+https://github.com/Goonie-Software/steamdb)"
)

// Client talks to Steam store/API and SteamCharts.
type Client struct {
	HTTP      *http.Client
	UserAgent string

	// Optional URL overrides (used by tests). Empty → production defaults.
	StoreSearchURL  string
	PlayerCountURL  string
	ChartDataURLFmt string
	AppDetailsURLFmt string
}

func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
		UserAgent: defaultUserAgent,
	}
}

func (c *Client) storeSearch() string {
	if c.StoreSearchURL != "" {
		return c.StoreSearchURL
	}
	return defaultStoreSearchURL
}

func (c *Client) playerCount() string {
	if c.PlayerCountURL != "" {
		return c.PlayerCountURL
	}
	return defaultPlayerCountURL
}

func (c *Client) chartDataFmt() string {
	if c.ChartDataURLFmt != "" {
		return c.ChartDataURLFmt
	}
	return defaultChartDataURLFmt
}

func (c *Client) appDetailsFmt() string {
	if c.AppDetailsURLFmt != "" {
		return c.AppDetailsURLFmt
	}
	return defaultAppDetailsFmt
}

// Game is a Steam store search hit.
type Game struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type storeSearchResponse struct {
	Total int    `json:"total"`
	Items []Game `json:"items"`
}

// SearchGames finds apps matching query (best-effort name search).
func (c *Client) SearchGames(query string, limit int) ([]Game, error) {
	if limit <= 0 {
		limit = 10
	}

	u, err := url.Parse(c.storeSearch())
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("term", query)
	q.Set("l", "english")
	q.Set("cc", "US")
	u.RawQuery = q.Encode()

	body, err := c.get(u.String())
	if err != nil {
		return nil, fmt.Errorf("store search: %w", err)
	}

	var resp storeSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode store search: %w", err)
	}

	games := make([]Game, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.Type != "" && item.Type != "app" {
			continue
		}
		games = append(games, item)
		if len(games) >= limit {
			break
		}
	}
	return games, nil
}

type playerCountResponse struct {
	Response struct {
		PlayerCount int `json:"player_count"`
		Result      int `json:"result"`
	} `json:"response"`
}

// CurrentPlayers returns live concurrent players for an app.
func (c *Client) CurrentPlayers(appID int) (int, error) {
	u, err := url.Parse(c.playerCount())
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("appid", strconv.Itoa(appID))
	u.RawQuery = q.Encode()

	body, err := c.get(u.String())
	if err != nil {
		return 0, fmt.Errorf("player count: %w", err)
	}

	var resp playerCountResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("decode player count: %w", err)
	}
	if resp.Response.Result != 1 {
		return 0, fmt.Errorf("player count unavailable for app %d", appID)
	}
	return resp.Response.PlayerCount, nil
}

// Point is a single concurrent-player sample.
type Point struct {
	Time  time.Time
	Count int
}

// AppName resolves a display name via the Steam store appdetails API.
func (c *Client) AppName(appID int) (string, error) {
	u := fmt.Sprintf(c.appDetailsFmt(), appID)
	body, err := c.get(u)
	if err != nil {
		return "", err
	}
	var resp map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	entry, ok := resp[strconv.Itoa(appID)]
	if !ok || !entry.Success || entry.Data.Name == "" {
		return "", fmt.Errorf("app name not found for %d", appID)
	}
	return entry.Data.Name, nil
}

// ChartHistory fetches historical concurrent players from SteamCharts.
func (c *Client) ChartHistory(appID int) ([]Point, error) {
	url := fmt.Sprintf(c.chartDataFmt(), appID)
	body, err := c.get(url)
	if err != nil {
		return nil, fmt.Errorf("chart history: %w", err)
	}

	var raw [][]float64
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode chart history: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("no chart data for app %d", appID)
	}

	points := make([]Point, 0, len(raw))
	for _, row := range raw {
		if len(row) < 2 {
			continue
		}
		ms := int64(row[0])
		points = append(points, Point{
			Time:  time.UnixMilli(ms).UTC(),
			Count: int(row[1]),
		})
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("no usable chart points for app %d", appID)
	}
	return points, nil
}

func (c *Client) get(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", rawURL, resp.StatusCode)
	}
	return body, nil
}
