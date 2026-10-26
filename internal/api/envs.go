package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// envErrorStatus maps environment errors to HTTP statuses.
func envErrorStatus(err error) int {
	switch {
	case errors.Is(err, envs.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, envs.ErrGameOver):
		return http.StatusConflict
	case errors.Is(err, envs.ErrTooMany):
		return http.StatusTooManyRequests
	case errors.Is(err, store.ErrNotFound), errors.Is(err, session.ErrGameDefNotFound), errors.Is(err, envs.ErrUnknownGame):
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}

func writeEnvError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(envErrorStatus(err))
	_ = json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(dst); err != nil {
		writeError(w, errors.New("api: malformed JSON body"))
		return false
	}
	return true
}

// envResetRequest is POST /envs' body (the env_reset tool's arguments).
type envResetRequest struct {
	GameDefID     string          `json:"game_def_id"`
	Seat          int             `json:"seat"`
	Opponent      string          `json:"opponent"`
	Seed          int64           `json:"seed"`
	MaxPlies      int             `json:"max_plies"`
	StartTPS      string          `json:"start_tps"`
	StartState    json.RawMessage `json:"start_state"`
	StartSeat     int             `json:"start_seat"`
	PuzzleID      string          `json:"puzzle_id"`
	BotIterations int             `json:"bot_iterations"`
	Include       []string        `json:"include"`
	PlaneDepth    int             `json:"plane_depth"`
}

// handleEnvReset implements POST /envs: start an episode.
func (h *handlers) handleEnvReset(w http.ResponseWriter, r *http.Request) {
	var req envResetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	opts := envs.ResetOptions{
		GameDefID: strings.TrimSpace(req.GameDefID), Seat: req.Seat, Opponent: strings.TrimSpace(req.Opponent),
		Seed: req.Seed, MaxPlies: req.MaxPlies, StartTPS: strings.TrimSpace(req.StartTPS),
		StartState: string(req.StartState), StartSeat: req.StartSeat,
		BotIterations: req.BotIterations, PlaneDepth: req.PlaneDepth,
	}
	if opts.Opponent == "" {
		opts.Opponent = "random"
	}
	for _, inc := range req.Include {
		switch strings.ToLower(strings.TrimSpace(inc)) {
		case "planes":
			opts.Planes = true
		case "action_ids", "action-ids":
			opts.ActionIDs = true
		default:
			writeError(w, errors.New(`api: unknown include (use "planes" or "action_ids")`))
			return
		}
	}
	if pid := strings.TrimSpace(req.PuzzleID); pid != "" {
		if err := envs.ApplyPuzzle(&opts, pid); err != nil {
			writeError(w, err)
			return
		}
	}
	if opts.GameDefID == "" {
		writeError(w, errors.New("api: game_def_id (or puzzle_id) is required"))
		return
	}
	obs, err := h.envs.Reset(r.Context(), opts)
	if err != nil {
		writeEnvError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obs)
}

func (h *handlers) handleEnvList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"env_ids": h.envs.List()})
}

func (h *handlers) handleEnvObserve(w http.ResponseWriter, r *http.Request) {
	obs, err := h.envs.Observe(chi.URLParam(r, "id"))
	if err != nil {
		writeEnvError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obs)
}

// envStepRequest is POST /envs/{id}/step's body: notation or a fixed action ID.
type envStepRequest struct {
	Move     string `json:"move"`
	ActionID *int   `json:"action_id"`
}

func (h *handlers) handleEnvStep(w http.ResponseWriter, r *http.Request) {
	var req envStepRequest
	if !decodeBody(w, r, &req) {
		return
	}
	id := chi.URLParam(r, "id")
	var obs envs.Observation
	var err error
	switch {
	case strings.TrimSpace(req.Move) != "":
		obs, err = h.envs.Step(r.Context(), id, strings.TrimSpace(req.Move))
	case req.ActionID != nil:
		obs, err = h.envs.StepAction(r.Context(), id, *req.ActionID)
	default:
		writeError(w, errors.New("api: move or action_id is required"))
		return
	}
	if err != nil {
		writeEnvError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obs)
}

func (h *handlers) handleEnvFork(w http.ResponseWriter, r *http.Request) {
	obs, err := h.envs.Fork(chi.URLParam(r, "id"))
	if err != nil {
		writeEnvError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obs)
}

func (h *handlers) handleEnvClose(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.envs.Close(id)
	writeJSON(w, http.StatusOK, map[string]any{"closed": id})
}

// handleEnvBatchStep implements POST /envs/batch-step: {"steps": [{env_id,
// move | action_id}, ...]}.
func (h *handlers) handleEnvBatchStep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Steps []envs.StepRequest `json:"steps"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Steps) == 0 || len(req.Steps) > envs.MaxBatch {
		writeError(w, errors.New("api: steps must hold between 1 and "+strconv.Itoa(envs.MaxBatch)+" entries"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": h.envs.StepMany(r.Context(), req.Steps)})
}

// handleEnvSpec implements GET /env-specs/{game}?include_actions=1&plane_depth=N.
func (h *handlers) handleEnvSpec(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	depth, _ := strconv.Atoi(q.Get("plane_depth"))
	spec, err := h.envs.Spec(chi.URLParam(r, "game"), q.Get("include_actions") == "1", depth)
	if err != nil {
		writeEnvError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

// matchRequest is POST /matches' body (the match_bots tool's arguments).
type matchRequest struct {
	GameDefID     string `json:"game_def_id"`
	BotA          string `json:"bot_a"`
	BotB          string `json:"bot_b"`
	Games         int    `json:"games"`
	Seed          int64  `json:"seed"`
	MaxPlies      int    `json:"max_plies"`
	IncludeMoves  bool   `json:"include_moves"`
	BotIterations int    `json:"bot_iterations"`
	Record        bool   `json:"record"`
	RecordPlanes  bool   `json:"record_planes"`
	PlaneDepth    int    `json:"plane_depth"`
}

func (h *handlers) handleMatch(w http.ResponseWriter, r *http.Request) {
	var req matchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Games <= 0 {
		req.Games = 20
	}
	if req.Games > 500 {
		req.Games = 500
	}
	ctx, cancel := context.WithTimeout(r.Context(), envs.DefaultMatchBudget)
	defer cancel()
	res, err := h.envs.PlayMatch(ctx, envs.MatchOptions{
		GameDefID: req.GameDefID, BotA: req.BotA, BotB: req.BotB, Games: req.Games, Seed: req.Seed,
		MaxPlies: req.MaxPlies, KeepMoves: req.IncludeMoves, BotIterations: req.BotIterations,
		Record: req.Record || req.RecordPlanes, RecordPlanes: req.RecordPlanes, PlaneDepth: req.PlaneDepth,
	})
	if err != nil {
		writeEnvError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
