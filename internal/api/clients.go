package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/session"
)

// inertPolicy is the Content-Security-Policy of a file the server hands out
// and a browser should not run anything in: a guide's picture, a client's
// file.
const inertPolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox"

// handleClientIndex implements GET /game-defs/{id}/clients/{client}: the
// files a client draws the game with (ADR 0147), each with its size, hash
// and type. 404 for a game with no files for that client.
func (h *handlers) handleClientIndex(w http.ResponseWriter, r *http.Request) {
	index, err := h.mgr.ClientIndex(chi.URLParam(r, "id"), chi.URLParam(r, "client"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, index)
}

// handleClientFile implements GET /game-defs/{id}/clients/{client}/files/{path}:
// one file of a client's, sent from the disk as it is read (serveUnread).
func (h *handlers) handleClientFile(w http.ResponseWriter, r *http.Request) {
	info, f, err := h.mgr.ClientFile(chi.URLParam(r, "id"), chi.URLParam(r, "client"), chi.URLParam(r, "*"))
	if err != nil {
		if errors.Is(err, session.ErrClientFileChanged) {
			h.logger().Error("api: a client's file is not the one its pack listed", "error", err)
		}
		writeError(w, err)
		return
	}
	defer func() { _ = f.Close() }()
	serveUnread(w, r, f, info.ContentType, info.SHA256, path.Base(info.Path))
}

// serveUnread sends a file the server hands out and never reads: a client's
// file, a client's build. Its hash is its strong ETag, so If-None-Match and
// If-Range are answered, and so are byte ranges. It goes out inert and as an
// attachment: a browser that opens it saves it rather than show it.
func serveUnread(w http.ResponseWriter, r *http.Request, f io.ReadSeeker, contentType, sha256, filename string) {
	hd := w.Header()
	hd.Set("Content-Type", contentType)
	hd.Set("ETag", `"`+sha256+`"`)
	hd.Set("Cache-Control", "no-cache")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", inertPolicy)
	hd.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	// No time: the hash says which file it is.
	http.ServeContent(w, r, "", time.Time{}, f)
}
