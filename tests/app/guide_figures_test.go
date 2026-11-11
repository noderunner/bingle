package app_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// The server draws no pictures of a game (ADR 0124): there is no board.svg
// for an areas board, and the list of games names no map picture.
func TestGuides_TheServerDrawsNoMap(t *testing.T) {
	base := newRESTTestServer(t)
	resp, err := http.Get(base + "/api/game-defs/test-council/assets/board.svg")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("board.svg of an areas board: %d, want 404", resp.StatusCode)
	}
	resp, err = http.Get(base + "/api/game-defs")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if strings.Contains(string(body), "map_art") {
		t.Error("the list of games still names a map picture")
	}
}

// A guide's ![alt|width](figure:name "caption") is a picture the game's page
// script draws: a figure with its caption when the script draws one, and the
// alt text when it does not.
func TestWebGuides_AFigureIsDrawnByTheGame(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	script := `import("/static/js/markdown.js").then(({ renderMarkdown }) => {
		const src = '![The map|300](figure:map "The map of this game")';
		const drawn = renderMarkdown(src, { figure: (name) => {
			if (name !== "map") return null;
			const box = document.createElement("div");
			box.append(document.createElementNS("http://www.w3.org/2000/svg", "svg"));
			return box;
		} }).node;
		const fig = drawn.querySelector("figure.md-figure");
		const pic = fig && fig.querySelector(".md-drawn");
		const without = renderMarkdown(src, {}).node.querySelector(".md-badimage");
		const other = renderMarkdown('![A cave](figure:cave)', { figure: () => null }).node.querySelector(".md-badimage");
		return [
			pic && pic.querySelector("svg") ? "drawn" : "not drawn",
			pic ? pic.getAttribute("role") + "|" + pic.getAttribute("aria-label") + "|" + pic.style.maxWidth : "",
			fig && fig.querySelector("figcaption") ? fig.querySelector("figcaption").textContent : "no caption",
			without ? without.textContent : "nothing",
			other ? other.textContent : "nothing",
		].join(" / ");
	})`
	var got string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &got, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	if want := "drawn / img|The map|300px / The map of this game / The map / A cave"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
