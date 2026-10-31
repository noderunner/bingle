package auth

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238's default, which every authenticator app speaks
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238) as every authenticator app reads it: HMAC-SHA1, six
// digits, thirty-second steps.
const (
	totpDigits = 6
	totpStep   = 30
	// totpSkew is how many steps either side of now a code is accepted in,
	// for a phone whose clock is a little off.
	totpSkew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret is a fresh secret, as the base32 an app is given.
func NewTOTPSecret() string { return b32.EncodeToString(randomBytes(20)) }

// hotp is RFC 4226's code for counter.
func hotp(key []byte, counter uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, v%mod)
}

// TOTPCode is the code for key at t with the given number of digits (tests
// read RFC 6238's eight-digit vectors through it).
func TOTPCode(key []byte, t time.Time, digits int) string {
	return hotp(key, uint64(t.Unix()/totpStep), digits)
}

// CheckTOTP reports whether code is secret's code at now, give or take a
// step, and which step it was. A step at or before last has been used and
// is refused, so a code seen over a shoulder cannot be replayed.
func CheckTOTP(secret, code string, now time.Time, last int64) (int64, bool) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return 0, false
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	step := now.Unix() / totpStep
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		c := step + d
		if c <= last || c < 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(key, uint64(c), totpDigits)), []byte(code)) == 1 {
			return c, true
		}
	}
	return 0, false
}

// TOTPURI is the otpauth:// link an authenticator app reads (typed in, or
// from a QR code made of it).
func TOTPURI(handle, secret string) string {
	label := url.PathEscape("Bingle:@" + handle)
	q := url.Values{"secret": {secret}, "issuer": {"Bingle"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Recovery codes: ten, each two groups of five from an alphabet without the
// look-alikes, about fifty bits apiece.
const recoveryCodes = 10

// NewRecoveryCodes returns ten fresh codes.
func NewRecoveryCodes() []string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	limit := byte(256 - 256%len(alphabet))
	out := make([]string, 0, recoveryCodes)
	for len(out) < recoveryCodes {
		var b []byte
		for len(b) < 10 {
			for _, c := range randomBytes(16) {
				if c < limit && len(b) < 10 {
					b = append(b, alphabet[int(c)%len(alphabet)])
				}
			}
		}
		out = append(out, string(b[:5])+"-"+string(b[5:]))
	}
	return out
}

// canonRecovery is a recovery code as it is hashed: lower case, no spaces,
// with its dash.
func canonRecovery(code string) string {
	c := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if len(c) == 10 && !strings.Contains(c, "-") {
		c = c[:5] + "-" + c[5:]
	}
	return c
}
