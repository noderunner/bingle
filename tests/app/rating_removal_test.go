package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

// Through the API (ADR 0157): only a moderator may look at an account's rated
// games or take one out of the ratings; the audit log has it, and both players
// are mailed.
func TestRatingRemoval_API(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	ana := member(t, e.App, e.URL, "ana")
	bea := member(t, e.App, e.URL, "bea")
	mod := administrator(t, e.App, e.URL, "kim")
	guest := newBrowser(t, e.URL)
	room := playRatedGame(t, ana, "ana", bea, "bea")

	for name, b := range map[string]*browser{"a guest": guest, "a player of the game": bea} {
		if code := b.do("GET", "/api/rated-games?player=ana", nil, nil); code != http.StatusForbidden {
			t.Errorf("%s reads Ana's rated games: %d, want 403", name, code)
		}
		if code := b.do("POST", "/api/rated-games/"+room+"/remove", map[string]any{}, nil); code != http.StatusForbidden {
			t.Errorf("%s removes the game: %d, want 403", name, code)
		}
	}

	var list struct {
		Games []struct {
			RoomCode string `json:"room_code"`
			Opponent string `json:"opponent"`
			Score    float64
			Removed  *struct {
				By string `json:"by"`
			} `json:"removed"`
		} `json:"games"`
	}
	if code := mod.do("GET", "/api/rated-games?player=ana", nil, &list); code != 200 || len(list.Games) != 1 || list.Games[0].RoomCode != room || list.Games[0].Opponent != "bea" || list.Games[0].Removed != nil {
		t.Fatalf("the moderator reads Ana's rated games: %d %+v", code, list)
	}
	if code := mod.do("GET", "/api/rated-games?player=nobody", nil, nil); code != http.StatusNotFound {
		t.Errorf("an unknown player: %d, want 404", code)
	}

	before := len(e.SMTP.Mails())
	var out struct {
		Recomputed bool `json:"recomputed"`
		Players    []struct {
			Handle string `json:"handle"`
			Before int    `json:"before"`
			After  *int   `json:"after"`
		} `json:"players"`
	}
	if code := mod.do("POST", "/api/rated-games/"+room+"/remove", map[string]any{}, &out); code != 200 || !out.Recomputed || len(out.Players) != 2 || out.Players[0].After != nil {
		t.Fatalf("the moderator removes it: %d %+v", code, out)
	}
	if code := mod.do("POST", "/api/rated-games/"+room+"/remove", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("again: %d, want 409", code)
	}
	if code := mod.do("POST", "/api/rated-games/NOSUCH/remove", map[string]any{}, nil); code != http.StatusNotFound {
		t.Errorf("a room with no rated result: %d, want 404", code)
	}

	var view ratedGameView
	if code := ana.do("GET", "/api/games/"+room, nil, &view); code != 200 || view.RatingStatus != "removed" || len(view.RatingChanges) != 0 {
		t.Errorf("the game after: %d %+v", code, view)
	}
	var own struct {
		Ratings []any `json:"ratings"`
	}
	if code := ana.do("GET", "/api/players/ana/ratings", nil, &own); code != 200 || len(own.Ratings) != 0 {
		t.Errorf("Ana's ratings after: %d %+v", code, own)
	}

	var events struct {
		Events []struct {
			Account string `json:"account"`
			Kind    string `json:"kind"`
			Detail  string `json:"detail"`
		} `json:"events"`
	}
	if code := mod.do("GET", "/auth/admin/events", nil, &events); code != 200 || len(events.Events) == 0 ||
		events.Events[0].Kind != "rating-removed" || events.Events[0].Account != "kim" || !strings.Contains(events.Events[0].Detail, room) {
		t.Errorf("the audit log: %d %+v", code, events.Events[:min(len(events.Events), 2)])
	}

	mails := e.waitMails(t, before+2)[before:]
	to := map[string]string{}
	for _, m := range mails {
		to[strings.Join(m.To, ",")] = m.Data
	}
	for _, h := range []string{"ana", "bea"} {
		got, ok := to[h+"@example.com"]
		if !ok {
			t.Errorf("@%s was not mailed: %v", h, to)
			continue
		}
		if !strings.Contains(got, "@kim") || !strings.Contains(got, "Yavalath") || !strings.Contains(got, "/players/"+h) {
			t.Errorf("@%s's mail: %s", h, got)
		}
	}
}
