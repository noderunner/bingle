//go:build ignore

// Command game creates a game on a running bingle server, plays the given
// moves (alternating the two players, starting with the first), and saves a
// screenshot of the resulting game page:
//
//	go run scripts/shot/game/main.go http://127.0.0.1:18099 draughts /tmp/d.png c3-d4 f6-e5
//
// SHOT_W / SHOT_H set the viewport (default 1280x900), SHOT_DARK=1 asks for
// the dark colour scheme and SHOT_HOVER=c3 moves the mouse over that square.
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
	"strconv"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func post(base, path string, body any) map[string]any {
	b, _ := json.Marshal(body)
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func main() {
	base := os.Args[1]
	game := os.Args[2]
	out := os.Args[3]
	r := post(base, "/api/games", map[string]any{"game_def_id": game, "player_name": "Alice"})
	room := r["room_code"].(string)
	post(base, "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"})
	moves := []string{"f6", "e6", "f5", "g5", "g6", "e7", "d7", "f7", "d8"}
	if len(os.Args) > 4 {
		moves = os.Args[4:]
	}
	for _, m := range moves {
		// Play as whoever is to move: games with turns made of several
		// decisions do not alternate.
		who := "Alice"
		if resp, err := http.Get(base + "/api/games/" + room + "/state"); err == nil {
			var st struct {
				TurnSeat int `json:"turn_seat"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()
			if st.TurnSeat == 2 {
				who = "Bob"
			}
		}
		if out := post(base, "/api/games/"+room+"/moves", map[string]any{"player_name": who, "notation": m}); out["error"] != nil {
			log.Printf("move %s: %v", m, out["error"])
		}
	}
	// A uniquely named profile directory lets us kill any browser processes
	// that outlive the context (the Flatpak-wrapper quirk in docs/test.md).
	profile, err := os.MkdirTemp("", "chromedp-runner-game-")
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
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	games := fmt.Sprintf(`[{"code":%q,"def":%q,"name":"Alice","ts":1}]`, room, game)
	w, h := int64(1280), int64(900)
	if v, err := strconv.Atoi(os.Getenv("SHOT_W")); err == nil {
		w = int64(v)
	}
	if v, err := strconv.Atoi(os.Getenv("SHOT_H")); err == nil {
		h = int64(v)
	}
	scheme := "light"
	if os.Getenv("SHOT_DARK") != "" {
		scheme = "dark"
	}
	hover := os.Getenv("SHOT_HOVER")
	var buf []byte
	if err = chromedp.Run(ctx,
		emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}),
		chromedp.Navigate(base+"/"),
		chromedp.Evaluate(`localStorage.setItem("bingle.games", JSON.stringify(`+games+`))`, nil),
		chromedp.EmulateViewport(w, h),
		chromedp.Navigate(base+"/game/"+room),
		chromedp.WaitVisible("#board-svg", chromedp.ByQuery),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if hover == "" {
				return nil
			}
			var rect []float64
			js := fmt.Sprintf(`(function(){var e=document.querySelector('#board .cell[data-position=%q] .cell-hit'); if(!e) return []; var r=e.getBoundingClientRect(); return [r.x+r.width/2, r.y+r.height/2];})()`, hover)
			if err := chromedp.Evaluate(js, &rect).Do(ctx); err != nil || len(rect) != 2 {
				return err
			}
			return input.DispatchMouseEvent(input.MouseMoved, rect[0], rect[1]).Do(ctx)
		}),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.FullScreenshot(&buf, 90),
	); err != nil {
		log.Fatal(err)
	}
	_ = os.WriteFile(out, buf, 0o644)
	fmt.Println("room", room)
}
