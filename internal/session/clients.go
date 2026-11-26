package session

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// ErrClientFileChanged reports a client's file that is no longer the one its
// index lists: it was changed on the server's disk after it was hashed.
var ErrClientFileChanged = errors.New("session: the file changed after it was listed")

// ClientIndex is the files one client draws a game with (ADR 0147): GET
// /game-defs/{id}/clients/{client}. A client fetches each from
// /game-defs/{id}/clients/{client}/files/{path} and keeps it by its hash.
type ClientIndex struct {
	Game   string `json:"game"`
	Client string `json:"client"`
	// Files are in the order of their paths.
	Files []ClientFileInfo `json:"files"`
}

// ClientFileInfo is one file of a client's.
type ClientFileInfo struct {
	// Path is its path in the client's folder, slash-separated.
	Path string `json:"path"`
	// Size is its length in bytes.
	Size int64 `json:"size"`
	// SHA256 is the hash of its contents, in lower-case hex, and its ETag.
	SHA256 string `json:"sha256"`
	// ContentType is the media type it is sent as.
	ContentType string `json:"content_type"`
}

// clientNames are the clients a game has files for, sorted; nil for none.
func clientNames(cg *catalog.CompiledGame) []string {
	if len(cg.Clients) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(cg.Clients))
}

// clientFiles finds the files of client for the game id.
func (m *Manager) clientFiles(id, client string) (*catalog.ClientFiles, error) {
	cg, ok := m.catalog.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGameDefNotFound, id)
	}
	files, ok := cg.Clients[client]
	if !ok {
		return nil, fmt.Errorf("%w: the game %q has no files for the client %q", ErrAssetNotFound, id, client)
	}
	return files, nil
}

// ClientIndex lists the files of client for the game id.
func (m *Manager) ClientIndex(id, client string) (ClientIndex, error) {
	files, err := m.clientFiles(id, client)
	if err != nil {
		return ClientIndex{}, err
	}
	out := ClientIndex{Game: id, Client: client, Files: make([]ClientFileInfo, len(files.Files))}
	for i, f := range files.Files {
		out.Files[i] = ClientFileInfo{Path: f.Path, Size: f.Size, SHA256: f.SHA256, ContentType: f.ContentType}
	}
	return out, nil
}

// ClientFile opens one file of client for the game id, to be sent as it is
// read: it is never held in memory. The caller closes it.
func (m *Manager) ClientFile(id, client, name string) (catalog.ClientFile, io.ReadSeekCloser, error) {
	files, err := m.clientFiles(id, client)
	if err != nil {
		return catalog.ClientFile{}, nil, err
	}
	info, ok := files.File(name)
	if !ok {
		return catalog.ClientFile{}, nil, fmt.Errorf("%w: %q", ErrAssetNotFound, name)
	}
	f, err := files.FS.Open(name)
	if err != nil {
		return catalog.ClientFile{}, nil, fmt.Errorf("%w: %w", ErrClientFileChanged, err)
	}
	rs, ok := f.(io.ReadSeekCloser)
	if !ok {
		_ = f.Close()
		return catalog.ClientFile{}, nil, fmt.Errorf("session: the file %q cannot be sent in ranges", name)
	}
	return info, rs, nil
}
