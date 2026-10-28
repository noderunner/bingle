// Package logging builds bingle's logger and holds the rules about what a log
// line may carry.
//
// The server logs through log/slog: one logger is built in main from
// BINGLE_LOG_LEVEL and BINGLE_LOG_FORMAT and handed to everything that logs.
// Nothing in bingle logs a credential, an address or anything that would let
// a reader of the log join a game: the helpers below (Room, Addr, Name) are
// the only way those things reach a line, and docs/decisions/0063 says why.
package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"strings"
)

// Format names for New.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// New returns a logger writing to w at level: lines of key=value text, or one
// JSON object per line when format is FormatJSON (for a log collector).
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == FormatJSON {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

// Room is the attribute for a room code. A room code is the only thing between
// a stranger and a game, so a log line gets the first two characters and a
// short hash of the whole code instead: enough to follow one room through a
// log or to match it against another line, and not enough to join it.
func Room(code string) slog.Attr { return slog.String("room", RoomRef(code)) }

// RoomRef is Room's value, for the rare message that has to carry a room
// inside its text rather than as an attribute.
func RoomRef(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	h := hash(code)
	if len(code) < 2 {
		return h
	}
	return code[:2] + "-" + h
}

// Addr is the attribute for an email address, hashed. It is never logged in
// full at any level: internal/session/notify.go hashes addresses for the same
// reason, and this is the same hash of the same lower-cased text.
func Addr(address string) slog.Attr {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return slog.String("addr", "")
	}
	return slog.String("addr", hash(address))
}

// Name is the attribute for a player's name, hashed. Most lines are better
// without a name at all; this is for the ones that would say nothing useful
// without knowing which player they are about.
func Name(name string) slog.Attr {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return slog.String("player", "")
	}
	return slog.String("player", hash(name))
}

// hash is the short form every helper above shares: twelve hex characters of
// the SHA-256 of the text, which is far too little to reverse by hand and
// still enough that two lines about the same thing match.
func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}
