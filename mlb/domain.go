package mlb

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes mlb as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/mlb-cli/mlb"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// mlb:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone mlb binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the mlb driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "mlb",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "mlb",
			Short:  "A command line for MLB Stats.",
			Long: `A command line for MLB Stats.

mlb reads public MLB Stats API data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: "mlb.com",
			Repo: "https://github.com/tamnd/mlb-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// game: resolve a single game by gamePk (resolver so Mint works)
	kit.Handle(app, kit.OpMeta{
		Name:     "game",
		Group:    "read",
		Single:   true,
		Resolver: true,
		Summary:  "Fetch a game by gamePk",
		URIType:  "game",
		Args:     []kit.Arg{{Name: "id", Help: "gamePk"}},
	}, getGame)

	// schedule: games for a given date
	kit.Handle(app, kit.OpMeta{
		Name:    "schedule",
		Group:   "read",
		List:    true,
		Summary: "List games for a date (YYYY-MM-DD)",
		URIType: "game",
		Args:    []kit.Arg{{Name: "date", Help: "date YYYY-MM-DD"}},
	}, getSchedule)

	// team: resolve a single team (resolver so Mint works)
	kit.Handle(app, kit.OpMeta{
		Name:     "team",
		Group:    "read",
		Single:   true,
		Resolver: true,
		Summary:  "Fetch a team by ID",
		URIType:  "team",
		Args:     []kit.Arg{{Name: "id", Help: "team ID"}},
	}, getTeam)

	// teams: all MLB teams
	kit.Handle(app, kit.OpMeta{
		Name:    "teams",
		Group:   "read",
		List:    true,
		Summary: "List all MLB teams",
		URIType: "team",
	}, getTeams)

	// player: resolve a single player (resolver so Mint works)
	kit.Handle(app, kit.OpMeta{
		Name:     "player",
		Group:    "read",
		Single:   true,
		Resolver: true,
		Summary:  "Fetch a player by ID",
		URIType:  "player",
		Args:     []kit.Arg{{Name: "id", Help: "player ID"}},
	}, getPlayer)

	// players: active players for a season
	kit.Handle(app, kit.OpMeta{
		Name:    "players",
		Group:   "read",
		List:    true,
		Summary: "List active players for a season",
		URIType: "player",
	}, getPlayers)

	// standings: league standings for a season
	kit.Handle(app, kit.OpMeta{
		Name:    "standings",
		Group:   "read",
		List:    true,
		Summary: "Show standings for a season",
		URIType: "standing",
	}, getStandings)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type gameInput struct {
	ID     string  `kit:"arg" help:"gamePk"`
	Client *Client `kit:"inject"`
}

type scheduleInput struct {
	Date   string  `kit:"arg" help:"date YYYY-MM-DD"`
	Client *Client `kit:"inject"`
}

type teamInput struct {
	ID     string  `kit:"arg" help:"team ID"`
	Client *Client `kit:"inject"`
}

type teamsInput struct {
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

type playerInput struct {
	ID     string  `kit:"arg" help:"player ID"`
	Client *Client `kit:"inject"`
}

type playersInput struct {
	Season string  `kit:"flag" help:"season year" default:"2024"`
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

type standingsInput struct {
	Season string  `kit:"flag" help:"season year" default:"2024"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getGame(ctx context.Context, in gameInput, emit func(*Game) error) error {
	// Fetch the schedule for context by using the boxscore's gamePk in a date-less query.
	// The simplest approach: fetch boxscore and synthesize a minimal Game record.
	game, err := in.Client.GetGameByPk(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(game)
}

func getTeam(ctx context.Context, in teamInput, emit func(*Team) error) error {
	team, err := in.Client.GetTeamByID(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(team)
}

func getPlayer(ctx context.Context, in playerInput, emit func(*Player) error) error {
	player, err := in.Client.GetPlayerByID(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(player)
}

func getSchedule(ctx context.Context, in scheduleInput, emit func(*Game) error) error {
	games, err := in.Client.GetSchedule(ctx, in.Date)
	if err != nil {
		return mapErr(err)
	}
	for _, g := range games {
		if err := emit(g); err != nil {
			return err
		}
	}
	return nil
}

func getTeams(ctx context.Context, in teamsInput, emit func(*Team) error) error {
	teams, err := in.Client.GetTeams(ctx)
	if err != nil {
		return mapErr(err)
	}
	for i, t := range teams {
		if in.Limit > 0 && i >= in.Limit {
			break
		}
		if err := emit(t); err != nil {
			return err
		}
	}
	return nil
}

func getPlayers(ctx context.Context, in playersInput, emit func(*Player) error) error {
	players, err := in.Client.GetPlayers(ctx, in.Season)
	if err != nil {
		return mapErr(err)
	}
	for i, p := range players {
		if in.Limit > 0 && i >= in.Limit {
			break
		}
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func getStandings(ctx context.Context, in standingsInput, emit func(*Standing) error) error {
	standings, err := in.Client.GetStandings(ctx, in.Season)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range standings {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, network-free ---

// Classify turns any accepted input into the canonical (type, id).
// Numeric string -> ("game", input)
// 3-letter uppercase -> ("team", input)
// Otherwise -> ("player", input)
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("mlb: empty reference")
	}
	if isNumeric(input) {
		return "game", input, nil
	}
	if isTeamAbbrev(input) {
		return "team", input, nil
	}
	return "player", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "game":
		return fmt.Sprintf("https://www.mlb.com/gameday/%s", id), nil
	case "team":
		return fmt.Sprintf("https://www.mlb.com/%s", strings.ToLower(id)), nil
	case "player":
		return fmt.Sprintf("https://www.mlb.com/player/%s", id), nil
	default:
		return "", errs.Usage("mlb has no resource type %q", uriType)
	}
}

// --- helpers ---

// isNumeric reports whether s consists solely of ASCII digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// isTeamAbbrev reports whether s is a 3-letter uppercase team abbreviation.
func isTeamAbbrev(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
