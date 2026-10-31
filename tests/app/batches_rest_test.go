package app_test

import (
	"fmt"
	"testing"
	"time"
)

// A practice batch runs in the background, reports progress and serves its
// aggregated report.
func TestRESTBatchRunsAndReports(t *testing.T) {
	baseURL, member := newMemberServer(t) // batches are for accounts (ADR 0071)

	var errBody map[string]any
	if code := doJSONMember(t, member, "POST", baseURL+"/api/batches", map[string]any{
		"base": "cthulhu-wars", "games": 2, "players": []map[string]any{{"faction": "gc", "bot": "tactical"}},
	}, &errBody); code != 400 {
		t.Errorf("a short line-up: %d, want 400", code)
	}

	var started struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Games  int    `json:"games"`
	}
	players := []map[string]any{}
	for _, f := range []string{"gc", "cc", "bg", "ys"} {
		players = append(players, map[string]any{"faction": f, "bot": "tactical"})
	}
	if code := doJSONMember(t, member, "POST", baseURL+"/api/batches", map[string]any{
		"name": "smoke", "base": "cthulhu-wars", "games": 3, "seed": 5, "rotate": true, "event_games": 1, "workers": 3, "players": players,
	}, &started); code != 201 || started.ID == 0 || started.Games != 3 {
		t.Fatalf("start: %d %+v", code, started)
	}
	url := fmt.Sprintf("%s/api/batches/%d", baseURL, started.ID)

	var rep struct {
		Batch struct {
			Status string `json:"status"`
			Done   int    `json:"done"`
		} `json:"batch"`
		Report struct {
			Games    int `json:"games"`
			Factions []struct {
				Key   string `json:"key"`
				Games int    `json:"games"`
				Areas []struct {
					Area string `json:"area"`
				} `json:"areas"`
				Units []struct {
					Type string `json:"type"`
				} `json:"units"`
			} `json:"factions"`
			AreaNames map[string]string `json:"area_names"`
		} `json:"report"`
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if code := doJSON(t, "GET", url+"/report", nil, &rep); code != 200 {
			t.Fatalf("report: %d", code)
		}
		if rep.Batch.Status == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("batch still %s after %d games", rep.Batch.Status, rep.Batch.Done)
		}
		time.Sleep(300 * time.Millisecond)
	}
	if rep.Report.Games != 3 || len(rep.Report.Factions) != 4 || len(rep.Report.AreaNames) == 0 {
		t.Fatalf("report = %+v", rep.Report)
	}
	for _, f := range rep.Report.Factions {
		if f.Games != 3 || len(f.Areas) == 0 || len(f.Units) == 0 {
			t.Errorf("faction %s: %+v", f.Key, f)
		}
	}

	var games struct {
		Games []struct {
			Index int `json:"index"`
		} `json:"games"`
	}
	if code := doJSON(t, "GET", url+"/games", nil, &games); code != 200 || len(games.Games) != 3 {
		t.Errorf("games: %d %+v", code, games)
	}
	var ev struct {
		Events []map[string]any `json:"events"`
	}
	if code := doJSON(t, "GET", url+"/games/0/events", nil, &ev); code != 200 || len(ev.Events) == 0 {
		t.Errorf("events of game 0: %d, %d events", code, len(ev.Events))
	}
	if code := doJSON(t, "GET", url+"/games/1/events", nil, &ev); code != 200 || len(ev.Events) != 0 {
		t.Errorf("game 1 keeps no events: %d, %d", code, len(ev.Events))
	}
	var list struct {
		Batches []struct{ ID int64 } `json:"batches"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/batches", nil, &list); code != 200 || len(list.Batches) != 1 {
		t.Errorf("list: %d %+v", code, list)
	}
	// Deleting is for an account, like starting.
	if code := doJSON(t, "DELETE", url, nil, nil); code != 403 {
		t.Errorf("an anonymous delete: %d, want 403", code)
	}
	if code := doJSONMember(t, member, "DELETE", url, nil, nil); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code := doJSON(t, "GET", url, nil, &errBody); code != 404 {
		t.Errorf("deleted batch: %d, want 404", code)
	}
}
