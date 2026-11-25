package wasmrules_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// smallGame is a game of draughts on a board of four by four, as a host
// would tell a module of it.
func smallGame(id string, seats int) guest.Game {
	def := map[string]any{"params": map[string]any{"rows": 1, "quiet": 10}}
	var list []map[string]any
	for seat, color := range []string{"black", "white", "red"}[:seats] {
		list = append(list, map[string]any{"seat": seat + 1, "color": color})
	}
	def["seats"] = list
	raw, _ := json.Marshal(def)
	g := guest.Game{ID: id, Definition: raw, Board: guest.Board{Kind: "grid", Width: 4, Height: 4}}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			g.Board.Positions = append(g.Board.Positions, guest.Point{ID: fmt.Sprintf("%c%d", 'a'+x, 4-y), XY: &[2]int{x, y}})
		}
	}
	return g
}

func initRequest(abi int, games ...guest.Game) []byte {
	raw, _ := json.Marshal(map[string]any{"abi": abi, "games": games})
	return raw
}

// The guest's dispatch answers every call with an envelope: ok, which is
// never null, or the call's error. A request it cannot serve is an error, not
// a panic, so a host's mistake does not cost it an instance.
func TestTheGuestAnswersEveryRequestWithAnEnvelope(t *testing.T) {
	h := guest.NewHandler(draughts.New)
	call := func(name, req string) (ok json.RawMessage, failure string) {
		t.Helper()
		var env struct {
			OK    json.RawMessage `json:"ok"`
			Error *string         `json:"error"`
		}
		reply := h.Call(name, []byte(req))
		if err := json.Unmarshal(reply, &env); err != nil {
			t.Fatalf("%s replied %s, which is not JSON", name, reply)
		}
		switch {
		case env.Error != nil && env.OK == nil:
			return nil, *env.Error
		case env.Error == nil && env.OK != nil && string(env.OK) != "null":
			return env.OK, ""
		}
		t.Fatalf("%s replied %s, which is no envelope", name, reply)
		return nil, ""
	}
	refused := func(name, req, want string) {
		t.Helper()
		if ok, failure := call(name, req); ok != nil || !strings.Contains(failure, want) {
			t.Errorf("%s %s = %s, %q, want an error that says %q", name, req, ok, failure, want)
		}
	}

	refused(guest.CallLegal, `{"game":"small"}`, "was given to init")
	refused(guest.CallInit, string(initRequest(guest.ABI+1, smallGame("small", 2))), "ABI")
	refused(guest.CallInit, string(initRequest(guest.ABI, smallGame("small", 2), smallGame("three", 3))), "two seats")
	refused(guest.CallLegal, `{"game":"small"}`, "was given to init") // nothing of a refused init is kept
	if ok, failure := call(guest.CallInit, string(initRequest(guest.ABI, smallGame("small", 2)))); string(ok) != "true" {
		t.Fatalf("init = %s, %q", ok, failure)
	}
	// A later init adds to the games, and one it refuses takes none away.
	if ok, failure := call(guest.CallInit, string(initRequest(guest.ABI, smallGame("other", 2)))); string(ok) != "true" {
		t.Fatalf("a second init = %s, %q", ok, failure)
	}
	refused(guest.CallInit, string(initRequest(guest.ABI, smallGame("three", 3))), "two seats")

	start, failure := call(guest.CallStart, `{"game":"small","seed":1}`)
	if failure != "" || !strings.Contains(string(start), `"v":2`) {
		t.Fatalf("start = %s, %q", start, failure)
	}
	if _, failure := call(guest.CallStart, `{"game":"other"}`); failure != "" {
		t.Fatalf("the second game does not start: %s", failure)
	}
	position := `"position":` + string(start)
	if ok, _ := call(guest.CallLegal, `{"game":"small","seat":2,`+position+`}`); string(ok) != "[]" {
		t.Errorf("legal for the seat that waits = %s, want an empty list", ok)
	}
	if ok, _ := call(guest.CallLegal, `{"game":"small","seat":1,`+position+`}`); !strings.Contains(string(ok), `"action":"move"`) {
		t.Errorf("legal = %s", ok)
	}
	if ok, _ := call(guest.CallImpulseSeat, `{"game":"small",`+position+`}`); string(ok) != "1" {
		t.Errorf("impulse_seat = %s", ok)
	}
	if ok, _ := call(guest.CallValidate, `{"game":"small",`+position+`}`); string(ok) != "true" {
		t.Errorf("validate = %s", ok)
	}
	if ok, _ := call(guest.CallOutcome, `{"game":"small",`+position+`}`); string(ok) != "{}" {
		t.Errorf("outcome = %s", ok)
	}
	applied, failure := call(guest.CallApply, `{"game":"small","seat":1,"selection":{"action":"move","from":"a1","to":"b2"},`+position+`}`)
	for _, want := range []string{`"position":{"v":2`, `"next_seat":2`, `"notation":"a1-b2"`} {
		if !strings.Contains(string(applied), want) {
			t.Errorf("apply = %s, %q, want %s in it", applied, failure, want)
		}
	}

	refused(guest.CallLegal, `{"game":"small"`, "not JSON")
	refused(guest.CallLegal, `{"game":"small"}`, "needs a position")
	refused(guest.CallLegal, `{"game":"small","position":{"v":1,"cells":{}}}`, "version")
	refused(guest.CallApply, `{"game":"small",`+position+`}`, "needs a selection")
	refused(guest.CallApply, `{"game":"small","seat":1,"selection":{"action":"move","from":"a1","to":"a2"},`+position+`}`, "not a legal move")
	refused(guest.CallParse, `{"game":"small","text":"castles"}`, "neither a move")
	refused("hint", `{"game":"small"}`, "no call")
	refused(guest.CallBot, `{"game":"small"}`, "needs a position")
	refused(guest.CallBot, `{"game":"small","seat":1,`+position+`}`, "needs a bot request")
	refused(guest.CallBot, `{"game":"small","seat":1,"bot":{"level":"oracle"},`+position+`}`, "no level")
	refused(guest.CallBot, `{"game":"small","seat":2,"bot":{"level":"mcts"},`+position+`}`, "no legal selection")
	for _, level := range []string{guest.LevelTactical, guest.LevelMinimax, guest.LevelMCTS} {
		ok, failure := call(guest.CallBot, `{"game":"small","seat":1,"bot":{"level":"`+level+`","iterations":20,"nodes":200,"seed":5},`+position+`}`)
		if !strings.Contains(string(ok), `"selection":{"action":"move"`) {
			t.Errorf("bot at %s = %s, %q", level, ok, failure)
		}
	}

	h = guest.NewHandler(nil)
	refused(guest.CallInit, string(initRequest(guest.ABI, smallGame("small", 2))), "registered no rules")
}
