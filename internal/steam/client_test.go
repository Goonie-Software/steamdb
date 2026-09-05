package steam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientDefaults(t *testing.T) {
	t.Parallel()
	c := NewClient()
	if c.HTTP == nil {
		t.Fatal("HTTP client is nil")
	}
	if c.UserAgent == "" {
		t.Fatal("UserAgent empty")
	}
	if c.storeSearch() != defaultStoreSearchURL {
		t.Fatalf("storeSearch = %q", c.storeSearch())
	}
	if c.playerCount() != defaultPlayerCountURL {
		t.Fatalf("playerCount = %q", c.playerCount())
	}
	if c.chartDataFmt() != defaultChartDataURLFmt {
		t.Fatalf("chartDataFmt = %q", c.chartDataFmt())
	}
	if c.appDetailsFmt() != defaultAppDetailsFmt {
		t.Fatalf("appDetailsFmt = %q", c.appDetailsFmt())
	}
}

func TestClientURLOverrides(t *testing.T) {
	t.Parallel()
	c := &Client{
		StoreSearchURL:   "http://example/search",
		PlayerCountURL:   "http://example/players",
		ChartDataURLFmt:  "http://example/chart/%d",
		AppDetailsURLFmt: "http://example/app/%d",
	}
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"store", c.storeSearch(), "http://example/search"},
		{"players", c.playerCount(), "http://example/players"},
		{"chart", c.chartDataFmt(), "http://example/chart/%d"},
		{"app", c.appDetailsFmt(), "http://example/app/%d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestSearchGames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     int
		body       string
		limit      int
		wantNames  []string
		wantErr    bool
		wantErrSub string
	}{
		{
			name:   "filters non-apps and respects limit",
			status: http.StatusOK,
			body: `{
				"total": 3,
				"items": [
					{"id": 730, "name": "Counter-Strike 2", "type": "app"},
					{"id": 1, "name": "Bundle", "type": "bundle"},
					{"id": 570, "name": "Dota 2", "type": "app"}
				]
			}`,
			limit:     1,
			wantNames: []string{"Counter-Strike 2"},
		},
		{
			name:   "default limit",
			status: http.StatusOK,
			body: `{
				"total": 1,
				"items": [{"id": 105600, "name": "Terraria", "type": "app"}]
			}`,
			limit:     0,
			wantNames: []string{"Terraria"},
		},
		{
			name:       "http error",
			status:     http.StatusBadGateway,
			body:       "nope",
			wantErr:    true,
			wantErrSub: "HTTP 502",
		},
		{
			name:       "bad json",
			status:     http.StatusOK,
			body:       "{",
			wantErr:    true,
			wantErrSub: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") == "" {
					t.Errorf("missing User-Agent")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			c := &Client{
				HTTP:           srv.Client(),
				StoreSearchURL: srv.URL,
				UserAgent:      "test-agent",
			}
			got, err := c.SearchGames("query", tt.limit)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrSub != "" && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error %q missing %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.wantNames), got)
			}
			for i, name := range tt.wantNames {
				if got[i].Name != name {
					t.Fatalf("got[%d].Name = %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}

func TestCurrentPlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     int
		body       string
		want       int
		wantErr    bool
		wantErrSub string
	}{
		{
			name:   "ok",
			status: http.StatusOK,
			body:   `{"response":{"player_count":12345,"result":1}}`,
			want:   12345,
		},
		{
			name:       "unavailable result",
			status:     http.StatusOK,
			body:       `{"response":{"player_count":0,"result":42}}`,
			wantErr:    true,
			wantErrSub: "unavailable",
		},
		{
			name:       "bad json",
			status:     http.StatusOK,
			body:       "null",
			wantErr:    true,
			wantErrSub: "unavailable", // result defaults to 0
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("appid"); got != "730" {
					t.Errorf("appid=%q", got)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			c := &Client{HTTP: srv.Client(), PlayerCountURL: srv.URL}
			got, err := c.CurrentPlayers(730)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrSub != "" && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error %q missing %q", err, tt.wantErrSub)
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
	}
}

func TestAppName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		body       string
		want       string
		wantErr    bool
		wantErrSub string
	}{
		{
			name: "ok",
			body: `{"730":{"success":true,"data":{"name":"Counter-Strike 2"}}}`,
			want: "Counter-Strike 2",
		},
		{
			name:       "missing",
			body:       `{"999":{"success":true,"data":{"name":"Nope"}}}`,
			wantErr:    true,
			wantErrSub: "not found",
		},
		{
			name:       "unsuccessful",
			body:       `{"730":{"success":false,"data":{"name":"x"}}}`,
			wantErr:    true,
			wantErrSub: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			c := &Client{
				HTTP:             srv.Client(),
				AppDetailsURLFmt: srv.URL + "?appids=%d",
			}
			got, err := c.AppName(730)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrSub != "" && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error %q missing %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChartHistory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		body       string
		wantN      int
		wantFirst  int
		wantErr    bool
		wantErrSub string
	}{
		{
			name:      "ok",
			body:      `[[1600000000000,100],[1600003600000,200]]`,
			wantN:     2,
			wantFirst: 100,
		},
		{
			name:       "empty array",
			body:       `[]`,
			wantErr:    true,
			wantErrSub: "no chart data",
		},
		{
			name:       "no usable rows",
			body:       `[[1],[2]]`,
			wantErr:    true,
			wantErrSub: "no usable",
		},
		{
			name:       "bad json",
			body:       `{`,
			wantErr:    true,
			wantErrSub: "decode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			c := &Client{
				HTTP:            srv.Client(),
				ChartDataURLFmt: srv.URL + "/%d",
			}
			got, err := c.ChartHistory(730)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrSub != "" && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error %q missing %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantN {
				t.Fatalf("len = %d, want %d", len(got), tt.wantN)
			}
			if got[0].Count != tt.wantFirst {
				t.Fatalf("first = %d, want %d", got[0].Count, tt.wantFirst)
			}
			if !got[0].Time.Equal(time.UnixMilli(1600000000000).UTC()) {
				t.Fatalf("unexpected time %v", got[0].Time)
			}
		})
	}
}

func BenchmarkClientJSONDecode(b *testing.B) {
	// Micro-benchmark the chart history decode path via a local server.
	raw := make([][]float64, 2000)
	base := float64(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
	for i := range raw {
		raw[i] = []float64{base + float64(i)*3600_000, float64(1000 + i)}
	}
	body, err := json.Marshal(raw)
	if err != nil {
		b.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	b.Cleanup(srv.Close)

	c := &Client{
		HTTP:            srv.Client(),
		ChartDataURLFmt: srv.URL + "/%d",
	}

	b.ReportAllocs()
	for b.Loop() {
		pts, err := c.ChartHistory(1)
		if err != nil {
			b.Fatal(err)
		}
		if len(pts) != len(raw) {
			b.Fatalf("len=%d", len(pts))
		}
	}
}

func BenchmarkSearchGames(b *testing.B) {
	items := make([]Game, 50)
	for i := range items {
		items[i] = Game{ID: i + 1, Name: fmt.Sprintf("Game %d", i), Type: "app"}
	}
	payload, _ := json.Marshal(storeSearchResponse{Total: len(items), Items: items})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	b.Cleanup(srv.Close)

	c := &Client{HTTP: srv.Client(), StoreSearchURL: srv.URL}
	b.ReportAllocs()
	for b.Loop() {
		games, err := c.SearchGames("game", 10)
		if err != nil {
			b.Fatal(err)
		}
		if len(games) != 10 {
			b.Fatalf("len=%d", len(games))
		}
	}
}
