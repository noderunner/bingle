// Package clientbuilds offers the builds of the bingle-3d client that the
// operator keeps on the server's disk (ADR 0151): for each platform, the
// newest one this server's API can be used by.
//
// The directory (BINGLE_CLIENT_BUILDS) holds a folder for each version, named
// for it, with that release's archives as they were published:
//
//	<dir>/0.0.2/bingle-3d-windows-x86_64.zip
//	<dir>/0.0.2/bingle-3d-linux-x86_64.tar.gz
//	<dir>/0.0.2/release.json            (optional)
//
// It is read once, at start. The server fetches nothing and never looks
// inside an archive: it hashes each one it offers and sends it from the disk.
package clientbuilds

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MaxBuildBytes is the most one archive may be. A release's are about 60 MB.
const MaxBuildBytes = 1 << 30

// ManifestName is the file a release may come with that says which API its
// builds were made for: {"version": "0.0.3", "api": "1.17.0"}.
const ManifestName = "release.json"

// maxManifest is the most of a manifest that is read.
const maxManifest = 64 << 10

// ErrNotFound is the error of asking for a file that is not on offer.
var ErrNotFound = errors.New("clientbuilds: no such build")

// ErrChanged is the error of opening an archive that is no longer the one
// that was hashed: another size, or written to since.
var ErrChanged = errors.New("clientbuilds: the file changed after the server started")

// archiveForm is a release's archive: the platform it is for is in its name.
var archiveForm = regexp.MustCompile(`^bingle-3d-([a-z0-9]+(?:[_-][a-z0-9]+)*)\.(zip|tar\.gz)$`)

// versionForm is a version's folder: 0.0.2, or v0.0.2 as the release is tagged.
var versionForm = regexp.MustCompile(`^v?(\d{1,6})\.(\d{1,6})\.(\d{1,6})$`)

// Build is one archive on offer.
type Build struct {
	// Platform is what it runs on, as its file is named: "windows-x86_64",
	// "linux-x86_64", "steam-frame".
	Platform string `json:"platform"`
	// Version is the client's version.
	Version string `json:"version"`
	// File is the archive's name, and the last part of its address:
	// /api/client/files/{file}.
	File string `json:"file"`
	// Size is its length in bytes.
	Size int64 `json:"size"`
	// SHA256 is the hash of its contents, in lower-case hex, and its ETag.
	SHA256 string `json:"sha256"`
	// API is the version of the API the build was made for, when its release
	// says so; such a build is offered only by a server it can use.
	API string `json:"api,omitempty"`
}

// ContentType is the media type the archive is sent as.
func (b Build) ContentType() string {
	if strings.HasSuffix(b.File, ".zip") {
		return "application/zip"
	}
	return "application/gzip"
}

// Set is the builds a server offers. A nil Set offers nothing.
type Set struct {
	root   *os.Root
	builds []Build
	kept   map[string]kept
}

// kept is where an offered archive is, and when it was last written as it
// was hashed.
type kept struct {
	build   Build
	path    string
	written time.Time
}

// Load reads dir and chooses, for each platform, the newest version that has
// an archive for it and that a server speaking serverAPI can be used by. A
// folder or a file that is not as it should be is logged and left out; only
// a directory that cannot be read at all is an error.
func Load(dir, serverAPI string, log *slog.Logger) (*Set, error) {
	if log == nil {
		log = slog.Default()
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("clientbuilds: %w", err)
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("clientbuilds: %w", err)
	}
	type release struct {
		name    string
		version [3]int
		api     string
	}
	var releases []release
	for _, e := range entries {
		name := e.Name()
		v, ok := parseVersion(name)
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case !e.IsDir() || !ok:
			log.Warn("client builds: left out, not a folder named for a version", "name", name)
			continue
		}
		api, err := manifestAPI(root, name, v)
		if err != nil {
			log.Warn("client builds: version left out", "folder", name, "error", err)
			continue
		}
		if api != "" && !Compatible(api, serverAPI) {
			log.Info("client builds: version left out, it cannot use this server's API", "folder", name, "made_for", api, "server", serverAPI)
			continue
		}
		releases = append(releases, release{name: name, version: v, api: api})
	}
	// The newest first: the first archive found for a platform is its build.
	slices.SortFunc(releases, func(a, b release) int { return slices.Compare(b.version[:], a.version[:]) })

	set := &Set{root: root, kept: map[string]kept{}}
	taken := map[string]bool{}
	for _, rel := range releases {
		files, err := fs.ReadDir(root.FS(), rel.name)
		if err != nil {
			log.Warn("client builds: version left out", "folder", rel.name, "error", err)
			continue
		}
		for _, f := range files {
			m := archiveForm.FindStringSubmatch(f.Name())
			if m == nil || taken[m[1]] {
				continue
			}
			at := rel.name + "/" + f.Name()
			size, sum, written, err := hashFile(root, at)
			if err != nil {
				log.Warn("client builds: file left out", "file", at, "error", err)
				continue
			}
			taken[m[1]] = true
			b := Build{Platform: m[1], Version: fmt.Sprintf("%d.%d.%d", rel.version[0], rel.version[1], rel.version[2]), File: f.Name(), Size: size, SHA256: sum, API: rel.api}
			set.builds = append(set.builds, b)
			set.kept[b.File] = kept{build: b, path: at, written: written}
		}
	}
	slices.SortFunc(set.builds, func(a, b Build) int { return cmp.Compare(a.Platform, b.Platform) })
	return set, nil
}

// Builds are the builds on offer, in the order of their platforms.
func (s *Set) Builds() []Build {
	if s == nil {
		return nil
	}
	return slices.Clone(s.builds)
}

// Open opens the archive named file, to be sent as it is read. The caller
// closes it. ErrNotFound when it is not on offer, ErrChanged when it is no
// longer the file that was hashed.
func (s *Set) Open(file string) (Build, *os.File, error) {
	if s == nil {
		return Build{}, nil, ErrNotFound
	}
	k, ok := s.kept[file]
	if !ok {
		return Build{}, nil, ErrNotFound
	}
	f, err := s.root.Open(k.path)
	if err != nil {
		return Build{}, nil, fmt.Errorf("%w: %w", ErrChanged, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != k.build.Size || !info.ModTime().Equal(k.written) {
		_ = f.Close()
		return Build{}, nil, fmt.Errorf("%w: %s", ErrChanged, k.path)
	}
	return k.build, f, nil
}

// Close lets go of the directory.
func (s *Set) Close() error {
	if s == nil {
		return nil
	}
	return s.root.Close()
}

// Compatible reports whether a client made for the API version build can use
// a server that speaks server, by the client's own rule: the same major
// version, and a minor the server has reached.
func Compatible(build, server string) bool {
	b, ok := parseAPI(build)
	s, ok2 := parseAPI(server)
	return ok && ok2 && b[0] == s[0] && b[1] <= s[1]
}

// parseAPI reads an API version's major and minor: "1.17" or "1.17.0".
func parseAPI(v string) ([2]int, bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return [2]int{}, false
	}
	var out [2]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [2]int{}, false
		}
		if i < 2 {
			out[i] = n
		}
	}
	return out, true
}

func parseVersion(name string) ([3]int, bool) {
	m := versionForm.FindStringSubmatch(name)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

// manifestAPI is the API version a version's manifest says its builds were
// made for, or "" when the folder has no manifest or it does not say. A
// manifest that cannot be read, or is another version's, is an error: the
// folder is not what its name says.
func manifestAPI(root *os.Root, folder string, version [3]int) (string, error) {
	f, err := root.Open(folder + "/" + ManifestName)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	var m struct {
		Version string `json:"version"`
		API     string `json:"api"`
	}
	if err := json.NewDecoder(io.LimitReader(f, maxManifest)).Decode(&m); err != nil {
		return "", fmt.Errorf("%s: %w", ManifestName, err)
	}
	if m.Version != "" {
		if v, ok := parseVersion(m.Version); !ok || v != version {
			return "", fmt.Errorf("%s is version %q's", ManifestName, m.Version)
		}
	}
	if m.API != "" {
		if _, ok := parseAPI(m.API); !ok {
			return "", fmt.Errorf("%s: %q is not an API version", ManifestName, m.API)
		}
	}
	return m.API, nil
}

// hashFile reads a plain file of at most MaxBuildBytes and returns its size,
// its hash, and when it was last written.
func hashFile(root *os.Root, at string) (int64, string, time.Time, error) {
	if info, err := root.Lstat(at); err != nil || !info.Mode().IsRegular() {
		return 0, "", time.Time{}, errors.New("not a plain file")
	}
	f, err := root.Open(at)
	if err != nil {
		return 0, "", time.Time{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	size, err := io.Copy(h, io.LimitReader(f, MaxBuildBytes+1))
	if err != nil {
		return 0, "", time.Time{}, err
	}
	if size > MaxBuildBytes {
		return 0, "", time.Time{}, fmt.Errorf("over %d bytes", MaxBuildBytes)
	}
	// When it was written is taken after it was read, so a file written to
	// while it was hashed is not served as the one hashed.
	info, err := f.Stat()
	if err != nil || info.Size() != size {
		return 0, "", time.Time{}, errors.New("changed while it was read")
	}
	return size, hex.EncodeToString(h.Sum(nil)), info.ModTime(), nil
}
