package wasmrules_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// echo is rules that answer every call with what they were asked, so that a
// test sees what the guest read of a request and how it writes a reply.
type echo struct{}

func (echo) Start(seed uint64) (*guest.Position, error) {
	p := guest.NewPosition()
	p.Counters = map[string]int{"low": int(seed & 0xffffffff), "high": int(seed >> 32)}
	return p, nil
}
func (echo) ImpulseSeat(pos *guest.Position) int { return pos.Ply }
func (echo) Legal(pos *guest.Position, seat int) []guest.Selection {
	var out []guest.Selection
	for id, stack := range pos.Cells {
		for _, piece := range stack {
			out = append(out, guest.Selection{From: id, Piece: piece.Piece, Aim: piece.Color, Count: seat})
		}
	}
	return out
}
func (echo) Apply(pos *guest.Position, seat int, sel guest.Selection) (*guest.Result, error) {
	return &guest.Result{
		Position: pos, Captures: sel.Path, NextSeat: seat, Notation: sel.Card,
		Facts:   []guest.Fact{{Name: sel.Action, Seat: sel.Count}},
		Outcome: guest.Outcome{Ended: sel.Count > 0, Winner: sel.Count, Drawn: sel.Drops, AllLost: sel.Aim != "", Reason: sel.From, Detail: sel.To},
	}, nil
}
func (echo) Format(sel guest.Selection) string { return sel.Card }
func (echo) Parse(text string) (guest.Selection, error) {
	if text == "" {
		return guest.Selection{}, fmt.Errorf("nothing to read: %q", "\x01\"\\")
	}
	return guest.Selection{Card: text, Path: strings.Split(text, "/"), Drops: []int{len(text), -len(text)}}, nil
}
func (echo) Gestures(sel guest.Selection) []guest.Gesture {
	return []guest.Gesture{{Kind: sel.Action, Value: sel.Card}}
}
func (echo) Validate(pos *guest.Position) error { return nil }
func (echo) Outcome(pos *guest.Position) guest.Outcome {
	return guest.Outcome{Ended: pos.Ply > 0, Winner: pos.Ply, Reason: string(pos.Ext)}
}

func echoHandler(t testing.TB) *guest.Handler {
	t.Helper()
	h := guest.NewHandler(func(*guest.Game) (guest.Rules, error) { return echo{}, nil })
	if reply := h.Call(guest.CallInit, initRequest(guest.ABI, guest.Game{ID: "echo"})); string(reply) != `{"ok":true}` {
		t.Fatalf("init = %s", reply)
	}
	return h
}

// reply reads an envelope with encoding/json, the way a host does.
func reply[T any](t testing.TB, raw []byte) (T, string) {
	t.Helper()
	var env struct {
		OK    *T      `json:"ok"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("the reply is not JSON: %v\n%s", err, raw)
	}
	switch {
	case env.Error != nil && env.OK == nil:
		var none T
		return none, *env.Error
	case env.OK != nil && env.Error == nil:
		return *env.OK, ""
	}
	t.Fatalf("the reply is no envelope: %s", raw)
	panic("unreachable")
}

// positionDoc is a position on the wire.
type positionDoc struct {
	V int `json:"v"`
	*guest.Position
}

// awkward are strings that need every kind of escape, on the way in and on
// the way out.
var awkward = []string{
	"", "a1", `quote " and \ backslash`, "tab\tline\nreturn\r", "\x00\x01\x1f", "/slash", "<html> & 'quotes'",
	"é ü 石 𝄞 😀", "\u2028\u2029", "ends with backslash\\", `\u0041 not an escape`, strings.Repeat("long ", 200),
}

func randomString(rng *rand.Rand) string {
	if rng.Intn(3) > 0 {
		return awkward[rng.Intn(len(awkward))]
	}
	var b strings.Builder
	for n := rng.Intn(12); n > 0; n-- {
		b.WriteRune(rune(rng.Intn(0x2fff)))
	}
	return strings.ToValidUTF8(b.String(), "?")
}

func randomPosition(rng *rand.Rand) *guest.Position {
	p := guest.NewPosition()
	for n := rng.Intn(6); n > 0; n-- {
		stack := []guest.Piece{}
		for k := rng.Intn(4); k > 0; k-- {
			stack = append(stack, guest.Piece{Piece: randomString(rng), Color: []string{"black", "white", ""}[rng.Intn(3)]})
		}
		p.Cells[randomString(rng)] = stack
	}
	if rng.Intn(2) == 0 {
		p.Reserves = map[string]map[string]int{randomString(rng): {randomString(rng): rng.Intn(100), "x": -rng.Intn(5)}, "empty": {}}
	}
	p.Ply = rng.Intn(1 << 30)
	if rng.Intn(2) == 0 {
		p.Counters = map[string]int{randomString(rng): rng.Int(), "taken_back": 2}
	}
	switch rng.Intn(4) {
	case 0:
		p.Ext, _ = json.Marshal(map[string]any{randomString(rng): []any{1.5e-7, -0.25, true, nil, randomString(rng), map[string]any{}}})
	case 1:
		p.Ext = json.RawMessage(`  [ 1 , {"a" : "\u00e9\ud834\udd1e" } , -12.5E+3 ]  `)
	case 2:
		p.Ext = json.RawMessage(`"just a string"`)
	}
	return p
}

func randomSelection(rng *rand.Rand) guest.Selection {
	s := guest.Selection{Action: randomString(rng), Piece: randomString(rng), From: randomString(rng), To: randomString(rng), Aim: randomString(rng), Card: randomString(rng)}
	if rng.Intn(2) == 0 {
		s.Count = rng.Intn(1000) - 500
	}
	for n := rng.Intn(4); n > 0; n-- {
		s.Path = append(s.Path, randomString(rng))
		s.Drops = append(s.Drops, rng.Intn(9))
	}
	return s
}

// same compares two values as the JSON encoding/json writes for them, which
// is what "the same on the wire" means: an empty list and none are one.
func same(t *testing.T, what string, got, want any) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	var gv, wv any
	_ = json.Unmarshal(g, &gv)
	_ = json.Unmarshal(w, &wv)
	if !reflect.DeepEqual(gv, wv) {
		t.Fatalf("%s:\n got %s\nwant %s", what, g, w)
	}
}

// The guest reads a request and writes a reply by hand, not with
// encoding/json. Whatever encoding/json writes, the guest reads as
// encoding/json would, and whatever the guest writes, encoding/json reads
// back as the value it was: every escape, every empty or missing member, a
// seed of sixty-four bits and a game's own data left as it was.
func TestTheGuestsJSONIsEncodingJSONs(t *testing.T) {
	h := echoHandler(t)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 400; i++ {
		pos, sel := randomPosition(rng), randomSelection(rng)
		seat := rng.Intn(9)
		req := func(fields map[string]any) []byte {
			fields["game"] = "echo"
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}
		doc := positionDoc{2, pos}

		// apply echoes the position and every field of the selection.
		type result struct {
			Position positionDoc  `json:"position"`
			Captures []string     `json:"captures"`
			NextSeat int          `json:"next_seat"`
			Notation string       `json:"notation"`
			Facts    []guest.Fact `json:"facts"`
			guest.Outcome
		}
		res, failure := reply[result](t, h.Call(guest.CallApply, req(map[string]any{"position": doc, "seat": seat, "selection": sel})))
		if failure != "" {
			t.Fatalf("apply: %s", failure)
		}
		want := *pos
		if want.Ext != nil {
			// The game's data, as JSON: white space apart, it is what was sent.
			var a, b any
			if json.Unmarshal(want.Ext, &a) != nil || json.Unmarshal(res.Position.Ext, &b) != nil || !reflect.DeepEqual(a, b) {
				t.Fatalf("ext: got %s, want %s", res.Position.Ext, want.Ext)
			}
			want.Ext, res.Position.Ext = nil, nil
		}
		same(t, "the position", res.Position.Position, &want)
		same(t, "apply's result", []any{res.Captures, res.NextSeat, res.Notation, res.Facts, res.Outcome}, []any{
			sel.Path, seat, sel.Card, []guest.Fact{{Name: sel.Action, Seat: sel.Count}},
			guest.Outcome{Ended: sel.Count > 0, Winner: sel.Count, Drawn: sel.Drops, AllLost: sel.Aim != "", Reason: sel.From, Detail: sel.To},
		})
		if res.Position.V != 2 {
			t.Fatalf("the position's version is %d", res.Position.V)
		}

		// legal lists a selection for every piece, in no order.
		legal, _ := reply[[]guest.Selection](t, h.Call(guest.CallLegal, req(map[string]any{"position": doc, "seat": seat})))
		pieces := 0
		for _, stack := range pos.Cells {
			pieces += len(stack)
		}
		if len(legal) != pieces {
			t.Fatalf("legal lists %d selections for %d pieces", len(legal), pieces)
		}
		for _, l := range legal {
			found := false
			for _, piece := range pos.Cells[l.From] {
				found = found || (piece.Piece == l.Piece && piece.Color == l.Aim)
			}
			if !found || l.Count != seat {
				t.Fatalf("legal lists %+v, which is no piece of %v", l, pos.Cells)
			}
		}

		// The rest: a string out, a selection out, gestures, numbers.
		text, _ := reply[string](t, h.Call(guest.CallFormat, req(map[string]any{"selection": sel})))
		if text != sel.Card {
			t.Fatalf("format: got %q, want %q", text, sel.Card)
		}
		if sel.Card != "" {
			parsed, _ := reply[guest.Selection](t, h.Call(guest.CallParse, req(map[string]any{"text": sel.Card})))
			same(t, "parse", parsed, guest.Selection{Card: sel.Card, Path: strings.Split(sel.Card, "/"), Drops: []int{len(sel.Card), -len(sel.Card)}})
		}
		gestures, _ := reply[[]guest.Gesture](t, h.Call(guest.CallGestures, req(map[string]any{"selection": sel})))
		same(t, "gestures", gestures, []guest.Gesture{{Kind: sel.Action, Value: sel.Card}})
		if seatReply, _ := reply[int](t, h.Call(guest.CallImpulseSeat, req(map[string]any{"position": doc}))); seatReply != pos.Ply {
			t.Fatalf("impulse_seat: got %d, want %d", seatReply, pos.Ply)
		}
		seed := rng.Uint64()
		start, _ := reply[positionDoc](t, h.Call(guest.CallStart, req(map[string]any{"seed": seed})))
		if got := uint64(start.Counters["high"])<<32 | uint64(start.Counters["low"]); got != seed {
			t.Fatalf("start: the seed %d arrived as %d", seed, got)
		}
	}

	// An error says what the rules said, whatever is in it.
	if _, failure := reply[guest.Selection](t, h.Call(guest.CallParse, []byte(`{"game":"echo","text":""}`))); failure != "nothing to read: \"\\x01\\\"\\\\\"" {
		t.Errorf("an error arrived as %q", failure)
	}
	// A position's ext that is not JSON is the call's error, not a broken reply.
	bad := positionDoc{2, guest.NewPosition()}
	raw, _ := json.Marshal(map[string]any{"game": "echo", "position": bad, "seat": 1, "selection": guest.Selection{}})
	raw = []byte(strings.Replace(string(raw), `"ply":0`, `"ply":0,"ext":{"a":1}`, 1))
	if _, failure := reply[json.RawMessage](t, h.Call(guest.CallApply, raw)); failure != "" {
		t.Errorf("a good ext was refused: %s", failure)
	}
}

// What is not JSON, or not what a call is asked with, is refused as an error
// and never read as something else.
func TestTheGuestRefusesWhatIsNotARequest(t *testing.T) {
	h := echoHandler(t)
	for _, req := range []string{
		``, `null`, `[]`, `"game"`, `{`, `{"game":"echo"`, `{"game":"echo",}`, `{"game" "echo"}`, `{"game":"echo"} x`,
		`{"game":"echo","seat":1.5}`, `{"game":"echo","seat":1e3}`, `{"game":"echo","seat":"1"}`, `{"game":"echo","seat":01}`,
		`{"game":"echo","seat":99999999999999999999}`, `{"game":"echo","seed":-1}`, `{"game":"echo","seed":18446744073709551616}`,
		`{"game":"echo","text":"a` + "\n" + `b"}`, `{"game":"echo","text":"\x"}`, `{"game":"echo","text":"\u12"}`, `{"game":"echo","text":"open`,
		`{"game":"echo","selection":[]}`, `{"game":"echo","selection":{"path":"a"}}`, `{"game":"echo","selection":{"drops":["a"]}}`,
		`{"game":"echo","position":{"v":2,"cells":[]}}`, `{"game":"echo","position":{"v":2,"cells":{"a":{}}}}`,
		`{"game":"echo","position":{"v":2,"cells":{},"ext":tru}}`, `{"game":"echo","position":{"v":2,"cells":{},"ext":1.}}`,
		`{"game":"echo","other":{"a":[1,2,}}`, `{"game":"echo","other":` + strings.Repeat("[", 5000) + `}`,
	} {
		if _, failure := reply[json.RawMessage](t, h.Call(guest.CallFormat, []byte(req))); !strings.Contains(failure, "not JSON") {
			t.Errorf("%.60s was not refused as no JSON: %q", req, failure)
		}
	}
	// What is JSON and more than a call needs is read past.
	for _, req := range []string{
		`{"other":[1,{"a":null},"b",true,false,-0.5e-3],"game":"echo","selection":{"card":"x","new":{}}}`,
		` { "game" : "echo" , "selection" : { "card" : "x" } , "seat" : null , "text" : null , "position" : null } `,
		`{"game":"echo","selection":{"card":"\u0078","path":null,"drops":[]}}`,
		`{"game":"other","game":"echo","selection":{"card":"y","card":"x"}}`,
	} {
		if text, failure := reply[string](t, h.Call(guest.CallFormat, []byte(req))); text != "x" {
			t.Errorf("%s: format = %q, %q", req, text, failure)
		}
	}
}

// Whatever bytes a call is asked with, the guest answers with an envelope
// that encoding/json reads. And what encoding/json makes of a request, written
// out again as a host writes it, the guest reads as the same. (The bytes
// themselves the guest may refuse where encoding/json does not: it wants a
// member's name as the wire spells it, and encoding/json takes any case.)
func FuzzTheGuestsDecoder(f *testing.F) {
	f.Add([]byte(`{"game":"echo","selection":{"action":"a","piece":"p","from":"f","to":"t","count":3,"path":["x"],"drops":[1],"aim":"m","card":"c"}}`))
	f.Add([]byte(`{"game":"echo","selection":{"card":"\ud834\udd1e\ud834 \n"},"seat":-4,"seed":18446744073709551615}`))
	f.Add([]byte(`{"game":"echo","selection":null,"text":"\\\"","other":[{}]}`))
	h := echoHandler(f)
	f.Fuzz(func(t *testing.T, req []byte) {
		reply[[]guest.Gesture](t, h.Call(guest.CallGestures, req))
		var want struct {
			Game      string           `json:"game"`
			Selection *guest.Selection `json:"selection"`
			Seat      int              `json:"seat"`
			Seed      uint64           `json:"seed"`
			Text      string           `json:"text"`
		}
		if json.Unmarshal(req, &want) != nil || want.Selection == nil {
			return
		}
		want.Game = "echo"
		written, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, failure := reply[[]guest.Gesture](t, h.Call(guest.CallGestures, written))
		if failure != "" {
			t.Fatalf("encoding/json wrote %s, and the guest says %s", written, failure)
		}
		if len(got) != 1 || got[0].Kind != want.Selection.Action || got[0].Value != want.Selection.Card {
			t.Fatalf("%s: the guest read %+v, encoding/json %+v", written, got, want.Selection)
		}
		// The whole selection, by way of apply's echo of it.
		position, _ := json.Marshal(map[string]any{"game": "echo", "seat": want.Seat, "selection": want.Selection, "position": positionDoc{2, guest.NewPosition()}})
		res, failure := reply[struct {
			Captures []string `json:"captures"`
			Notation string   `json:"notation"`
			NextSeat int      `json:"next_seat"`
			guest.Outcome
		}](t, h.Call(guest.CallApply, position))
		sel := want.Selection
		if failure != "" || res.NextSeat != want.Seat || res.Notation != sel.Card || res.Reason != sel.From || res.Detail != sel.To ||
			len(res.Captures) != len(sel.Path) || len(res.Drawn) != len(sel.Drops) || res.Winner != sel.Count {
			t.Fatalf("%s: apply echoed %+v, %q", position, res, failure)
		}
	})
}
