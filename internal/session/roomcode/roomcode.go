// Package roomcode generates short, human-typeable room codes, per
// docs/history/plan.md section 5 step 10: it is deliberately its own subpackage of
// internal/session (not a flat session.New()) since it is a self-contained
// utility with no dependency on the rest of session, and a flat
// session.New() would read ambiguously next to session.NewManager.
//
// Collision-retry against internal/store belongs to the call site
// (session.Manager.CreateGame), not to this package —
// roomcode only knows how to draw one random code.
package roomcode

import (
	"crypto/rand"
	"math/big"
)

// Alphabet is the unambiguous character set room codes are drawn from:
// digits and uppercase letters with the easily-confused characters 0, O, 1,
// I, and L removed.
const Alphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// codeLength is the number of characters New returns.
const codeLength = 6

// New returns a 6-character room code drawn uniformly at random from
// Alphabet, using crypto/rand for each character selection.
func New() string {
	code := make([]byte, codeLength)
	alphabetLen := big.NewInt(int64(len(Alphabet)))
	for i := range code {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			// crypto/rand.Reader failing is a fatal environment problem
			// (no entropy source available); there is no sane fallback
			// that preserves this package's uniform-randomness contract.
			panic("roomcode: crypto/rand unavailable: " + err.Error())
		}
		code[i] = Alphabet[n.Int64()]
	}
	return string(code)
}
