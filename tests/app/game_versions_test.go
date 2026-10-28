package app_test

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/store"
)

type gameDefInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	SeatsRequired int      `json:"seats_required"`
	Description   string   `json:"description"`
	Family        string   `json:"family"`
	Version       string   `json:"version"`
	Designers     []string `json:"designers"`
	Publisher     string   `json:"publisher"`
}

// Every game says which version of its implementation this is (and what the
// list needs to show about it); variants share their game's version.
func TestGameDefs_CarryVersionDesignersAndPublisher(t *testing.T) {
	base := newRESTTestServer(t)
	var defs []gameDefInfo
	if code := doJSON(t, "GET", base+"/api/game-defs", nil, &defs); code != 200 || len(defs) < 20 {
		t.Fatalf("game-defs: %d, %d games", code, len(defs))
	}
	semver := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	byID := map[string]gameDefInfo{}
	family := map[string]string{}
	for _, d := range defs {
		byID[d.ID] = d
		if !semver.MatchString(d.Version) {
			t.Errorf("%s: version %q is not major.minor.patch", d.ID, d.Version)
		}
		if d.Description == "" {
			t.Errorf("%s: no description for the list", d.ID)
		}
		if v, ok := family[d.Family]; ok && v != d.Version {
			t.Errorf("%s: version %s, but the rest of its family is on %s", d.ID, d.Version, v)
		}
		family[d.Family] = d.Version
		// The game list marks everything below 1.0.0 as a beta: all games are for now.
		if !strings.HasPrefix(d.Version, "0.") {
			t.Errorf("%s: version %s, but no game is out of beta yet", d.ID, d.Version)
		}
	}
	for id, want := range map[string]struct {
		designers []string
		publisher string
	}{
		"tak-5x5":      {[]string{"James Ernest", "Patrick Rothfuss"}, "Cheapass Games"},
		"hive":         {[]string{"John Yianni"}, "Gen42 Games"},
		"cthulhu-wars": {[]string{"Sandy Petersen"}, "Petersen Games"},
		"onitama":      {[]string{"Shimpei Sato"}, "Arcane Wonders"},
		"blooms":       {[]string{"Nick Bentley"}, ""},
		"fist":         {[]string{"James Ernest"}, "Crab Fragment Games"},
	} {
		got := byID[id]
		if strings.Join(got.Designers, "|") != strings.Join(want.designers, "|") || got.Publisher != want.publisher {
			t.Errorf("%s: designers %v, publisher %q; want %v, %q", id, got.Designers, got.Publisher, want.designers, want.publisher)
		}
	}
	if got := byID["amazons"].Name; got != "Amazons" {
		t.Errorf("the game of the Amazons is called %q", got)
	}
	// The seat counts the list turns into "3–5 players".
	if byID["cthulhu-wars-3p"].SeatsRequired != 3 || byID["cthulhu-wars-5p"].SeatsRequired != 5 || byID["nyet-2"].SeatsRequired != 2 || byID["nyet-5"].SeatsRequired != 5 {
		t.Errorf("seats: cw 3p %d, 5p %d, nyet 2 %d, 5 %d", byID["cthulhu-wars-3p"].SeatsRequired, byID["cthulhu-wars-5p"].SeatsRequired, byID["nyet-2"].SeatsRequired, byID["nyet-5"].SeatsRequired)
	}
}

// The database keeps the versions the server has run: once each, however often it starts.
func TestGameVersions_AreRecordedInTheDatabaseOnce(t *testing.T) {
	db := filepath.Join(t.TempDir(), "bingle.db")
	for i := 0; i < 2; i++ {
		a, err := app.New(config.Config{DBPath: db})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	versions, err := st.GameVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string][]string{}
	for _, v := range versions {
		seen[v.Family] = append(seen[v.Family], v.Version)
		if v.FirstSeen == "" {
			t.Errorf("%s %s: no first-seen time", v.Family, v.Version)
		}
	}
	for family, want := range map[string]string{"tak-5x5": "0.1.0", "hive": "0.1.0", "cthulhu-wars": "0.1.0", "amazons": "0.2.0", "go-9x9": "0.2.0", "nyet": "0.2.0", "fist": "0.1.1"} {
		if got := seen[family]; len(got) != 1 || got[0] != want {
			t.Errorf("%s: recorded versions %v, want [%s] (once, though the server started twice)", family, got, want)
		}
	}
	// A new version of a game is a new row, and the old one stays.
	if err := st.RecordGameVersion(context.Background(), "tak-5x5", "0.2.0"); err != nil {
		t.Fatal(err)
	}
	versions, _ = st.GameVersions(context.Background())
	var tak []string
	for _, v := range versions {
		if v.Family == "tak-5x5" {
			tak = append(tak, v.Version)
		}
	}
	if strings.Join(tak, ",") != "0.1.0,0.2.0" {
		t.Errorf("tak versions %v", tak)
	}
}
