package api

import (
	"net/http"
	"regexp"
	"strings"
)

// changelogEntry is one release of CHANGELOG.md.
type changelogEntry struct {
	Version  string `json:"version"`
	Date     string `json:"date,omitempty"`
	Markdown string `json:"markdown"`
}

// entryHeading is the heading of a release: "## 0.4.0 - 2026-10-26" (the
// version may be in brackets, the date is optional, the dash may be – or —).
var entryHeading = regexp.MustCompile(`^##\s+\[?(\d+\.\d+\.\d+)\]?(?:\s+[-–—]\s+(\d{4}-\d{2}-\d{2}))?\s*$`)

// latestChangelogEntry returns the first release of a changelog, the newest
// one: its version, its date and the Markdown under its heading (up to the next
// release). ok is false when the text has no release.
func latestChangelogEntry(md string) (e changelogEntry, ok bool) {
	var body []string
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if m := entryHeading.FindStringSubmatch(line); m != nil {
			if ok {
				break
			}
			e.Version, e.Date, ok = m[1], m[2], true
			continue
		}
		if ok && strings.HasPrefix(line, "## ") {
			break
		}
		if ok {
			body = append(body, line)
		}
	}
	e.Markdown = strings.TrimSpace(strings.Join(body, "\n"))
	return e, ok
}

// WithChangelog sets the changelog (the text of CHANGELOG.md) whose newest
// release GET /changelog reports, for the Recent Changes of the help window.
func WithChangelog(md string) Option {
	return func(h *handlers) {
		if e, ok := latestChangelogEntry(md); ok {
			h.changelog = &e
		}
	}
}

// handleChangelog implements GET /changelog: the newest release of the
// changelog, or 404 when the server has none.
func (h *handlers) handleChangelog(w http.ResponseWriter, r *http.Request) {
	if h.changelog == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no changelog"})
		return
	}
	writeJSON(w, http.StatusOK, h.changelog)
}
