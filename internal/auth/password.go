package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the cost of a password hash. They are stored with every
// hash, so raising them later leaves the old hashes readable, and a password
// hashed with weaker ones is rehashed at its next sign-in.
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
}

// DefaultArgon2 is OWASP's recommendation for Argon2id (19 MiB, two passes,
// one lane): about 50–100 ms on a small server, and little enough memory that
// a handful of sign-ins at once do not starve the games.
var DefaultArgon2 = Argon2Params{Memory: 19 * 1024, Time: 2, Threads: 1}

const (
	saltLen = 16
	keyLen  = 32
)

// MinPasswordLen and MaxPasswordLen bound a password. The upper bound stops a
// megabyte "password" from being hashed; nothing else about it is checked.
const (
	MinPasswordLen = 10
	MaxPasswordLen = 256
)

// ErrWeakPassword is the answer to a password outside the bounds.
var ErrWeakPassword = fmt.Errorf("auth: a password is %d to %d characters", MinPasswordLen, MaxPasswordLen)

// CheckPassword reports whether a new password may be used.
func CheckPassword(pw string) error {
	if n := len([]rune(pw)); n < MinPasswordLen || len(pw) > MaxPasswordLen {
		return ErrWeakPassword
	}
	return nil
}

// HashPassword returns pw's Argon2id hash in the PHC string format:
// $argon2id$v=19$m=…,t=…,p=…$salt$hash.
func HashPassword(pw string, p Argon2Params) string {
	salt := randomBytes(saltLen)
	sum := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, keyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Time, p.Threads, b64.EncodeToString(salt), b64.EncodeToString(sum))
}

var errBadHash = errors.New("auth: not an argon2id hash")

// parseHash reads a hash written by HashPassword.
func parseHash(h string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(h, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, errBadHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return Argon2Params{}, nil, nil, errBadHash
	}
	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil || p.Memory == 0 || p.Time == 0 || p.Threads == 0 {
		return Argon2Params{}, nil, nil, errBadHash
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	sum, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(sum) == 0 {
		return Argon2Params{}, nil, nil, errBadHash
	}
	return p, salt, sum, nil
}

// VerifyPassword reports whether pw is the password h was made from, and
// whether h should be remade with params (it was made with others).
func VerifyPassword(pw, h string, params Argon2Params) (ok, rehash bool) {
	p, salt, sum, err := parseHash(h)
	if err != nil {
		return false, false
	}
	got := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, uint32(len(sum)))
	if subtle.ConstantTimeCompare(got, sum) != 1 {
		return false, false
	}
	return true, p != params
}

// RandomPassword is a password for an account somebody else will sign in to
// first (and must change): sixteen characters from an alphabet without the
// look-alikes.
func RandomPassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// Bytes past the last whole multiple of the alphabet are skipped, so
	// every character is equally likely.
	limit := byte(256 - 256%len(alphabet))
	out := make([]byte, 0, 16)
	for len(out) < 16 {
		for _, c := range randomBytes(32) {
			if c < limit && len(out) < 16 {
				out = append(out, alphabet[int(c)%len(alphabet)])
			}
		}
	}
	return string(out)
}
