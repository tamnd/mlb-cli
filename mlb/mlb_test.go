package mlb_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/mlb-cli/mlb"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := mlb.NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := mlb.NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetSchedule(t *testing.T) {
	payload := map[string]any{
		"dates": []any{
			map[string]any{
				"date": "2024-07-01",
				"games": []any{
					map[string]any{
						"gamePk":   745455,
						"gameDate": "2024-07-01T17:07:00Z",
						"status":   map[string]any{"abstractGameState": "Final"},
						"teams": map[string]any{
							"home": map[string]any{
								"score": 5,
								"team":  map[string]any{"name": "Toronto Blue Jays"},
							},
							"away": map[string]any{
								"score": 3,
								"team":  map[string]any{"name": "Chicago White Sox"},
							},
						},
						"venue": map[string]any{"name": "Rogers Centre"},
					},
				},
			},
		},
	}
	srv := apiServer(t, "/api/v1/schedule", payload)
	defer srv.Close()

	c := clientFor(srv)
	games, err := c.GetSchedule(context.Background(), "2024-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1", len(games))
	}
	g := games[0]
	if g.GamePK != 745455 {
		t.Errorf("GamePK = %d, want 745455", g.GamePK)
	}
	if g.HomeTeam != "Toronto Blue Jays" {
		t.Errorf("HomeTeam = %q, want Toronto Blue Jays", g.HomeTeam)
	}
	if g.AwayTeam != "Chicago White Sox" {
		t.Errorf("AwayTeam = %q, want Chicago White Sox", g.AwayTeam)
	}
	if g.HomeScore != 5 {
		t.Errorf("HomeScore = %d, want 5", g.HomeScore)
	}
	if g.AwayScore != 3 {
		t.Errorf("AwayScore = %d, want 3", g.AwayScore)
	}
	if g.Status != "Final" {
		t.Errorf("Status = %q, want Final", g.Status)
	}
	if g.Venue != "Rogers Centre" {
		t.Errorf("Venue = %q, want Rogers Centre", g.Venue)
	}
}

func TestGetTeams(t *testing.T) {
	payload := map[string]any{
		"teams": []any{
			map[string]any{
				"id":             141,
				"name":           "Toronto Blue Jays",
				"abbreviation":   "TOR",
				"teamName":       "Blue Jays",
				"locationName":   "Toronto",
				"firstYearOfPlay": "1977",
			},
		},
	}
	srv := apiServer(t, "/api/v1/teams", payload)
	defer srv.Close()

	c := clientFor(srv)
	teams, err := c.GetTeams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 {
		t.Fatalf("got %d teams, want 1", len(teams))
	}
	tm := teams[0]
	if tm.ID != 141 {
		t.Errorf("ID = %d, want 141", tm.ID)
	}
	if tm.Abbreviation != "TOR" {
		t.Errorf("Abbreviation = %q, want TOR", tm.Abbreviation)
	}
	if tm.Location != "Toronto" {
		t.Errorf("Location = %q, want Toronto", tm.Location)
	}
	if tm.FirstYear != "1977" {
		t.Errorf("FirstYear = %q, want 1977", tm.FirstYear)
	}
}

func TestGetPlayers(t *testing.T) {
	payload := map[string]any{
		"people": []any{
			map[string]any{
				"id":            656140,
				"fullName":      "Andrew Abbott",
				"primaryNumber": "41",
				"currentTeam":   map[string]any{"id": 113},
				"position":      map[string]any{"name": "Pitcher"},
			},
		},
	}
	srv := apiServer(t, "/api/v1/sports/1/players", payload)
	defer srv.Close()

	c := clientFor(srv)
	players, err := c.GetPlayers(context.Background(), "2024")
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 {
		t.Fatalf("got %d players, want 1", len(players))
	}
	p := players[0]
	if p.ID != 656140 {
		t.Errorf("ID = %d, want 656140", p.ID)
	}
	if p.FullName != "Andrew Abbott" {
		t.Errorf("FullName = %q, want Andrew Abbott", p.FullName)
	}
	if p.Number != "41" {
		t.Errorf("Number = %q, want 41", p.Number)
	}
	if p.Position != "Pitcher" {
		t.Errorf("Position = %q, want Pitcher", p.Position)
	}
	if p.TeamID != 113 {
		t.Errorf("TeamID = %d, want 113", p.TeamID)
	}
}

func TestGetStandings(t *testing.T) {
	payload := map[string]any{
		"records": []any{
			map[string]any{
				"division": map[string]any{"name": "AL East"},
				"teamRecords": []any{
					map[string]any{
						"team":      map[string]any{"name": "Baltimore Orioles"},
						"wins":      101,
						"losses":    61,
						"pct":       ".623",
						"gamesBack": "0.0",
						"streak":    map[string]any{"streakCode": "W4"},
					},
				},
			},
		},
	}
	srv := apiServer(t, "/api/v1/standings", payload)
	defer srv.Close()

	c := clientFor(srv)
	standings, err := c.GetStandings(context.Background(), "2024")
	if err != nil {
		t.Fatal(err)
	}
	if len(standings) != 1 {
		t.Fatalf("got %d standings, want 1", len(standings))
	}
	s := standings[0]
	if s.Division != "AL East" {
		t.Errorf("Division = %q, want AL East", s.Division)
	}
	if s.TeamName != "Baltimore Orioles" {
		t.Errorf("TeamName = %q, want Baltimore Orioles", s.TeamName)
	}
	if s.Wins != 101 {
		t.Errorf("Wins = %d, want 101", s.Wins)
	}
	if s.Losses != 61 {
		t.Errorf("Losses = %d, want 61", s.Losses)
	}
	if s.Pct != ".623" {
		t.Errorf("Pct = %q, want .623", s.Pct)
	}
	if s.Streak != "W4" {
		t.Errorf("Streak = %q, want W4", s.Streak)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := mlb.DefaultConfig()
	if cfg.Rate <= 0 {
		t.Error("DefaultConfig.Rate should be positive")
	}
	if cfg.Retries <= 0 {
		t.Error("DefaultConfig.Retries should be positive")
	}
	if cfg.Timeout <= 0 {
		t.Error("DefaultConfig.Timeout should be positive")
	}
}

// --- helpers ---

// apiServer creates a test server that serves payload as JSON for the given path prefix.
func apiServer(t *testing.T, pathPrefix string, payload any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !startsWith(r.URL.Path, pathPrefix) {
			t.Errorf("unexpected path %q, want prefix %q", r.URL.Path, pathPrefix)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

// clientFor returns a Client pointed at the test server with no rate-limiting.
func clientFor(srv *httptest.Server) *mlb.Client {
	c := mlb.NewClient()
	c.Rate = 0
	// redirect BaseURL to the test server by overriding the HTTP transport
	c.HTTP.Transport = &prefixTransport{serverURL: srv.URL}
	return c
}

// prefixTransport rewrites the host of every request to the test server URL.
type prefixTransport struct {
	serverURL string // e.g. http://127.0.0.1:PORT
}

func (pt *prefixTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// strip the scheme prefix to get host:port
	hostPort := pt.serverURL
	if len(hostPort) > 7 && hostPort[:7] == "http://" {
		hostPort = hostPort[7:]
	}
	u := *req.URL
	u.Scheme = "http"
	u.Host = hostPort
	req2 := req.Clone(req.Context())
	req2.URL = &u
	req2.Host = hostPort
	return http.DefaultTransport.RoundTrip(req2)
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
