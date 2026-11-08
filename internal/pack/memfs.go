package pack

import (
	"bytes"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// memFS is the unpacked files of a pack, by path.
type memFS map[string][]byte

// Open implements fs.FS.
func (m memFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if content, ok := m[name]; ok {
		return &memFile{info: memInfo{name: path.Base(name), size: int64(len(content))}, r: bytes.NewReader(content)}, nil
	}
	prefix := name + "/"
	if name == "." {
		prefix = ""
	}
	names := map[string]bool{}
	for file := range m {
		if rest, ok := strings.CutPrefix(file, prefix); ok {
			first, _, _ := strings.Cut(rest, "/")
			names[first] = true
		}
	}
	if len(names) == 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	var entries []fs.DirEntry
	for entry := range names {
		_, isFile := m[prefix+entry]
		size := int64(len(m[prefix+entry]))
		entries = append(entries, fs.FileInfoToDirEntry(memInfo{name: entry, size: size, dir: !isFile}))
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return &memDir{info: memInfo{name: path.Base(name), dir: true}, entries: entries}, nil
}

type memInfo struct {
	name string
	size int64
	dir  bool
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return i.size }
func (i memInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.dir }
func (i memInfo) Sys() any           { return nil }

type memFile struct {
	info memInfo
	r    *bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Read(b []byte) (int, error) { return f.r.Read(b) }
func (f *memFile) Close() error               { return nil }

type memDir struct {
	info    memInfo
	entries []fs.DirEntry
	at      int
}

func (d *memDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *memDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: fs.ErrInvalid}
}
func (d *memDir) Close() error { return nil }
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.at:]
	if n <= 0 {
		d.at = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	d.at += n
	return rest[:n], nil
}
