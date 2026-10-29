// Package repro writes down everything needed to reproduce one impossible
// thing, and reproduces it.
//
// Game bugs are diagnosed by replaying the position, not by reading logs.
// Stage 7 of docs/history/observability-plan.md calls this the plan's
// highest-value piece, and the reason is the shape of the problem: a ruleset
// that goes wrong does so for one position, reached by one path, and a log
// line can describe that but not hand it back.
//
// A bundle is **full fidelity by definition** — it carries the whole position,
// including committed dice, hidden hands and the random state, because
// anything less may not reproduce. That is exactly why it goes to a directory
// of its own (BINGLE_DEBUG_DIR) and never to the log stream, which may be
// shipped somewhere else (ADR 0063).
//
// Nothing writes a bundle unless something impossible has happened: Apply
// refusing a move Legal listed, a ruleset panicking, a position that no longer
// validates, notation that will not round-trip, a public view that is the
// whole position, or a batch game ending for one of the two reasons that mean
// the rules have a hole in them.
package repro

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Kind says what went wrong. It is part of a bundle's filename, so it is a
// short closed set rather than a sentence.
type Kind string

// The impossible things.
const (
	// ApplyRejectedALegalMove: Legal listed the move and Apply refused it.
	// This is the one that makes a game unplayable in a way nobody can
	// reproduce from a report.
	ApplyRejectedALegalMove Kind = "apply-rejected-a-legal-move"
	// LegalDidNotListAnAppliedMove: Apply accepted a move Legal left out
	// (paranoid mode's first check).
	LegalDidNotListAnAppliedMove Kind = "legal-did-not-list-an-applied-move"
	// RulesetPanicked: a ruleset panicked inside Legal or Apply.
	RulesetPanicked Kind = "ruleset-panicked"
	// ValidateFailed: the position no longer validates after a move.
	ValidateFailed Kind = "validate-failed"
	// NotationDidNotRoundTrip: Format -> Parse -> Format changed the move, so
	// a replay of this game will not reproduce it.
	NotationDidNotRoundTrip Kind = "notation-did-not-round-trip"
	// PublicViewLeaked: a ruleset's public view is the whole position.
	PublicViewLeaked Kind = "public-view-leaked"
	// BatchIllegalMove: a batch game ended because Apply refused a move the
	// bot took from Legal's own list.
	BatchIllegalMove Kind = "batch-illegal-move"
	// BatchNoLegalMoves: a batch game ran out of moves with no win rule
	// ending it, which means the rules have a hole.
	BatchNoLegalMoves Kind = "batch-no-legal-moves"
)

// Bundle is one reproduction, as it is written to disk.
type Bundle struct {
	// Bingle is the engine's version, because a bundle outlives the build
	// that wrote it.
	Bingle  string    `json:"bingle"`
	ID      string    `json:"id"`
	Kind    Kind      `json:"kind"`
	Written time.Time `json:"written"`

	// GameDefID is the compiled game, variants and all
	// ("courtfall~arena").
	GameDefID string `json:"game_def_id"`
	// Seed is the game's seed where it has one, for a ruleset whose opening
	// position is drawn.
	Seed uint64 `json:"seed,omitempty"`

	// StartState is the game's opening position document, and Moves the
	// notations played from it. Both are optional: they say how the position
	// was reached, which is often the actual bug, and a replay reports a
	// divergence between replaying them and Position as a finding of its own.
	StartState json.RawMessage `json:"start_state,omitempty"`
	Moves      []string        `json:"moves,omitempty"`

	// Position is the position immediately before the thing that went wrong,
	// in full. This is what makes a bundle reproduce rather than approximate.
	Position json.RawMessage `json:"position"`
	// Seat is whose decision it was.
	Seat int `json:"seat"`
	// Move and Notation are the decision, where there was one.
	Move     json.RawMessage `json:"move,omitempty"`
	Notation string          `json:"notation,omitempty"`

	// Error is what the server saw, as text.
	Error string `json:"error,omitempty"`
	// Board is state.RenderText of the position: a bundle a person can read
	// before they run anything.
	Board string `json:"board,omitempty"`
	// Note is anything else worth saying, such as which paranoid check failed.
	Note string `json:"note,omitempty"`
}

// NewID is a bundle's identifier: short, random, and safe in a filename. It
// goes in the ERROR line as well, so a log and a directory can be matched up.
func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return hex.EncodeToString(b[:])
}

// Filename is what a bundle is stored as: sortable by time, searchable by
// kind, unique by id.
func (b Bundle) Filename() string {
	at := b.Written
	if at.IsZero() {
		at = time.Now()
	}
	id := b.ID
	if id == "" {
		id = NewID()
	}
	kind := b.Kind
	if kind == "" {
		kind = "unknown"
	}
	return fmt.Sprintf("%s-%s-%s.json", at.UTC().Format("20060102T150405Z"), kind, id)
}
