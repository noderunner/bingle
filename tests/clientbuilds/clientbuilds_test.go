// Package clientbuilds_test checks which builds of the bingle-3d client a
// server offers from the directory its operator keeps (ADR 0151).
package clientbuilds_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/clientbuilds"
)

const (
	windows = "bingle-3d-windows-x86_64.zip"
	linux   = "bingle-3d-linux-x86_64.tar.gz"
	frame   = "bingle-3d-steam-frame.tar.gz"
)

// put writes files into dir, making their folders: a path and its contents.
func put(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		at := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func load(t *testing.T, dir, api string) *clientbuilds.Set {
	t.Helper()
	set, err := clientbuilds.Load(dir, api, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = set.Close() })
	return set
}

// offered is what a set offers: each platform's version.
func offered(set *clientbuilds.Set) map[string]string {
	out := map[string]string{}
	for _, b := range set.Builds() {
		out[b.Platform] = b.Version
	}
	return out
}

func same(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestLoad_EachPlatformGetsTheNewestVersionThatHasIt(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, map[string]string{
		"0.0.2/" + windows: "w2", "0.0.2/" + linux: "l2", "0.0.2/" + frame: "f2",
		"0.0.10/" + windows: "w10", "0.0.10/" + linux: "l10",
		"v0.1.0/" + linux: "l-0.1.0",
		// None of these is a version's archive.
		"0.1.0/notes.txt": "x", "0.1.0/bingle-3d-linux-x86_64.tar.gz.part": "x", "0.1.0/bingle-3d-.zip": "x",
		"latest/" + windows: "x", "1.0/" + windows: "x", "9.9.9": "a file, not a folder", ".hidden/" + windows: "x",
	})
	set := load(t, dir, "1.17.0")
	want := map[string]string{"windows-x86_64": "0.0.10", "linux-x86_64": "0.1.0", "steam-frame": "0.0.2"}
	if got := offered(set); !same(got, want) {
		t.Fatalf("offered %v, want %v", got, want)
	}
	builds := set.Builds()
	if builds[0].Platform != "linux-x86_64" || builds[1].Platform != "steam-frame" || builds[2].Platform != "windows-x86_64" {
		t.Errorf("not in the order of their platforms: %+v", builds)
	}
	sum := sha256.Sum256([]byte("l-0.1.0"))
	if b := builds[0]; b.File != linux || b.Size != 7 || b.SHA256 != hex.EncodeToString(sum[:]) || b.API != "" || b.ContentType() != "application/gzip" {
		t.Errorf("the Linux build: %+v", b)
	}
	if b := builds[2]; b.ContentType() != "application/zip" {
		t.Errorf("the Windows build is sent as %s", b.ContentType())
	}
}

func TestLoad_AReleaseThatSaysItsAPIIsOfferedOnlyByAServerItCanUse(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, map[string]string{
		// No manifest: offered as it is.
		"0.0.2/" + windows: "w2", "0.0.2/" + linux: "l2", "0.0.2/" + frame: "f2",
		// Made for this server's API, or an earlier minor of it.
		"0.0.3/" + windows: "w3", "0.0.3/" + linux: "l3", "0.0.3/release.json": `{"version": "0.0.3", "api": "1.16.0"}`,
		// Uses what a later minor added: this server is too old for it.
		"0.0.4/" + windows: "w4", "0.0.4/release.json": `{"version": "0.0.4", "api": "1.18.0"}`,
		// Another major.
		"0.0.5/" + windows: "w5", "0.0.5/release.json": `{"version": "0.0.5", "api": "2.0.0"}`,
		// Not what its folder says, and not a manifest at all.
		"0.0.6/" + windows: "w6", "0.0.6/release.json": `{"version": "0.0.5", "api": "1.17.0"}`,
		"0.0.7/" + windows: "w7", "0.0.7/release.json": `not json`,
		"0.0.8/" + windows: "w8", "0.0.8/release.json": `{"version": "0.0.8", "api": "soon"}`,
	})
	set := load(t, dir, "1.17.0")
	want := map[string]string{"windows-x86_64": "0.0.3", "linux-x86_64": "0.0.3", "steam-frame": "0.0.2"}
	if got := offered(set); !same(got, want) {
		t.Fatalf("offered %v, want %v", got, want)
	}
	for _, b := range set.Builds() {
		if wantAPI := map[string]string{"0.0.3": "1.16.0"}[b.Version]; b.API != wantAPI {
			t.Errorf("%s %s says it was made for %q, want %q", b.Platform, b.Version, b.API, wantAPI)
		}
	}
	// A server that has moved on offers the newer one.
	if got := offered(load(t, dir, "1.18.2")); got["windows-x86_64"] != "0.0.4" {
		t.Errorf("a server of API 1.18 offers Windows %s", got["windows-x86_64"])
	}
}

func TestCompatible_IsTheClientsOwnRule(t *testing.T) {
	for _, c := range []struct {
		build, server string
		want          bool
	}{
		{"1.17.0", "1.17.0", true}, {"1.17.0", "1.17.5", true}, {"1.17.3", "1.17.0", true}, {"1.5", "1.17.0", true},
		{"1.18.0", "1.17.0", false}, {"2.0.0", "1.17.0", false}, {"1.17.0", "2.0.0", false},
		{"", "1.17.0", false}, {"1", "1.17.0", false}, {"1.x", "1.17.0", false}, {"1.17.0", "", false},
	} {
		if got := clientbuilds.Compatible(c.build, c.server); got != c.want {
			t.Errorf("a build for %q on a server of %q: %v", c.build, c.server, got)
		}
	}
}

func TestLoad_ALinkIsNotFollowedAndAnEmptyDirectoryOffersNothing(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	put(t, outside, map[string]string{"secret": "not a build"})
	put(t, dir, map[string]string{"0.0.2/" + linux: "l2"})
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "0.0.2", windows)); err != nil {
		t.Skip("no links here:", err)
	}
	if got := offered(load(t, dir, "1.17.0")); !same(got, map[string]string{"linux-x86_64": "0.0.2"}) {
		t.Errorf("offered %v", got)
	}
	empty := load(t, t.TempDir(), "1.17.0")
	if len(empty.Builds()) != 0 {
		t.Errorf("an empty directory offers %v", empty.Builds())
	}
	if _, err := clientbuilds.Load(filepath.Join(dir, "nowhere"), "1.17.0", nil); err == nil {
		t.Error("a directory that is not there loaded")
	}
	var none *clientbuilds.Set
	if none.Builds() != nil || none.Close() != nil {
		t.Error("a nil set is not an empty one")
	}
	if _, _, err := none.Open(linux); !errors.Is(err, clientbuilds.ErrNotFound) {
		t.Errorf("opening from a nil set: %v", err)
	}
}

func TestOpen_SendsTheFileThatWasHashedAndNoOther(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, map[string]string{"0.0.2/" + linux: "the archive", "0.0.1/" + windows: "older"})
	set := load(t, dir, "1.17.0")
	b, f, err := set.Open(linux)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(f)
	_ = f.Close()
	if string(body) != "the archive" || b.Version != "0.0.2" {
		t.Errorf("opened %q of %+v", body, b)
	}
	for _, name := range []string{"0.0.2/" + linux, "../0.0.2/" + linux, frame, "release.json", ""} {
		if _, _, err := set.Open(name); !errors.Is(err, clientbuilds.ErrNotFound) {
			t.Errorf("opening %q: %v", name, err)
		}
	}
	// Written to after the server started: it is not the file that was listed.
	at := filepath.Join(dir, "0.0.2", linux)
	if err := os.WriteFile(at, []byte("another one"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	_ = os.Chtimes(at, later, later)
	if _, _, err := set.Open(linux); !errors.Is(err, clientbuilds.ErrChanged) {
		t.Errorf("opening a file that changed: %v", err)
	}
}
