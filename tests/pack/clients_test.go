package pack_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/pack"
	"github.com/noderunner/bingle/tests/testdata"
)

// madeUp is the fixture's made-up client, and its files in path order.
const madeUp = "made-up"

var madeUpFiles = []struct{ path, contentType string }{
	{"look.toml", "application/toml"},
	{"models/man.glb", "model/gltf-binary"},
	{"textures/board.png", "image/png"},
}

// hashOf is the SHA-256 of a file of the pack in dir, in hex.
func hashOf(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// glb writes a binary glTF of size bytes, most of them a hole in the file,
// at name in dir.
func glb(t *testing.T, dir, name string, size int64) {
	t.Helper()
	header := make([]byte, 12)
	copy(header, "glTF")
	binary.LittleEndian.PutUint32(header[4:], 2)
	binary.LittleEndian.PutUint32(header[8:], uint32(size))
	write(t, dir, name, string(header))
	if err := os.Truncate(filepath.Join(dir, name), size); err != nil {
		t.Fatal(err)
	}
}

func TestThePacksClientFilesAreListed(t *testing.T) {
	_, dir := testdata.DraughtsPack(t)
	p, err := read(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Module.Clients) != 1 {
		t.Fatalf("clients %v, want only %s", p.Module.Clients, madeUp)
	}
	files := p.Module.Clients[madeUp]
	if files == nil || len(files.Files) != len(madeUpFiles) {
		t.Fatalf("the %s client's files: %+v", madeUp, files)
	}
	for i, want := range madeUpFiles {
		got := files.Files[i]
		at := filepath.Join("clients", madeUp, want.path)
		info, _ := os.Stat(filepath.Join(dir, at))
		if got.Path != want.path || got.ContentType != want.contentType || got.SHA256 != hashOf(t, dir, at) || got.Size != info.Size() {
			t.Errorf("file %d is %+v, want %s (%s, %d bytes, %s)", i, got, want.path, want.contentType, info.Size(), hashOf(t, dir, at))
		}
		// It is opened from the disk, not from a copy in memory.
		f, err := files.FS.Open(want.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := f.(*os.File); !ok {
			t.Errorf("%s opens as a %T", want.path, f)
		}
		_ = f.Close()
	}
	for _, name := range []string{"../../manifest.yaml", "look.TOML", "models", "rules.wasm"} {
		if f, err := files.FS.Open(name); err == nil {
			_ = f.Close()
			t.Errorf("%q opens", name)
		}
	}
	// Its variant offers the same files.
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(p.Module); err != nil {
		t.Fatal(err)
	}
	for _, id := range p.Games {
		if cg, _ := cat.Get(id); cg.Clients[madeUp] != files {
			t.Errorf("%s does not offer the %s client's files", id, madeUp)
		}
	}
}

func TestAPackWithoutClientsHasNone(t *testing.T) {
	_, dir := testdata.DraughtsPack(t)
	if err := os.RemoveAll(filepath.Join(dir, "clients")); err != nil {
		t.Fatal(err)
	}
	p, err := read(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Module.Clients != nil {
		t.Errorf("clients %v", p.Module.Clients)
	}
}

// A client's file that changes after the pack loaded is not served as the
// one that was hashed.
func TestAClientsFileChangedSinceIsNotOpened(t *testing.T) {
	_, dir := testdata.DraughtsPack(t)
	p, err := read(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "clients/made-up/look.toml", "board = \"elsewhere.png\"\n")
	f, err := p.Module.Clients[madeUp].FS.Open("look.toml")
	if err == nil {
		_ = f.Close()
		t.Fatal("a changed file opens")
	}
	if !strings.Contains(err.Error(), "changed") {
		t.Errorf("refused with %q", err)
	}
}

// Each pack here is the fixture with its client folder made bad in one way.
func TestABadClientFolderRefusesThePack(t *testing.T) {
	client := "clients/" + madeUp + "/"
	cases := []struct {
		name string
		bad  func(t *testing.T, dir string)
		says string
	}{
		{"a link out of the folder", func(t *testing.T, dir string) {
			outside := filepath.Join(t.TempDir(), "secret.toml")
			write(t, filepath.Dir(outside), "secret.toml", "a = 1")
			if err := os.Symlink(outside, filepath.Join(dir, client, "secret.toml")); err != nil {
				t.Skip("no links here:", err)
			}
		}, "is a link"},
		{"a link inside the folder", func(t *testing.T, dir string) {
			if err := os.Symlink("look.toml", filepath.Join(dir, client, "again.toml")); err != nil {
				t.Skip("no links here:", err)
			}
		}, "is a link"},
		{"a client that is a link", func(t *testing.T, dir string) {
			if err := os.Symlink(madeUp, filepath.Join(dir, "clients", "other")); err != nil {
				t.Skip("no links here:", err)
			}
		}, "is a link"},
		{"a file over the limit", func(t *testing.T, dir string) { glb(t, dir, client+"models/big.glb", pack.MaxClientFileBytes+1) }, "over"},
		{"a kind not on the list", func(t *testing.T, dir string) { write(t, dir, client+"shaders/board.wgsl", "fn main() {}") }, "no other kind"},
		{"a script", func(t *testing.T, dir string) { write(t, dir, client+"look.js", "alert(1)") }, "no other kind"},
		{"a picture that is not one", func(t *testing.T, dir string) {
			write(t, dir, client+"textures/board.png", "<script>alert(1)</script>")
		}, "is not image/png"},
		{"a model that is not one", func(t *testing.T, dir string) { write(t, dir, client+"models/man.glb", "<html></html>") }, "is not model/gltf-binary"},
		{"a model of another length", func(t *testing.T, dir string) {
			glb(t, dir, client+"models/man.glb", 64)
			f, _ := os.OpenFile(filepath.Join(dir, client, "models/man.glb"), os.O_APPEND|os.O_WRONLY, 0)
			_, _ = f.WriteString("more")
			_ = f.Close()
		}, "of its own length"},
		{"a sound that is not one", func(t *testing.T, dir string) { write(t, dir, client+"sounds/move.ogg", "RIFF....WAVE") }, "is not audio/ogg"},
		{"text that is not text", func(t *testing.T, dir string) { write(t, dir, client+"look.toml", "board = \"\xff\"") }, "is not text"},
		{"text with a NUL", func(t *testing.T, dir string) { write(t, dir, client+"notes.md", "a\x00b") }, "is not text"},
		{"a file of the clients folder's own", func(t *testing.T, dir string) { write(t, dir, "clients/look.toml", "a = 1") }, "no file of its own"},
		{"a client named out of form", func(t *testing.T, dir string) { write(t, dir, "clients/Made_Up/look.toml", "a = 1") }, "lower-case"},
		{"a path no system can name", func(t *testing.T, dir string) { write(t, dir, client+"my look.toml", "a = 1") }, "letters, digits"},
		{"a path too long", func(t *testing.T, dir string) { write(t, dir, client+strings.Repeat("a/", 100)+"x.toml", "a = 1") }, "at most"},
		{"the same name twice", func(t *testing.T, dir string) { write(t, dir, client+"Look.toml", "a = 1") }, "twice"},
		{"an empty client", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, "clients", "empty"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "holds no files"},
		{"too many clients", func(t *testing.T, dir string) {
			for i := range pack.MaxClients {
				write(t, dir, fmt.Sprintf("clients/client-%d/look.toml", i), "a = 1")
			}
		}, "over 8 clients"},
		{"too many files", func(t *testing.T, dir string) {
			for i := range pack.MaxClientFiles {
				write(t, dir, fmt.Sprintf("%smany/%d.txt", client, i), "a")
			}
		}, "over 4096 files"},
		{"over the limit of a folder", func(t *testing.T, dir string) {
			for i := range pack.MaxClientBytes/pack.MaxClientFileBytes + 1 {
				glb(t, dir, fmt.Sprintf("%smodels/big-%d.glb", client, i), pack.MaxClientFileBytes)
			}
		}, "come to over"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if testing.Short() && (strings.HasPrefix(tc.name, "too many files") || strings.Contains(tc.name, "limit of a folder")) {
				t.Skip("writes or hashes a lot")
			}
			_, dir := testdata.DraughtsPack(t)
			tc.bad(t, dir)
			_, err := read(t, dir)
			if err == nil {
				t.Fatal("the pack was accepted")
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("refused with %q, which does not say %q", err, tc.says)
			}
		})
	}
}
