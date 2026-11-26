package catalog

import (
	"io/fs"
	"slices"
	"strings"
)

// ClientFiles is the files one client other than the web page draws a
// family's games with (ADR 0147, "Every client's files"): bingle-3d's models,
// textures and sounds, in that client's own formats. The server lists them
// and hands them out, and never reads them.
type ClientFiles struct {
	// Files lists them in the order of their paths.
	Files []ClientFile
	// FS opens one by its path. What it opens is read from where the file
	// is kept as it is sent, and is an io.ReadSeeker, so a range of it can be
	// sent without the rest.
	FS fs.FS
}

// ClientFile is one file of a client's.
type ClientFile struct {
	// Path is its path inside the client's folder, slash-separated.
	Path string
	// Size is its length in bytes.
	Size int64
	// SHA256 is the hash of its contents, in lower-case hex.
	SHA256 string
	// ContentType is the media type it is sent as.
	ContentType string
}

// File returns the file at path, if the client has one there.
func (c *ClientFiles) File(path string) (ClientFile, bool) {
	i, found := slices.BinarySearchFunc(c.Files, path, func(f ClientFile, path string) int { return strings.Compare(f.Path, path) })
	if !found {
		return ClientFile{}, false
	}
	return c.Files[i], true
}
