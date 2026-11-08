// Helpers the browser tests share, from files that left with Cthulhu Wars (ADR 0127).

package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// jsErrorCollector records uncaught exceptions of a page.
func collectJSErrors(ctx context.Context, mu *sync.Mutex, out *[]string) {
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			*out = append(*out, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})
}

func evalBool(t *testing.T, ctx context.Context, expr string) bool {
	t.Helper()
	var v bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`Boolean(`+expr+`)`, &v)); err != nil {
		t.Fatal(err)
	}
	return v
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

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

// playUntil makes the first legal move of whoever is on impulse, over REST,
// until stop says the moves on offer are the ones wanted.
func playUntil(t *testing.T, baseURL, room string, stop func(moves []string) bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		var st struct {
			ImpulseSeat int `json:"impulse_seat"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
		var legal struct {
			Moves []struct {
				Notation string `json:"notation"`
			} `json:"options"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/selections", nil, &legal)
		if len(legal.Moves) == 0 {
			t.Fatalf("no legal move for seat %d after %d moves", st.ImpulseSeat, i)
		}
		var notations []string
		for _, m := range legal.Moves {
			notations = append(notations, m.Notation)
		}
		if stop(notations) {
			return
		}
		var info struct {
			Seats []struct {
				Seat int    `json:"seat"`
				Name string `json:"name"`
			} `json:"seats"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info)
		name := ""
		for _, s := range info.Seats {
			if s.Seat == st.ImpulseSeat {
				name = s.Name
			}
		}
		if code, out := moveNotation(t, baseURL, room, name, notations[0]); code != 200 {
			t.Fatalf("move %q by %s: %d %v", notations[0], name, code, out)
		}
	}
	t.Fatal("the position wanted was never reached")
}

// tricksTable is a game of the Tricks fixture (tests/testdata/tricks) with Ana
// in every seat, played through its board of terms to the first trick.
func tricksTable(t *testing.T, baseURL, def string, seats int) string {
	t.Helper()
	var also []int
	for s := 2; s <= seats; s++ {
		also = append(also, s)
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": def, "player_name": "Ana", "also_seats": also}, &created); code != 200 {
		t.Fatalf("create %s: %d", def, code)
	}
	playUntil(t, baseURL, created.RoomCode, func(m []string) bool { return !strings.HasPrefix(m[0], "cross:") })
	return created.RoomCode
}
