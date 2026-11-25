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
	// CallBot is optional: a module built before it has no bingle_bot, and
	// the host then searches through the other calls.
	CallBot = "bot"
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
	games map[string]*played
}

// played is a game the handler has been told of: its rules, and what the
// search needs to know of the game.
type played struct {
	rules  Rules
	colors map[int]string
	cells  int
}

// NewHandler returns a Handler that makes each game's rules with n.
func NewHandler(n New) *Handler { return &Handler{make: n} }

// initRequest is the request of init.
type initRequest struct {
	ABI   int    `json:"abi"`
	Games []Game `json:"games"`
}

// Call answers one call. name is the call's name ("legal") and req its JSON
// request; the reply is always an envelope, {"ok": …} or {"error": "…"}, and
// its ok is never null. A panic in the rules is not caught: in a module it
// ends the instance, which the host replaces.
func (h *Handler) Call(name string, req []byte) []byte {
	if name == CallInit {
		var r initRequest
		if err := json.Unmarshal(req, &r); err != nil {
			return fail(fmt.Errorf("guest: %s: the request is not JSON: %w", name, err))
		}
		return h.init(&r)
	}
	r, err := decodeRequest(req)
	if err != nil {
		return fail(fmt.Errorf("guest: %s: the request is not JSON: %w", name, err))
	}
	h.mu.RLock()
	game, known := h.games[r.Game]
	h.mu.RUnlock()
	if !known {
		return fail(fmt.Errorf("guest: %s: no game %q was given to init", name, r.Game))
	}
	rules := game.rules

	pos := r.Position
	switch name {
	case CallImpulseSeat, CallLegal, CallApply, CallValidate, CallOutcome, CallBot:
		if pos == nil {
			return fail(fmt.Errorf("guest: %s needs a position", name))
		}
		if r.Version != positionVersion {
			return fail(fmt.Errorf("guest: %s: position document version %d, want %d", name, r.Version, positionVersion))
		}
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

	// The reply is written straight after its `{"ok":`, and closed by done.
	// It starts with room for a reply as long as its request, which is what a
	// position that comes back needs. A request longer than any position
	// gets the least, and its reply grows as it is written.
	room := replyRoom
	if len(req) < replyRoomMost {
		room += len(req)
	}
	out := append(make([]byte, 0, room), `{"ok":`...)
	done := func(out []byte) []byte { return append(out, '}') }
	switch name {
	case CallStart:
		start, err := rules.Start(r.Seed)
		if err != nil {
			return fail(err)
		}
		if start == nil {
			return fail(errors.New("guest: start returned no position"))
		}
		if out, err = appendPosition(out, start); err != nil {
			return fail(err)
		}
		return done(out)
	case CallImpulseSeat:
		return done(appendInt(out, rules.ImpulseSeat(pos)))
	case CallLegal:
		out = append(out, '[')
		for i, legal := range rules.Legal(pos, r.Seat) {
			if i > 0 {
				out = append(out, ',')
			}
			out = appendSelection(out, &legal)
		}
		return done(append(out, ']'))
	case CallApply:
		res, err := rules.Apply(pos, r.Seat, sel)
		if err != nil {
			return fail(err)
		}
		if res == nil || res.Position == nil {
			return fail(errors.New("guest: apply returned no position"))
		}
		notation := res.Notation
		if notation == "" {
			notation = rules.Format(sel)
		}
		if out, err = appendResult(out, res, notation); err != nil {
			return fail(err)
		}
		return done(out)
	case CallFormat:
		return done(appendString(out, rules.Format(sel)))
	case CallParse:
		parsed, err := rules.Parse(r.Text)
		if err != nil {
			return fail(err)
		}
		return done(appendSelection(out, &parsed))
	case CallGestures:
		out = append(out, '[')
		for i, g := range rules.Gestures(sel) {
			if i > 0 {
				out = append(out, ',')
			}
			out = appendString(append(out, `{"kind":`...), g.Kind)
			out = append(appendString(append(out, `,"value":`...), g.Value), '}')
		}
		return done(append(out, ']'))
	case CallValidate:
		if err := rules.Validate(pos); err != nil {
			return fail(err)
		}
		return done(append(out, "true"...))
	case CallOutcome:
		outcome := rules.Outcome(pos)
		out = append(out, '{')
		return done(append(appendOutcome(out, len(out), &outcome), '}'))
	case CallBot:
		// One call a decision: encoding/json costs nothing beside the search.
		var br BotRequest
		if len(r.Bot) == 0 {
			return fail(errors.New("guest: bot needs a bot request"))
		}
		if err := json.Unmarshal(r.Bot, &br); err != nil {
			return fail(fmt.Errorf("guest: bot: %w", err))
		}
		reply, err := Search(rules, game.colors, game.cells, pos, r.Seat, br)
		if err != nil {
			return fail(err)
		}
		body, err := json.Marshal(&reply)
		if err != nil {
			return fail(err)
		}
		return done(append(out, body...))
	}
	return fail(fmt.Errorf("guest: no call %q", name))
}

// A reply's first room: replyRoom, and the length of its request on top
// when that is under replyRoomMost.
const (
	replyRoom     = 256
	replyRoomMost = 1 << 20
)

// init makes the rules of the games the host names, and adds them to the
// games the handler knows: the host tells an instance of a game before the
// first call that names it. A game told again is made again. Nothing is added
// unless every game of the request can be played.
func (h *Handler) init(r *initRequest) []byte {
	if r.ABI != ABI {
		return fail(fmt.Errorf("guest: the host speaks ABI %d, this module ABI %d", r.ABI, ABI))
	}
	if h.make == nil {
		return fail(errors.New("guest: the module registered no rules"))
	}
	made := make([]*played, len(r.Games))
	for i := range r.Games {
		g := &r.Games[i]
		rules, err := h.make(g)
		if err != nil {
			return fail(fmt.Errorf("guest: game %q: %w", g.ID, err))
		}
		if rules == nil {
			return fail(fmt.Errorf("guest: game %q: no rules were made", g.ID))
		}
		var def struct {
			Seats []struct {
				Seat  int    `json:"seat"`
				Color string `json:"color"`
			} `json:"seats"`
		}
		if err := json.Unmarshal(g.Definition, &def); err != nil {
			return fail(fmt.Errorf("guest: game %q: the definition: %w", g.ID, err))
		}
		p := &played{rules: rules, colors: make(map[int]string, len(def.Seats)), cells: len(g.Board.Positions)}
		for _, seat := range def.Seats {
			p.colors[seat.Seat] = seat.Color
		}
		made[i] = p
	}
	h.mu.Lock()
	if h.games == nil {
		h.games = make(map[string]*played, len(made))
	}
	for i, p := range made {
		h.games[r.Games[i].ID] = p
	}
	h.mu.Unlock()
	return ok()
}

// ok is the reply of a call that has nothing to say but that it worked.
func ok() []byte { return []byte(`{"ok":true}`) }

func fail(err error) []byte {
	return append(appendString([]byte(`{"error":`), err.Error()), '}')
}
