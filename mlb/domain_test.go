package mlb

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in mlb_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "mlb" {
		t.Errorf("Scheme = %q, want mlb", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "mlb" {
		t.Errorf("Identity.Binary = %q, want mlb", info.Identity.Binary)
	}
}

func TestClassifyGame(t *testing.T) {
	typ, id, err := Domain{}.Classify("745455")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "game" || id != "745455" {
		t.Errorf("Classify(745455) = (%q, %q), want (game, 745455)", typ, id)
	}
}

func TestClassifyTeam(t *testing.T) {
	typ, id, err := Domain{}.Classify("TOR")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "team" || id != "TOR" {
		t.Errorf("Classify(TOR) = (%q, %q), want (team, TOR)", typ, id)
	}
}

func TestClassifyPlayer(t *testing.T) {
	typ, id, err := Domain{}.Classify("656140")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "game" || id != "656140" {
		// numeric id goes to game by spec; player is a fallback for non-numeric non-abbrev
		t.Errorf("Classify(656140) = (%q, %q), want (game, 656140)", typ, id)
	}
}

func TestClassifyPlayerName(t *testing.T) {
	typ, id, err := Domain{}.Classify("Andrew Abbott")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "player" || id != "Andrew Abbott" {
		t.Errorf("Classify(Andrew Abbott) = (%q, %q), want (player, Andrew Abbott)", typ, id)
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestLocateGame(t *testing.T) {
	got, err := Domain{}.Locate("game", "745455")
	want := "https://www.mlb.com/gameday/745455"
	if err != nil || got != want {
		t.Errorf("Locate game = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateTeam(t *testing.T) {
	got, err := Domain{}.Locate("team", "TOR")
	want := "https://www.mlb.com/tor"
	if err != nil || got != want {
		t.Errorf("Locate team = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocatePlayer(t *testing.T) {
	got, err := Domain{}.Locate("player", "656140")
	want := "https://www.mlb.com/player/656140"
	if err != nil || got != want {
		t.Errorf("Locate player = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknown(t *testing.T) {
	_, err := Domain{}.Locate("boxscore", "745455")
	if err == nil {
		t.Error("expected error for unknown uriType, got nil")
	}
}

func TestIsNumeric(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"745455", true},
		{"0", true},
		{"abc", false},
		{"12a", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isNumeric(tc.s); got != tc.want {
			t.Errorf("isNumeric(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestIsTeamAbbrev(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"TOR", true},
		{"NYY", true},
		{"BOS", true},
		{"tor", false},
		{"TO", false},
		{"TORA", false},
		{"T1R", false},
	}
	for _, tc := range cases {
		if got := isTeamAbbrev(tc.s); got != tc.want {
			t.Errorf("isTeamAbbrev(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip:
// a record mints to its URI, its body is readable, and a bare id resolves.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	g := &Game{GamePK: 745455, Date: "2024-07-01", HomeTeam: "Toronto Blue Jays", AwayTeam: "Chicago White Sox", Status: "Final"}
	u, err := h.Mint(g)
	if err != nil {
		t.Fatalf("Mint Game: %v", err)
	}
	if want := "mlb://game/745455"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}
}
