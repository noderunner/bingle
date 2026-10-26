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

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/wire"
)

// handleGameDef implements GET /game-defs/{id}: the complete description of
// a game (board layout, pieces, actions, rules text, notation help).
func (h *handlers) handleGameDef(w http.ResponseWriter, r *http.Request) {
	info, err := h.mgr.GameInfo(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleGameGuide implements GET /game-defs/{id}/guide: the Markdown pages
// of the rules window (the game's own, or ones generated from its
// definition).
func (h *handlers) handleGameGuide(w http.ResponseWriter, r *http.Request) {
	guide, err := h.mgr.Guide(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, guide)
}

// handleGameAsset implements GET /game-defs/{id}/assets/{path}: an image or
// other file of a game's guide. Files are served inert: no scripts run in
// them even if opened directly.
func (h *handlers) handleGameAsset(w http.ResponseWriter, r *http.Request) {
	data, contentType, err := h.mgr.Asset(chi.URLParam(r, "id"), chi.URLParam(r, "*"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleAddBot implements POST /games/{code}/bots: a seated player fills
// the next open seat of a waiting game with a built-in bot.
func (h *handlers) handleAddBot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerName string `json:"player_name"`
		Bot        string `json:"bot"`
		// Seat optionally names the open seat the bot takes.
		Seat int `json:"seat,omitempty"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Bot) == "" {
		writeError(w, errors.New("api: bot is required"))
		return
	}
	res, err := h.mgr.AddBotAt(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Bot, req.Seat)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleListBots implements GET /bots: the built-in computer opponents.
func (h *handlers) handleListBots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.mgr.Bots().List())
}

// legalMoveDTO is one entry of GET /games/{code}/moves/legal.
type legalMoveDTO struct {
	Notation string       `json:"notation"`
	Action   string       `json:"action"`
	Piece    string       `json:"piece,omitempty"`
	From     string       `json:"from,omitempty"`
	To       string       `json:"to,omitempty"`
	Count    int          `json:"count,omitempty"`
	Path     []string     `json:"path,omitempty"`
	Drops    []int        `json:"drops,omitempty"`
	Aim      string       `json:"aim,omitempty"`
	Steps    []state.Step `json:"steps,omitempty"`
}

func legalMoveDTOs(moves []session.LegalMove, withSteps bool) []legalMoveDTO {
	out := make([]legalMoveDTO, len(moves))
	for i, lm := range moves {
		d := legalMoveDTO{
			Notation: lm.Notation,
			Action:   lm.Move.Action,
			Piece:    lm.Move.Piece,
			From:     string(lm.Move.From),
			To:       string(lm.Move.To),
			Count:    lm.Move.Count,
			Drops:    lm.Move.Drops,
			Aim:      string(lm.Move.Aim),
		}
		for _, p := range lm.Move.Path {
			d.Path = append(d.Path, string(p))
		}
		if withSteps {
			d.Steps = lm.Steps
		}
		out[i] = d
	}
	return out
}

// legalMovesResponse is GET /games/{code}/moves/legal's body.
type legalMovesResponse struct {
	Seat  int            `json:"seat"`
	Moves []legalMoveDTO `json:"moves"`
}

// handleLegalMovesAll implements GET /games/{code}/moves/legal: every legal
// move for the seat to move (or ?seat=N), with its notation and — when
// ?steps=1 — the click-by-click selection steps that build it.
func (h *handlers) handleLegalMovesAll(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	seat := 0
	if raw := r.URL.Query().Get("seat"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, errors.New("api: seat must be an integer"))
			return
		}
		seat = n
	}
	withSteps := r.URL.Query().Get("steps") != "" && r.URL.Query().Get("steps") != "0"

	seat, moves, err := h.mgr.LegalMoves(r.Context(), code, seat)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, legalMovesResponse{Seat: seat, Moves: legalMoveDTOs(moves, withSteps)})
}

// playerRequest is the body of the undo/resign endpoints.
type playerRequest struct {
	PlayerName string `json:"player_name"`
}

// undoResponse is POST /games/{code}/undo's body.
type undoResponse struct {
	Removed []int                `json:"removed"`
	State   getGameStateResponse `json:"state"`
}

// handleUndo implements POST /games/{code}/undo: take back the caller's
// last move (and any bot replies to it).
func (h *handlers) handleUndo(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req playerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	out, err := h.mgr.Undo(r.Context(), code, req.PlayerName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, undoResponse{Removed: out.Removed, State: stateResponseFromSnapshot(out.Snapshot)})
}

// handleResign implements POST /games/{code}/resign.
func (h *handlers) handleResign(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req playerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	snap, err := h.mgr.Resign(r.Context(), code, req.PlayerName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stateResponseFromSnapshot(snap))
}

func stateResponseFromSnapshot(s session.StateSnapshot) getGameStateResponse {
	return getGameStateResponse{
		BoardState: json.RawMessage(s.BoardState),
		State:      json.RawMessage(s.State),
		TurnSeat:   s.TurnSeat,
		Status:     s.Status,
		MoveSeq:    s.MoveSeq,
		WinnerSeat: s.WinnerSeat,
		EndReason:  s.EndReason,
		EndDetail:  s.EndDetail,

		DrawOfferSeat: s.DrawOfferSeat,

		TakebackSeat:  s.TakebackSeat,
		TakebackToSeq: s.TakebackToSeq,
	}
}

// maxSimulatedMoves bounds one simulate call.
const maxSimulatedMoves = 400

// takebackRequest is POST /games/{code}/takeback's body.
type takebackRequest struct {
	PlayerName string `json:"player_name"`
	// Action is "request", "cancel", "accept" or "decline".
	Action string `json:"action"`
}

// handleTakeback implements POST /games/{code}/takeback.
func (h *handlers) handleTakeback(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req takebackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	snap, err := h.mgr.Takeback(r.Context(), code, req.PlayerName, req.Action)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stateResponseFromSnapshot(snap))
}

// drawRequest is POST /games/{code}/draw's body.
type drawRequest struct {
	PlayerName string `json:"player_name"`
	// Action is "offer", "cancel", "accept" or "decline".
	Action string `json:"action"`
}

// handleDraw implements POST /games/{code}/draw.
func (h *handlers) handleDraw(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req drawRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}

	snap, err := h.mgr.Draw(r.Context(), code, req.PlayerName, req.Action)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stateResponseFromSnapshot(snap))
}

// simulateRequest is POST /games/{code}/simulate's body.
type simulateRequest struct {
	// AtSeq starts from the position after that move; omitted = current.
	AtSeq *int `json:"at_seq"`
	// Moves are applied in order, in the game's notation.
	Moves []string `json:"moves"`
}

// simulateResponse is POST /games/{code}/simulate's body.
type simulateResponse struct {
	BoardState json.RawMessage `json:"board_state"`
	State      json.RawMessage `json:"state"`
	TurnSeat   *int            `json:"turn_seat"`
	GameEnded  bool            `json:"game_ended"`
	WinnerSeat *int            `json:"winner_seat"`
	EndReason  string          `json:"end_reason,omitempty"`
	Applied    []string        `json:"applied"`
	Legal      []legalMoveDTO  `json:"legal_moves"`
	TPS        string          `json:"tps,omitempty"`
}

// handleSimulate implements POST /games/{code}/simulate: apply moves to a
// position without changing the game (an analysis board / lookahead).
func (h *handlers) handleSimulate(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req simulateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}

	if len(req.Moves) > maxSimulatedMoves {
		writeError(w, fmt.Errorf("api: at most %d moves may be simulated at once", maxSimulatedMoves))
		return
	}

	sim, err := h.mgr.Simulate(r.Context(), code, req.AtSeq, req.Moves)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, simulateResponse{
		BoardState: json.RawMessage(sim.BoardState),
		State:      json.RawMessage(sim.State),
		TurnSeat:   sim.TurnSeat,
		GameEnded:  sim.GameEnded,
		WinnerSeat: sim.WinnerSeat,
		EndReason:  sim.EndReason,
		Applied:    sim.Applied,
		Legal:      legalMoveDTOs(sim.Legal, true),
		TPS:        sim.TPS,
	})
}

// handleExport implements GET /games/{code}/export: the complete game
// record as JSON, or just the PTN text with ?format=ptn.
func (h *handlers) handleExport(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	rec, err := h.mgr.Export(r.Context(), code)
	if err != nil {
		writeError(w, err)
		return
	}
	if r.URL.Query().Get("format") == "ptn" {
		if rec.PTN == "" {
			writeError(w, errors.New("api: this game has no PTN representation"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(rec.PTN))
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// observationResponse is GET /games/{code}/observe's body.
type observationResponse struct {
	RoomCode   string          `json:"room_code"`
	GameDefID  string          `json:"game_def_id"`
	Status     string          `json:"status"`
	Seat       int             `json:"seat"`
	Color      string          `json:"color"`
	YourTurn   bool            `json:"your_turn"`
	TurnSeat   *int            `json:"turn_seat"`
	MoveSeq    int             `json:"move_seq"`
	Ply        int             `json:"ply"`
	BoardText  string          `json:"board_text"`
	TPS        string          `json:"tps,omitempty"`
	State      json.RawMessage `json:"state"`
	LegalMoves []string        `json:"legal_moves"`
	LastMove   *wire.Move      `json:"last_move"`
	Done       bool            `json:"done"`
	WinnerSeat *int            `json:"winner_seat"`
	EndReason  string          `json:"end_reason,omitempty"`
	Result     string          `json:"result,omitempty"`
	Reward     float64         `json:"reward"`

	MoveTimeSeconds int  `json:"move_time_seconds,omitempty"`
	SecondsLeft     *int `json:"seconds_left,omitempty"`
}

func observationToResponse(o session.Observation) observationResponse {
	resp := observationResponse{
		RoomCode: o.RoomCode, GameDefID: o.GameDefID, Status: o.Status, Seat: o.Seat, Color: o.Color,
		YourTurn: o.YourTurn, TurnSeat: o.TurnSeat, MoveSeq: o.MoveSeq, Ply: o.Ply,
		BoardText: o.BoardText, TPS: o.TPS, State: json.RawMessage(o.State), LegalMoves: o.LegalMoves,
		Done: o.Done, WinnerSeat: o.WinnerSeat, EndReason: o.EndReason, Result: o.Result, Reward: o.Reward,
		MoveTimeSeconds: o.MoveTimeLimit, SecondsLeft: o.SecondsLeft,
	}
	if o.LastMove != nil {
		mr := wire.MoveFromRecord(*o.LastMove)
		resp.LastMove = &mr
	}
	return resp
}

// handleObserve implements GET /games/{code}/observe?player_name=: a
// self-contained decision context for agents (see session.Observation).
func (h *handlers) handleObserve(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	obs, err := h.mgr.Observe(r.Context(), code, r.URL.Query().Get("player_name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, observationToResponse(obs))
}

// hintRequest is POST /games/{code}/hint's body.
type hintRequest struct {
	PlayerName string `json:"player_name"`
	// AtSeq and Moves pick the position: after that move (default: now),
	// then the given variation in notation.
	AtSeq *int     `json:"at_seq"`
	Moves []string `json:"moves"`
	// Bot names the bot to ask (default mcts).
	Bot string `json:"bot"`
}

// hintResponse is POST /games/{code}/hint's body.
type hintResponse struct {
	Notation string       `json:"notation"`
	Steps    []state.Step `json:"steps,omitempty"`
	TurnSeat int          `json:"turn_seat"`
	Bot      string       `json:"bot"`
	// WinRate is the bot's estimate (0..1) of the side to move winning
	// with this move, present when the bot estimates; Alternatives lists
	// the best few moves with their estimates.
	WinRate      *float64      `json:"win_rate,omitempty"`
	Alternatives []hintMoveDTO `json:"alternatives,omitempty"`
}

type hintMoveDTO struct {
	Notation string  `json:"notation"`
	WinRate  float64 `json:"win_rate"`
}

// handleHint implements POST /games/{code}/hint: a bot's suggestion, only
// available against bots (or after the game), never against a person.
func (h *handlers) handleHint(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var req hintRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if len(req.Moves) > maxSimulatedMoves {
		writeError(w, fmt.Errorf("api: at most %d moves may be simulated at once", maxSimulatedMoves))
		return
	}
	hint, err := h.mgr.Hint(r.Context(), code, req.PlayerName, req.AtSeq, req.Moves, req.Bot)
	if err != nil {
		writeError(w, err)
		return
	}
	resp := hintResponse{Notation: hint.Notation, Steps: hint.Steps, TurnSeat: hint.TurnSeat, Bot: hint.Bot}
	if hint.Estimated {
		wr := hint.WinRate
		resp.WinRate = &wr
		for _, a := range hint.Alternatives {
			resp.Alternatives = append(resp.Alternatives, hintMoveDTO{Notation: a.Notation, WinRate: a.WinRate})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// importRequest is POST /games/import's body.
type importRequest struct {
	// Text is a PTN document or a plain list of moves.
	Text string `json:"text"`
	// GameDefID is optional for PTN with a [Size] header.
	GameDefID string `json:"game_def_id"`
	Player1   string `json:"player1"`
	Player2   string `json:"player2"`
}

type importResponse struct {
	RoomCode  string    `json:"room_code"`
	GameDefID string    `json:"game_def_id"`
	Status    string    `json:"status"`
	Moves     int       `json:"moves"`
	Players   [2]string `json:"players"`
}

// handleImportGame implements POST /games/import: load a recorded game (PTN
// or a move list) into a new room with full history, for review and analysis.
func (h *handlers) handleImportGame(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	res, err := h.mgr.ImportGame(r.Context(), session.ImportOptions{
		Text: req.Text, GameDefID: strings.TrimSpace(req.GameDefID),
		Player1: req.Player1, Player2: req.Player2,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importResponse{
		RoomCode: res.RoomCode, GameDefID: res.GameDefID, Status: res.Status, Moves: res.Moves, Players: res.Players,
	})
}

// botMatchRequest is POST /bot-matches' body.
type botMatchRequest struct {
	GameDefID string `json:"game_def_id"`
	BotA      string `json:"bot_a"`
	BotB      string `json:"bot_b"`
	// DelayMS is the pause between moves (150..10000, default 700).
	DelayMS int `json:"delay_ms"`
}

// handleBotMatch implements POST /bot-matches: start a room where two
// built-in bots play each other at a watchable pace.
func (h *handlers) handleBotMatch(w http.ResponseWriter, r *http.Request) {
	var req botMatchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return
	}
	res, err := h.mgr.StartBotMatch(r.Context(), session.BotMatchOptions{
		GameDefID: strings.TrimSpace(req.GameDefID), BotA: req.BotA, BotB: req.BotB,
		Delay: time.Duration(req.DelayMS) * time.Millisecond,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"room_code": res.RoomCode, "status": res.Status})
}
