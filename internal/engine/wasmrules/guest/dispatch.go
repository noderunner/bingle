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
	rules, known := h.games[r.Game]
	h.mu.RUnlock()
	if !known {
		return fail(fmt.Errorf("guest: %s: no game %q was given to init", name, r.Game))
	}

	pos := r.Position
	switch name {
	case CallImpulseSeat, CallLegal, CallApply, CallValidate, CallOutcome:
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
	out := append(make([]byte, 0, 256+len(req)), `{"ok":`...)
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
	}
	return fail(fmt.Errorf("guest: no call %q", name))
}

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
	return ok()
}

// ok is the reply of a call that has nothing to say but that it worked.
func ok() []byte { return []byte(`{"ok":true}`) }

func fail(err error) []byte {
	return append(appendString([]byte(`{"error":`), err.Error()), '}')
}
