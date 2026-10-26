package app_test

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The abstract games ship one Rules page with the whole rules (a second tab
// only where it earns its place), with diagrams where they help, and every
// picture a page mentions can be loaded.
func TestGuides_EveryAbstractGameShipsItsRulesAndPictures(t *testing.T) {
	base := newRESTTestServer(t)
	rules := []string{"rules"}
	cases := map[string][]string{
		"tak-5x5": rules, "tak-3x3": rules, "tak-8x8": rules,
		"tic-tac-toe": rules, "connect-four": rules, "gomoku": rules, "hex-11": rules, "hex-7": rules,
		"draughts": rules, "breakthrough": rules, "reversi": rules, "go-9x9": rules, "go-19x19": rules,
		"yavalath": rules, "havannah-8": rules, "havannah-4": rules, "amazons": rules, "amazons-6": rules,
		"ataxx": rules, "pente": rules, "brandubh": rules, "blooms": rules, "blooms-4": rules,
		"fendo": rules, "fendo-9": rules, "yinsh": rules, "yinsh-blitz": rules,
		"onitama": {"rules", "cards"},
	}
	image := regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)`)
	for id, wantTabs := range cases {
		var guide struct {
			Generated bool `json:"generated"`
			Tabs      []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Markdown string `json:"markdown"`
			} `json:"tabs"`
		}
		if code := doJSON(t, "GET", base+"/api/game-defs/"+id+"/guide", nil, &guide); code != 200 {
			t.Errorf("%s: guide %d", id, code)
			continue
		}
		if guide.Generated {
			t.Errorf("%s: no guide of its own", id)
		}
		var got []string
		for _, tab := range guide.Tabs {
			got = append(got, tab.ID)
			if tab.Title == "Pieces" || tab.Title == "Overview" {
				t.Errorf("%s: a tab called %q (the rules are one tab called Rules)", id, tab.Title)
			}
			if len(tab.Markdown) < 500 {
				t.Errorf("%s/%s: only %d bytes of rules", id, tab.ID, len(tab.Markdown))
			}
			for _, m := range image.FindAllStringSubmatch(tab.Markdown, -1) {
				src := m[1]
				if strings.Contains(src, ":") {
					continue // piece:flat:white and the like
				}
				resp, err := http.Get(base + "/api/game-defs/" + id + "/assets/" + src)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != 200 || !strings.Contains(string(body), "<svg") {
					t.Errorf("%s/%s: picture %q: status %d", id, tab.ID, src, resp.StatusCode)
				}
			}
		}
		if strings.Join(got, ",") != strings.Join(wantTabs, ",") {
			t.Errorf("%s: tabs %v, want %v", id, got, wantTabs)
		}
	}
}

// Rules that show diagrams for the games where movement and capture need one.
func TestGuides_TakExplainsStacksRoadsWallsAndTheCapstone(t *testing.T) {
	base := newRESTTestServer(t)
	var guide struct {
		Tabs []struct {
			Markdown string `json:"markdown"`
		} `json:"tabs"`
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/tak-5x5/guide", nil, &guide); code != 200 || len(guide.Tabs) != 1 {
		t.Fatalf("guide: %d %+v", code, guide)
	}
	md := guide.Tabs[0].Markdown
	for _, want := range []string{"## Stacking & Movement", "## Roads", "## Walls", "## The capstone", "tak-stack-move.svg", "tak-road.svg", "tak-not-a-road.svg", "tak-wall-and-cap.svg"} {
		if !strings.Contains(md, want) {
			t.Errorf("the Tak rules lack %q", want)
		}
	}
}

// Onitama's cards page is drawn from the card table: every card is on it.
func TestGuides_OnitamaCardsAreDrawnFromTheCardTable(t *testing.T) {
	base := newRESTTestServer(t)
	resp, err := http.Get(base + "/api/game-defs/onitama/assets/cards.svg")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	svg := string(body)
	for _, card := range []string{"Tiger", "Dragon", "Frog", "Rabbit", "Crab", "Elephant", "Goose", "Rooster", "Monkey", "Mantis", "Horse", "Ox", "Crane", "Boar", "Eel", "Cobra"} {
		if !strings.Contains(svg, ">"+card+"<") {
			t.Errorf("cards.svg lacks the %s card", card)
		}
	}
}

// Nyet!'s rules show all the cards, a row for each suit with the cards splayed
// in the order of their values: sixty of them (the three 1s of each suit).
func TestGuides_NyetShowsEveryCardBySuitAndValue(t *testing.T) {
	base := newRESTTestServer(t)
	var guide struct {
		Tabs []struct {
			ID       string `json:"id"`
			Markdown string `json:"markdown"`
		} `json:"tabs"`
	}
	if code := doJSON(t, "GET", base+"/api/game-defs/nyet/guide", nil, &guide); code != 200 || len(guide.Tabs) == 0 {
		t.Fatalf("guide: %d %+v", code, guide)
	}
	if !strings.Contains(guide.Tabs[0].Markdown, "](cards.svg") {
		t.Error("the cards of Nyet! are not pictured in its rules")
	}
	if strings.Contains(strings.ToLower(guide.Tabs[0].Markdown), "red, blue, yellow") {
		t.Error("the rules still name the colours of the old cards")
	}
	resp, err := http.Get(base + "/api/game-defs/nyet/assets/cards.svg")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	svg := string(body)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "svg") {
		t.Fatalf("cards.svg: status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if n := strings.Count(svg, `rx="9"`); n != 60 {
		t.Errorf("cards.svg draws %d cards, want 60", n)
	}
	// Four rows of fifteen, one per suit, each card in its suit's colour.
	for _, colour := range []string{"#c62f2f", "#1f2630"} {
		if n := strings.Count(svg, "color:"+colour); n != 30 {
			t.Errorf("%d cards in %s, want 30 (two suits)", n, colour)
		}
	}
}
