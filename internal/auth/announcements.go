package auth

// Major announcements (ADR 0155): an administrator publishes one, each guest
// or account that existed before it is offered it once in the welcome tour's
// card, and anybody can look back at every one from the help window.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
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

// An announcement's pictures (#357): PNG or JPEG files, at most twelve, of a
// megabyte each and four in all, which is a lot for a card of slides.
const (
	maxPicture      = 1 << 20
	maxPictures     = 12
	maxPictureBytes = 4 << 20
)

// pictureName is what a picture may be called: the name its body shows it by,
// which is also the last part of its path.
var pictureName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*\.(?i:png|jpe?g)$`)

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
	// Pictures are the names of its pictures, which its body shows by name:
	// each is at /api/announcements/{id}/pictures/{name}.
	Pictures []string `json:"pictures,omitempty"`
}

func announcementOf(a store.Announcement) Announcement {
	at := a.PublishedAt
	if t, err := time.Parse(time.DateTime, at); err == nil {
		at = t.UTC().Format(time.RFC3339)
	}
	return Announcement{ID: a.ID, Title: a.Title, Body: a.Body, PublishedAt: at, Withdrawn: a.Withdrawn, Pictures: a.Pictures}
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

// HandleAnnouncementPicture is GET /api/announcements/{id}/pictures/{name}:
// one picture of an announcement not withdrawn, for anybody.
func (rs *Resolver) HandleAnnouncementPicture(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	name := chi.URLParam(r, "name")
	if err != nil || !pictureName.MatchString(name) {
		writeError(w, http.StatusNotFound, "auth: there is no such picture")
		return
	}
	p, err := rs.guests.AnnouncementPicture(r.Context(), id, name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "auth: there is no such picture")
		return
	}
	if err != nil {
		rs.log.Error("reading an announcement's picture", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: the picture could not be read; try again")
		return
	}
	w.Header().Set("Content-Type", p.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A picture never changes: a new one is a new announcement.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(p.Data)
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

// errPictures is the answer to pictures beyond the limits.
var errPictures = fmt.Errorf("auth: an announcement may have at most %d pictures, of at most %d KiB each and %d KiB in all", maxPictures, maxPicture>>10, maxPictureBytes>>10)

// handleAdminAnnounce publishes an announcement: {title, body} as JSON, or as
// a multipart form with its pictures too, each a "pictures" file part named
// as its body shows it.
func (rs *Resolver) handleAdminAnnounce(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.admin(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	var pictures []store.Picture
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
		var status int
		var err error
		req.Title, req.Body, pictures, status, err = readAnnouncementForm(w, r)
		if err != nil {
			writeError(w, status, err.Error())
			return
		}
	} else if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*maxAnnouncementBody)).Decode(&req); err != nil {
		// The body is longer than the other requests here allow.
		writeError(w, http.StatusBadRequest, "auth: malformed JSON body")
		return
	}
	title, body := strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	if title == "" || body == "" || utf8.RuneCountInString(title) > maxAnnouncementTitle || utf8.RuneCountInString(body) > maxAnnouncementBody || strings.ContainsAny(title, "\r\n") {
		writeError(w, http.StatusBadRequest, errAnnouncement.Error())
		return
	}
	a, err := rs.accounts.st.CreateAnnouncement(r.Context(), title, body, pictures, rs.now())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.accounts.audit(r.Context(), acc.ID, "announcement-published", title, rs.transport.PeerIP(r))
	writeJSON(w, http.StatusCreated, announcementOf(a))
}

// readAnnouncementForm reads an announcement as a multipart form: its title,
// its body and its pictures, each checked to be a PNG or JPEG within the
// limits. On an error it says the status to answer with.
func readAnnouncementForm(w http.ResponseWriter, r *http.Request) (title, body string, pictures []store.Picture, status int, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPictureBytes+8*maxAnnouncementBody+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		return "", "", nil, http.StatusBadRequest, errors.New("auth: malformed form")
	}
	total := 0
	seen := map[string]bool{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return "", "", nil, http.StatusRequestEntityTooLarge, errPictures
			}
			return "", "", nil, http.StatusBadRequest, errors.New("auth: malformed form")
		}
		limit := 4 * maxAnnouncementBody
		if part.FormName() == "pictures" {
			limit = maxPicture
		}
		data, err := io.ReadAll(io.LimitReader(part, int64(limit)+1))
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return "", "", nil, http.StatusRequestEntityTooLarge, errPictures
			}
			return "", "", nil, http.StatusBadRequest, errors.New("auth: malformed form")
		}
		switch part.FormName() {
		case "title", "body":
			if len(data) > limit {
				return "", "", nil, http.StatusBadRequest, errAnnouncement
			}
			if part.FormName() == "title" {
				title = string(data)
			} else {
				body = string(data)
			}
		case "pictures":
			name := part.FileName()
			if name == "" && len(data) == 0 {
				continue // a file input left empty
			}
			if !pictureName.MatchString(name) || seen[name] {
				return "", "", nil, http.StatusBadRequest, fmt.Errorf("auth: %q: a picture's name is letters, digits, dots, dashes and underscores, ends .png, .jpg or .jpeg, and is used once", name)
			}
			kind := http.DetectContentType(data)
			if kind != "image/png" && kind != "image/jpeg" {
				return "", "", nil, http.StatusBadRequest, fmt.Errorf("auth: %q is not a PNG or JPEG picture", name)
			}
			total += len(data)
			if len(data) > maxPicture || total > maxPictureBytes || len(pictures) == maxPictures {
				return "", "", nil, http.StatusRequestEntityTooLarge, errPictures
			}
			seen[name] = true
			pictures = append(pictures, store.Picture{Name: name, ContentType: kind, Data: data})
		}
	}
	return title, body, pictures, 0, nil
}

// handleAdminDelete deletes an announcement for good, with its pictures and
// who has seen it.
func (rs *Resolver) handleAdminDelete(w http.ResponseWriter, r *http.Request) {
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
		err = rs.accounts.st.DeleteAnnouncement(r.Context(), id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "auth: there is no such announcement")
		return
	}
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.accounts.audit(r.Context(), acc.ID, "announcement-deleted", a.Title, rs.transport.PeerIP(r))
	w.WriteHeader(http.StatusNoContent)
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
