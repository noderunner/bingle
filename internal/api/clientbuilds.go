package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/clientbuilds"
)

// The bingle-3d client itself, for download (ADR 0151): the builds the
// operator keeps on the server's disk, and a setup file made for this server
// and whoever asks. A game's own files for a client are another thing
// (clients.go).

// WithClientBuilds mounts GET /client, GET /client/files/{file} and, with
// WithGuests, POST /client/setup. A nil set leaves all three unmounted: a
// server that was not given a directory of builds offers nothing.
func WithClientBuilds(set *clientbuilds.Set) Option {
	return func(h *handlers) { h.clientBuilds = set }
}

// clientBuildsResponse is GET /client's answer.
type clientBuildsResponse struct {
	// Builds are the builds of the bingle-3d client this server offers: for
	// each platform, the newest it has that can use its API. Each is fetched
	// from /api/client/files/{file}.
	Builds []clientbuilds.Build `json:"builds"`
}

// handleClientBuilds implements GET /client.
func (h *handlers) handleClientBuilds(w http.ResponseWriter, r *http.Request) {
	builds := h.clientBuilds.Builds()
	if builds == nil {
		builds = []clientbuilds.Build{}
	}
	writeJSON(w, http.StatusOK, clientBuildsResponse{Builds: builds})
}

// handleClientBuildFile implements GET /client/files/{file}: one build's
// archive, sent from the disk as it is read (serveUnread).
func (h *handlers) handleClientBuildFile(w http.ResponseWriter, r *http.Request) {
	build, f, err := h.clientBuilds.Open(chi.URLParam(r, "file"))
	switch {
	case errors.Is(err, clientbuilds.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "api: this server has no such build"})
		return
	case err != nil:
		h.logger().Error("api: a client's build is not the one that was listed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "api: that build cannot be sent just now"})
		return
	}
	defer func() { _ = f.Close() }()
	serveUnread(w, r, f, build.ContentType(), build.SHA256, build.File)
}
