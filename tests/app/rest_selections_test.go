// Tests of the selections query over REST and MCP: a piece's destinations are the options from its square.

package app_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// sortedStrings returns a sorted copy of ss, for order-independent slice
// comparison.
func sortedStrings(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

// selectionView is the part of GET /selections and list_selections the tests read.
type selectionView struct {
	Seat    int `json:"seat"`
	Options []struct {
		Notation string `json:"notation"`
		From     string `json:"from"`
		To       string `json:"to"`
		Gestures []struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"gestures"`
	} `json:"options"`
}

// destinations lists the squares the options take a piece on from to.
func (v selectionView) destinations(from string) []string {
	out := []string{}
	for _, o := range v.Options {
		if o.From == from {
			out = append(out, o.To)
		}
	}
	return sortedStrings(out)
}

// Right after the start black's stone at "4,0" has three empty neighbours on the y=0 edge, and no
// selection starts from an empty square.
func TestSelections_APieceGoesToEachEmptyNeighbour(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := setupActiveGame(t, baseURL)
	want := sortedStrings([]string{"3,0", "5,0", "4,1"})

	var v selectionView
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/selections", nil, &v); code != http.StatusOK {
		t.Fatalf("selections: %d", code)
	}
	if got := v.destinations("4,0"); !slices.Equal(got, want) {
		t.Errorf("from 4,0: %v, want %v", got, want)
	}
	if got := v.destinations("5,5"); len(got) != 0 {
		t.Errorf("from the empty 5,5: %v", got)
	}
	for _, o := range v.Options {
		if len(o.Gestures) == 0 {
			t.Fatalf("%s comes without its gestures", o.Notation)
		}
	}

	var m selectionView
	decodeToolResult(t, callTool(t, newMCPClient(t, baseURL), "list_selections", map[string]any{"room_code": room}), &m)
	if got := m.destinations("4,0"); !slices.Equal(got, want) {
		t.Errorf("MCP from 4,0: %v, want %v", got, want)
	}
}

// An unknown room is a 404 with an error over REST, and a tool error over MCP.
func TestSelections_UnknownRoom(t *testing.T) {
	baseURL := newRESTTestServer(t)
	resp, raw := getJSON(t, baseURL, "/api/games/ZZZZZZ/selections")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", resp.StatusCode, raw)
	}
	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil || body.Error == "" {
		t.Errorf("error body: %s", raw)
	}
	if res := callTool(t, newMCPClient(t, baseURL), "list_selections", map[string]any{"room_code": "ZZZZZZ"}); !res.IsError {
		t.Errorf("list_selections of an unknown room is no error: %+v", res.Content)
	}
}

// The old names are gone, without aliases (ADR 0114).
func TestSelections_TheOldNamesAreGone(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := setupActiveGame(t, baseURL)
	for _, path := range []string{"/moves/legal", "/legal-moves?from=4,0"} {
		if resp, _ := getJSON(t, baseURL, "/api/games/"+room+path); resp.StatusCode == http.StatusOK {
			t.Errorf("GET %s still answers", path)
		}
	}
	res, err := newMCPClient(t, baseURL).ListTools(t.Context(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if slices.Contains([]string{"get_legal_moves", "list_legal_moves", "auto_play_turn"}, tool.Name) {
			t.Errorf("MCP %s is still a tool", tool.Name)
		}
	}
}
