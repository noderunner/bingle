package auth

// Major announcements (ADR 0155): an administrator publishes one, each guest
// or account that existed before it is offered it once in the welcome tour's
// card, and anybody can look back at every one from the help window.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/noderunner/bingle/internal/store"
)

// How long an announcement may be: a title of a line, and a body of a page
// or two of Markdown.
const (
	maxAnnouncementTitle = 120
	maxAnnouncementBody  = 20000
)

// Announcement is one announcement as the API sends it.
type Announcement struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// Body is Markdown, in the one language its administrator wrote it in;
	// it is not translated.
	Body string `json:"body"`
	// PublishedAt is when it was published (RFC 3339).
	PublishedAt string `json:"published_at"`
	// Withdrawn: it is offered and listed no more. Only the admin page's
	// list has withdrawn ones.
	Withdrawn bool `json:"withdrawn,omitempty"`
}

func announcementOf(a store.Announcement) Announcement {
	at := a.PublishedAt
	if t, err := time.Parse(time.DateTime, at); err == nil {
		at = t.UTC().Format(time.RFC3339)
	}
	return Announcement{ID: a.ID, Title: a.Title, Body: a.Body, PublishedAt: at, Withdrawn: a.Withdrawn}
}

// AnnouncementsResponse is GET /api/announcements' answer.
type AnnouncementsResponse struct {
	// Announcements are every one published and not withdrawn, oldest first.
	Announcements []Announcement `json:"announcements"`
}

// AnnouncementsSeenRequest is POST /auth/announcements-seen's body.
type AnnouncementsSeenRequest struct {
	// IDs are the announcements the card showed when it was closed.
	IDs []int64 `json:"ids"`
}

// AnnouncementsSeenResponse is POST /auth/announcements-seen's answer.
type AnnouncementsSeenResponse struct {
	// Announcements are the ids still waiting after these were marked: what
	// /auth/me would now say.
	Announcements []int64 `json:"announcements"`
}

// announcementsWaiting are the ids of the announcements waiting for the
// principal. When they cannot be read there are none: an announcement shown
// twice by mistake is the worse error.
func (rs *Resolver) announcementsWaiting(r *http.Request, principalID string) []int64 {
	if principalID == "" {
		return nil
	}
	ids, err := rs.guests.AnnouncementsWaiting(r.Context(), principalID)
	if err != nil {
		rs.log.Error("reading the announcements waiting", "err", err)
		return nil
	}
	return ids
}

// HandleAnnouncements is GET /api/announcements: every announcement published
// and not withdrawn, oldest first, for anybody, so that the help window can
// link each under its release.
func (rs *Resolver) HandleAnnouncements(w http.ResponseWriter, r *http.Request) {
	list, err := rs.guests.Announcements(r.Context(), false)
	if err != nil {
		rs.log.Error("listing the announcements", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: the announcements could not be read; try again")
		return
	}
	out := AnnouncementsResponse{Announcements: []Announcement{}}
	for _, a := range list {
		out.Announcements = append(out.Announcements, announcementOf(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAnnouncementsSeen marks announcements seen by whoever asks: an
// account, or a guest, which a page's first request makes if the browser has
// none yet.
func (rs *Resolver) handleAnnouncementsSeen(w http.ResponseWriter, r *http.Request) {
	p, ok := From(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "auth: there is nobody to remember that for: a guest is made by the page's first request")
		return
	}
	var req AnnouncementsSeenRequest
	if !decode(w, r, &req) {
		return
	}
	if err := rs.guests.MarkAnnouncementsSeen(r.Context(), p.ID, req.IDs, rs.now()); err != nil {
		rs.log.Error("marking announcements seen", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: could not remember that; try again")
		return
	}
	out := AnnouncementsSeenResponse{Announcements: rs.announcementsWaiting(r, p.ID)}
	if out.Announcements == nil {
		out.Announcements = []int64{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAdminAnnouncements lists every announcement for the admin page, the
// withdrawn ones too, newest first.
func (rs *Resolver) handleAdminAnnouncements(w http.ResponseWriter, r *http.Request) {
	if _, ok := rs.admin(w, r); !ok {
		return
	}
	list, err := rs.accounts.st.Announcements(r.Context(), true)
	if err != nil {
		rs.fail(w, err)
		return
	}
	out := AnnouncementsResponse{Announcements: []Announcement{}}
	for i := len(list) - 1; i >= 0; i-- {
		out.Announcements = append(out.Announcements, announcementOf(list[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// errAnnouncement is the answer to an announcement without its words, or with
// too many.
var errAnnouncement = errors.New("auth: an announcement needs a title of at most 120 characters and a body of at most 20000")

// handleAdminAnnounce publishes an announcement: {title, body}.
func (rs *Resolver) handleAdminAnnounce(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.admin(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	// The body is longer than the other requests here allow.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*maxAnnouncementBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "auth: malformed JSON body")
		return
	}
	title, body := strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	if title == "" || body == "" || utf8.RuneCountInString(title) > maxAnnouncementTitle || utf8.RuneCountInString(body) > maxAnnouncementBody || strings.ContainsAny(title, "\r\n") {
		writeError(w, http.StatusBadRequest, errAnnouncement.Error())
		return
	}
	a, err := rs.accounts.st.CreateAnnouncement(r.Context(), title, body, rs.now())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.accounts.audit(r.Context(), acc.ID, "announcement-published", title, rs.transport.PeerIP(r))
	writeJSON(w, http.StatusCreated, announcementOf(a))
}

// handleAdminWithdraw withdraws an announcement: nobody is offered it from
// then on, and the help window lists it no more.
func (rs *Resolver) handleAdminWithdraw(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.admin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "auth: there is no such announcement")
		return
	}
	a, err := rs.accounts.st.AnnouncementByID(r.Context(), id)
	if err == nil {
		err = rs.accounts.st.WithdrawAnnouncement(r.Context(), id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "auth: there is no such announcement")
		return
	}
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.accounts.audit(r.Context(), acc.ID, "announcement-withdrawn", a.Title, rs.transport.PeerIP(r))
	a.Withdrawn = true
	writeJSON(w, http.StatusOK, announcementOf(a))
}
