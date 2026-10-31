// Package auth_test checks the guest resolver (ADR 0067) against a real
// store: keys that stop working, and how many guests one address may make.
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "bingle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// whoAmI answers with the request's principal ID ("" when anonymous).
var whoAmI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(auth.IDFrom(r.Context()))) })

func call(h http.Handler, method, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func newKey(t *testing.T, rs *auth.Resolver) string {
	t.Helper()
	w := httptest.NewRecorder()
	rs.HandleNewGuest(w, httptest.NewRequest("POST", "/api/guest", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("new guest: %d %s", w.Code, w.Body)
	}
	return w.Body.String()[len(`{"key":"`) : len(w.Body.String())-len("\"}\n")]
}

func TestAKeyIsAGuestUntilItGoesUnusedTooLong(t *testing.T) {
	rs := auth.NewResolver(newStore(t), auth.Options{Idle: 48 * time.Hour})
	now := time.Now()
	rs.SetClock(func() time.Time { return now })
	key := newKey(t, rs)
	h := rs.Middleware(whoAmI)
	id := call(h, "GET", key).Body.String()
	if len(id) < 10 || id[:2] != "p_" {
		t.Fatalf("the key is not a guest: %q", id)
	}
	// Used every day, it keeps working well past the idle time.
	for day := 1; day <= 5; day++ {
		now = now.Add(24 * time.Hour)
		if got := call(h, "GET", key).Body.String(); got != id {
			t.Fatalf("day %d: %q, want the same guest %q", day, got, id)
		}
	}
	// Left alone for longer than the idle time, it stops.
	now = now.Add(49 * time.Hour)
	if w := call(h, "GET", key); w.Code != http.StatusUnauthorized {
		t.Errorf("an idle key: %d %q, want 401", w.Code, w.Body)
	}
}

func TestOneAddressMakesOnlySoManyGuests(t *testing.T) {
	rs := auth.NewResolver(newStore(t), auth.Options{NewGuestsPerHour: 3})
	now := time.Now()
	rs.SetClock(func() time.Time { return now })
	for i := 0; i < 3; i++ {
		newKey(t, rs)
	}
	w := httptest.NewRecorder()
	rs.HandleNewGuest(w, httptest.NewRequest("POST", "/api/guest", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("a fourth guest within the hour: %d, want 429", w.Code)
	}
	// A page past the limit goes on anonymous, as before there were guests.
	req := httptest.NewRequest("POST", "/x", nil)
	req.Header.Set(auth.PageHeader, "1")
	rec := httptest.NewRecorder()
	rs.Middleware(whoAmI).ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "" || len(rec.Result().Cookies()) != 0 {
		t.Errorf("a page past the limit: %d %q %v, want an anonymous 200", rec.Code, rec.Body, rec.Result().Cookies())
	}
	now = now.Add(61 * time.Minute)
	newKey(t, rs)
}

func TestAKeyAndItsIDLookNothingAlike(t *testing.T) {
	a, b := auth.NewGuestKey(), auth.NewGuestKey()
	if a == b || len(a) != 47 || a[:4] != "bgg_" {
		t.Errorf("keys %q %q", a, b)
	}
	if id := auth.NewPrincipalID(); len(id) != 28 || id[:2] != "p_" {
		t.Errorf("an ID %q", id)
	}
	if auth.HashKey(a) == a || len(auth.HashKey(a)) != 64 {
		t.Errorf("a key's hash %q", auth.HashKey(a))
	}
}
