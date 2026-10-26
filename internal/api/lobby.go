package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/session"
)

// configResponse is GET /config: what the server knows about how it is
// reached from outside.
type configResponse struct {
	// PublicURL is the base URL other players use to reach this server
	// (BINGLE_PUBLIC_URL); empty when it is not configured.
	PublicURL string `json:"public_url"`
}

// handleConfig implements GET /config.
func (h *handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, configResponse{PublicURL: h.publicURL})
}

// inviteURL is the link that brings a person to the game: the configured
// public URL (or, failing that, the address this request came to) plus the
// game's page.
func (h *handlers) inviteURL(r *http.Request, code string) string {
	base := h.publicURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		host := r.Host
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
			host = fh
		}
		if host == "" {
			return ""
		}
		base = scheme + "://" + host
	}
	return strings.TrimRight(base, "/") + "/game/" + strings.ToUpper(code)
}

// nameBody is the request body of the actions that only name the player.
type nameBody struct {
	PlayerName string `json:"player_name"`
	Seat       int    `json:"seat,omitempty"`
}

// handleStartGame implements POST /games/{code}/start: the host begins a
// game that waits in its lobby.
func (h *handlers) handleStartGame(w http.ResponseWriter, r *http.Request) {
	var req nameBody
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	snap, err := h.mgr.StartGame(r.Context(), chi.URLParam(r, "code"), req.PlayerName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusFromSnapshot(snap))
}

// handleChooseSeat implements POST /games/{code}/seat: a player moves to an
// open seat before the game starts.
func (h *handlers) handleChooseSeat(w http.ResponseWriter, r *http.Request) {
	var req nameBody
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if req.Seat < 1 {
		writeError(w, errors.New("api: seat is required"))
		return
	}
	snap, err := h.mgr.ChooseSeat(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Seat)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusFromSnapshot(snap))
}

// cultBody is POST /games/{code}/seat-cult's body.
type cultBody struct {
	PlayerName string `json:"player_name"`
	Seat       int    `json:"seat"`
	// Cult is the key of the cult the seat should play (gc, cc, bg, ys, ww, sl).
	Cult string `json:"cult"`
}

// handleSetSeatCult implements POST /games/{code}/seat-cult: before the game
// starts, anyone at the table changes the cult of an open seat.
func (h *handlers) handleSetSeatCult(w http.ResponseWriter, r *http.Request) {
	var req cultBody
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if req.Seat < 1 || strings.TrimSpace(req.Cult) == "" {
		writeError(w, errors.New("api: seat and cult are required"))
		return
	}
	snap, err := h.mgr.SetSeatCult(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Seat, req.Cult)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusFromSnapshot(snap))
}

// handleLeave implements POST /games/{code}/leave: a player gives up their
// seat before the game starts.
func (h *handlers) handleLeave(w http.ResponseWriter, r *http.Request) {
	var req nameBody
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	snap, err := h.mgr.LeaveSeat(r.Context(), chi.URLParam(r, "code"), req.PlayerName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusFromSnapshot(snap))
}

// handleRemoveSeat implements POST /games/{code}/remove: the host clears
// another seat before the game starts.
func (h *handlers) handleRemoveSeat(w http.ResponseWriter, r *http.Request) {
	var req nameBody
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if req.Seat < 1 {
		writeError(w, errors.New("api: seat is required"))
		return
	}
	snap, err := h.mgr.RemoveSeat(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Seat)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusFromSnapshot(snap))
}

// gameStatusResponse is the small answer of the lobby actions.
type gameStatusResponse struct {
	Status   string `json:"status"`
	Lobby    bool   `json:"lobby"`
	HostSeat int    `json:"host_seat,omitempty"`
	MoveSeq  int    `json:"move_seq"`
	TurnSeat *int   `json:"turn_seat"`
}

func statusFromSnapshot(snap session.StateSnapshot) gameStatusResponse {
	return gameStatusResponse{Status: snap.Status, Lobby: snap.Lobby, HostSeat: snap.HostSeat, MoveSeq: snap.MoveSeq, TurnSeat: snap.TurnSeat}
}

// --- reactions ------------------------------------------------------------------

// handleGetReactions implements GET /games/{code}/reactions?player_name=: the
// reaction abilities the player holds, the occasions on which they want to be
// asked about them (the rest are declined for them) and the seats that choice
// may name.
func (h *handlers) handleGetReactions(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("player_name")
	if err := checkName(name); err != nil {
		writeError(w, err)
		return
	}
	set, err := h.mgr.ReactionSettings(r.Context(), chi.URLParam(r, "code"), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// reactionsRequest is PUT /games/{code}/reactions' body: the occasions on
// which the player wants to be asked. Omitted fields keep their value.
type reactionsRequest struct {
	PlayerName string `json:"player_name"`
	// Targeted: when an action is aimed at the player; AskAll: after any other
	// player's action; AskSeats: after the actions of those seats.
	Targeted *bool  `json:"targeted"`
	AskAll   *bool  `json:"ask_all"`
	AskSeats *[]int `json:"ask_seats"`
	// Ability is the id of the reaction the choice is for (as listed in the
	// abilities of GET); without it the choice is made for every reaction.
	Ability string `json:"ability"`

	// All and Seats are the earlier choice, of which reactions to *pass*
	// (everything / those seats' actions): still understood, as the
	// equivalent of the above, when none of the above is given.
	All   *bool  `json:"all"`
	Seats *[]int `json:"seats"`
}

// handleSetReactions implements PUT /games/{code}/reactions.
func (h *handlers) handleSetReactions(w http.ResponseWriter, r *http.Request) {
	var req reactionsRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	var set session.ReactionSettings
	var err error
	if req.Targeted == nil && req.AskAll == nil && req.AskSeats == nil && (req.All != nil || req.Seats != nil) {
		all, seats := false, []int{}
		if req.All != nil {
			all = *req.All
		}
		if req.Seats != nil {
			seats = *req.Seats
		}
		set, err = h.mgr.SetReactionsLegacy(r.Context(), chi.URLParam(r, "code"), req.PlayerName, all, seats)
	} else {
		set, err = h.mgr.SetReactions(r.Context(), chi.URLParam(r, "code"), req.PlayerName,
			session.ReactionUpdate{Ability: req.Ability, Targeted: req.Targeted, AskAll: req.AskAll, AskSeats: req.AskSeats})
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// --- auto-play timeout ---------------------------------------------------------------

// handleGetAutoPlay implements GET /games/{code}/autoplay?player_name=: the
// person's Auto-Play Timeout.
func (h *handlers) handleGetAutoPlay(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("player_name")
	if err := checkName(name); err != nil {
		writeError(w, err)
		return
	}
	set, err := h.mgr.AutoPlaySettings(r.Context(), chi.URLParam(r, "code"), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// autoPlayRequest is PUT /games/{code}/autoplay's body. Omitted fields keep
// their value.
type autoPlayRequest struct {
	PlayerName string `json:"player_name"`
	Enabled    *bool  `json:"enabled"`
	// TimeoutSeconds is the wait before a bot plays (a minute to a year).
	TimeoutSeconds *int `json:"timeout_seconds"`
}

// handleSetAutoPlay implements PUT /games/{code}/autoplay.
func (h *handlers) handleSetAutoPlay(w http.ResponseWriter, r *http.Request) {
	var req autoPlayRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	set, err := h.mgr.SetAutoPlay(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Enabled, req.TimeoutSeconds)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// --- chat ----------------------------------------------------------------------------

// handleGetChat implements GET /games/{code}/chat?after=ID&limit=N: the
// latest messages, or those after message ID.
func (h *handlers) handleGetChat(w http.ResponseWriter, r *http.Request) {
	after, limit, err := chatPaging(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lines, err := h.mgr.ChatLog(r.Context(), chi.URLParam(r, "code"), after, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

// chatPaging reads the after and limit query parameters of a chat request.
func chatPaging(r *http.Request) (after int64, limit int, err error) {
	if raw := r.URL.Query().Get("after"); raw != "" {
		n, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || n < 0 {
			return 0, 0, errors.New("api: after must be a message id")
		}
		after = n
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n < 1 {
			return 0, 0, errors.New("api: limit must be a positive integer")
		}
		limit = n
	}
	if limit > 200 {
		limit = 200
	}
	return after, limit, nil
}

// chatRequest is POST /games/{code}/chat's body.
type chatRequest struct {
	PlayerName string `json:"player_name"`
	Text       string `json:"text"`
}

// handlePostChat implements POST /games/{code}/chat: say something at the
// table, as a player or a spectator.
func (h *handlers) handlePostChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	line, err := h.mgr.SendChat(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, line)
}

// handleGetGlobalChat implements GET /chat?after=ID&limit=N: the latest
// messages of the global chat room, or those after message ID.
func (h *handlers) handleGetGlobalChat(w http.ResponseWriter, r *http.Request) {
	after, limit, err := chatPaging(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lines, err := h.mgr.GlobalChatLog(r.Context(), after, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

// handlePostGlobalChat implements POST /chat: say something in the global chat.
func (h *handlers) handlePostGlobalChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	line, err := h.mgr.SendGlobalChat(r.Context(), req.PlayerName, req.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, line)
}

// presenceRequest is POST /games/{code}/presence's body.
type presenceRequest struct {
	PlayerName string `json:"player_name"`
	Present    bool   `json:"present"`
}

// handleGetPresence implements GET /games/{code}/presence: the seats whose
// players say they are watching the game right now.
func (h *handlers) handleGetPresence(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]int{"seats": h.mgr.Present(chi.URLParam(r, "code"))})
}

// handlePostPresence implements POST /games/{code}/presence: a player marks
// themselves present (or away). A present mark lapses after a couple of
// minutes unless it is renewed by posting it again.
func (h *handlers) handlePostPresence(w http.ResponseWriter, r *http.Request) {
	var req presenceRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := checkName(req.PlayerName); err != nil {
		writeError(w, err)
		return
	}
	if err := h.mgr.SetPresent(r.Context(), chi.URLParam(r, "code"), req.PlayerName, req.Present); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]int{"seats": h.mgr.Present(chi.URLParam(r, "code"))})
}
