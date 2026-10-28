package api

import (
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/noderunner/bingle/internal/web/botguide"
)

// The guide to the bots, for the Analysis tab: GET /bot-guide is the Markdown,
// GET /bot-guide/assets/* its drawings. It is one embedded document, so there
// is nothing to configure and nothing to go missing.

func (h *handlers) handleBotGuide(w http.ResponseWriter, r *http.Request) {
	text, err := botguide.Text()
	if err != nil {
		http.Error(w, "api: the guide to the bots is missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(text)
}

func (h *handlers) handleBotGuideAsset(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(strings.TrimPrefix(r.URL.Path, "/api/bot-guide/assets/"))
	if name == "." || strings.HasPrefix(name, "..") || strings.ContainsRune(name, '/') {
		http.NotFound(w, r)
		return
	}
	assets, err := botguide.Assets()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := assets.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	if strings.HasSuffix(name, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = io.Copy(w, f)
}
