package wasmrules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// Native returns rules written for a module as a state.Ruleset linked into
// the binary: no module, no JSON, only a position copied into the guest's
// type and back. It is what a module is measured against, how a pack's rules
// are tested with plain go test, and the way back to the private overlay
// should packs be dropped (ADR 0147).
func Native(n guest.New) state.SeededRuleset { return &native{make: n} }

type native struct {
	make  guest.New
	rules sync.Map // *catalog.CompiledGame → *made
}

type made struct {
	rules guest.Rules
	err   error
}

func (n *native) of(game *catalog.CompiledGame) (guest.Rules, error) {
	if m, ok := n.rules.Load(game); ok {
		return m.(*made).rules, m.(*made).err
	}
	m := &made{}
	if g, err := gameOf(game); err != nil {
		m.err = err
	} else if m.rules, err = n.make(g); err != nil {
		m.err = fmt.Errorf("wasmrules: game %s: %w", game.Definition.ID, err)
	} else if m.rules == nil {
		m.err = fmt.Errorf("wasmrules: game %s: no rules were made", game.Definition.ID)
	}
	v, _ := n.rules.LoadOrStore(game, m)
	return v.(*made).rules, v.(*made).err
}

func (n *native) Seeded() bool { return true }

func (n *native) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	r, err := n.of(game)
	if err != nil {
		return nil, err
	}
	pos, err := r.Start(seed)
	if err != nil {
		return nil, err
	}
	if pos == nil {
		return nil, errors.New("wasmrules: start returned no position")
	}
	return fromGuest(pos), nil
}

func (n *native) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
	r, err := n.of(game)
	if err != nil {
		return 0
	}
	return r.ImpulseSeat(toGuest(st))
}

func (n *native) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Selection {
	r, err := n.of(game)
	if err != nil {
		return nil
	}
	legal := r.Legal(toGuest(st), seat)
	if len(legal) == 0 {
		return nil
	}
	out := make([]state.Selection, len(legal))
	for i, sel := range legal {
		out[i] = selectionFromGuest(sel)
	}
	return out
}

func (n *native) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	r, err := n.of(game)
	if err != nil {
		return nil, err
	}
	sel := selectionToGuest(mv)
	res, err := r.Apply(toGuest(st), seat, sel)
	if err != nil {
		return nil, err
	}
	if res == nil || res.Position == nil {
		return nil, errors.New("wasmrules: apply returned no position")
	}
	out := &state.MoveResult{
		NewState: fromGuest(res.Position), NextImpulseSeat: res.NextSeat,
		GameEnded: res.Ended, DrawnSeats: res.Drawn, AllLost: res.AllLost,
		Move: mv, Notation: res.Notation, EndReason: res.Reason, EndDetail: res.Detail,
	}
	if out.Notation == "" {
		out.Notation = r.Format(sel)
	}
	if res.Winner != 0 {
		w := res.Winner
		out.WinnerSeat = &w
	}
	for _, id := range res.Captures {
		out.Captures = append(out.Captures, board.PositionID(id))
	}
	if len(out.Captures) > 0 {
		out.Captured = &out.Captures[0]
	}
	for _, f := range res.Facts {
		out.Facts = append(out.Facts, state.Fact(f))
	}
	return out, nil
}

func (n *native) Format(game *catalog.CompiledGame, mv state.Selection) string {
	r, err := n.of(game)
	if err != nil {
		return ""
	}
	return r.Format(selectionToGuest(mv))
}

func (n *native) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	r, err := n.of(game)
	if err != nil {
		return state.Selection{}, err
	}
	sel, err := r.Parse(text)
	if err != nil {
		return state.Selection{}, err
	}
	return selectionFromGuest(sel), nil
}

func (n *native) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	r, err := n.of(game)
	if err != nil {
		return nil
	}
	gestures := r.Gestures(selectionToGuest(mv))
	if len(gestures) == 0 {
		return nil
	}
	out := make([]state.Gesture, len(gestures))
	for i, g := range gestures {
		out[i] = state.Gesture(g)
	}
	return out
}

func (n *native) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	r, err := n.of(game)
	if err != nil {
		return err
	}
	return r.Validate(toGuest(st))
}

func (n *native) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	r, err := n.of(game)
	if err != nil {
		return state.Outcome{}
	}
	o := r.Outcome(toGuest(st))
	out := state.Outcome{Ended: o.Ended, Drawn: o.Drawn, AllLost: o.AllLost, Reason: o.Reason, Detail: o.Detail}
	if o.Winner != 0 {
		w := o.Winner
		out.Winner = &w
	}
	return out
}

// toGuest copies a position into the guest's type. The game's own data is
// the JSON it was stored as: the engine never decodes it.
func toGuest(st *state.BoardState) *guest.Position {
	occupied := st.Occupied()
	pos := &guest.Position{Cells: make(map[string][]guest.Piece, len(occupied)), Ply: st.Ply(), Counters: st.Counters()}
	for _, id := range occupied {
		stack := st.Stack(id)
		pieces := make([]guest.Piece, len(stack))
		for i, o := range stack {
			pieces[i] = guest.Piece(o)
		}
		pos.Cells[string(id)] = pieces
	}
	if reserves := st.Reserves(); len(reserves) > 0 {
		pos.Reserves = reserves
	}
	switch ext := st.Ext().(type) {
	case nil:
	case json.RawMessage:
		pos.Ext = ext
	default:
		// A position some other ruleset made: its data as it would be stored.
		if raw, err := json.Marshal(ext); err == nil {
			pos.Ext = raw
		}
	}
	return pos
}

// fromGuest copies a position out of the guest's type.
func fromGuest(pos *guest.Position) *state.BoardState {
	st := state.NewBoardState()
	for id, pieces := range pos.Cells {
		if len(pieces) == 0 {
			continue
		}
		stack := make([]state.Occupant, len(pieces))
		for i, p := range pieces {
			stack[i] = state.Occupant(p)
		}
		st.SetStack(board.PositionID(id), stack)
	}
	for color, hand := range pos.Reserves {
		for piece, n := range hand {
			st.SetReserve(color, piece, n)
		}
	}
	st.SetPly(pos.Ply)
	for name, n := range pos.Counters {
		st.SetCounter(name, n)
	}
	if len(pos.Ext) > 0 && string(pos.Ext) != "null" {
		st.SetExt(pos.Ext)
	}
	return st
}

func selectionToGuest(mv state.Selection) guest.Selection {
	sel := guest.Selection{
		Action: mv.Action, Piece: mv.Piece, From: string(mv.From), To: string(mv.To),
		Count: mv.Count, Drops: mv.Drops, Aim: string(mv.Aim), Card: mv.Card,
	}
	if len(mv.Path) > 0 {
		sel.Path = ids(mv.Path)
	}
	return sel
}

func selectionFromGuest(sel guest.Selection) state.Selection {
	mv := state.Selection{
		Action: sel.Action, Piece: sel.Piece, From: board.PositionID(sel.From), To: board.PositionID(sel.To),
		Count: sel.Count, Drops: sel.Drops, Aim: board.PositionID(sel.Aim), Card: sel.Card,
	}
	for _, id := range sel.Path {
		mv.Path = append(mv.Path, board.PositionID(id))
	}
	return mv
}

// Encoded is Native by way of a module's JSON: every call is written, sent
// through the guest's own dispatch in process and read back, as it would be
// for a module. It is the boundary without WebAssembly: what the guest's
// dispatch is tested through, and what tells the cost of the JSON apart from
// the cost of the module. It answers the bot call, as a module does, so it
// is a state.Searcher.
func Encoded(n guest.New) state.SeededRuleset {
	return searcher{&adapter{t: &loopback{handler: guest.NewHandler(n)}, log: slog.Default()}}
}

// loopback is a transport that answers in process.
type loopback struct {
	handler *guest.Handler
	games   games
	told    sync.Map // game id → *toldGame the handler was last sent
}

// roundTrip answers at once: nothing in process can stop a call, so ctx and
// extra are not read.
func (l *loopback) roundTrip(_ context.Context, c call, game *catalog.CompiledGame, req []byte, _ time.Duration, read func([]byte) error) error {
	told, err := l.games.of(game)
	if err != nil {
		return err
	}
	if was, _ := l.told.Load(told.id); was != told {
		var env envelope[bool]
		if err := json.Unmarshal(l.handler.Call(guest.CallInit, told.init), &env); err != nil {
			return fmt.Errorf("wasmrules: init of %s: %w", told.id, err)
		}
		if env.Error != nil {
			return fmt.Errorf("wasmrules: init: %s", *env.Error)
		}
		l.told.Store(told.id, told)
	}
	if err := read(l.handler.Call(c.String(), req)); err != nil {
		return fmt.Errorf("wasmrules: %s of %s: %w", c, told.id, err)
	}
	return nil
}
