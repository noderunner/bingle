package app_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// quoteJS is a Go string as a JavaScript literal.
func quoteJS(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// A batch can be run on any game, not only Cthulhu Wars: the report says how
// each seat fared, which bot played it, how the games ended and how long they
// ran.
func TestBatches_AnyGameCanBeBatched(t *testing.T) {
	baseURL, member := newMemberServer(t) // batches are for accounts (ADR 0071)
	var started struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	body := map[string]any{
		"name": "Tic-Tac-Toe · tactical v random",
		"base": "tic-tac-toe",
		"players": []map[string]any{
			{"bot": "tactical"},
			{"bot": "random"},
		},
		"games": 12, "rotate": true,
	}
	if code := doJSONMember(t, member, "POST", baseURL+"/api/batches", body, &started); code != 201 {
		t.Fatalf("start a batch of Tic-Tac-Toe: %d", code)
	}
	// It finishes quickly: wait for it.
	var rep struct {
		Batch struct {
			Status string `json:"status"`
			Done   int    `json:"done"`
		} `json:"batch"`
		Report struct {
			Games      int            `json:"games"`
			Decided    int            `json:"decided"`
			EndReasons map[string]int `json:"end_reasons"`
			Plies      struct {
				Min, Mean, Max float64
			} `json:"plies"`
			Seats []struct {
				Seat  int      `json:"seat"`
				Label string   `json:"label"`
				Color string   `json:"color"`
				Bots  []string `json:"bots"`
				Games int      `json:"games"`
				Wins  int      `json:"wins"`
				Draws int      `json:"draws"`
			} `json:"seats"`
			Bots []struct {
				Bot   string `json:"bot"`
				Games int    `json:"games"`
				Wins  int    `json:"wins"`
			} `json:"bots"`
			Factions []any `json:"factions"`
		} `json:"report"`
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if code := doJSON(t, "GET", fmt.Sprintf("%s/api/batches/%d/report", baseURL, started.ID), nil, &rep); code != 200 {
			t.Fatalf("report: %d", code)
		}
		if rep.Batch.Status == "done" {
			break
		}
		if rep.Batch.Status == "failed" {
			t.Fatalf("the batch failed")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if rep.Batch.Status != "done" {
		t.Fatalf("the batch is %q after a minute", rep.Batch.Status)
	}
	if rep.Report.Games != 12 {
		t.Fatalf("the report counts %d games, want 12", rep.Report.Games)
	}
	// Tic-Tac-Toe always ends, one way or the other.
	if rep.Report.EndReasons["unfinished"] != 0 {
		t.Errorf("unfinished games in Tic-Tac-Toe: %v", rep.Report.EndReasons)
	}
	if rep.Report.Plies.Min < 5 || rep.Report.Plies.Max > 9 {
		t.Errorf("games of %v–%v moves, want 5 to 9", rep.Report.Plies.Min, rep.Report.Plies.Max)
	}
	// Two seats, each with the game's own colours, each playing all 12 games,
	// and the wins and draws adding up.
	if len(rep.Report.Seats) != 2 {
		t.Fatalf("the report has %d seats: %+v", len(rep.Report.Seats), rep.Report.Seats)
	}
	total := 0
	for _, s := range rep.Report.Seats {
		if s.Games != 12 || s.Color == "" {
			t.Errorf("seat %d = %+v", s.Seat, s)
		}
		total += s.Wins
		// The bots were moved round the seats, so both sat here.
		if len(s.Bots) != 2 {
			t.Errorf("seat %d was played by %v, want both bots", s.Seat, s.Bots)
		}
	}
	if total != rep.Report.Decided {
		t.Errorf("the seats won %d games between them, but %d were decided", total, rep.Report.Decided)
	}
	// By bot: tactical should be ahead of random at Tic-Tac-Toe.
	if len(rep.Report.Bots) != 2 {
		t.Fatalf("the report has %d bots: %+v", len(rep.Report.Bots), rep.Report.Bots)
	}
	wins := map[string]int{}
	for _, b := range rep.Report.Bots {
		wins[b.Bot] = b.Wins
		if b.Games != 12 {
			t.Errorf("%s played %d seats, want 12", b.Bot, b.Games)
		}
	}
	if wins["tactical"] <= wins["random"] {
		t.Errorf("tactical won %d and random %d over 12 games of Tic-Tac-Toe", wins["tactical"], wins["random"])
	}
	// No cult report for a game that has no cults.
	if len(rep.Report.Factions) != 0 {
		t.Errorf("a game with no cults has %d faction reports", len(rep.Report.Factions))
	}
}

// A spec that names a cult for some seats but not others is refused, and so is
// a Cthulhu Wars option on a game that has none.
func TestBatches_SpecIsCheckedForEveryGame(t *testing.T) {
	baseURL, member := newMemberServer(t)
	for name, tc := range map[string]struct {
		body map[string]any
		says string
	}{
		"half a table of cults": {map[string]any{"base": "tic-tac-toe", "games": 1, "players": []map[string]any{{"bot": "random", "faction": "gc"}, {"bot": "random"}}}, "name one for all of them or for none"},
		"high priests nowhere":  {map[string]any{"base": "tic-tac-toe", "games": 1, "high_priests": true, "players": []map[string]any{{"bot": "random"}, {"bot": "random"}}}, "Cthulhu Wars option"},
		"too few seats":         {map[string]any{"base": "tic-tac-toe", "games": 1, "players": []map[string]any{{"bot": "random"}}}, "seats 2 players"},
		"an unknown bot":        {map[string]any{"base": "tic-tac-toe", "games": 1, "players": []map[string]any{{"bot": "gandalf"}, {"bot": "random"}}}, "unknown bot"},
	} {
		var out map[string]any
		code := doJSONMember(t, member, "POST", baseURL+"/api/batches", tc.body, &out)
		if code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
			continue
		}
		if msg, _ := out["error"].(string); !strings.Contains(msg, tc.says) {
			t.Errorf("%s says %q, want it to mention %q", name, msg, tc.says)
		}
	}
}

// The Analysis tab is where a batch is set going: pick a game, pick the bots,
// run it, and read the report — with the guide to the bots under it.
func TestWebAnalysis_TheTabRunsABatchAndExplainsTheBots(t *testing.T) {
	baseURL, member := newMemberServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	signInChrome(t, ctx, baseURL, member) // batches are for accounts (ADR 0071)
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tabs", `document.getElementById("tab-analysis") !== null`, 20*time.Second)
	clickSel(t, ctx, "#tab-analysis")
	waitJS(t, ctx, "the form", `document.getElementById("analysis-start") !== null && document.querySelectorAll("#analysis-seats .analysis-bot").length > 0`, 20*time.Second)

	// Every game is on offer, and picking one shows a bot for each of its seats.
	if n := evalInt(t, ctx, `document.querySelectorAll("#analysis-game option").length`); n < 20 {
		t.Errorf("%d games on offer", n)
	}
	evalString(t, ctx, `(function () {
		var g = document.getElementById("analysis-game");
		g.value = "tic-tac-toe";
		g.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	waitJS(t, ctx, "two seats", `document.querySelectorAll("#analysis-seats .analysis-bot").length === 2`, 10*time.Second)
	// Run a small batch of tactical against random.
	evalString(t, ctx, `(function () {
		var sels = document.querySelectorAll("#analysis-seats .analysis-bot");
		sels[0].value = "tactical";
		sels[1].value = "random";
		document.getElementById("analysis-games").value = "8";
		return "ok";
	})()`)
	clickSel(t, ctx, "#analysis-start")
	waitJS(t, ctx, "the batch in the list", `document.querySelectorAll("#analysis-batches li[data-batch]").length === 1`, 20*time.Second)
	waitJS(t, ctx, "the report", `document.querySelector("#analysis-seat-card") !== null && document.querySelectorAll("#analysis-seat-card .chart").length === 1`, 40*time.Second)
	// The report follows the batch as it runs, so the count settles at eight.
	waitJS(t, ctx, "all eight games counted", `(function () {
		var dd = document.querySelectorAll("#analysis-endings .stat-list dd");
		return dd.length > 0 && dd[0].textContent === "8";
	})()`, 40*time.Second)
	if got := evalString(t, ctx, `document.getElementById("analysis-endings").textContent`); !strings.Contains(got, "Moves per game") {
		t.Errorf("the endings card reads %q", got)
	}
	// Win rate by bot, since two different bots played.
	waitJS(t, ctx, "the bot card", `document.getElementById("analysis-bot-card") !== null`, 20*time.Second)
	// And the full report is a click away.
	if got := evalString(t, ctx, `(document.querySelector('#analysis-report a[href^="/batches/"]') || {}).textContent || ""`); got == "" {
		t.Error("the report does not link to the full one")
	}

	// The guide: its headings, its three drawings, and its references.
	waitJS(t, ctx, "the guide", `document.querySelectorAll("#analysis-guide .md-h").length > 5`, 30*time.Second)
	for _, want := range []string{"Monte Carlo tree search", "alpha-beta", "pie rule", "Kocsis"} {
		if !evalBool(t, ctx, `document.getElementById("analysis-guide").textContent.indexOf(`+quoteJS(want)+`) >= 0`) {
			t.Errorf("the guide does not mention %q", want)
		}
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#analysis-guide img[src^="/api/bot-guide/assets/"]').length`); n != 3 {
		t.Errorf("%d drawings in the guide, want 3", n)
	}
	// The drawings really are served.
	if !evalBool(t, ctx, `(function () { var img = document.querySelector('#analysis-guide img'); return img.complete === false || img.naturalWidth > 0; })()`) {
		t.Error("a drawing of the guide did not load")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
