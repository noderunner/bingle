package api

import (
	"net/http"
	"regexp"
	"strconv"
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

// changelogEntries returns the releases of a changelog in the order written,
// the newest first: each one's version, date and the Markdown under its
// heading (up to the next release or the next heading of its level). Text
// before the first release belongs to none.
func changelogEntries(md string) []changelogEntry {
	var out []changelogEntry
	var body []string
	in := false
	flush := func() {
		if in {
			out[len(out)-1].Markdown = strings.TrimSpace(strings.Join(body, "\n"))
		}
		body, in = nil, false
	}
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if m := entryHeading.FindStringSubmatch(line); m != nil {
			flush()
			out = append(out, changelogEntry{Version: m[1], Date: m[2]})
			in = true
			continue
		}
		if strings.HasPrefix(line, "## ") {
			flush()
			continue
		}
		if in {
			body = append(body, line)
		}
	}
	flush()
	return out
}

// newerThan reports whether version a comes after version b ("0.10.0" after
// "0.9.3"). A version that is not three numbers comes before every other.
func newerThan(a, b string) bool {
	pa, okA := versionParts(a)
	pb, okB := versionParts(b)
	switch {
	case !okA:
		return false
	case !okB:
		return true
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func versionParts(v string) ([3]int, bool) {
	var p [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return p, false
	}
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return p, false
		}
		p[i] = n
	}
	return p, true
}

// WithChangelog sets the changelog (the text of CHANGELOG.md) whose releases
// GET /changelog and GET /changelog/entries report, for the Recent Changes of
// the help window.
func WithChangelog(md string) Option {
	return func(h *handlers) {
		h.changelog = changelogEntries(md)
	}
}

// handleChangelog implements GET /changelog: the newest release of the
// changelog, or 404 when the server has none.
func (h *handlers) handleChangelog(w http.ResponseWriter, r *http.Request) {
	if len(h.changelog) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no changelog"})
		return
	}
	writeJSON(w, http.StatusOK, h.changelog[0])
}

// changelogEntriesResponse is GET /changelog/entries' answer.
type changelogEntriesResponse struct {
	// Entries are the releases asked for, the newest first.
	Entries []changelogEntry `json:"entries"`
}

// handleChangelogEntries implements GET /changelog/entries?since=: every
// release newer than since, the newest first, so that a player who last read
// the changes at one version can read all that came after it. Without since,
// every release. A since that is not a version is refused.
func (h *handlers) handleChangelogEntries(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	if _, ok := versionParts(since); since != "" && !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "api: since must be a version such as 0.4.0"})
		return
	}
	out := changelogEntriesResponse{Entries: []changelogEntry{}}
	for _, e := range h.changelog {
		if since == "" || newerThan(e.Version, since) {
			out.Entries = append(out.Entries, e)
		}
	}
	writeJSON(w, http.StatusOK, out)
}
