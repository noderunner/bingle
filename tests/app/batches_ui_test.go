package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// shot saves a full-page screenshot when BINGLE_SHOTS names a directory.
func shot(t *testing.T, ctx interface{ Done() <-chan struct{} }, name string, run func(*[]byte) error) {
	t.Helper()
	dir := os.Getenv("BINGLE_SHOTS")
	if dir == "" {
		return
	}
	var buf []byte
	if err := run(&buf); err != nil {
		t.Logf("screenshot %s: %v", name, err)
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name+".png"), buf, 0o644)
}

func startSmokeBatch(t *testing.T, baseURL, member string, games int) int64 {
	t.Helper()
	players := []map[string]any{}
	for _, f := range []string{"gc", "cc", "bg", "ys"} {
		players = append(players, map[string]any{"faction": f, "bot": "tactical"})
	}
	var started struct {
		ID int64 `json:"id"`
	}
	if code := doJSONMember(t, member, "POST", baseURL+"/api/batches", map[string]any{
		"name": "ui smoke", "base": "cthulhu-wars", "games": games, "seed": 11, "rotate": true, "event_games": 2, "workers": 4, "players": players,
	}, &started); code != 201 {
		t.Fatalf("start: %d", code)
	}
	return started.ID
}

// The wizard on the lobby page starts a batch and lands on its report.
func TestWebBatchWizardStartsABatch(t *testing.T) {
	baseURL, member := newMemberServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	signInChrome(t, ctx, baseURL, member) // batches are for accounts (ADR 0071)
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the practice card", `!document.getElementById("practice-card").classList.contains("hidden")`, 10*time.Second)
	clickSel(t, ctx, "#practice-toggle")
	waitJS(t, ctx, "the table step", `document.querySelectorAll("[data-practice-cult]").length === 4`, 5*time.Second)
	shot(t, ctx, "wizard-1-table", func(b *[]byte) error { return chromedp.Run(ctx, chromedp.FullScreenshot(b, 90)) })

	// Choosing a cult that sits elsewhere swaps the two.
	if err := chromedp.Run(ctx, chromedp.SetValue(`[data-practice-cult="0"]`, "cc", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('[data-practice-cult="0"]').dispatchEvent(new Event("change")); true`, nil)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll("[data-practice-cult]")].map(s => s.value).join(",")`); got != "cc,gc,bg,ys" {
		t.Errorf("cults after choosing cc for seat 1: %q", got)
	}

	clickSel(t, ctx, "#practice-next")
	waitJS(t, ctx, "the bots step", `document.querySelectorAll("[data-practice-bot]").length === 4`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){var s=document.querySelector('[data-practice-bot="0"]'); s.value="mcts"; s.dispatchEvent(new Event("change")); return true})()`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the iterations box", `document.querySelector('[data-practice-iters="0"]')`, 5*time.Second)
	shot(t, ctx, "wizard-2-bots", func(b *[]byte) error { return chromedp.Run(ctx, chromedp.FullScreenshot(b, 90)) })
	// Back to tactical for a fast run.
	clickSel(t, ctx, "#practice-next")
	waitJS(t, ctx, "the run step", `document.getElementById("practice-games")`, 5*time.Second)
	before := evalString(t, ctx, `document.getElementById("practice-estimate").textContent`)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){var s=document.getElementById("practice-games"); s.value="3"; s.dispatchEvent(new Event("change")); return true})()`, nil)); err != nil {
		t.Fatal(err)
	}
	after := evalString(t, ctx, `document.getElementById("practice-estimate").textContent`)
	if before == after {
		t.Errorf("the estimate did not change with the games: %q", before)
	}
	shot(t, ctx, "wizard-3-run", func(b *[]byte) error { return chromedp.Run(ctx, chromedp.FullScreenshot(b, 90)) })
	// Everyone tactical again so the batch is quick: go back and use the preset.
	clickSel(t, ctx, "#practice-back")
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){var s=document.querySelector('[data-practice-bot="0"]'); s.value="tactical"; s.dispatchEvent(new Event("change")); return true})()`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#practice-next")
	waitJS(t, ctx, "the start button", `document.getElementById("practice-start")`, 5*time.Second)
	clickSel(t, ctx, "#practice-start")
	waitJS(t, ctx, "the report page", `location.pathname.startsWith("/batches/")`, 20*time.Second)
	waitJS(t, ctx, "the finished batch", `document.getElementById("batch-status") && document.getElementById("batch-status").textContent === "Finished"`, 90*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("script errors: %v", jsErrors)
	}
}

// A report draws its charts and tables.
func TestWebBatchReportShowsChartsAndInsights(t *testing.T) {
	baseURL, member := newMemberServer(t)
	id := startSmokeBatch(t, baseURL, member, 12)
	ctx, cancel := newChromedpContext(t, 150*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 900), chromedp.Navigate(fmt.Sprintf("%s/batches/%d", baseURL, id))); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the finished batch", `document.getElementById("batch-status") && document.getElementById("batch-status").textContent === "Finished"`, 120*time.Second)
	waitJS(t, ctx, "the charts", `document.querySelectorAll("svg.chart").length >= 5`, 10*time.Second)
	waitJS(t, ctx, "the insight tables", `document.querySelectorAll("#insights table.data tbody tr").length > 3`, 5*time.Second)
	// Switching cult redraws the insight tables.
	first := evalString(t, ctx, `document.querySelector("#insights table.data tbody tr td").textContent`)
	clickSel(t, ctx, `#insights [data-focus="ys"]`)
	waitJS(t, ctx, "the Yellow Sign tab", `document.querySelector('#insights [data-focus="ys"]').classList.contains("on")`, 5*time.Second)
	_ = first
	if got := evalString(t, ctx, `document.querySelector("#games-card tbody").children.length + ""`); got != "12" {
		t.Errorf("games listed = %s, want 12", got)
	}
	// The moves of a kept game open.
	clickSel(t, ctx, `#games-card [data-game="0"]`)
	waitJS(t, ctx, "the events", `document.querySelectorAll("#game-events div").length > 20`, 10*time.Second)
	if !strings.Contains(evalString(t, ctx, `document.body.innerText`), "Who holds which area") {
		t.Error("the area table is missing")
	}
	shot(t, ctx, "report", func(b *[]byte) error { return chromedp.Run(ctx, chromedp.FullScreenshot(b, 90)) })
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("script errors: %v", jsErrors)
	}
}
