package auth

import (
	"errors"
	"regexp"
	"strings"
)

// handleRe is what a handle is made of: 3–24 of a–z, 0–9, "_" and "-".
var handleRe = regexp.MustCompile(`^[a-z0-9_-]{3,24}$`)

// reserved are handles nobody can take: they would read as the server's own
// voice.
var reserved = map[string]bool{
	"admin": true, "administrator": true, "bingle": true, "system": true, "support": true,
	"root": true, "moderator": true, "mod": true, "staff": true, "help": true, "official": true,
	"guest": true, "anonymous": true, "nobody": true, "everyone": true, "here": true,
}

// ErrBadHandle is the answer to a handle that is not one.
var ErrBadHandle = errors.New("auth: a handle is 3 to 24 letters, digits, '_' or '-'")

// ErrReservedHandle is the answer to a handle nobody may have.
var ErrReservedHandle = errors.New("auth: that handle is reserved")

// CanonHandle returns the handle in its stored form (lower case, without a
// leading "@"), or why it cannot be one. allowReserved lets the seeded
// administrator be "admin".
func CanonHandle(h string, allowReserved bool) (string, error) {
	h = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(h), "@"))
	if !handleRe.MatchString(h) {
		return "", ErrBadHandle
	}
	if reserved[h] && !allowReserved {
		return "", ErrReservedHandle
	}
	return h, nil
}

// HandleOf is a name as a handle would be written ("@Ana" is "ana"), and
// whether it could be one at all.
func HandleOf(name string) (string, bool) {
	h := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "@"))
	return h, handleRe.MatchString(h)
}

// IsReserved reports whether a handle is one nobody may have.
func IsReserved(h string) bool { return reserved[h] }
