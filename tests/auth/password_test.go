package auth_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/auth"
)

var cheap = auth.Argon2Params{Memory: 64, Time: 1, Threads: 1}

// The parameters are written into the hash, so a hash made with older ones
// still checks, and says it wants remaking with the new ones.
func TestPasswordHashesCarryTheirParameters(t *testing.T) {
	h := auth.HashPassword("correct horse battery", cheap)
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("hash %q", h)
	}
	if ok, rehash := auth.VerifyPassword("correct horse battery", h, cheap); !ok || rehash {
		t.Errorf("with its own parameters: ok=%v rehash=%v, want true false", ok, rehash)
	}
	stronger := auth.Argon2Params{Memory: 128, Time: 2, Threads: 1}
	if ok, rehash := auth.VerifyPassword("correct horse battery", h, stronger); !ok || !rehash {
		t.Errorf("after the parameters were raised: ok=%v rehash=%v, want true true", ok, rehash)
	}
	if ok, _ := auth.VerifyPassword("Correct horse battery", h, cheap); ok {
		t.Error("a different password checked")
	}
	if h2 := auth.HashPassword("correct horse battery", cheap); h2 == h {
		t.Error("two hashes of one password are the same: no salt")
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=64,t=1,p=1$AAAA$AAAA", strings.Replace(h, "m=64", "m=0", 1), h[:len(h)-4]} {
		if ok, _ := auth.VerifyPassword("correct horse battery", bad, cheap); ok {
			t.Errorf("a malformed hash checked: %q", bad)
		}
	}
}

func TestPasswordBounds(t *testing.T) {
	for pw, ok := range map[string]bool{"short": false, "ten chars!": true, strings.Repeat("x", 256): true, strings.Repeat("x", 257): false} {
		if err := auth.CheckPassword(pw); (err == nil) != ok {
			t.Errorf("CheckPassword(%d characters) = %v", len(pw), err)
		}
	}
	a, b := auth.RandomPassword(), auth.RandomPassword()
	if a == b || len(a) != 16 || strings.ContainsAny(a, "0O1lI") {
		t.Errorf("random passwords %q %q", a, b)
	}
}

func TestHandles(t *testing.T) {
	for in, want := range map[string]string{"Ana": "ana", "@bob_1": "bob_1", " x-y-z ": "x-y-z"} {
		if got, err := auth.CanonHandle(in, false); err != nil || got != want {
			t.Errorf("CanonHandle(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"ab", strings.Repeat("a", 25), "a b", "ánä", "a.b", ""} {
		if _, err := auth.CanonHandle(bad, false); err == nil {
			t.Errorf("CanonHandle(%q) took it", bad)
		}
	}
	if _, err := auth.CanonHandle("Admin", false); err != auth.ErrReservedHandle {
		t.Errorf("admin as somebody's handle: %v", err)
	}
	if h, err := auth.CanonHandle("admin", true); err != nil || h != "admin" {
		t.Errorf("the seeded administrator's handle: %q %v", h, err)
	}
}
