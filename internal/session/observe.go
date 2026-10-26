package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// Observation is everything an agent needs to decide its next move, in one
// reinforcement-learning-style bundle: the position in several encodings,
// whose turn it is, the exact set of legal moves, and — once the game is
// over — the outcome from the observer's point of view.
type Observation struct {
	RoomCode  string
	GameDefID string
	Status    string
	Seats     []SeatInfo

	// Seat/Color identify the observer's seat (0/"" for a spectator).
	Seat  int
	Color string
	// YourTurn is true when the observer is seated and it is their move.
	YourTurn bool
	TurnSeat *int

	MoveSeq int
	Ply     int

	// BoardText is a human/LLM-readable rendering of the position; TPS is
	// the standard one-line position string for games that define one.
	BoardText string
	TPS       string
	State     string // full position document (JSON)

	// LegalMoves lists the moves available in canonical notation when it is
	// the observer's turn.
	LegalMoves []string

	LastMove *MoveRecord

	Done       bool
	WinnerSeat *int
	EndReason  string
	// Result is "win", "loss" or "draw" from the observer's seat once the
	// game is over; "" otherwise (and for spectators).
	Result string
	// Reward is +1 for a win, -1 for a loss and 0 for a draw or an
	// unfinished game, from the observer's seat.
	Reward float64

	// MoveTimeLimit is the per-move allowance in seconds (0 = untimed) and
	// SecondsLeft what remains for the player to move (nil if untimed).
	MoveTimeLimit int
	SecondsLeft   *int
}

// Observe builds an Observation of roomCode for the player called name
// (name may be empty or unseated to observe as a spectator).
func (m *Manager) Observe(ctx context.Context, roomCode, name string) (Observation, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return Observation{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return Observation{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return Observation{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	snap, err := m.snapshot(compiled, game, players, game.BoardState)
	if err != nil {
		return Observation{}, err
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return Observation{}, err
	}

	obs := Observation{
		RoomCode:   roomCode,
		GameDefID:  game.GameDefID,
		Status:     game.Status,
		Seats:      snap.Seats,
		TurnSeat:   game.TurnSeat,
		MoveSeq:    game.MoveSeq,
		Ply:        bs.Ply(),
		BoardText:  state.RenderText(compiled, bs),
		State:      snap.State,
		Done:       game.Status == "finished",
		WinnerSeat: game.WinnerSeat,
		EndReason:  game.EndReason,
	}

	if name != "" {
		var viewer *store.Player
		for i := range players {
			if strings.EqualFold(players[i].Name, name) {
				viewer = &players[i]
			}
		}
		if viewer != nil {
			obs.Seat, obs.Color = viewer.Seat, viewer.Color
		}
	}

	if tps, err := state.FormatTPS(compiled, bs, turnOrFirst(game, compiled)); err == nil && compiled.Definition.Notation == "ptn" {
		obs.TPS = tps
	}
	if state.IsRuleset(compiled) {
		// The observer sees their own secrets and nobody else's.
		if doc, err := state.Public(compiled, bs, obs.Seat).MarshalJSON(); err == nil {
			obs.State = string(doc)
		}
	}

	if game.Status == "active" && game.TurnSeat != nil && obs.Seat != 0 && *game.TurnSeat == obs.Seat {
		obs.YourTurn = true
		for _, lm := range legalMoveList(compiled, bs, obs.Seat) {
			obs.LegalMoves = append(obs.LegalMoves, lm.Notation)
		}
	}
	if obs.LegalMoves == nil {
		obs.LegalMoves = []string{}
	}

	if obs.Done && obs.Seat != 0 {
		switch {
		case game.WinnerSeat == nil:
			obs.Result = "draw"
		case *game.WinnerSeat == obs.Seat:
			obs.Result, obs.Reward = "win", 1
		default:
			obs.Result, obs.Reward = "loss", -1
		}
	}

	obs.MoveTimeLimit = game.MoveTimeLimit
	if d := m.deadline(game); d != nil {
		left := int(d.Sub(m.now()).Seconds())
		if left < 0 {
			left = 0
		}
		obs.SecondsLeft = &left
	}

	if game.MoveSeq > 0 {
		recs, err := m.ListMoves(ctx, roomCode, game.MoveSeq-1)
		if err == nil && len(recs) > 0 {
			last := recs[len(recs)-1]
			obs.LastMove = &last
		}
	}

	return obs, nil
}

func turnOrFirst(game store.Game, compiled *catalog.CompiledGame) int {
	if game.TurnSeat != nil {
		return *game.TurnSeat
	}
	return firstSeatOf(compiled, game)
}

// GameRecord is a complete, portable record of a game: its definition,
// players, every move with its notation, and the result. It is meant for
// archiving games and for building training datasets.
type GameRecord struct {
	RoomCode   string       `json:"room_code"`
	GameDefID  string       `json:"game_def_id"`
	GameName   string       `json:"game_name"`
	Status     string       `json:"status"`
	Seats      []SeatInfo   `json:"seats"`
	WinnerSeat *int         `json:"winner_seat"`
	EndReason  string       `json:"end_reason,omitempty"`
	Moves      []MoveRecord `json:"moves"`
	// Notation lists the moves in canonical notation, in order.
	Notation     []string `json:"notation"`
	InitialState string   `json:"initial_state"`
	FinalState   string   `json:"final_state"`
	// PTN is the game as a PTN document for games that use PTN notation.
	PTN string `json:"ptn,omitempty"`
}

// Export returns the complete record of roomCode's game.
func (m *Manager) Export(ctx context.Context, roomCode string) (GameRecord, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return GameRecord{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return GameRecord{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return GameRecord{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	snap, err := m.snapshot(compiled, game, players, game.BoardState)
	if err != nil {
		return GameRecord{}, err
	}
	moves, err := m.ListMoves(ctx, roomCode, 0)
	if err != nil {
		return GameRecord{}, err
	}
	initial, err := startDoc(compiled, game)
	if err != nil {
		return GameRecord{}, err
	}
	if game.Status != "finished" {
		// A live game's start position carries its secret seed.
		initial = publicDoc(compiled, initial)
	}

	rec := GameRecord{
		RoomCode:     roomCode,
		GameDefID:    game.GameDefID,
		GameName:     compiled.Definition.Name,
		Status:       game.Status,
		Seats:        snap.Seats,
		WinnerSeat:   game.WinnerSeat,
		EndReason:    game.EndReason,
		Moves:        moves,
		Notation:     make([]string, len(moves)),
		InitialState: initial,
		FinalState:   snap.State,
	}
	for i, mv := range moves {
		rec.Notation[i] = mv.Notation
	}
	if compiled.Definition.Notation == "ptn" {
		tps := ""
		if game.InitialState != "" {
			// A custom start is recorded as a TPS header so the document
			// replays correctly.
			start := state.NewBoardState()
			if err := start.UnmarshalJSON([]byte(game.InitialState)); err == nil {
				tps, _ = state.FormatTPS(compiled, start, firstSeatOf(compiled, game))
			}
		}
		rec.PTN = formatPTNDocument(compiled.Board.Width(), rec, game, tps)
	}
	return rec, nil
}

// formatPTNDocument renders a finished or in-progress Tak game as PTN.
func formatPTNDocument(size int, rec GameRecord, game store.Game, tps string) string {
	var b strings.Builder
	header := func(k, v string) { fmt.Fprintf(&b, "[%s %q]\n", k, v) }
	header("Site", "bingle")
	header("Event", "Casual game "+rec.RoomCode)
	if t, err := time.Parse("2006-01-02 15:04:05", game.CreatedAt); err == nil {
		header("Date", t.Format("2006.01.02"))
	} else if len(game.CreatedAt) >= 10 {
		header("Date", strings.ReplaceAll(game.CreatedAt[:10], "-", "."))
	}
	names := map[int]string{}
	for _, s := range rec.Seats {
		names[s.Seat] = s.Name
	}
	header("Player1", names[1])
	header("Player2", names[2])
	header("Size", fmt.Sprintf("%d", size))
	moveNo, blackFirst := 1, false
	if tps != "" {
		header("TPS", tps)
		// TPS ends "<player to move> <move number>".
		if f := strings.Fields(tps); len(f) == 3 {
			if n, err := strconv.Atoi(f[2]); err == nil && n > 0 {
				moveNo = n
			}
			blackFirst = f[1] == "2"
		}
	}
	if res := ptnResult(rec); res != "" {
		header("Result", res)
	}
	b.WriteByte('\n')
	i := 0
	if blackFirst && len(rec.Notation) > 0 {
		fmt.Fprintf(&b, "%d. -- %s\n", moveNo, rec.Notation[0])
		i, moveNo = 1, moveNo+1
	}
	for ; i < len(rec.Notation); i += 2 {
		fmt.Fprintf(&b, "%d. %s", moveNo, rec.Notation[i])
		if i+1 < len(rec.Notation) {
			fmt.Fprintf(&b, " %s", rec.Notation[i+1])
		}
		b.WriteByte('\n')
		moveNo++
	}
	if res := ptnResult(rec); res != "" {
		b.WriteString(res + "\n")
	}
	return b.String()
}

// ptnResult maps a finished game to PTN's result tokens: R-0/0-R (road),
// F-0/0-F (flats), 1-0/0-1 (any other decisive result, e.g. resignation) and
// 1/2-1/2 (draw).
func ptnResult(rec GameRecord) string {
	if rec.Status != "finished" {
		return ""
	}
	if rec.WinnerSeat == nil {
		return "1/2-1/2"
	}
	kind := "1"
	switch rec.EndReason {
	case "road":
		kind = "R"
	case "flat-win":
		kind = "F"
	}
	if *rec.WinnerSeat == 1 {
		return kind + "-0"
	}
	return "0-" + kind
}
