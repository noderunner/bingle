package wasmrules

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// The calls of ABI v0, in the order of callNames.
type call int

const (
	callInit call = iota
	callStart
	callImpulseSeat
	callLegal
	callApply
	callFormat
	callParse
	callGestures
	callValidate
	callOutcome
	numCalls
)

var callNames = [numCalls]string{
	guest.CallInit, guest.CallStart, guest.CallImpulseSeat, guest.CallLegal, guest.CallApply,
	guest.CallFormat, guest.CallParse, guest.CallGestures, guest.CallValidate, guest.CallOutcome,
}

func (c call) String() string { return callNames[c] }

// transport carries one call to a game's rules and back: a module's pool of
// instances, or a guest.Handler in process.
type transport interface {
	// roundTrip sends the JSON request req of call c, which names game, and
	// gives the reply to read. A read that fails means the reply is not one:
	// a transport that can replace what answered does so, and asks again. read
	// may therefore run twice, and only its last run counts.
	roundTrip(c call, game *catalog.CompiledGame, req []byte, read func(reply []byte) error) error
}

// adapter is a state.Ruleset over the JSON calls of ABI v0 (ADR 0147), so the
// session, the API, MCP, the bots and the page play a pack's game as they
// play any other. Only the Ruleset's nine methods cross. No optional
// interface of a ruleset is bridged but SeededRuleset, which costs no call.
type adapter struct {
	t   transport
	log *slog.Logger
}

var _ state.SeededRuleset = (*adapter)(nil)

// request is a call's request: each call sets the fields it takes.
type request struct {
	Game      string            `json:"game"`
	Position  *state.BoardState `json:"position,omitempty"`
	Seat      int               `json:"seat,omitempty"`
	Selection *state.Selection  `json:"selection,omitempty"`
	Text      string            `json:"text,omitempty"`
	Seed      uint64            `json:"seed,omitempty"`
}

// envelope is every reply: its ok, or the call's own error.
type envelope[T any] struct {
	OK    *T      `json:"ok"`
	Error *string `json:"error"`
}

// applyReply is the ok of apply; outcomeReply the ok of outcome, and the
// part of apply's that says how the new position stands.
type applyReply struct {
	Position *state.BoardState  `json:"position"`
	Captures []board.PositionID `json:"captures"`
	NextSeat int                `json:"next_seat"`
	Notation string             `json:"notation"`
	Facts    []state.Fact       `json:"facts"`
	outcomeReply
}

type outcomeReply struct {
	Ended   bool   `json:"ended"`
	Winner  int    `json:"winner"`
	Drawn   []int  `json:"drawn"`
	AllLost bool   `json:"all_lost"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail"`
}

// ask makes one call and reads its ok. check, when not nil, says whether the
// ok makes sense for the game: one that does not counts as no reply at all.
// The error is the call's own (an illegal selection, notation that cannot be
// read) or the transport's (see Module).
func ask[T any](a *adapter, c call, game *catalog.CompiledGame, req request, check func(*T) error) (T, error) {
	var out T
	req.Game = game.Definition.ID
	body, err := json.Marshal(&req)
	if err != nil {
		return out, fmt.Errorf("wasmrules: %s of %s: the request cannot be written: %w", c, req.Game, err)
	}
	var own error
	err = a.t.roundTrip(c, game, body, func(reply []byte) error {
		var env envelope[T]
		if err := json.Unmarshal(reply, &env); err != nil {
			return fmt.Errorf("the reply is not what the call replies with: %w", err)
		}
		switch {
		case env.Error != nil:
			own = errors.New(*env.Error)
			return nil
		case env.OK == nil:
			return errors.New("the reply is neither ok nor an error")
		}
		if check != nil {
			if err := check(env.OK); err != nil {
				return err
			}
		}
		out, own = *env.OK, nil
		return nil
	})
	if err != nil {
		return out, err
	}
	return out, own
}

// quiet makes a call for a method that cannot fail, and says so in the log
// when the call did: the method then answers with something that leaves the
// game as it stands.
func quiet[T any](a *adapter, c call, game *catalog.CompiledGame, req request, check func(*T) error) (T, bool) {
	out, err := ask(a, c, game, req, check)
	if err != nil {
		a.log.Warn("wasmrules: a call failed, and was answered with nothing",
			"call", c.String(), "game", game.Definition.ID, "error", err)
		return out, false
	}
	return out, true
}

func seatOrNone(game *catalog.CompiledGame, what string, seat int) error {
	if seat == 0 {
		return nil
	}
	if _, ok := game.Seat(seat); !ok {
		return fmt.Errorf("%s is seat %d, which the game does not have", what, seat)
	}
	return nil
}

func (r *outcomeReply) check(game *catalog.CompiledGame) error {
	if err := seatOrNone(game, "the winner", r.Winner); err != nil {
		return err
	}
	for _, seat := range r.Drawn {
		if _, ok := game.Seat(seat); !ok {
			return fmt.Errorf("a draw is shared by seat %d, which the game does not have", seat)
		}
	}
	return nil
}

func (r *outcomeReply) winner() *int {
	if r.Winner == 0 {
		return nil
	}
	w := r.Winner
	return &w
}

// Seeded says a start is recorded with its game: what start does with the
// seed is the module's business, so a replay must not ask for it again.
func (a *adapter) Seeded() bool { return true }

func (a *adapter) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st, err := ask[state.BoardState](a, callStart, game, request{Seed: seed}, nil)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// ImpulseSeat answers 0 when the call fails. The engine reads that as nothing
// to say, not as the end of the game: it then names the first seat at a
// start and the seat that last moved after a move (state.StartSeat,
// state.ImpulseAfter). A game ends only by Apply's result or by Outcome.
func (a *adapter) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
	seat, _ := quiet(a, callImpulseSeat, game, request{Position: st}, func(seat *int) error {
		return seatOrNone(game, "the seat on impulse", *seat)
	})
	return seat
}

func (a *adapter) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Selection {
	legal, _ := quiet[[]state.Selection](a, callLegal, game, request{Position: st, Seat: seat}, nil)
	return legal
}

func (a *adapter) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	r, err := ask(a, callApply, game, request{Position: st, Seat: seat, Selection: &mv}, func(r *applyReply) error {
		if r.Position == nil {
			return errors.New("apply replied without a position")
		}
		if !r.Ended && r.NextSeat == 0 {
			return errors.New("apply names no seat for a game that goes on")
		}
		if err := seatOrNone(game, "the next seat", r.NextSeat); err != nil {
			return err
		}
		return r.check(game)
	})
	if err != nil {
		return nil, err
	}
	res := &state.MoveResult{
		NewState: r.Position, Captures: r.Captures, NextImpulseSeat: r.NextSeat,
		GameEnded: r.Ended, WinnerSeat: r.winner(), DrawnSeats: r.Drawn, AllLost: r.AllLost,
		Move: mv, Notation: r.Notation, EndReason: r.Reason, EndDetail: r.Detail, Facts: r.Facts,
	}
	if len(r.Captures) > 0 {
		res.Captured = &r.Captures[0]
	}
	return res, nil
}

func (a *adapter) Format(game *catalog.CompiledGame, mv state.Selection) string {
	text, _ := quiet[string](a, callFormat, game, request{Selection: &mv}, nil)
	return text
}

func (a *adapter) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	return ask[state.Selection](a, callParse, game, request{Text: text}, nil)
}

func (a *adapter) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	gestures, _ := quiet[[]state.Gesture](a, callGestures, game, request{Selection: &mv}, nil)
	return gestures
}

func (a *adapter) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	_, err := ask[bool](a, callValidate, game, request{Position: st}, nil)
	return err
}

func (a *adapter) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	r, ok := quiet(a, callOutcome, game, request{Position: st}, func(r *outcomeReply) error { return r.check(game) })
	if !ok {
		return state.Outcome{}
	}
	return state.Outcome{Ended: r.Ended, Winner: r.winner(), Drawn: r.Drawn, AllLost: r.AllLost, Reason: r.Reason, Detail: r.Detail}
}
