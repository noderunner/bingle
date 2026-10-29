package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session/roomcode"
	"github.com/noderunner/bingle/internal/store"
)

// maxCreateGameAttempts bounds how many times CreateGame retries generating
// a fresh room code and inserting it, in the astronomically unlikely event
// of a collision (or a persistent non-collision DB error, which will simply
// fail all attempts and correctly surface).
const maxCreateGameAttempts = 5

// CreateGameResult is what CreateGame hands back to its caller: the new
// room's code, and the creator's own assigned seat/color — matching the REST
// table's POST /api/games response shape.
type CreateGameResult struct {
	RoomCode string
	Seat     int
	Color    string
	Status   string
	// BotMoves are moves bots already played because they move first.
	BotMoves []MoveOutcome
}

// SeatInfo describes one seat a game definition declares: its number, the
// color it plays, and the name of whoever occupies it ("" if open).
type SeatInfo struct {
	Seat  int
	Color string
	Label string
	Name  string // "" if this seat hasn't been filled yet
	Bot   string // bot ID when a built-in bot plays this seat
}

// ErrGameDefNotFound is returned by CreateGame when gameDefID isn't
// registered in the Manager's catalog.
var ErrGameDefNotFound = errors.New("session: game definition not found")

// ErrUnknownBot is returned by CreateGame when the requested opponent bot
// isn't registered.
var ErrUnknownBot = errors.New("session: unknown bot")

// ErrInvalidOptions is returned by CreateGame when the game's setup options
// are not accepted (an unknown option or choice, a faction chosen twice).
var ErrInvalidOptions = errors.New("session: invalid setup options")

// ErrInvalidSeat is returned by CreateGame when the requested seat doesn't
// exist in the game definition.
var ErrInvalidSeat = errors.New("session: invalid seat")

// CreateOptions configures CreateGameWithOptions.
type CreateOptions struct {
	GameDefID   string
	CreatorName string
	// Seat is the seat the creator takes; 0 means seat 1.
	Seat int
	// Opponent, when non-empty, is a built-in bot ID that fills every other
	// seat so the game starts immediately.
	Opponent string
	// SeatBots assigns a bot to individual seats (seat → bot ID); the other
	// seats are left open for people unless Opponent fills them.
	SeatBots map[int]string
	// Also lists seats, besides Seat, that the creator plays as well: a game
	// against oneself, to try a position or a cult out. Each such seat is a
	// player of its own, named after the creator and the seat ("Alice · Black
	// Goat"), so every seat still has someone to move for it.
	Also []int
	// MoveTimeSeconds, when positive, gives each player that many seconds
	// per move; a player who runs out loses on time. Zero means no limit.
	MoveTimeSeconds int
	// ClockSeconds, when set, plays the game with a game clock (a chess
	// clock): one time in seconds per seat, in seat order, so the two sides
	// may differ for handicap play. A player whose clock runs out forfeits.
	// It is for a game of two seats, and not together with MoveTimeSeconds.
	ClockSeconds []int
	// Lobby makes the game wait, with every seat filled, until its host
	// starts it: players gather through the invite link, choose their seats
	// and add bots first.
	Lobby bool
	// Options are the setup choices of a game that takes them (see
	// dsl.GameDefinition.Options): which cults sit at the table, an
	// expansion to include. The game that results has an id of its own.
	Options map[string]any

	// StartTPS starts the game from a TPS position (PTN games such as
	// Tak); the TPS names who moves first. StartState starts it from a
	// position document (the "state" of any game's observation) and
	// StartSeat says who moves first from it (0 = the definition's first
	// seat). At most one of the two; both are for puzzles, handicaps and
	// "play from this position".
	StartTPS   string
	StartState string
	StartSeat  int
}

// ErrInvalidStart is returned by CreateGame for a start position that does
// not parse, does not fit the game, or is already decided.
var ErrInvalidStart = errors.New("session: invalid start position")

// resolveStart returns the position and first mover a new game begins from,
// and whether it is a custom one.
func resolveStart(compiled *catalog.CompiledGame, opts CreateOptions) (*state.BoardState, int, bool, error) {
	if opts.StartTPS != "" && opts.StartState != "" {
		return nil, 0, false, fmt.Errorf("%w: give start_tps or start_state, not both", ErrInvalidStart)
	}
	var st *state.BoardState
	var seat int
	switch {
	case opts.StartTPS != "":
		var err error
		st, seat, err = state.ParseTPS(compiled, opts.StartTPS)
		if err != nil {
			return nil, 0, false, fmt.Errorf("%w: %v", ErrInvalidStart, err)
		}
	case opts.StartState != "":
		st = state.NewBoardState()
		if err := st.UnmarshalJSON([]byte(opts.StartState)); err != nil {
			return nil, 0, false, fmt.Errorf("%w: %v", ErrInvalidStart, err)
		}
		seat = opts.StartSeat
		if seat == 0 {
			seat = state.FirstSeat(compiled)
		}
		if _, ok := compiled.Seat(seat); !ok {
			return nil, 0, false, fmt.Errorf("%w: start_seat %d does not exist", ErrInvalidStart, seat)
		}
	default:
		// A game whose start carries a secret seed must have it recorded so
		// that replays, undo and exports see the same position.
		return state.InitialState(compiled), state.FirstSeat(compiled), state.NeedsStartRecord(compiled), nil
	}
	if err := state.Validate(compiled, st); err != nil {
		return nil, 0, false, fmt.Errorf("%w: %v", ErrInvalidStart, err)
	}
	// A position that is already won (for the side that just moved, or for
	// the side to move) is not a game.
	for _, mover := range compiled.Definition.Seats {
		if out, err := state.Evaluate(compiled, st, mover.Seat); err == nil && out.Ended {
			return nil, 0, false, fmt.Errorf("%w: the position is already decided (%s)", ErrInvalidStart, out.Reason)
		}
	}
	return st, seat, true, nil
}

// firstSeatOf is the seat that moves first in game: the one a custom start
// position names, else the definition's first seat.
func firstSeatOf(compiled *catalog.CompiledGame, game store.Game) int {
	if game.InitialTurnSeat > 0 {
		return game.InitialTurnSeat
	}
	return state.FirstSeat(compiled)
}

// startDoc returns the position document game began from.
func startDoc(compiled *catalog.CompiledGame, game store.Game) (string, error) {
	if game.InitialState != "" {
		return game.InitialState, nil
	}
	data, err := state.InitialState(compiled).MarshalJSON()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MaxMoveTimeSeconds bounds a per-move time limit (90 days).
const MaxMoveTimeSeconds = 90 * 24 * 3600

// ErrInvalidTimeLimit is returned by CreateGame for an out-of-range limit.
var ErrInvalidTimeLimit = errors.New("session: invalid move time limit")

// CreateGame registers a new game of gameDefID, seating creatorName at seat
// 1, and returns its freshly-generated room code and the creator's assigned
// seat/color. Returns ErrGameDefNotFound if gameDefID isn't registered in
// m's catalog.
func (m *Manager) CreateGame(ctx context.Context, gameDefID, creatorName string) (CreateGameResult, error) {
	return m.CreateGameWithOptions(ctx, CreateOptions{GameDefID: gameDefID, CreatorName: creatorName})
}

// CreateGameWithOptions is CreateGame with a choice of seat and an optional
// built-in bot opponent.
func (m *Manager) CreateGameWithOptions(ctx context.Context, opts CreateOptions) (CreateGameResult, error) {
	if len(opts.Options) > 0 {
		id, err := m.catalog.Derive(opts.GameDefID, opts.Options)
		if err != nil {
			return CreateGameResult{}, fmt.Errorf("%w: %v", ErrInvalidOptions, err)
		}
		opts.GameDefID = id
	}
	compiled, ok := m.catalog.Get(opts.GameDefID)
	if !ok {
		return CreateGameResult{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, opts.GameDefID)
	}

	// also names the seats the creator plays too.
	also := map[int]bool{}
	for _, n := range opts.Also {
		if _, ok := compiled.Seat(n); !ok {
			return CreateGameResult{}, fmt.Errorf("%w: game %q has no seat %d", ErrInvalidSeat, opts.GameDefID, n)
		}
		if n != max(opts.Seat, 1) {
			also[n] = true
		}
	}

	// fill maps the seats that get a bot to that bot: SeatBots chooses per
	// seat, Opponent fills every other seat.
	fill := map[int]bots.Bot{}
	for _, sd := range compiled.Definition.Seats {
		id := opts.SeatBots[sd.Seat]
		if id == "" {
			id = opts.Opponent
		}
		if id == "" || sd.Seat == max(opts.Seat, 1) || also[sd.Seat] {
			continue
		}
		b, found := m.bots.Get(id)
		if !found {
			return CreateGameResult{}, fmt.Errorf("%w: %q", ErrUnknownBot, id)
		}
		fill[sd.Seat] = b
	}

	creatorSeat := opts.Seat
	if creatorSeat == 0 {
		creatorSeat = 1
	}
	seatDef, found := compiled.Seat(creatorSeat)
	if !found {
		if opts.Seat == 0 {
			return CreateGameResult{}, fmt.Errorf("session: game definition %q declares no seat 1", opts.GameDefID)
		}
		return CreateGameResult{}, fmt.Errorf("%w: game %q has no seat %d", ErrInvalidSeat, opts.GameDefID, opts.Seat)
	}

	if opts.MoveTimeSeconds < 0 || opts.MoveTimeSeconds > MaxMoveTimeSeconds {
		return CreateGameResult{}, fmt.Errorf("%w: %d seconds (use 0 for none, at most %d)", ErrInvalidTimeLimit, opts.MoveTimeSeconds, MaxMoveTimeSeconds)
	}

	// A game is played with a clock or with a per-move allowance, not both:
	// two deadlines on one turn would be a riddle rather than a rule.
	if len(opts.ClockSeconds) > 0 && opts.MoveTimeSeconds > 0 {
		return CreateGameResult{}, fmt.Errorf("%w: a game clock and a time per move cannot both be set", ErrInvalidClock)
	}
	seatNumbers := make([]int, 0, len(compiled.Definition.Seats))
	for _, s := range compiled.Definition.Seats {
		seatNumbers = append(seatNumbers, s.Seat)
	}
	clock, err := newGameClock(seatNumbers, opts.ClockSeconds)
	if err != nil {
		return CreateGameResult{}, err
	}

	startState, firstSeat, customStart, err := resolveStart(compiled, opts)
	if err != nil {
		return CreateGameResult{}, err
	}
	boardJSON, err := startState.MarshalJSON()
	if err != nil {
		return CreateGameResult{}, fmt.Errorf("session: marshal initial board state: %w", err)
	}

	var game store.Game
	var player store.Player
	var createErr error
	for attempt := 0; attempt < maxCreateGameAttempts; attempt++ {
		code := roomcode.New()
		game, player, createErr = m.store.CreateGameAt(ctx, code, opts.GameDefID, string(boardJSON), opts.CreatorName, seatDef.Color, creatorSeat)
		if createErr == nil {
			break
		}
	}
	if createErr != nil {
		return CreateGameResult{}, fmt.Errorf("session: create game after %d attempts: %w", maxCreateGameAttempts, createErr)
	}

	if customStart {
		if err := m.store.SetInitialPosition(ctx, game.ID, string(boardJSON), firstSeat); err != nil {
			return CreateGameResult{}, fmt.Errorf("session: recording start position: %w", err)
		}
	}

	if opts.MoveTimeSeconds > 0 {
		if err := m.store.SetMoveTimeLimit(ctx, game.ID, opts.MoveTimeSeconds); err != nil {
			return CreateGameResult{}, fmt.Errorf("session: setting move time limit: %w", err)
		}
	}
	if clock != nil {
		stored, err := clock.marshal()
		if err != nil {
			return CreateGameResult{}, err
		}
		if err := m.store.SetGameClock(ctx, game.ID, stored); err != nil {
			return CreateGameResult{}, fmt.Errorf("session: setting the game clock: %w", err)
		}
		game.Clock = stored
	}
	if opts.Lobby {
		if err := m.store.SetLobby(ctx, game.ID, true); err != nil {
			return CreateGameResult{}, fmt.Errorf("session: setting lobby: %w", err)
		}
	}

	result := CreateGameResult{
		RoomCode: game.RoomCode,
		Seat:     player.Seat,
		Color:    player.Color,
		Status:   game.Status,
	}

	if len(fill) == 0 && len(also) == 0 {
		return result, nil
	}

	// Fill every remaining seat — with the bot, or with the creator again —
	// and start the game when the table is full.
	unlock := m.lockRoom(ctx, game.RoomCode)
	defer unlock()

	total := len(compiled.Definition.Seats)
	filled := 1
	for _, sd := range compiled.Definition.Seats {
		bot, isBot := fill[sd.Seat]
		if !isBot && !also[sd.Seat] {
			continue
		}
		filled++
		status := game.Status
		var turn *int
		if filled >= total && !opts.Lobby {
			status = "active"
			first := firstSeat
			turn = &first
		}
		botID, name := "", ""
		if isBot {
			botID, name = bot.ID(), botName(bot, sd, total)
		} else {
			name = aliasName(opts.CreatorName, sd)
		}
		if _, err := m.store.AddBotPlayer(ctx, game.ID, sd.Seat, name, sd.Color, botID, status, turn); err != nil {
			return CreateGameResult{}, fmt.Errorf("session: seating %q: %w", name, err)
		}
		result.Status = status
	}

	result.BotMoves = m.runBots(ctx, game.RoomCode)
	return result, nil
}

// shortNames are the names bots take at tables of three or more seats.
var shortNames = []string{"Ash", "Bex", "Cato", "Dara", "Eli", "Fay", "Gus", "Hana"}

// botName names a bot: "Bot (Random)" in two-seat games; at bigger tables the
// seat's faction with the bot's kind ("Black Goat (MCTS)"), so the list of
// players says at a glance who plays what and by which kind of bot. The name
// is given once, when the bot takes its seat, and never changes.
// AliasSeparator joins a player's name to the seat of an alias: "Alice · Black
// Goat". Clients treat every seat named that way after a player as theirs.
const AliasSeparator = " · "

// ownerOf is the person a seat's name belongs to: the name before the alias
// separator ("Alice" for "Alice · Black Goat"), or the whole name.
func ownerOf(name string) string {
	if i := strings.Index(name, AliasSeparator); i > 0 {
		return strings.TrimSpace(name[:i])
	}
	return strings.TrimSpace(name)
}

// sameOwner says two seat names are one person's: a player and the aliases
// they took for the other seats of a game against themselves.
func sameOwner(a, b string) bool {
	return strings.EqualFold(ownerOf(a), ownerOf(b))
}

// aliasName is the name of the seat a creator plays besides their own: their
// name and the seat's faction (or colour), at most 40 characters.
func aliasName(creator string, sd dsl.SeatDef) string {
	who := sd.Label
	if who == "" {
		who = sd.Color
	}
	if who == "" {
		who = fmt.Sprintf("seat %d", sd.Seat)
	}
	suffix := AliasSeparator + who
	room := 40 - len([]rune(suffix))
	runes := []rune(strings.TrimSpace(creator))
	if room < 1 {
		room = 1
	}
	if len(runes) > room {
		runes = runes[:room]
	}
	return string(runes) + suffix
}

func botName(bot bots.Bot, sd dsl.SeatDef, total int) string {
	if total <= 2 {
		return "Bot (" + bot.Label() + ")"
	}
	who := sd.Label
	if who == "" {
		who = shortNames[(sd.Seat-1)%len(shortNames)]
	}
	return who + " (" + bot.Label() + ")"
}
