package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/wire"
)

// createGameRequest is POST /games' request body.
// clockSeconds is a game clock in a create call: either one number, which
// both players get, or one time per seat for handicap play.
type clockSeconds []int

func (c *clockSeconds) UnmarshalJSON(b []byte) error {
	var one int
	if err := json.Unmarshal(b, &one); err == nil {
		*c = clockSeconds{one, one}
		return nil
	}
	var many []int
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("clock_seconds: a number of seconds, or one per seat")
	}
	*c = many
	return nil
}

type createGameRequest struct {
	GameDefID  string `json:"game_def_id"`
	PlayerName string `json:"player_name"`
	// Seat optionally picks the creator's seat (default 1).
	Seat int `json:"seat,omitempty"`
	// Opponent optionally names a built-in bot (see GET /bots) that fills
	// the remaining seats so the game starts immediately.
	Opponent string `json:"opponent,omitempty"`
	// SeatBots puts a built-in bot in chosen seats ("2": "random"); seats not
	// listed stay open for people unless Opponent fills them.
	SeatBots map[string]string `json:"seat_bots,omitempty"`
	// AlsoSeats lists seats, besides Seat, that the creator plays as well (a
	// game against oneself): each is a player of its own, named after the
	// creator and the seat ("Alice · Black Goat").
	AlsoSeats []int `json:"also_seats,omitempty"`
	// MoveTimeSeconds optionally limits each move's thinking time; a player
	// who runs out of time loses. Omitted or 0 means untimed.
	MoveTimeSeconds int `json:"move_time_seconds,omitempty"`
	// ClockSeconds plays the game with a game clock (a chess clock) instead:
	// a number gives both players that many seconds, a list of two gives the
	// seats their own times (handicap play). A player whose clock runs out
	// forfeits. For a game of two seats, and not with MoveTimeSeconds.
	ClockSeconds clockSeconds `json:"clock_seconds,omitempty"`
	// Lobby makes the game wait, even with every seat filled, until its host
	// starts it (POST /games/{code}/start).
	Lobby bool `json:"lobby,omitempty"`
	// Options are the game's setup choices (see GET /game-defs/{id}'s
	// "options"): the cults at the table, an expansion. The game created has
	// its own id (base~options), reported by GET /games/{code}.
	Options map[string]any `json:"options,omitempty"`
	// StartTPS starts from a TPS position (Tak); StartState from a position
	// document (the "state" object of any observation) with StartSeat
	// moving first. For puzzles, handicaps and "play from here".
	StartTPS   string          `json:"start_tps,omitempty"`
	StartState json.RawMessage `json:"start_state,omitempty"`
	StartSeat  int             `json:"start_seat,omitempty"`
}

// createGameResponse is POST /games' 200 response body, from
// session.CreateGameResult.
type createGameResponse struct {
	RoomCode string      `json:"room_code"`
	Seat     int         `json:"seat"`
	Color    string      `json:"color"`
	Status   string      `json:"status"`
	BotMoves []wire.Move `json:"bot_moves,omitempty"`
}

// handleCreateGame implements POST /games: decode {"game_def_id",
// "player_name"}, reject blank/whitespace-only fields with 400 before
// calling Manager, then create the game and respond with its room
// code/seat/color/status.
func (h *handlers) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	var req createGameRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}

	if strings.TrimSpace(req.GameDefID) == "" {
		writeError(w, errors.New("api: game_def_id is required"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	seatBots := map[int]string{}
	for k, v := range req.SeatBots {
		n, convErr := strconv.Atoi(k)
		if convErr != nil || n < 1 {
			writeError(w, fmt.Errorf("api: seat_bots key %q is not a seat number", k))
			return
		}
		seatBots[n] = strings.TrimSpace(v)
	}

	result, err := h.mgr.CreateGameWithOptions(r.Context(), session.CreateOptions{
		GameDefID:   req.GameDefID,
		CreatorName: req.PlayerName,
		Seat:        req.Seat,
		Opponent:    strings.TrimSpace(req.Opponent),
		SeatBots:    seatBots,
		Also:        req.AlsoSeats,

		MoveTimeSeconds: req.MoveTimeSeconds,
		ClockSeconds:    req.ClockSeconds,
		Lobby:           req.Lobby,
		Options:         req.Options,

		StartTPS:   strings.TrimSpace(req.StartTPS),
		StartState: string(req.StartState),
		StartSeat:  req.StartSeat,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, createGameResponse{
		RoomCode: result.RoomCode,
		Seat:     result.Seat,
		Color:    result.Color,
		Status:   result.Status,
		BotMoves: wire.MovesFromOutcomes(result.BotMoves),
	})
}

// handleListGameDefs implements GET /game-defs: respond with every
// registered game definition's id/name/seats_required.
func (h *handlers) handleListGameDefs(w http.ResponseWriter, r *http.Request) {
	defs := h.mgr.ListGameDefs(r.Context())

	writeJSON(w, http.StatusOK, wire.GameDefs(defs))
}

// getGameResponse is GET /games/{code}'s 200 response body, from
// session.StateSnapshot — deliberately omits board_state/move_seq, which
// live at GET /games/{code}/state instead.
type getGameResponse struct {
	GameDefID  string        `json:"game_def_id"`
	Status     string        `json:"status"`
	Seats      []getGameSeat `json:"seats"`
	TurnSeat   *int          `json:"turn_seat"`
	MoveSeq    int           `json:"move_seq"`
	WinnerSeat *int          `json:"winner_seat"`
	EndReason  string        `json:"end_reason,omitempty"`
	EndDetail  string        `json:"end_detail,omitempty"`

	// Timed games: the per-move allowance, when the player to move runs
	// out of time (RFC 3339) and the server's clock for skew correction.
	MoveTimeSeconds int    `json:"move_time_seconds,omitempty"`
	TurnDeadline    string `json:"turn_deadline,omitempty"`
	ServerTime      string `json:"server_time"`
	// Clock is the game clock of a game played with one: what each seat has
	// left, and whose clock is running (it counts down from TurnDeadline).
	Clock *session.ClockSnapshot `json:"clock,omitempty"`

	// DrawOfferSeat is the seat offering a draw, or null.
	DrawOfferSeat *int `json:"draw_offer_seat"`

	// TakebackSeat is the seat asking to take back its last move, or null;
	// TakebackToSeq is how many moves would remain.
	TakebackSeat  *int `json:"takeback_seat"`
	TakebackToSeq int  `json:"takeback_to_seq,omitempty"`

	// StartState is the position the game began from, present only for
	// games that did not start from the standard position.
	StartState    json.RawMessage `json:"start_state,omitempty"`
	StartTurnSeat int             `json:"start_turn_seat,omitempty"`

	// Lobby: the game waits for its host to start it; HostSeat is the seat of
	// the person who created it. InviteURL is the link that brings a person
	// to the game (the configured public URL, else this server's address).
	Lobby     bool   `json:"lobby,omitempty"`
	HostSeat  int    `json:"host_seat,omitempty"`
	InviteURL string `json:"invite_url,omitempty"`

	// Retired: this server no longer offers the game, GameName is what it
	// was called. The answer is then a record: the seats, the result and
	// move_seq; the moves come from GET .../moves, and nothing else about
	// the game can be asked for or done.
	Retired  bool   `json:"retired,omitempty"`
	GameName string `json:"game_name,omitempty"`

	// Tutorial is "family/id" of the tutorial the room plays: the page
	// fetches it from GET /game-defs/{family}/tutorials/{id}.
	Tutorial string `json:"tutorial,omitempty"`
}

// getGameSeat is one entry of getGameResponse's seats array, from
// session.SeatInfo.
type getGameSeat struct {
	Seat  int    `json:"seat"`
	Color string `json:"color"`
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Bot   string `json:"bot,omitempty"`
	// Handle is the account holding the seat, Guest says a guest does, and
	// Yours that it is the asker's own (stage 7 of the accounts plan).
	Handle string `json:"handle,omitempty"`
	Guest  bool   `json:"guest,omitempty"`
	Yours  bool   `json:"yours,omitempty"`
}

// handleGetGame implements GET /games/{code}: look up the room's current
// state (no at_seq support here — that's .../state's job) and respond with
// its game_def_id/status/seats/turn_seat.
func (h *handlers) handleGetGame(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	snapshot, err := h.mgr.GetState(r.Context(), code, nil)
	if errors.Is(err, session.ErrGameRetired) {
		h.writeRetiredGame(w, r, code)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}

	seats := make([]getGameSeat, len(snapshot.Seats))
	for i, s := range snapshot.Seats {
		seats[i] = getGameSeat{Seat: s.Seat, Color: s.Color, Name: s.Name, Label: s.Label, Bot: s.Bot, Handle: s.Handle, Guest: s.Guest,
			Yours: s.PrincipalID != "" && s.PrincipalID == auth.IDFrom(r.Context())}
	}

	resp := getGameResponse{
		GameDefID:  snapshot.GameDefID,
		Status:     snapshot.Status,
		Seats:      seats,
		TurnSeat:   snapshot.TurnSeat,
		MoveSeq:    snapshot.MoveSeq,
		WinnerSeat: snapshot.WinnerSeat,
		EndReason:  snapshot.EndReason,
		EndDetail:  snapshot.EndDetail,
		Tutorial:   snapshot.Tutorial,

		MoveTimeSeconds: snapshot.MoveTimeLimit,
		Clock:           snapshot.Clock,
		ServerTime:      snapshot.ServerTime.Format(time.RFC3339),
		DrawOfferSeat:   snapshot.DrawOfferSeat,

		TakebackSeat:  snapshot.TakebackSeat,
		TakebackToSeq: snapshot.TakebackToSeq,

		Lobby:     snapshot.Lobby,
		HostSeat:  snapshot.HostSeat,
		InviteURL: h.inviteURL(r, code),
	}
	if snapshot.StartState != "" {
		resp.StartState = json.RawMessage(snapshot.StartState)
		resp.StartTurnSeat = snapshot.StartTurnSeat
	}
	if snapshot.TurnDeadline != nil {
		resp.TurnDeadline = snapshot.TurnDeadline.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// joinGameRequest is POST /games/{code}/join's request body. Role is
// optional; a blank value defaults to "player" (session.RolePlayer).
type joinGameRequest struct {
	PlayerName string `json:"player_name"`
	Role       string `json:"role"`
	// Seat optionally asks for a particular open seat (a player already at
	// the table keeps theirs).
	Seat int `json:"seat,omitempty"`
}

// joinGameResponse is POST /games/{code}/join's 200 response body, from
// session.JoinResult. Seat/Color are tagged omitempty so a spectator's
// response (Seat == 0, Color == "") genuinely omits those keys rather than
// sending zero values.
type joinGameResponse struct {
	Role   string `json:"role"`
	Seat   int    `json:"seat,omitempty"`
	Color  string `json:"color,omitempty"`
	Status string `json:"status"`
}

// handleJoinGame implements POST /games/{code}/join: decode
// {"player_name", "role"}, reject a blank/whitespace-only player_name with
// 400 before calling Manager, resolve role (blank/omitted defaults to
// "player"; anything other than "player"/"spectator" is a 400), then join
// the room and respond with the resulting role/seat/color/status.
func (h *handlers) handleJoinGame(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req joinGameRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}

	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	var role session.Role
	switch strings.TrimSpace(req.Role) {
	case "", string(session.RolePlayer):
		role = session.RolePlayer
	case string(session.RoleSpectator):
		role = session.RoleSpectator
	default:
		writeError(w, fmt.Errorf("api: invalid role %q", req.Role))
		return
	}

	result, err := h.mgr.JoinGameAt(r.Context(), code, req.PlayerName, role, req.Seat)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, joinGameResponse{
		Role:   string(result.Role),
		Seat:   result.Seat,
		Color:  result.Color,
		Status: result.Status,
	})
}

// getGameStateResponse is GET /games/{code}/state's 200 response body, from
// session.StateSnapshot. board_state is embedded as a raw JSON object (it's
// already a JSON string from state.BoardState.MarshalJSON), not a plain
// string field, so it appears nested rather than doubly-encoded.
type getGameStateResponse struct {
	// BoardState is the v1 flat view (position -> top piece), kept for
	// backwards compatibility; State is the complete position (stacks,
	// reserves, ply) and what new clients should read.
	BoardState json.RawMessage `json:"board_state"`
	State      json.RawMessage `json:"state,omitempty"`
	TurnSeat   *int            `json:"turn_seat"`
	Status     string          `json:"status"`
	MoveSeq    int             `json:"move_seq"`
	WinnerSeat *int            `json:"winner_seat"`
	EndReason  string          `json:"end_reason,omitempty"`
	EndDetail  string          `json:"end_detail,omitempty"`
	// DrawOfferSeat is the seat currently offering a draw, or null.
	DrawOfferSeat *int `json:"draw_offer_seat"`
	// TakebackSeat is the seat asking to take back its last move, or null;
	// TakebackToSeq is how many moves would remain.
	TakebackSeat  *int `json:"takeback_seat"`
	TakebackToSeq int  `json:"takeback_to_seq,omitempty"`
}

// handleGetGameState implements GET /games/{code}/state: look up the room's
// current state, or its historical state at ?at_seq=N if given, and respond
// with its board_state/turn_seat/status/move_seq.
func (h *handlers) handleGetGameState(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var atSeq *int
	if raw := r.URL.Query().Get("at_seq"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, errors.New("api: at_seq must be an integer"))
			return
		}
		atSeq = &n
	}

	// With ?player_name= a player of a game with secrets also sees their own.
	snapshot, err := h.mgr.GetStateFor(r.Context(), code, atSeq, r.URL.Query().Get("player_name"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, getGameStateResponse{
		BoardState: json.RawMessage(snapshot.BoardState),
		State:      json.RawMessage(snapshot.State),
		TurnSeat:   snapshot.TurnSeat,
		Status:     snapshot.Status,
		MoveSeq:    snapshot.MoveSeq,
		WinnerSeat: snapshot.WinnerSeat,
		EndReason:  snapshot.EndReason,
		EndDetail:  snapshot.EndDetail,

		DrawOfferSeat: snapshot.DrawOfferSeat,
	})
}

// makeMoveRequest is POST /games/{code}/moves' request body. A move is
// given in one of three ways:
//
//   - legacy: {"from", "to"} (step moves);
//   - structured: {"action", "piece", "from", "to", "count", "path",
//     "drops"} with only the fields the action needs;
//   - notation: {"notation": "3c3>111"} in the game's notation.
type makeMoveRequest struct {
	PlayerName string   `json:"player_name"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Action     string   `json:"action"`
	Piece      string   `json:"piece"`
	Count      int      `json:"count"`
	Path       []string `json:"path"`
	Drops      []int    `json:"drops"`
	Aim        string   `json:"aim"`
	Notation   string   `json:"notation"`
}

// makeMoveResponse is POST /games/{code}/moves' 200 response body, from
// docs/history/plan.md section 6.8: {"move": {...}, "state": {...}}. State reuses
// getGameStateResponse's shape. BotMoves lists replies built-in bot
// opponents played immediately afterwards (already reflected in State).
type makeMoveResponse struct {
	Move     wire.Move            `json:"move"`
	State    getGameStateResponse `json:"state"`
	BotMoves []wire.Move          `json:"bot_moves,omitempty"`
}

// handleMakeMove implements POST /games/{code}/moves: decode the move (see
// makeMoveRequest), reject a request that names no move with 400 before
// calling Manager (matching handleCreateGame/handleJoinGame's style), then
// apply the move and respond with the resulting move/state.
func (h *handlers) handleMakeMove(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req makeMoveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}

	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	var outcome session.MoveOutcome
	var err error
	switch {
	case strings.TrimSpace(req.Notation) != "":
		outcome, err = h.mgr.PlayNotation(r.Context(), code, req.PlayerName, req.Notation)
	case strings.TrimSpace(req.Action) != "" || strings.TrimSpace(req.Piece) != "" || req.Count != 0 || len(req.Path) > 0 || strings.TrimSpace(req.Aim) != "":
		mv := state.Move{
			Action: strings.TrimSpace(req.Action),
			Piece:  strings.TrimSpace(req.Piece),
			From:   board.PositionID(strings.TrimSpace(req.From)),
			To:     board.PositionID(strings.TrimSpace(req.To)),
			Count:  req.Count,
			Drops:  req.Drops,
			Aim:    board.PositionID(strings.TrimSpace(req.Aim)),
		}
		for _, p := range req.Path {
			mv.Path = append(mv.Path, board.PositionID(strings.TrimSpace(p)))
		}
		outcome, err = h.mgr.Play(r.Context(), code, req.PlayerName, mv)
	default:
		if strings.TrimSpace(req.From) == "" {
			writeError(w, errors.New("api: from is required"))
			return
		}
		if strings.TrimSpace(req.To) == "" {
			writeError(w, errors.New("api: to is required"))
			return
		}
		outcome, err = h.mgr.MakeMove(r.Context(), code, req.PlayerName, board.PositionID(req.From), board.PositionID(req.To))
	}
	if err != nil {
		writeError(w, err)
		return
	}

	// The response state reflects any bot replies too.
	final := outcome
	for _, b := range outcome.BotMoves {
		final = b
	}
	var turnSeat *int
	if !final.GameEnded {
		turnSeat = &final.NextTurnSeat
	}

	writeJSON(w, http.StatusOK, makeMoveResponse{
		Move: wire.MoveFromOutcome(outcome),
		State: getGameStateResponse{
			BoardState: json.RawMessage(final.BoardState),
			State:      json.RawMessage(final.State),
			TurnSeat:   turnSeat,
			Status:     final.Status,
			MoveSeq:    final.Seq,
			WinnerSeat: final.WinnerSeat,
			EndReason:  final.EndReason,
			EndDetail:  final.EndDetail,
		},
		BotMoves: wire.MovesFromOutcomes(outcome.BotMoves),
	})
}

// handleListMoves implements GET /games/{code}/moves: look up the room's
// move history, optionally starting after ?since_seq=N (default 0, i.e. all
// moves), and respond with a JSON array of moves in ascending seq order.
// Always writes "[]" rather than "null" when there are no moves.
func (h *handlers) handleListMoves(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	sinceSeq := 0
	if raw := r.URL.Query().Get("since_seq"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, errors.New("api: since_seq must be an integer"))
			return
		}
		sinceSeq = n
	}

	records, err := h.mgr.ListMoves(r.Context(), code, sinceSeq)
	if err != nil {
		writeError(w, err)
		return
	}

	history := make([]wire.Move, 0, len(records))
	for _, rec := range records {
		history = append(history, wire.MoveFromRecord(rec))
	}

	writeJSON(w, http.StatusOK, history)
}

// handleHistory implements GET /games/{code}/history: what every move of the
// game did, as the game narrated it, oldest first ("[]" for a game that does
// not narrate).
func (h *handlers) handleHistory(w http.ResponseWriter, r *http.Request) {
	entries, err := h.mgr.History(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleListLegalMoves implements GET /games/{code}/legal-moves: decode the
// REQUIRED ?from= query param (400 if missing/blank), then respond with a
// JSON array of every position that piece can legally move to right now.
// Always writes "[]" rather than "null" when there are none.
func (h *handlers) handleListLegalMoves(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	from := r.URL.Query().Get("from")
	if strings.TrimSpace(from) == "" {
		writeError(w, errors.New("api: from is required"))
		return
	}

	positions, err := h.mgr.ListLegalMoves(r.Context(), code, board.PositionID(from))
	if err != nil {
		writeError(w, err)
		return
	}

	out := make([]string, 0, len(positions))
	for _, pos := range positions {
		out = append(out, string(pos))
	}

	writeJSON(w, http.StatusOK, out)
}

// writeJSON writes v as a JSON response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleOwnGames implements GET /me/games: the games the asker has a seat
// in, on whatever device it asks from ("My games", stage 7 of the accounts
// plan).
func (h *handlers) handleOwnGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.mgr.OwnGames(r.Context(), 60)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"games": games})
}

// writeRetiredGame answers GET /games/{code} for a game whose kind was
// retired: its record, marked retired.
func (h *handlers) writeRetiredGame(w http.ResponseWriter, r *http.Request, code string) {
	rec, err := h.mgr.RetiredGame(r.Context(), code)
	if err != nil {
		writeError(w, err)
		return
	}
	seats := make([]getGameSeat, len(rec.Seats))
	for i, s := range rec.Seats {
		seats[i] = getGameSeat{Seat: s.Seat, Color: s.Color, Name: s.Name, Bot: s.Bot,
			Yours: s.PrincipalID != "" && s.PrincipalID == auth.IDFrom(r.Context())}
	}
	writeJSON(w, http.StatusOK, getGameResponse{
		GameDefID: rec.GameDefID, Status: rec.Status, Seats: seats, MoveSeq: rec.MoveSeq,
		WinnerSeat: rec.WinnerSeat, EndReason: rec.EndReason, EndDetail: rec.EndDetail,
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		Retired:    true, GameName: rec.Name,
	})
}
