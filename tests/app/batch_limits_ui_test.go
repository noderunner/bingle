package app_test

// What a player sees of the limits on batches (ADR 0161): the Analysis form
// says which allowance refused a batch, and a batch that a limit ended says so
// in place of its status, in the list and on its page.

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/store"
)

func TestWebBatchLimits_TheFormAndTheBatchSayWhichLimit(t *testing.T) {
	base, ana, _, dbPath := limitedServerAt(t, config.BatchLimits{GamesPerDay: 150})
	// A batch ended at its time limit, with two of its five games played.
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	bg := context.Background()
	id, err := st.CreateBatch(bg, "", "hex-11", `{"base":"hex-11","players":[{"bot":"random"},{"bot":"random"}],"games":5,"seed":1,"rotate":true}`, 5, "p_other")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := st.AddBatchGame(bg, store.BatchGame{BatchID: id, Index: i, Seed: int64(i), GameID: "hex-11", Seating: "[\"\",\"\"]", Winner: 1, EndReason: "win", Plies: 40, Metrics: "null"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.EndBatchAtLimit(bg, id, "time"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana)
	if err := runSteps(ctx, stepNavigate(base+"/#analysis")); err != nil {
		t.Fatal(err)
	}
	waitScripts(t, ctx)
	waitJS(t, ctx, "the batch in the list", `document.querySelector("#analysis-batches li[data-batch] .batch-status.limit")`, 20*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#analysis-batches .batch-status").textContent`); got != "Time limit reached · 2/5" {
		t.Errorf("the list says %q", got)
	}

	// Two batches of 100 games are more than 150 in a day: the second is
	// refused, and the form says which allowance.
	waitJS(t, ctx, "the form", `document.getElementById("analysis-start") !== null && document.querySelectorAll("#analysis-seats .analysis-bot").length > 0`, 20*time.Second)
	evalString(t, ctx, `(function () {
		var g = document.getElementById("analysis-game");
		g.value = "tic-tac-toe";
		g.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	waitJS(t, ctx, "two seats", `document.querySelectorAll("#analysis-seats .analysis-bot").length === 2`, 10*time.Second)
	evalString(t, ctx, `(function () {
		document.querySelectorAll("#analysis-seats .analysis-bot").forEach((s) => { s.value = "random"; });
		document.getElementById("analysis-games").value = "100";
		return "ok";
	})()`)
	clickSel(t, ctx, "#analysis-start")
	waitJS(t, ctx, "two batches in the list", `document.querySelectorAll("#analysis-batches li[data-batch]").length === 2`, 20*time.Second)
	waitJS(t, ctx, "the first batch finished", `[...document.querySelectorAll("#analysis-batches .batch-status.done")].length === 1`, 60*time.Second)
	clickSel(t, ctx, "#analysis-start")
	waitJS(t, ctx, "the refusal", `document.getElementById("analysis-error").textContent.indexOf("games_per_day") >= 0`, 20*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#analysis-batches li[data-batch]").length`); n != 2 {
		t.Errorf("%d batches after a refused one, want 2", n)
	}

	// The batch's own page says the same in its header.
	if err := runSteps(ctx, stepNavigate(base+"/batches/"+strconv.FormatInt(id, 10))); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the header", `document.getElementById("batch-status") !== null`, 20*time.Second)
	if got := evalString(t, ctx, `document.getElementById("batch-status").textContent + "|" + document.getElementById("batch-status").className`); got != "Time limit reached|batch-status cancelled limit" {
		t.Errorf("the page says %q", got)
	}
}
