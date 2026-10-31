package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// statusForError maps a Manager error to the HTTP status code that should
// be returned for it, per docs/history/plan.md section 6.8. The default case covers
// both ApplyMove's illegal-move rejections (a deliberate 400) and any truly
// unexpected internal error — see plan.md 6.8's "Known, deliberate v1
// simplification" note.
func statusForError(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrBatchNotFound), errors.Is(err, session.ErrGameDefNotFound), errors.Is(err, session.ErrAssetNotFound), errors.Is(err, session.ErrNotifyOff),
		errors.Is(err, session.ErrNoSuchPerson):
		return http.StatusNotFound
	case errors.Is(err, session.ErrSeatsFull), errors.Is(err, session.ErrGameNotActive), errors.Is(err, session.ErrNameIsAHandle), errors.Is(err, session.ErrNameIsABot),
		errors.Is(err, session.ErrAlreadyStarted), errors.Is(err, session.ErrSeatsOpen), errors.Is(err, session.ErrSeatTaken), errors.Is(err, session.ErrNoCultChoice),
		errors.Is(err, session.ErrNothingToUndo), errors.Is(err, session.ErrCannotUndo),
		errors.Is(err, session.ErrDrawNotAllowed), errors.Is(err, session.ErrTakebackNotAllowed), errors.Is(err, session.ErrHintNotAllowed), errors.Is(err, session.ErrTooManyBotMatches):
		return http.StatusConflict
	case errors.Is(err, session.ErrNotYourTurn), errors.Is(err, session.ErrNotAPlayer), errors.Is(err, session.ErrNotHost), errors.Is(err, session.ErrNotSeated), errors.Is(err, session.ErrVoiceNotJoined), errors.Is(err, session.ErrBadToken), errors.Is(err, session.ErrNotYourSeat), errors.Is(err, session.ErrNoPrincipal),
		errors.Is(err, session.ErrNotHere):
		return http.StatusForbidden
	case errors.Is(err, session.ErrVoicePeerGone), errors.Is(err, session.ErrSuppressed), errors.Is(err, session.ErrNotVerified), errors.Is(err, session.ErrNotInCall), errors.Is(err, session.ErrTooManyHere):
		return http.StatusConflict
	case errors.Is(err, session.ErrChatTooFast), errors.Is(err, session.ErrSignalTooFast), errors.Is(err, session.ErrNotifyLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, session.ErrRulesetPanic):
		// A ruleset that panicked is the server's fault, not the client's:
		// the move may well have been perfectly legal. A repro bundle will
		// have been written if BINGLE_DEBUG_DIR is set.
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

// errBadSecret is reported for a missing or overlong browser secret (POST
// /here and the direct messages that build on it).
var errBadSecret = errors.New("api: secret is required, and at most " + strconv.Itoa(maxSecretRunes) + " characters")

// maxBodyBytes caps JSON request bodies; every real request is tiny.
const maxBodyBytes = 1 << 20

// maxNameRunes caps player names.
const maxNameRunes = 40

// checkName validates a player name: non-blank, reasonably short, and free
// of control characters. It returns the error to report, or nil.
func checkName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("api: player_name is required")
	}
	if utf8.RuneCountInString(trimmed) > maxNameRunes {
		return fmt.Errorf("api: player_name must be at most %d characters", maxNameRunes)
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return errors.New("api: player_name must not contain control characters")
		}
	}
	return nil
}

// errorResponse is the JSON shape of every error response: {"error": "<message>"}.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError writes err to w as a JSON error body, with the status mapped
// by statusForError.
func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusForError(err))
	_ = json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
}
