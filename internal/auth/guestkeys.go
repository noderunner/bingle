package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/noderunner/bingle/internal/store"
)

// A guest's key, from the guest's side (stage 7 of the accounts plan, §3a):
// copied to another device and adopted there, replaced, or given up.
//
//	GET  /api/guest/key     the browser's own key, to copy (its page only)
//	POST /api/guest/adopt   {key}: this browser becomes that guest
//	POST /api/guest/rotate  a new key; the old one stops working everywhere
//	POST /api/guest/discard this browser starts over as a new guest

// ErrBadGuestKey is the one answer to a key that does not work, whatever
// the reason, so keys cannot be told apart by trying them.
var ErrBadGuestKey = errors.New("auth: that guest key does not work here (it may have been replaced, or gone unused too long)")

// ErrNotAGuest is the answer to a guest-key request from a browser that is
// not a guest by its cookie.
var ErrNotAGuest = errors.New("auth: this browser is not a guest (a signed-in account has no guest key)")

// cookieGuest is the guest the request is by its cookie, and that cookie's
// key.
func cookieGuest(r *http.Request) (Principal, string, bool) {
	p, ok := From(r.Context())
	if !ok || p.Account != nil || !p.Cookie {
		return Principal{}, "", false
	}
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return Principal{}, "", false
	}
	return p, c.Value, true
}

// HandleGuestKey answers with the browser's own key and a link that carries
// it in the fragment, which never reaches a server log or a Referer.
func (rs *Resolver) HandleGuestKey(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(PageHeader) == "" {
		writeError(w, http.StatusForbidden, "auth: the guest key is for bingle's own page")
		return
	}
	_, key, ok := cookieGuest(r)
	if !ok {
		writeError(w, http.StatusConflict, ErrNotAGuest.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "link": "/#guest=" + key})
}

// HandleAdopt makes this browser the guest whose key it was given: its own
// guest, if it had one, is merged in, so nothing it played is lost. A
// signed-in account adopting a key takes that guest's games into itself.
func (rs *Resolver) HandleAdopt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &req) {
		return
	}
	now := rs.now()
	if !rs.adopts.allow(rs.transport.PeerIP(r), now) {
		writeError(w, http.StatusTooManyRequests, ErrTooManyTries.Error())
		return
	}
	target, err := rs.guests.GuestByKey(r.Context(), HashKey(req.Key), now.Add(-rs.idle), touchEvery, now)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusBadRequest, ErrBadGuestKey.Error())
		return
	case err != nil:
		rs.log.Error("adopting a guest key", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: could not check that key; try again")
		return
	}
	p, ok := From(r.Context())
	st, isStore := rs.guests.(*store.Store)
	if !isStore {
		writeError(w, http.StatusServiceUnavailable, "auth: no store")
		return
	}
	switch {
	case ok && p.Account != nil:
		// An account takes the guest in, and the key stops working.
		if err := st.MergePrincipal(r.Context(), target, p.ID); err != nil {
			rs.log.Error("merging a guest into an account", "err", err)
			writeError(w, http.StatusServiceUnavailable, "auth: could not do that; try again")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"kind": store.KindAccount})
		return
	case ok && p.ID != target:
		if err := st.MergePrincipal(r.Context(), p.ID, target); err != nil {
			rs.log.Error("merging a guest", "err", err)
			writeError(w, http.StatusServiceUnavailable, "auth: could not do that; try again")
			return
		}
	}
	rs.setCookie(w, r, CookieName, req.Key, int(cookieMaxAge/time.Second))
	writeJSON(w, http.StatusOK, map[string]string{"kind": store.KindGuest})
}

// HandleRotate gives the guest a new key and retires the old one wherever
// it was copied; the guest, and its seats, stay.
func (rs *Resolver) HandleRotate(w http.ResponseWriter, r *http.Request) {
	p, _, ok := cookieGuest(r)
	st, isStore := rs.guests.(*store.Store)
	if !ok || !isStore {
		writeError(w, http.StatusConflict, ErrNotAGuest.Error())
		return
	}
	key := NewGuestKey()
	if err := st.ReplaceGuestKey(r.Context(), p.ID, HashKey(key)); err != nil {
		rs.log.Error("replacing a guest key", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: could not do that; try again")
		return
	}
	rs.setCookie(w, r, CookieName, key, int(cookieMaxAge/time.Second))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleDiscard gives up the guest: its key stops working, and this
// browser's next action makes a new guest. The old seats stay with the old
// guest, which nobody can be again.
func (rs *Resolver) HandleDiscard(w http.ResponseWriter, r *http.Request) {
	p, _, ok := cookieGuest(r)
	st, isStore := rs.guests.(*store.Store)
	if !ok || !isStore {
		writeError(w, http.StatusConflict, ErrNotAGuest.Error())
		return
	}
	if err := st.RevokeGuestKeys(r.Context(), p.ID); err != nil {
		rs.log.Error("discarding a guest", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: could not do that; try again")
		return
	}
	rs.clearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
