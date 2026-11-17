package app_test

import (
	"reflect"
	"testing"
)

// A game's info says, as data, which regions each seat must connect, so a client can show the goal without reading its words (#216).
func TestGameInfo_GoalsSayWhichRegionsEachSeatMustJoin(t *testing.T) {
	baseURL := newRESTTestServer(t)
	type goal struct {
		Seat    int        `json:"seat"`
		Type    string     `json:"type"`
		Regions [][]string `json:"regions"`
		Of      []string   `json:"of"`
		Touch   int        `json:"touch"`
		Piece   string     `json:"piece"`
	}
	for id, want := range map[string][]goal{
		"hex-11": {
			{Seat: 1, Type: "connect", Regions: [][]string{{"top", "bottom"}}},
			{Seat: 2, Type: "connect", Regions: [][]string{{"left", "right"}}},
		},
		"hex-7": {
			{Seat: 1, Type: "connect", Regions: [][]string{{"top", "bottom"}}},
			{Seat: 2, Type: "connect", Regions: [][]string{{"left", "right"}}},
		},
		"havannah-8": {
			{Type: "connect", Of: []string{"corner-0", "corner-1", "corner-2", "corner-3", "corner-4", "corner-5"}, Touch: 2},
			{Type: "connect", Of: []string{"top", "bottom", "upper-left", "lower-left", "upper-right", "lower-right"}, Touch: 3},
		},
		"breakthrough": nil, // its goal is to reach a row, not to connect
		"tic-tac-toe":  nil,
	} {
		var info struct {
			Goals []goal `json:"goals"`
			Board struct {
				Regions map[string][]string `json:"regions"`
			} `json:"board"`
		}
		if code := doJSON(t, "GET", baseURL+"/api/game-defs/"+id, nil, &info); code != 200 {
			t.Fatalf("GET /api/game-defs/%s: %d", id, code)
		}
		if !reflect.DeepEqual(info.Goals, want) {
			t.Errorf("%s: goals = %+v, want %+v", id, info.Goals, want)
		}
		// Every region a goal names is one of the board's.
		for _, g := range info.Goals {
			for _, name := range append(g.Of, goalRegionPairs(g.Regions)...) {
				if _, ok := info.Board.Regions[name]; !ok {
					t.Errorf("%s: goal %+v names %q, not a region of the board", id, g, name)
				}
			}
		}
	}
}

func goalRegionPairs(pairs [][]string) []string {
	var out []string
	for _, p := range pairs {
		out = append(out, p...)
	}
	return out
}
