package state

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Ruleset is the Go implementation of a game whose rules the declarative
// DSL cannot express — hidden choices, dice, per-unit conditions, turns made
// of many small decisions. A game opts in with `"engine": "ruleset"` in its
// definition and registers the ruleset in its catalog.Module.
//
// The definition still describes the board, seats, pieces and display; the
// ruleset takes over everything the `actions` and `win_condition` of a
// declarative game would do. It keeps its own data (terrain, conditions,
// resources, the pending decision) in the position's ext payload, and it
// speaks the engine's ordinary vocabulary — Move, MoveResult, Step — so the
// session, API, MCP, bots, environments and the web UI work unchanged.
//
// A decision is one move: a ruleset turn is a sequence of moves, often by
// the same seat, and ToMove says whose decision the position is waiting for.
type Ruleset interface {
	// Start returns the opening position. seed seeds the game's chance (0
	// is valid but predictable).
	Start(game *catalog.CompiledGame, seed uint64) (*BoardState, error)
	// ToMove is the seat the position waits for, or 0 once it is over.
	ToMove(game *catalog.CompiledGame, st *BoardState) int
	// Legal lists every decision seat may make, in a deterministic order.
	Legal(game *catalog.CompiledGame, st *BoardState, seat int) []Move
	// Apply validates and applies one decision. It must reject exactly the
	// moves Legal does not list.
	Apply(game *catalog.CompiledGame, st *BoardState, seat int, mv Move) (*MoveResult, error)
	// Format and Parse convert a move to and from its notation.
	Format(game *catalog.CompiledGame, mv Move) string
	Parse(game *catalog.CompiledGame, text string) (Move, error)
	// Steps decomposes a legal move into the clicks that build it.
	Steps(game *catalog.CompiledGame, mv Move) []Step
	// Validate checks a position document (a custom start).
	Validate(game *catalog.CompiledGame, st *BoardState) error
	// Outcome reports whether the position is over, and who won.
	Outcome(game *catalog.CompiledGame, st *BoardState) Outcome
}

// SeededRuleset is implemented by rulesets whose start position carries a
// secret seed: the session records the start document so replays reuse it.
type SeededRuleset interface {
	Ruleset
	Seeded() bool
}

// HiddenInfo is implemented by rulesets with secrets (a committed choice, the
// random number generator) that clients must not see while a game is live.
type HiddenInfo interface {
	Ruleset
	// Redact returns a copy of st fit to show viewer: viewer 0 is the
	// public (spectators, opponents, events); a seat sees its own secrets.
	// The copy also carries the ruleset's derived view for clients.
	Redact(game *catalog.CompiledGame, st *BoardState, viewer int) *BoardState
	// RedactMove hides what a move record reveals while it is unresolved;
	// st is the game's current position.
	RedactMove(game *catalog.CompiledGame, st *BoardState, mv Move, notation string) (Move, string)
	// Determinize returns a copy of st in which everything viewer cannot
	// know is replaced by a random sample, for searching and simulating.
	Determinize(game *catalog.CompiledGame, st *BoardState, viewer int, rng *rand.Rand) *BoardState
}

// UndoGuard is implemented by rulesets where chance makes taking moves back
// unfair: rewinding across a die roll would let a player see the dice and
// choose again.
type UndoGuard interface {
	// CanTakeBack reports whether the game may return from after to before.
	CanTakeBack(game *catalog.CompiledGame, before, after *BoardState) bool
}

// CanTakeBack reports whether rewinding a game from after to before is
// allowed (always, unless the ruleset forbids it).
func CanTakeBack(game *catalog.CompiledGame, before, after *BoardState) bool {
	if rs, ok := RulesetOf(game); ok {
		if g, ok := rs.(UndoGuard); ok {
			return g.CanTakeBack(game, before, after)
		}
	}
	return true
}

// Searchable is implemented by rulesets that know when a decision is not
// worth searching (the setup of a skirmish): search bots then play a random
// legal move at once.
type Searchable interface {
	WorthSearching(game *catalog.CompiledGame, st *BoardState) bool
}

// WorthSearching reports whether a search bot should think about st at all
// (always, unless the game's ruleset says otherwise).
func WorthSearching(game *catalog.CompiledGame, st *BoardState) bool {
	if rs, ok := RulesetOf(game); ok {
		if s, ok := rs.(Searchable); ok {
			return s.WorthSearching(game, st)
		}
	}
	return true
}

// Heuristic is implemented by rulesets that can value a position for a seat
// — a number in roughly [-1, 1] — so search bots have something to chase
// when the game is too long to play out.
type Heuristic interface {
	Heuristic(game *catalog.CompiledGame, st *BoardState, seat int) float64
}

// RepetitionGuard is implemented by rulesets whose bots could shuffle
// pieces back and forth without achieving anything. It lets a caller notice
// that a decision only rearranges a seat's pieces into an arrangement the
// seat has already had recently.
type RepetitionGuard interface {
	// RepeatKey describes where seat's pieces stand (and nothing else).
	RepeatKey(game *catalog.CompiledGame, st *BoardState, seat int) string
	// Repositions reports whether mv merely rearranges pieces.
	Repositions(mv Move) bool
}

// RepetitionGuardOf returns game's ruleset as a RepetitionGuard, if it is one.
func RepetitionGuardOf(game *catalog.CompiledGame) (RepetitionGuard, bool) {
	rs, ok := RulesetOf(game)
	if !ok {
		return nil, false
	}
	g, ok := rs.(RepetitionGuard)
	return g, ok
}

// BotGuide is implemented by rulesets that know which of the legal decisions
// are worth a computer player's consideration: the strategy tips of the game
// (never give up a Gate, do not spend every Power point at once, ...). The
// session lets the built-in bots choose among the guided moves only, so every
// bot — random included — plays within the sense of the game, while people
// keep the full list of legal moves.
type BotGuide interface {
	// GuideMoves returns the moves of legal a bot should consider at st, best
	// first or in the original order. It must return at least one move when
	// legal is not empty and must only return moves taken from legal.
	GuideMoves(game *catalog.CompiledGame, st *BoardState, seat int, legal []Move) []Move
}

// GuidedMoves narrows legal to the moves the game's ruleset recommends to a
// bot (all of them unless the ruleset is a BotGuide).
func GuidedMoves(game *catalog.CompiledGame, st *BoardState, seat int, legal []Move) []Move {
	rs, ok := RulesetOf(game)
	if !ok || len(legal) < 2 {
		return legal
	}
	g, ok := rs.(BotGuide)
	if !ok {
		return legal
	}
	if guided := g.GuideMoves(game, st, seat, legal); len(guided) > 0 {
		return guided
	}
	return legal
}

// Reactor is implemented by rulesets with reactions: abilities a player may
// use on another player's turn, in the gap after that player's action (an
// interrupt, as card and board games have them). Such a decision asks a
// player who is not the one acting whether they want to use the ability, which
// can stall a game when the other players must wait for an answer; the
// session therefore declines for a person by default ("auto-pass"), and asks
// them only on the occasions they chose: when the action was aimed at them
// (Reaction.Targeted), after any other player's action, or after chosen
// players' actions. Bots are always asked.
type Reactor interface {
	// PendingReaction reports whether the decision st waits for is an
	// optional reaction: who may react, whose action opened the window, which
	// ability it is and the move that declines it. It returns nil otherwise.
	PendingReaction(game *catalog.CompiledGame, st *BoardState) *Reaction
	// ReactionAbilities lists the reaction abilities of seat's faction (what a
	// client offers the settings of when to be asked for), each flagged with
	// whether the seat can use it yet — so the choice can be made before it is
	// earned.
	ReactionAbilities(game *catalog.CompiledGame, st *BoardState, seat int) []ReactionAbility
}

// Reaction describes a pending decision that is a reaction window.
type Reaction struct {
	// Seat is who decides; Trigger is the seat whose action opened the window.
	Seat    int `json:"seat"`
	Trigger int `json:"trigger"`
	// Ability names what may be used ("Devolve").
	Ability string `json:"ability"`
	// Targeted says the action (or battle) that opened the window was aimed
	// at the reacting seat: at the player, at one of its units or at an area
	// where it has a unit or a Gate. Effects that follow an action are not
	// actions and never open a window.
	Targeted bool `json:"targeted,omitempty"`
	// Pass is the move that declines the reaction.
	Pass Move `json:"-"`
}

// ReactionAbility is one reaction of a seat's faction.
type ReactionAbility struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text,omitempty"`
	// Held is true once the seat can use it (a spellbook it has earned). The
	// faction's reactions are listed from the start, so that the player can
	// choose when to be asked before the first window opens.
	Held bool `json:"held"`
	// AskAlways says the player is asked about it on every occasion until
	// they choose otherwise (Madness: a unit of theirs may be sent away in
	// any battle, so a person must not miss the question).
	AskAlways bool `json:"ask_always,omitempty"`
}

// PendingReaction asks the game's ruleset (when it is a Reactor) whether the
// position waits for an optional reaction.
func PendingReaction(game *catalog.CompiledGame, st *BoardState) *Reaction {
	rs, ok := RulesetOf(game)
	if !ok {
		return nil
	}
	r, ok := rs.(Reactor)
	if !ok {
		return nil
	}
	return r.PendingReaction(game, st)
}

// ReactionAbilities lists the reaction abilities of seat's faction in st,
// flagged with whether it holds them yet (none for a game without reactions).
func ReactionAbilities(game *catalog.CompiledGame, st *BoardState, seat int) []ReactionAbility {
	rs, ok := RulesetOf(game)
	if !ok {
		return nil
	}
	r, ok := rs.(Reactor)
	if !ok {
		return nil
	}
	return r.ReactionAbilities(game, st, seat)
}

// ErrCultHeld is what SeatCultOptions returns for a cult that a taken seat
// plays: it cannot change places.
var ErrCultHeld = errors.New("state: that cult is played by a seat that is taken")

// SeatCults is implemented by rulesets whose seats are cults chosen when the
// game is set up (Cthulhu Wars): while a game waits in its lobby, the cult of
// an open seat may be changed.
type SeatCults interface {
	// SeatCultOptions returns the setup options (see catalog.Deriver) of the
	// table that has cult in seat instead of what sits there now. A cult that
	// another open seat plays changes places with it; taken marks the seats a
	// player or bot holds, whose cults stay. It returns nil options when
	// nothing changes, and an error for a cult that does not exist, cannot sit
	// on this map or is held by a taken seat.
	SeatCultOptions(game *catalog.CompiledGame, seat int, cult string, taken map[int]bool) (map[string]any, error)
}

// SeatCultOptions asks the game's ruleset (when it is a SeatCults) for the
// options of the table with seat's cult changed; ok is false for a game whose
// seats are not cults.
func SeatCultOptions(game *catalog.CompiledGame, seat int, cult string, taken map[int]bool) (options map[string]any, ok bool, err error) {
	rs, found := RulesetOf(game)
	if !found {
		return nil, false, nil
	}
	sc, found := rs.(SeatCults)
	if !found {
		return nil, false, nil
	}
	options, err = sc.SeatCultOptions(game, seat, cult, taken)
	return options, true, err
}

// PlaneEncoder is implemented by rulesets with their own numeric encoding.
type PlaneEncoder interface {
	// PlaneLayout names the channels (viewer-relative) and the plane size.
	PlaneLayout(game *catalog.CompiledGame) (names []string, height, width int)
	// PlaneCells lists the board position at every plane cell, row-major.
	PlaneCells(game *catalog.CompiledGame) []string
	// EncodePlanes fills [channel][y][x] for viewer.
	EncodePlanes(game *catalog.CompiledGame, st *BoardState, viewer int) []float32
}

// ActionLister is implemented by rulesets with a fixed set of move notations.
type ActionLister interface {
	ActionNotations(game *catalog.CompiledGame) []string
}

// TextRenderer is implemented by rulesets that draw their own text board.
type TextRenderer interface {
	Text(game *catalog.CompiledGame, st *BoardState) string
}

// RulesetInfo is what a client needs to label a ruleset game's decisions.
type RulesetInfo struct {
	// Actions are the decision kinds that appear as buttons (step kind
	// "action"), in a sensible order.
	Actions []RulesetAction `json:"actions"`
	// NotationHelp explains the move notation, one line each.
	NotationHelp []string `json:"notation_help"`
	// WinRules says how the game is won and ends, one line each (shown in
	// the generated guide and the MCP rules resource).
	WinRules []string `json:"win_rules,omitempty"`
	// CountLabels says what a count step means after a given action ("how
	// many attack dice"); the empty key is the default.
	CountLabels map[string]string `json:"count_labels,omitempty"`
}

// RulesetAction labels one action id.
type RulesetAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Hint is a one-line tooltip.
	Hint string `json:"hint,omitempty"`
	// Source says where the action comes from, when that is worth a mark on
	// its button: "book" for an ability a spellbook gives (earned), and
	// "requirement" for an action that meets a spellbook requirement.
	Source string `json:"source,omitempty"`
}

// Describer is implemented by rulesets that describe themselves to clients.
type Describer interface {
	Describe(game *catalog.CompiledGame) RulesetInfo
}

// RulesetOf returns game's Go ruleset, if it has one.
func RulesetOf(game *catalog.CompiledGame) (Ruleset, bool) {
	if game == nil || game.Rules == nil {
		return nil, false
	}
	rs, ok := game.Rules.(Ruleset)
	return rs, ok
}

// IsRuleset reports whether game's rules are a Go ruleset.
func IsRuleset(game *catalog.CompiledGame) bool {
	_, ok := RulesetOf(game)
	return ok
}

// NeedsStartRecord reports whether a new game's start position must be
// stored with the game (it carries a random seed that replays must reuse).
func NeedsStartRecord(game *catalog.CompiledGame) bool {
	if rs, ok := RulesetOf(game); ok {
		if sr, ok := rs.(SeededRuleset); ok {
			return sr.Seeded()
		}
	}
	return false
}

// InitialStateSeeded is InitialState for a ruleset game with its chance
// seeded by seed; for declarative games the seed is ignored.
func InitialStateSeeded(game *catalog.CompiledGame, seed uint64) (*BoardState, error) {
	rs, ok := RulesetOf(game)
	if !ok {
		return InitialState(game), nil
	}
	return rs.Start(game, seed)
}

// Public returns st as viewer may see it while the game is live. Games
// without secrets return st itself; viewer 0 is the public.
func Public(game *catalog.CompiledGame, st *BoardState, viewer int) *BoardState {
	if rs, ok := RulesetOf(game); ok {
		if h, ok := rs.(HiddenInfo); ok {
			return h.Redact(game, st, viewer)
		}
	}
	return st
}

// PublicMove hides what an unresolved move record would give away.
func PublicMove(game *catalog.CompiledGame, st *BoardState, mv Move, notation string) (Move, string) {
	if rs, ok := RulesetOf(game); ok {
		if h, ok := rs.(HiddenInfo); ok {
			return h.RedactMove(game, st, mv, notation)
		}
	}
	return mv, notation
}

// Determinize returns a copy of st with everything viewer cannot know
// sampled at random — what a search or a simulation must work on so it
// cannot peek at the dice or at the opponent's committed choice. Games
// without secrets return st itself.
func Determinize(game *catalog.CompiledGame, st *BoardState, viewer int, rng *rand.Rand) *BoardState {
	if rs, ok := RulesetOf(game); ok {
		if h, ok := rs.(HiddenInfo); ok {
			return h.Determinize(game, st, viewer, rng)
		}
	}
	return st
}

// HeuristicValue values st for seat if the game's ruleset can (ok is false
// otherwise).
func HeuristicValue(game *catalog.CompiledGame, st *BoardState, seat int) (float64, bool) {
	if rs, ok := RulesetOf(game); ok {
		if h, ok := rs.(Heuristic); ok {
			return h.Heuristic(game, st, seat), true
		}
	}
	return 0, false
}

// DescribeRuleset returns the client-facing labels of a ruleset game.
func DescribeRuleset(game *catalog.CompiledGame) (RulesetInfo, bool) {
	if rs, ok := RulesetOf(game); ok {
		if d, ok := rs.(Describer); ok {
			return d.Describe(game), true
		}
	}
	return RulesetInfo{}, false
}

// errNoTPS is returned for ruleset games, which have no TPS.
func errNoTPS(game *catalog.CompiledGame) error {
	return fmt.Errorf("state: game %q has no TPS notation", game.Definition.ID)
}
