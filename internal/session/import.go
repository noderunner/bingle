package session

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
)

// ErrInvalidImport is returned by ImportGame for text that cannot be read as
// a game of the chosen definition.
var ErrInvalidImport = errors.New("session: cannot import game")

// MaxImportMoves bounds an imported game's length.
const MaxImportMoves = 600

// ImportOptions describes a game record to load.
type ImportOptions struct {
	// Text is a PTN document (with or without headers) or a plain move
	// list; move numbers, results, comments and annotations are ignored.
	Text string
	// GameDefID selects the game; empty derives it from a PTN [Size]
	// header (tak-5x5 for "5").
	GameDefID string
	// Player1 and Player2 name the two seats; empty falls back to the PTN
	// headers, then "Player 1" / "Player 2".
	Player1, Player2 string
}

// ImportResult describes the room an import created.
type ImportResult struct {
	RoomCode  string
	GameDefID string
	Status    string
	Moves     int
	Players   [2]string
}

var (
	ptnHeader    = regexp.MustCompile(`(?m)^\s*\[(\w+)\s+"([^"]*)"\]\s*$`)
	ptnComment   = regexp.MustCompile(`\{[^}]*\}`)
	ptnVariant   = regexp.MustCompile(`\([^)]*\)`)
	ptnNumber    = regexp.MustCompile(`^\d+\.+`)
	recordResult = regexp.MustCompile(`^(1-0|0-1|1/2-1/2|[RF0]-[RF0]|\*)$`)
)

// parseRecord splits a PTN-ish document into its headers and move tokens.
func parseRecord(text string) (map[string]string, []string) {
	headers := map[string]string{}
	for _, m := range ptnHeader.FindAllStringSubmatch(text, -1) {
		headers[strings.ToLower(m[1])] = m[2]
	}
	body := ptnHeader.ReplaceAllString(text, " ")
	body = ptnComment.ReplaceAllString(body, " ")
	body = ptnVariant.ReplaceAllString(body, " ")
	var moves []string
	for _, raw := range strings.FieldsFunc(body, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ',' }) {
		tok := ptnNumber.ReplaceAllString(strings.TrimSpace(raw), "")
		if tok == "" || tok == "--" || recordResult.MatchString(tok) {
			continue
		}
		if _, err := strconv.Atoi(tok); err == nil {
			continue // a bare move number
		}
		tok = strings.TrimRight(tok, "!?'")
		if tok != "" {
			moves = append(moves, tok)
		}
	}
	return headers, moves
}

// ImportGame creates a room holding the record's game: the moves are
// validated by the rules engine first (an illegal move names its number),
// then played in order so the room has full history, replay and export.
// Both seats are filled by the named players and nobody is "you": open the
// room as a spectator to review or analyse it.
func (m *Manager) ImportGame(ctx context.Context, opts ImportOptions) (ImportResult, error) {
	if len(opts.Text) > 256<<10 {
		return ImportResult{}, fmt.Errorf("%w: the text is too long", ErrInvalidImport)
	}
	headers, moves := parseRecord(opts.Text)
	if len(moves) == 0 {
		return ImportResult{}, fmt.Errorf("%w: no moves found", ErrInvalidImport)
	}
	if len(moves) > MaxImportMoves {
		return ImportResult{}, fmt.Errorf("%w: at most %d moves", ErrInvalidImport, MaxImportMoves)
	}

	defID := strings.TrimSpace(opts.GameDefID)
	if defID == "" {
		if size, err := strconv.Atoi(strings.TrimSpace(headers["size"])); err == nil {
			defID = fmt.Sprintf("tak-%dx%d", size, size)
		}
	}
	if defID == "" {
		return ImportResult{}, fmt.Errorf("%w: say which game this is (no PTN [Size] header)", ErrInvalidImport)
	}
	compiled, ok := m.catalog.Get(defID)
	if !ok {
		return ImportResult{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, defID)
	}
	if len(compiled.Definition.Seats) != 2 {
		return ImportResult{}, fmt.Errorf("%w: only two-player games can be imported", ErrInvalidImport)
	}
	if state.NeedsStartRecord(compiled) {
		return ImportResult{}, fmt.Errorf("%w: %s involves chance, so a move list cannot be replayed", ErrInvalidImport, compiled.Definition.Name)
	}

	start := ""
	if tps := strings.TrimSpace(headers["tps"]); tps != "" {
		start = tps
	}

	// Validate the whole record before creating anything.
	var st *state.BoardState
	var seat int
	if start != "" {
		var err error
		st, seat, err = state.ParseTPS(compiled, start)
		if err != nil {
			return ImportResult{}, fmt.Errorf("%w: TPS header: %v", ErrInvalidImport, err)
		}
	} else {
		st, seat = state.InitialState(compiled), state.FirstSeat(compiled)
	}
	parsed := make([]state.Move, 0, len(moves))
	for i, text := range moves {
		mv, err := state.ParseMove(compiled, text)
		if err != nil {
			return ImportResult{}, fmt.Errorf("%w: move %d (%q): %v", ErrInvalidImport, i+1, text, err)
		}
		res, err := state.Apply(compiled, st, seat, mv)
		if err != nil {
			return ImportResult{}, fmt.Errorf("%w: move %d (%q) is illegal: %v", ErrInvalidImport, i+1, text, err)
		}
		parsed = append(parsed, mv)
		if res.GameEnded && i < len(moves)-1 {
			return ImportResult{}, fmt.Errorf("%w: the game is over after move %d (%q) but %d more move(s) follow", ErrInvalidImport, i+1, text, len(moves)-i-1)
		}
		st, seat = res.NewState, res.NextTurnSeat
	}

	names := [2]string{strings.TrimSpace(opts.Player1), strings.TrimSpace(opts.Player2)}
	for i, key := range []string{"player1", "player2"} {
		if names[i] == "" {
			names[i] = strings.TrimSpace(headers[key])
		}
		if names[i] == "" {
			names[i] = fmt.Sprintf("Player %d", i+1)
		}
		if r := []rune(names[i]); len(r) > 40 {
			names[i] = string(r[:40])
		}
	}
	if strings.EqualFold(names[0], names[1]) {
		names[1] += " (2)"
	}

	created, err := m.CreateGameWithOptions(ctx, CreateOptions{GameDefID: defID, CreatorName: names[0], StartTPS: start})
	if err != nil {
		return ImportResult{}, err
	}
	nameFor := map[int]string{}
	// The creator took seat 1; the second name joins the other seat.
	nameFor[1] = names[0]
	if _, err := m.JoinGame(ctx, created.RoomCode, names[1], RolePlayer); err != nil {
		return ImportResult{}, err
	}
	nameFor[2] = names[1]

	// Replay the moves as whoever's turn it is.
	for i, mv := range parsed {
		snap, err := m.GetState(ctx, created.RoomCode, nil)
		if err != nil {
			return ImportResult{}, err
		}
		if snap.TurnSeat == nil {
			return ImportResult{}, fmt.Errorf("%w: the game ended before move %d", ErrInvalidImport, i+1)
		}
		if _, err := m.Play(ctx, created.RoomCode, nameFor[*snap.TurnSeat], mv); err != nil {
			return ImportResult{}, fmt.Errorf("%w: replaying move %d: %v", ErrInvalidImport, i+1, err)
		}
	}
	final, err := m.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{RoomCode: created.RoomCode, GameDefID: defID, Status: final.Status, Moves: len(parsed), Players: names}, nil
}
