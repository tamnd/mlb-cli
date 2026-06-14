// Package mlb is the library behind the mlb command line:
// the HTTP client, request shaping, and the typed data models for the
// MLB Stats API (statsapi.mlb.com/api/v1).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package mlb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to the MLB Stats API.
const DefaultUserAgent = "mlb-cli/dev (+https://github.com/tamnd/mlb-cli)"

// Host is the Stats API hostname.
const Host = "statsapi.mlb.com"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Client talks to the MLB Stats API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// Config holds user-overridable settings for the client.
type Config struct {
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns production-ready defaults.
func DefaultConfig() Config {
	return Config{
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
		Timeout:   30 * time.Second,
	}
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 200ms
// minimum gap between requests, and five retries on transient errors.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- Output types ---

// Game is one game entry from the schedule endpoint.
type Game struct {
	GamePK    int    `kit:"id" json:"game_pk"`
	Date      string `json:"date"`
	HomeTeam  string `json:"home_team"`
	AwayTeam  string `json:"away_team"`
	HomeScore int    `json:"home_score"`
	AwayScore int    `json:"away_score"`
	Status    string `json:"status"`
	Venue     string `json:"venue"`
}

// Team is one team from the teams endpoint.
type Team struct {
	ID           int    `kit:"id" json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	TeamName     string `json:"team_name"`
	Location     string `json:"location"`
	FirstYear    string `json:"first_year"`
}

// Player is one player from the players endpoint.
type Player struct {
	ID       int    `kit:"id" json:"id"`
	FullName string `json:"full_name"`
	Number   string `json:"number"`
	Position string `json:"position"`
	TeamID   int    `json:"team_id"`
}

// Standing is one team's row in the standings, flattened with division name.
type Standing struct {
	Division  string `kit:"id" json:"division"`
	TeamName  string `json:"team_name"`
	Wins      int    `json:"wins"`
	Losses    int    `json:"losses"`
	Pct       string `json:"pct"`
	GamesBack string `json:"games_back"`
	Streak    string `json:"streak"`
}

// --- Wire structs for decoding API responses ---

type scheduleResp struct {
	Dates []struct {
		Date  string `json:"date"`
		Games []struct {
			GamePK   int    `json:"gamePk"`
			GameDate string `json:"gameDate"`
			Status   struct {
				AbstractGameState string `json:"abstractGameState"`
			} `json:"status"`
			Teams struct {
				Home struct {
					Score int `json:"score"`
					Team  struct {
						Name string `json:"name"`
					} `json:"team"`
				} `json:"home"`
				Away struct {
					Score int `json:"score"`
					Team  struct {
						Name string `json:"name"`
					} `json:"team"`
				} `json:"away"`
			} `json:"teams"`
			Venue struct {
				Name string `json:"name"`
			} `json:"venue"`
		} `json:"games"`
	} `json:"dates"`
}

type teamsResp struct {
	Teams []struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Abbreviation string `json:"abbreviation"`
		TeamName     string `json:"teamName"`
		LocationName string `json:"locationName"`
		FirstYear    string `json:"firstYearOfPlay"`
	} `json:"teams"`
}

type playersResp struct {
	People []struct {
		ID            int    `json:"id"`
		FullName      string `json:"fullName"`
		PrimaryNumber string `json:"primaryNumber"`
		CurrentTeam   struct {
			ID int `json:"id"`
		} `json:"currentTeam"`
		Position struct {
			Name string `json:"name"`
		} `json:"position"`
	} `json:"people"`
}

type standingsResp struct {
	Records []struct {
		Division struct {
			Name string `json:"name"`
		} `json:"division"`
		TeamRecords []struct {
			Team struct {
				Name string `json:"name"`
			} `json:"team"`
			Wins      int    `json:"wins"`
			Losses    int    `json:"losses"`
			Pct       string `json:"pct"`
			GamesBack string `json:"gamesBack"`
			Streak    struct {
				StreakCode string `json:"streakCode"`
			} `json:"streak"`
		} `json:"teamRecords"`
	} `json:"records"`
}

// --- API call methods ---

// GetSchedule fetches games for the given date (YYYY-MM-DD).
func (c *Client) GetSchedule(ctx context.Context, date string) ([]*Game, error) {
	url := BaseURL + "/api/v1/schedule?sportId=1&date=" + date
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp scheduleResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode schedule: %w", err)
	}
	var out []*Game
	for _, d := range resp.Dates {
		for _, g := range d.Games {
			out = append(out, &Game{
				GamePK:    g.GamePK,
				Date:      d.Date,
				HomeTeam:  g.Teams.Home.Team.Name,
				AwayTeam:  g.Teams.Away.Team.Name,
				HomeScore: g.Teams.Home.Score,
				AwayScore: g.Teams.Away.Score,
				Status:    g.Status.AbstractGameState,
				Venue:     g.Venue.Name,
			})
		}
	}
	return out, nil
}

// GetTeams fetches all MLB teams.
func (c *Client) GetTeams(ctx context.Context) ([]*Team, error) {
	url := BaseURL + "/api/v1/teams?sportId=1"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp teamsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode teams: %w", err)
	}
	var out []*Team
	for _, t := range resp.Teams {
		out = append(out, &Team{
			ID:           t.ID,
			Name:         t.Name,
			Abbreviation: t.Abbreviation,
			TeamName:     t.TeamName,
			Location:     t.LocationName,
			FirstYear:    t.FirstYear,
		})
	}
	return out, nil
}

// GetPlayers fetches all active players for a season year (e.g. "2024").
func (c *Client) GetPlayers(ctx context.Context, season string) ([]*Player, error) {
	url := BaseURL + "/api/v1/sports/1/players?season=" + season
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp playersResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode players: %w", err)
	}
	var out []*Player
	for _, p := range resp.People {
		out = append(out, &Player{
			ID:       p.ID,
			FullName: p.FullName,
			Number:   p.PrimaryNumber,
			Position: p.Position.Name,
			TeamID:   p.CurrentTeam.ID,
		})
	}
	return out, nil
}

// GetGameByPk fetches a single game's metadata using the boxscore endpoint.
func (c *Client) GetGameByPk(ctx context.Context, gamePk string) (*Game, error) {
	url := BaseURL + "/api/v1/game/" + gamePk + "/boxscore"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	// The boxscore response is rich; we extract just the teams.
	var resp struct {
		Teams struct {
			Home struct {
				Team struct {
					Name string `json:"name"`
				} `json:"team"`
				TeamStats struct {
					Batting struct {
						Runs int `json:"runs"`
					} `json:"batting"`
				} `json:"teamStats"`
			} `json:"home"`
			Away struct {
				Team struct {
					Name string `json:"name"`
				} `json:"team"`
				TeamStats struct {
					Batting struct {
						Runs int `json:"runs"`
					} `json:"batting"`
				} `json:"teamStats"`
			} `json:"away"`
		} `json:"teams"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode boxscore: %w", err)
	}
	// Convert gamePk string to int
	var pk int
	fmt.Sscanf(gamePk, "%d", &pk)
	return &Game{
		GamePK:    pk,
		HomeTeam:  resp.Teams.Home.Team.Name,
		AwayTeam:  resp.Teams.Away.Team.Name,
		HomeScore: resp.Teams.Home.TeamStats.Batting.Runs,
		AwayScore: resp.Teams.Away.TeamStats.Batting.Runs,
	}, nil
}

// GetTeamByID fetches a single team by its numeric ID.
func (c *Client) GetTeamByID(ctx context.Context, id string) (*Team, error) {
	url := BaseURL + "/api/v1/teams/" + id
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp teamsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode team: %w", err)
	}
	if len(resp.Teams) == 0 {
		return nil, fmt.Errorf("team %s not found", id)
	}
	t := resp.Teams[0]
	return &Team{
		ID:           t.ID,
		Name:         t.Name,
		Abbreviation: t.Abbreviation,
		TeamName:     t.TeamName,
		Location:     t.LocationName,
		FirstYear:    t.FirstYear,
	}, nil
}

// GetPlayerByID fetches a single player by their numeric ID.
func (c *Client) GetPlayerByID(ctx context.Context, id string) (*Player, error) {
	url := BaseURL + "/api/v1/people/" + id
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp playersResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode player: %w", err)
	}
	if len(resp.People) == 0 {
		return nil, fmt.Errorf("player %s not found", id)
	}
	p := resp.People[0]
	return &Player{
		ID:       p.ID,
		FullName: p.FullName,
		Number:   p.PrimaryNumber,
		Position: p.Position.Name,
		TeamID:   p.CurrentTeam.ID,
	}, nil
}

// GetStandings fetches standings for both leagues for the given season year.
func (c *Client) GetStandings(ctx context.Context, season string) ([]*Standing, error) {
	url := BaseURL + "/api/v1/standings?leagueId=103,104&season=" + season
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp standingsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode standings: %w", err)
	}
	var out []*Standing
	for _, rec := range resp.Records {
		division := rec.Division.Name
		for _, tr := range rec.TeamRecords {
			out = append(out, &Standing{
				Division:  division,
				TeamName:  tr.Team.Name,
				Wins:      tr.Wins,
				Losses:    tr.Losses,
				Pct:       tr.Pct,
				GamesBack: tr.GamesBack,
				Streak:    tr.Streak.StreakCode,
			})
		}
	}
	return out, nil
}
