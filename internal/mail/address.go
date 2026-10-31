// Package mail sends bingle's email notifications. It is deliberately narrow:
// a few fixed templates, one recipient per message, addresses checked strictly,
// and TLS always (a server that cannot do it gets no mail and no password).
// Nothing here takes free-form content from a player.
package mail

import (
	"errors"
	stdmail "net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxAddressLen is the longest address accepted (RFC 5321's path limit).
const MaxAddressLen = 254

// ValidAddress checks that s is one plain address ("name@example.com", with no
// display name, quotes, comments, lists or control characters) and returns it
// with the domain in lower case.
func ValidAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > MaxAddressLen {
		return "", errBadAddress
	}
	for _, r := range s {
		if r > unicode.MaxASCII || unicode.IsControl(r) || unicode.IsSpace(r) || strings.ContainsRune(`,;<>()[]\":`, r) {
			return "", errBadAddress
		}
	}
	a, err := stdmail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" {
		return "", errBadAddress
	}
	at := strings.LastIndexByte(s, '@')
	local, domain := s[:at], strings.ToLower(s[at+1:])
	if local == "" || len(local) > 64 || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", errBadAddress
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", errBadAddress
		}
	}
	return local + "@" + domain, nil
}

type addressError string

func (e addressError) Error() string { return string(e) }

const errBadAddress = addressError("that is not an email address")

// IsAddressError reports whether err is ValidAddress refusing an address.
func IsAddressError(err error) bool {
	var ae addressError
	return errors.As(err, &ae)
}

// Mask hides most of an address for display: the first character and the
// domain are all that is left ("m••••@gmail.com"). The result cannot be used
// to rebuild the address.
func Mask(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at < 1 {
		return "••••"
	}
	r, _ := utf8.DecodeRuneInString(addr)
	return string(r) + "••••" + addr[at:]
}

// Sanitize makes a value safe to put in a message: letters, digits, spaces and a
// few punctuation marks only, whitespace collapsed, at most max characters.
func Sanitize(s string, max int) string {
	var b strings.Builder
	space := false
	n := 0
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(".-_',:!()&·×", r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
				n++
			}
			space = false
			if n >= max {
				return strings.TrimSpace(b.String())
			}
			b.WriteRune(r)
			n++
		case unicode.IsSpace(r):
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
