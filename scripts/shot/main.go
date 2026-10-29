//go:build ignore

// Command shot drives a running bingle server with headless Chrome and saves
// screenshots of the lobby and a mid-game Tak position, for visual review of
// UI changes:
//
//	BINGLE_ADDR=127.0.0.1:18099 BINGLE_DB_PATH=/tmp/shot.db ./bin/bingle &
//	go run scripts/shot/main.go http://127.0.0.1:18099 /tmp/shots
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

func post(base, path string, body any) map[string]any {
	b, _ := json.Marshal(body)
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != 200 {
		log.Fatalf("POST %s: %d %s", path, resp.StatusCode, raw)
	}
	return out
}

func emulateLight() chromedp.Action {
	return emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "light"}})
}

func emulateDark() chromedp.Action {
	return emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "dark"}})
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: shot <base-url> <out-dir>")
	}
	base, out := os.Args[1], os.Args[2]
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}

	// A Tak game with some structure on the board.
	created := post(base, "/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice"})
	room := created["room_code"].(string)
	post(base, "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"})
	moves := []struct{ who, mv string }{
		{"Alice", "c3"}, {"Bob", "b2"},
		{"Alice", "Sd4"}, {"Bob", "Cb3"}, {"Alice", "c4"}, {"Bob", "b4"},
		{"Alice", "c4-"}, {"Bob", "a1"}, {"Alice", "Sa5"}, {"Bob", "b3+"},
	}
	for _, m := range moves {
		resp := post(base, "/api/games/"+room+"/moves", map[string]any{"player_name": m.who, "notation": m.mv})
		_ = resp
	}

	// A second game too.
	second := post(base, "/api/games", map[string]any{"game_def_id": "reversi", "player_name": "Alice", "opponent": "random"})
	secondRoom := second["room_code"].(string)

	// A uniquely named profile directory lets us kill any browser processes
	// that outlive the context (the Flatpak-wrapper quirk in docs/test.md).
	profile, err := os.MkdirTemp("", "chromedp-runner-shot-")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = exec.Command("pkill", "-9", "-f", "--", filepath.Base(profile)).Run()
		_ = os.RemoveAll(profile)
	}()
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.UserDataDir(profile))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	shot := func(name string, w, h int, url string, wait string, extra ...chromedp.Action) {
		var buf []byte
		actions := []chromedp.Action{
			chromedp.EmulateViewport(int64(w), int64(h)),
			chromedp.Navigate(url),
			chromedp.WaitVisible(wait, chromedp.ByQuery),
			chromedp.Sleep(700 * time.Millisecond),
		}
		actions = append(actions, extra...)
		actions = append(actions, chromedp.FullScreenshot(&buf, 90))
		if err := chromedp.Run(ctx, actions...); err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(out, name+".png"), buf, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("wrote", name)
	}

	// Remember Alice as the player in both rooms (the UI's identity store).
	games := fmt.Sprintf(`[{"code":%q,"def":"tak-5x5","name":"Alice","ts":%d},{"code":%q,"def":"reversi","name":"Alice","ts":%d}]`, room, time.Now().UnixMilli(), secondRoom, time.Now().UnixMilli())
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Evaluate(`localStorage.setItem("bingle.games", JSON.stringify(`+games+`)); localStorage.setItem("bingle.name", JSON.stringify("Alice"))`, nil),
	); err != nil {
		log.Fatal(err)
	}

	// A second Tak room where Alice has a tall stack, for the move-building
	// screenshots.
	stackRoom := post(base, "/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice"})["room_code"].(string)
	post(base, "/api/games/"+stackRoom+"/join", map[string]any{"player_name": "Bob"})
	for _, m := range []struct{ who, mv string }{
		{"Alice", "a1"}, {"Bob", "e5"}, // opening
		{"Alice", "b1"}, {"Bob", "d5"}, {"Alice", "b1<"}, {"Bob", "d4"},
		{"Alice", "c1"}, {"Bob", "d3"}, {"Alice", "c1<"}, {"Bob", "c5"},
		{"Alice", "Cc2"}, {"Bob", "Se4"},
	} {
		post(base, "/api/games/"+stackRoom+"/moves", map[string]any{"player_name": m.who, "notation": m.mv})
	}
	games2 := fmt.Sprintf(`[{"code":%q,"def":"tak-5x5","name":"Alice","ts":%d}]`, stackRoom, time.Now().UnixMilli())
	click := func(pos string) chromedp.Action {
		return chromedp.Evaluate(fmt.Sprintf(`document.querySelector('#board .cell[data-position=%q] .cell-hit').dispatchEvent(new MouseEvent('click',{bubbles:true}))`, pos), nil)
	}
	_ = games2

	shot("home", 1200, 900, base+"/", "#create-game-form")
	shot("tak-desktop", 1280, 900, base+"/game/"+room, "#board-svg")
	shot("tak-mobile", 390, 844, base+"/game/"+room, "#board-svg")
	shot("reversi-desktop", 1280, 900, base+"/game/"+secondRoom, "#board-svg")

	// Mid-move: pick up the 3-stack on a1 and start dropping.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Evaluate(`localStorage.setItem("bingle.games", JSON.stringify(`+games2+`))`, nil),
	); err != nil {
		log.Fatal(err)
	}
	shot("tak-stack-start", 1280, 900, base+"/game/"+stackRoom, "#board-svg")
	shot("tak-stack-count", 1280, 900, base+"/game/"+stackRoom, "#board-svg", click("a1"), chromedp.Sleep(300*time.Millisecond))
	shot("tak-stack-drops", 1280, 900, base+"/game/"+stackRoom, "#board-svg",
		click("a1"), chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`document.querySelector('#count-picker button[data-count="2"]').click()`, nil), chromedp.Sleep(150*time.Millisecond),
		click("b1"), chromedp.Sleep(150*time.Millisecond), click("c1"), chromedp.Sleep(500*time.Millisecond))
	shot("tak-history", 1280, 900, base+"/game/"+stackRoom, "#board-svg",
		chromedp.Evaluate(`document.querySelector('#move-list button.mv[data-seq="6"]').click()`, nil), chromedp.Sleep(500*time.Millisecond))
	shot("tak-dark", 1280, 900, base+"/game/"+stackRoom, "#board-svg",
		emulateDark(), chromedp.Sleep(300*time.Millisecond))
	shot("home-dark", 1200, 1000, base+"/", "#create-game-form",
		emulateDark(), chromedp.Sleep(800*time.Millisecond))
	shot("home-mobile", 390, 844, base+"/", "#create-game-form", emulateLight(), chromedp.Sleep(800*time.Millisecond))
	fmt.Println("room", room, "second", secondRoom)
}
