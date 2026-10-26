package app_test

import (
	"strings"
	"testing"
)

// Fist ships its own rules guide: an overview, a page explaining every piece
// and a page on moving and capturing — not the generated table of pieces.
func TestFistRulesGuideExplainsEveryPiece(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var guide struct {
		Generated bool `json:"generated"`
		Tabs      []struct {
			ID       string `json:"id"`
			Markdown string `json:"markdown"`
		} `json:"tabs"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/fist/guide", nil, &guide); code != 200 {
		t.Fatalf("guide: %d", code)
	}
	if guide.Generated || len(guide.Tabs) != 3 {
		t.Fatalf("generated=%v tabs=%d", guide.Generated, len(guide.Tabs))
	}
	var pieces string
	for _, tab := range guide.Tabs {
		if tab.ID == "pieces" {
			pieces = tab.Markdown
		}
	}
	for _, heading := range []string{"## Yellow guard", "## Blue guard", "## Ace", "## Mage", "## King", "## Verdigris pawn"} {
		if !strings.Contains(pieces, heading) {
			t.Errorf("the pieces page has no %q section", heading)
		}
	}
	for _, word := range []string{"Convert", "Empower", "pinning"} {
		if !strings.Contains(pieces, word) {
			t.Errorf("the pieces page never explains %s", word)
		}
	}
}
