package guest

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// The calls of ABI v0, as their exports are named after the "bingle_" prefix.
const (
	CallInit        = "init"
	CallStart       = "start"
	CallImpulseSeat = "impulse_seat"
	CallLegal       = "legal"
	CallApply       = "apply"
	CallFormat      = "format"
	CallParse       = "parse"
	CallGestures    = "gestures"
	CallValidate    = "validate"
	CallOutcome     = "outcome"
)

// positionVersion is the version of a position document, its "v".
const positionVersion = 2

// Handler answers the calls of ABI v0 for the games it has been told of: a
// JSON request in, a JSON reply out. A module's exports are a Handler behind
// linear memory, and the same Handler answers in process, so the dispatch is
// tested without a module.
type Handler struct {
	make New

	mu    sync.RWMutex
	games map[string]Rules
}

// NewHandler returns a Handler that makes each game's rules with n.
func NewHandler(n New) *Handler { return &Handler{make: n} }

// request is every call's request: a call reads the fields it takes.
type request struct {
	// init
	ABI   int    `json:"abi"`
	Games []Game `json:"games"`
	// every other call
	Game      string       `json:"game"`
	Position  *positionDoc `json:"position"`
	Seat      int          `json:"seat"`
	Selection *Selection   `json:"selection"`
	Text      string       `json:"text"`
	Seed      uint64       `json:"seed"`
}

// positionDoc is a position as it crosses: the engine's document, with its
// version.
type positionDoc struct {
	V int `json:"v"`
	*Position
}

// resultDoc is the reply of apply.
type resultDoc struct {
	Position positionDoc `json:"position"`
	Captures []string    `json:"captures,omitempty"`
	NextSeat int         `json:"next_seat"`
	Notation string      `json:"notation,omitempty"`
	Facts    []Fact      `json:"facts,omitempty"`
	Outcome
}

// Call answers one call. name is the call's name ("legal") and req its JSON
// request; the reply is always an envelope, {"ok": …} or {"error": "…"}, and
// its ok is never null. A panic in the rules is not caught: in a module it
// ends the instance, which the host replaces.
func (h *Handler) Call(name string, req []byte) []byte {
	var r request
	if err := json.Unmarshal(req, &r); err != nil {
		return fail(fmt.Errorf("guest: %s: the request is not JSON: %w", name, err))
	}
	if name == CallInit {
		return h.init(&r)
	}
	h.mu.RLock()
	rules, known := h.games[r.Game]
	h.mu.RUnlock()
	if !known {
		return fail(fmt.Errorf("guest: %s: no game %q was given to init", name, r.Game))
	}

	var pos *Position
	switch name {
	case CallImpulseSeat, CallLegal, CallApply, CallValidate, CallOutcome:
		if r.Position == nil || r.Position.Position == nil {
			return fail(fmt.Errorf("guest: %s needs a position", name))
		}
		if r.Position.V != positionVersion {
			return fail(fmt.Errorf("guest: %s: position document version %d, want %d", name, r.Position.V, positionVersion))
		}
		pos = r.Position.Position
		if pos.Cells == nil {
			pos.Cells = map[string][]Piece{}
		}
	}
	var sel Selection
	switch name {
	case CallApply, CallFormat, CallGestures:
		if r.Selection == nil {
			return fail(fmt.Errorf("guest: %s needs a selection", name))
		}
		sel = *r.Selection
	}

	switch name {
	case CallStart:
		start, err := rules.Start(r.Seed)
		if err != nil {
			return fail(err)
		}
		if start == nil {
			return fail(errors.New("guest: start returned no position"))
		}
		return ok(positionDoc{positionVersion, start})
	case CallImpulseSeat:
		return ok(rules.ImpulseSeat(pos))
	case CallLegal:
		legal := rules.Legal(pos, r.Seat)
		if legal == nil {
			legal = []Selection{}
		}
		return ok(legal)
	case CallApply:
		res, err := rules.Apply(pos, r.Seat, sel)
		if err != nil {
			return fail(err)
		}
		if res == nil || res.Position == nil {
			return fail(errors.New("guest: apply returned no position"))
		}
		doc := resultDoc{
			Position: positionDoc{positionVersion, res.Position},
			Captures: res.Captures, NextSeat: res.NextSeat, Notation: res.Notation,
			Facts: res.Facts, Outcome: res.Outcome,
		}
		if doc.Notation == "" {
			doc.Notation = rules.Format(sel)
		}
		return ok(doc)
	case CallFormat:
		return ok(rules.Format(sel))
	case CallParse:
		parsed, err := rules.Parse(r.Text)
		if err != nil {
			return fail(err)
		}
		return ok(parsed)
	case CallGestures:
		gestures := rules.Gestures(sel)
		if gestures == nil {
			gestures = []Gesture{}
		}
		return ok(gestures)
	case CallValidate:
		if err := rules.Validate(pos); err != nil {
			return fail(err)
		}
		return ok(true)
	case CallOutcome:
		return ok(rules.Outcome(pos))
	}
	return fail(fmt.Errorf("guest: no call %q", name))
}

// init makes the rules of the games the host names, and adds them to the
// games the handler knows: the host tells an instance of a game before the
// first call that names it. A game told again is made again. Nothing is added
// unless every game of the request can be played.
func (h *Handler) init(r *request) []byte {
	if r.ABI != ABI {
		return fail(fmt.Errorf("guest: the host speaks ABI %d, this module ABI %d", r.ABI, ABI))
	}
	if h.make == nil {
		return fail(errors.New("guest: the module registered no rules"))
	}
	made := make([]Rules, len(r.Games))
	for i := range r.Games {
		g := &r.Games[i]
		rules, err := h.make(g)
		if err != nil {
			return fail(fmt.Errorf("guest: game %q: %w", g.ID, err))
		}
		if rules == nil {
			return fail(fmt.Errorf("guest: game %q: no rules were made", g.ID))
		}
		made[i] = rules
	}
	h.mu.Lock()
	if h.games == nil {
		h.games = make(map[string]Rules, len(made))
	}
	for i, rules := range made {
		h.games[r.Games[i].ID] = rules
	}
	h.mu.Unlock()
	return ok(true)
}

func ok(v any) []byte {
	out, err := json.Marshal(struct {
		OK any `json:"ok"`
	}{v})
	if err != nil {
		return fail(fmt.Errorf("guest: the reply cannot be written: %w", err))
	}
	return out
}

func fail(err error) []byte {
	out, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{err.Error()})
	return out
}
