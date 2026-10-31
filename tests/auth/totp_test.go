package auth_test

import (
	"encoding/base32"
	"regexp"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
)

// RFC 6238, appendix B, for HMAC-SHA1 (the eight-digit codes).
func TestTOTPMatchesTheRFC(t *testing.T) {
	key := []byte("12345678901234567890")
	for unix, want := range map[int64]string{
		59: "94287082", 1111111109: "07081804", 1111111111: "14050471",
		1234567890: "89005924", 2000000000: "69279037", 20000000000: "65353130",
	} {
		if got := auth.TOTPCode(key, time.Unix(unix, 0), 8); got != want {
			t.Errorf("T=%d: %s, want %s", unix, got, want)
		}
	}
}

func TestTOTPCheckAcceptsANeighbouringStepOnceOnly(t *testing.T) {
	secret := auth.NewTOTPSecret()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) != 20 {
		t.Fatalf("secret %q: %v", secret, err)
	}
	now := time.Unix(1_700_000_000, 0)
	code := auth.TOTPCode(key, now, 6)
	step, ok := auth.CheckTOTP(secret, code, now, 0)
	if !ok {
		t.Fatal("the code of now is refused")
	}
	if _, ok := auth.CheckTOTP(secret, code, now, step); ok {
		t.Error("a code was accepted twice")
	}
	if _, ok := auth.CheckTOTP(secret, auth.TOTPCode(key, now.Add(-30*time.Second), 6), now, 0); !ok {
		t.Error("the previous step's code is refused (a phone a little slow)")
	}
	if _, ok := auth.CheckTOTP(secret, auth.TOTPCode(key, now.Add(-90*time.Second), 6), now, 0); ok {
		t.Error("a code a minute and a half old was accepted")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := auth.CheckTOTP(secret, bad, now, 0); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
	if uri := auth.TOTPURI("ana", secret); !regexp.MustCompile(`^otpauth://totp/Bingle:@ana\?.*secret=` + secret).MatchString(uri) {
		t.Errorf("uri %q", uri)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes := auth.NewRecoveryCodes()
	seen := map[string]bool{}
	for _, c := range codes {
		if !regexp.MustCompile(`^[a-z2-9]{5}-[a-z2-9]{5}$`).MatchString(c) || seen[c] {
			t.Errorf("code %q", c)
		}
		seen[c] = true
	}
	if len(codes) != 10 {
		t.Errorf("%d codes, want 10", len(codes))
	}
}
