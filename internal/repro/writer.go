package repro

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// maxBundles is how many bundles one run of the server will write.
//
// A ruleset that has gone wrong goes wrong on every move, and a batch of five
// hundred games would then write five hundred identical bundles and fill the
// disk. The first few are the ones worth having; after that the ERROR lines
// say it is still happening.
const maxBundles = 50

// Writer writes bundles into a directory. A nil Writer, or one with no
// directory, writes nothing — which is what a server run without
// BINGLE_DEBUG_DIR does, and what every test that is not about bundles wants.
type Writer struct {
	dir     string
	log     *slog.Logger
	version string
	written atomic.Int64
}

// NewWriter returns a writer for dir, or nil when dir is empty.
func NewWriter(dir, version string, lg *slog.Logger) *Writer {
	if dir == "" {
		return nil
	}
	if lg == nil {
		lg = slog.Default()
	}
	return &Writer{dir: dir, log: lg, version: version}
}

// Dir is where bundles are written, or "" when they are off.
func (w *Writer) Dir() string {
	if w == nil {
		return ""
	}
	return w.dir
}

// On reports whether bundles are being written. Call sites use it to skip the
// work of building one — reading a game's history, rendering a board — when
// nothing would come of it.
func (w *Writer) On() bool {
	return w != nil && w.dir != "" && w.written.Load() < maxBundles
}

// Write stores a bundle and logs one ERROR line carrying its id, which is how
// a log and a directory of bundles are matched up.
//
// It never returns an error to its caller: a bundle is written on a path where
// something has already gone wrong, and failing to record it must not become a
// second failure. What went wrong with the recording goes in the log.
func (w *Writer) Write(b Bundle) string {
	if w == nil || w.dir == "" {
		return ""
	}
	if n := w.written.Add(1); n > maxBundles {
		// Said once, at the limit, rather than on every attempt after it.
		if n == maxBundles+1 {
			w.log.Error("no more repro bundles will be written this run",
				"limit", maxBundles, "dir", w.dir,
				"why", "a ruleset that has gone wrong goes wrong on every move")
		}
		return ""
	}

	if b.Bingle == "" {
		b.Bingle = w.version
	}
	if b.ID == "" {
		b.ID = NewID()
	}
	if b.Written.IsZero() {
		b.Written = time.Now().UTC()
	}

	name := b.Filename()
	path := filepath.Join(w.dir, name)
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		w.log.Error("a repro bundle could not be written", "dir", w.dir, "err", err)
		return ""
	}
	payload, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		w.log.Error("a repro bundle could not be written", "id", b.ID, "err", err)
		return ""
	}
	// 0600: a bundle is the full position, which carries the game's secrets.
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		w.log.Error("a repro bundle could not be written", "path", path, "err", err)
		return ""
	}

	w.log.Error("something impossible happened; a repro bundle was written",
		"kind", string(b.Kind), "id", b.ID, "game", b.GameDefID, "bundle", path,
		"replay", "go run ./scripts/replay "+path)
	return path
}

// Of builds a bundle from a position and a decision, filling in the parts
// every caller would otherwise repeat: the board a person reads, and the move
// in both its forms.
//
// compiled may be nil, in which case the board and the notation are left out.
func Of(kind Kind, compiled *catalog.CompiledGame, defID string, st *state.BoardState, seat int, mv *state.Move, cause error) Bundle {
	b := Bundle{Kind: kind, GameDefID: defID, Seat: seat}
	if cause != nil {
		b.Error = cause.Error()
	}
	if st != nil {
		if raw, err := st.MarshalJSON(); err == nil {
			b.Position = raw
		}
		if compiled != nil {
			b.Board = state.RenderText(compiled, st)
		}
	}
	if mv != nil {
		if raw, err := json.Marshal(mv); err == nil {
			b.Move = raw
		}
		if compiled != nil {
			b.Notation = state.FormatMove(compiled, *mv)
		}
	}
	return b
}

// Load reads a bundle from disk.
func Load(path string) (Bundle, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the operator names the file
	if err != nil {
		return Bundle{}, fmt.Errorf("repro: reading %s: %w", path, err)
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bundle{}, fmt.Errorf("repro: %s is not a bundle: %w", path, err)
	}
	if b.GameDefID == "" {
		return Bundle{}, fmt.Errorf("repro: %s names no game", path)
	}
	if len(b.Position) == 0 {
		return Bundle{}, fmt.Errorf("repro: %s carries no position", path)
	}
	return b, nil
}
