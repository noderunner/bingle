package pack

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The limits of a pack's clients' files (ADR 0147), enforced on what is
// really read. A client's folder is clients/<client>/, and each limit but
// MaxClients is per folder.
const (
	MaxClientFileBytes = 64 << 20  // one file
	MaxClientBytes     = 512 << 20 // every file of one client
	MaxClientFiles     = 4096      // files of one client
	MaxClients         = 8         // clients of one pack
	maxClientPath      = 200       // a path inside a client's folder, in bytes
)

// clientKind is a kind of file a client may have: the type it is sent as,
// and the bytes it starts with, or none for text.
type clientKind struct {
	contentType string
	signature   []byte
}

// clientKinds are the kinds of file a client may have, by extension: data,
// models, textures, pictures and sounds, and nothing that runs.
var clientKinds = map[string]clientKind{
	".toml": {"application/toml", nil},
	".json": {"application/json", nil},
	".txt":  {"text/plain; charset=utf-8", nil},
	".md":   {"text/markdown; charset=utf-8", nil},
	".glb":  {"model/gltf-binary", []byte("glTF")},
	".ktx2": {"image/ktx2", []byte("\xabKTX 20\xbb\r\n\x1a\n")},
	".png":  {"image/png", []byte("\x89PNG\r\n\x1a\n")},
	".jpg":  {"image/jpeg", []byte("\xff\xd8\xff")},
	".jpeg": {"image/jpeg", []byte("\xff\xd8\xff")},
	".ogg":  {"audio/ogg", []byte("OggS")},
}

// segmentForm is the form of each part of a client's path: one that any
// system can name a file by, in a URL as it is.
var segmentForm = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// readClients checks the folder of every client in the pack and hashes each
// file once. A client's files are never read again: they are served from the
// disk, through a root of their own that stays open with the pack.
func readClients(root *os.Root) (map[string]*catalog.ClientFiles, []*os.Root, error) {
	entries, err := fs.ReadDir(root.FS(), ClientsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	clients := map[string]*catalog.ClientFiles{}
	var roots []*os.Root
	fail := func(err error) (map[string]*catalog.ClientFiles, []*os.Root, error) {
		for _, r := range roots {
			_ = r.Close()
		}
		return nil, nil, err
	}
	for _, e := range entries {
		name := e.Name()
		at := ClientsDir + "/" + name
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case e.Type() == fs.ModeSymlink:
			return fail(fmt.Errorf("%q is a link: a pack holds plain files only", at))
		case !e.IsDir():
			return fail(fmt.Errorf("%q: %s holds a folder for each client, and no file of its own", at, ClientsDir))
		case !nameForm.MatchString(name) || len(name) > maxNameLen:
			return fail(fmt.Errorf("%q: a client is named in lower-case words joined by dashes, of at most %d characters", at, maxNameLen))
		case len(clients) == MaxClients:
			return fail(fmt.Errorf("%s: over %d clients", ClientsDir, MaxClients))
		}
		r, err := root.OpenRoot(at)
		if err != nil {
			return fail(err)
		}
		roots = append(roots, r)
		files, written, err := readClient(r.FS(), at)
		if err != nil {
			return fail(err)
		}
		files.FS = clientFS{root: r, at: at, files: files, written: written}
		clients[name] = files
	}
	if len(clients) == 0 {
		return nil, nil, nil
	}
	return clients, roots, nil
}

// readClient checks and hashes the files of the client whose folder (at,
// for errors) fsys is, and says when each was last written.
func readClient(fsys fs.FS, at string) (*catalog.ClientFiles, map[string]time.Time, error) {
	files := &catalog.ClientFiles{}
	written := map[string]time.Time{}
	seen := map[string]bool{}
	total := int64(0)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		where := at + "/" + name
		if strings.HasPrefix(path.Base(name), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !segmentForm.MatchString(path.Base(name)) {
			return fmt.Errorf("%q: a client's files and folders are named in letters, digits, dots, dashes and underscores", where)
		}
		if len(name) > maxClientPath {
			return fmt.Errorf("%q: a path in a client's folder has at most %d characters", where, maxClientPath)
		}
		if lower := strings.ToLower(name); seen[lower] {
			return fmt.Errorf("%q is in the pack twice", where)
		} else {
			seen[lower] = true
		}
		switch d.Type() {
		case fs.ModeDir:
			return nil
		case 0:
		case fs.ModeSymlink:
			return fmt.Errorf("%q is a link: a pack holds plain files only", where)
		default:
			return fmt.Errorf("%q is not a plain file", where)
		}
		if len(files.Files) == MaxClientFiles {
			return fmt.Errorf("%s: over %d files", at, MaxClientFiles)
		}
		kind, ok := clientKinds[strings.ToLower(path.Ext(name))]
		if !ok {
			return fmt.Errorf("%q: a client's files are TOML, JSON, text, Markdown, binary glTF, KTX2, PNG, JPEG and Ogg, and no other kind", where)
		}
		file, when, err := hashClientFile(fsys, name, kind, MaxClientFileBytes)
		if err != nil {
			return fmt.Errorf("%q %w", where, err)
		}
		written[name] = when
		if total += file.Size; total > MaxClientBytes {
			return fmt.Errorf("%s: its files come to over %d bytes", at, MaxClientBytes)
		}
		files.Files = append(files.Files, file)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(files.Files) == 0 {
		return nil, nil, fmt.Errorf("%s holds no files", at)
	}
	// WalkDir goes in lexical order of each folder's names, which is not
	// quite the order of the paths ("a-b/x" and "a/x").
	slices.SortFunc(files.Files, func(a, b catalog.ClientFile) int { return strings.Compare(a.Path, b.Path) })
	return files, written, nil
}

// clientBuffer is how much of a client's file is read at a time.
const clientBuffer = 64 << 10

// hashClientFile reads one file, no more than limit bytes of it whatever its
// size said when it was listed, holds it to its kind and hashes it, and says
// when it was last written. Only a piece of it is in memory at a time. Its
// error follows the file's name.
func hashClientFile(fsys fs.FS, name string, kind clientKind, limit int64) (catalog.ClientFile, time.Time, error) {
	file, err := hashClient(fsys, name, kind, limit)
	if err != nil {
		return catalog.ClientFile{}, time.Time{}, err
	}
	// When it was written is taken after it was read, so a file written to
	// while it was hashed is not served as the one hashed.
	info, err := fs.Stat(fsys, name)
	if err != nil || info.Size() != file.Size {
		return catalog.ClientFile{}, time.Time{}, errors.New("changed while it was read")
	}
	return file, info.ModTime(), nil
}

func hashClient(fsys fs.FS, name string, kind clientKind, limit int64) (catalog.ClientFile, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return catalog.ClientFile{}, fmt.Errorf("could not be read: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return catalog.ClientFile{}, errors.New("is not a plain file")
	}
	h := sha256.New()
	text := kind.signature == nil
	var head []byte
	buf := make([]byte, clientBuffer)
	carry, size := 0, int64(0)
	for {
		n, err := f.Read(buf[carry:])
		if n > 0 {
			if size += int64(n); size > limit {
				return catalog.ClientFile{}, fmt.Errorf("is over %d bytes", limit)
			}
			fresh := buf[carry : carry+n]
			h.Write(fresh)
			if len(head) < 16 {
				head = append(head, fresh[:min(16-len(head), len(fresh))]...)
			}
		}
		end := err == io.EOF
		if err != nil && !end {
			return catalog.ClientFile{}, fmt.Errorf("could not be read: %w", err)
		}
		if text {
			chunk := buf[:carry+n]
			cut := len(chunk)
			if !end {
				cut = unfinishedRune(chunk)
			}
			if !utf8.Valid(chunk[:cut]) || bytes.IndexByte(chunk[:cut], 0) >= 0 {
				return catalog.ClientFile{}, errors.New("is not text")
			}
			carry = copy(buf, chunk[cut:])
		}
		if end {
			break
		}
	}
	if !text {
		if !bytes.HasPrefix(head, kind.signature) {
			return catalog.ClientFile{}, fmt.Errorf("is not %s", kind.contentType)
		}
		// A binary glTF says its version and its length.
		if kind.contentType == "model/gltf-binary" && (len(head) < 12 || binary.LittleEndian.Uint32(head[4:]) != 2 || int64(binary.LittleEndian.Uint32(head[8:])) != size) {
			return catalog.ClientFile{}, errors.New("is not binary glTF 2.0 of its own length")
		}
	}
	return catalog.ClientFile{Path: name, Size: size, SHA256: hex.EncodeToString(h.Sum(nil)), ContentType: kind.contentType}, nil
}

// unfinishedRune returns where a rune that b ends in the middle of starts,
// or len(b) if it ends between runes.
func unfinishedRune(b []byte) int {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax+1; i-- {
		if utf8.RuneStart(b[i]) {
			if utf8.FullRune(b[i:]) {
				return len(b)
			}
			return i
		}
	}
	return len(b)
}

// ErrChanged is the error of opening a client's file that is no longer the
// one that was hashed: another size, or written to since.
var ErrChanged = errors.New("pack: the file changed after the pack was loaded")

// clientFS opens a client's files from the disk, through the root of its
// folder, and only those that were listed. A file that changed since is not
// opened, so what is served is what was checked.
type clientFS struct {
	root  *os.Root
	at    string
	files *catalog.ClientFiles
	// written is when each file was last written as it was hashed.
	written map[string]time.Time
}

// Open implements fs.FS. The file it returns is an *os.File.
func (c clientFS) Open(name string) (fs.File, error) {
	want, ok := c.files.File(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	f, err := c.root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != want.Size || !info.ModTime().Equal(c.written[name]) {
		_ = f.Close()
		return nil, &fs.PathError{Op: "open", Path: c.at + "/" + name, Err: ErrChanged}
	}
	return f, nil
}
