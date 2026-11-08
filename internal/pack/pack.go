// Package pack reads a data pack: one game as a zip of data, which an admin
// installs on a running server (ADR 0128). A pack holds a definition, its look,
// its rules guide and pictures, and no code: a rule that needs Go belongs in a
// build (ADR 0127). A pack may come from anywhere, so Read treats it as hostile:
// nothing in it is trusted until every check here has passed.
//
// The layout, at the root of the zip:
//
//	game.yaml   the definition; its id is the pack's name
//	web.yaml    how the web page draws it (optional, ADR 0124)
//	...         the guide's Markdown and pictures, in folders if it likes
//
// Each version of a pack is a game of its own, named pack-name-version, on
// ADR 0098's terms: a game in progress keeps the rules it started with, and
// the family's name, pack-name, means the newest version.
package pack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/web/look"
)

// Prefix begins the id of every pack's game. A built-in game may not use it
// (tests/registry), so a pack can never shadow one.
const Prefix = "pack-"

// The files a pack is read from.
const (
	DefinitionFile = "game.yaml"
	LookFile       = look.FileName
)

// The limits of a pack. They are generous for a definition and its pictures,
// and small enough that a zip bomb stops at the first file over them.
const (
	MaxPackBytes  = 20 << 20 // the zip itself
	MaxFileBytes  = 8 << 20  // one file, unpacked
	MaxTotalBytes = 40 << 20 // every file, unpacked
	MaxFiles      = 300
	// MaxDefinitionBytes limits the definition with its includes and aliases
	// expanded: each of its variants is a whole copy of it.
	MaxDefinitionBytes = 1 << 20
	maxNameLen         = 40
)

// The trial game each of a pack's games is played before it is accepted.
const (
	trialPlies = 300
	trialTime  = 5 * time.Second
)

// Pack is a pack that has passed every check.
type Pack struct {
	// Name is the pack's name (game.yaml's id), Version its version.
	Name, Version string
	// Title is the game's name, for people.
	Title string
	// Family is pack-name, and Home the id of this version, pack-name-version.
	Family, Home string
	// Games lists the ids this version registers: Home and its variants.
	Games []string
	// SHA256 is the hash of the zip, for the audit log.
	SHA256 string
	// Module is what to register in a catalog.
	Module catalog.Module
}

var (
	nameForm = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	// A colour reaches a style attribute, so it may only ever be a colour: not
	// url(...), which would have a player's browser call somebody's server, and
	// not a semicolon and a rule of its own.
	colorForm = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{1,30}|(rgb|rgba|hsl|hsla)\([0-9., %/deg-]{1,40}\))$`)
	// What each kind of picture must really be (http.DetectContentType).
	pictureTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}
)

// Read checks the zip in data and returns the pack. Its error says what is
// wrong in words for whoever made the pack.
func Read(data []byte) (*Pack, error) {
	if len(data) > MaxPackBytes {
		return nil, fmt.Errorf("pack: the zip is %d bytes, over the limit of %d", len(data), MaxPackBytes)
	}
	files, err := unpack(data)
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	p, err := build(files)
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	sum := sha256.Sum256(data)
	p.SHA256 = hex.EncodeToString(sum[:])
	return p, nil
}

// unpack reads every file of the zip into memory, checked, and returns them as
// a file system. Nothing is ever written to disk, so a hostile path has nowhere
// to go, but it is refused all the same.
func unpack(data []byte) (fs.FS, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a zip: %w", err)
	}
	if len(zr.File) > MaxFiles {
		return nil, fmt.Errorf("%d entries, over the limit of %d", len(zr.File), MaxFiles)
	}
	files := memFS{}
	seen := map[string]bool{}
	total := int64(0)
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") {
			if f.Mode().Type() != fs.ModeDir && !f.Mode().IsRegular() {
				return nil, fmt.Errorf("%q is not a plain folder", name)
			}
			continue
		}
		if !fs.ValidPath(name) || strings.Contains(name, `\`) {
			return nil, fmt.Errorf("%q is not a path inside the pack", name)
		}
		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf("%q is not a plain file (a link, perhaps)", name)
		}
		if lower := strings.ToLower(name); seen[lower] {
			return nil, fmt.Errorf("%q is in the pack twice", name)
		} else {
			seen[lower] = true
		}
		// The size a zip claims is not to be believed: read one byte past the limit.
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%q: %w", name, err)
		}
		content, err := io.ReadAll(io.LimitReader(rc, MaxFileBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%q: %w", name, err)
		}
		if len(content) > MaxFileBytes {
			return nil, fmt.Errorf("%q unpacks to over %d bytes", name, MaxFileBytes)
		}
		if total += int64(len(content)); total > MaxTotalBytes {
			return nil, fmt.Errorf("the pack unpacks to over %d bytes", MaxTotalBytes)
		}
		if err := checkContent(name, content); err != nil {
			return nil, err
		}
		files[name] = content
	}
	return files, nil
}

// checkContent holds a file to its extension: a pack carries text and pictures
// and nothing else, and a picture has to be one.
func checkContent(name string, content []byte) error {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".yaml", ".yml", ".md":
		if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
			return fmt.Errorf("%q is not text", name)
		}
		return nil
	case ".svg":
		if !utf8.Valid(content) || !bytes.Contains(content, []byte("<svg")) {
			return fmt.Errorf("%q is not an SVG picture", name)
		}
		return nil
	}
	want, ok := pictureTypes[ext]
	if !ok {
		return fmt.Errorf("%q: a pack holds YAML, Markdown and pictures (PNG, JPEG, GIF, WebP, SVG), and no other kind of file", name)
	}
	if got := http.DetectContentType(content); got != want {
		return fmt.Errorf("%q is %s, not %s", name, got, want)
	}
	return nil
}

// build reads the definition and the look from the files, names them for this
// version, compiles them and plays them.
func build(files fs.FS) (*Pack, error) {
	raw, err := dsl.Load(files, DefinitionFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no %s at the root of the zip", DefinitionFile)
		}
		return nil, fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	if len(raw) > MaxDefinitionBytes {
		return nil, fmt.Errorf("%s comes to %d bytes, over the limit of %d", DefinitionFile, len(raw), MaxDefinitionBytes)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	name, _ := doc["id"].(string)
	if !nameForm.MatchString(name) || len(name) > maxNameLen {
		return nil, fmt.Errorf("the id %q has to be lower-case words joined by dashes, of at most %d characters", name, maxNameLen)
	}
	if strings.HasPrefix(name, Prefix) {
		return nil, fmt.Errorf("the id %q has to be the game's own name: the server adds %q", name, Prefix)
	}
	version, _ := doc["version"].(string)
	if version == "" {
		return nil, fmt.Errorf("%s has no version", DefinitionFile)
	}
	for _, key := range []string{"family", "prototype"} {
		if _, set := doc[key]; set {
			return nil, fmt.Errorf("%s sets %q, which the server decides for a pack", DefinitionFile, key)
		}
	}
	p := &Pack{Name: name, Version: version, Family: Prefix + name, Home: Prefix + name + "-" + version}
	p.Title, _ = doc["name"].(string)

	// Each version is a game of its own (ADR 0098), and the name of a variant
	// says the version too.
	doc["id"], doc["family"], doc["prototype"] = p.Home, p.Family, true
	variantID := func(id string) (string, error) {
		rest, ok := strings.CutPrefix(id, name+"-")
		if !ok || !nameForm.MatchString(rest) {
			return "", fmt.Errorf("the variant %q has to be named %q and a word", id, name)
		}
		return p.Home + "-" + rest, nil
	}
	if vs, ok := doc["variants"].([]any); ok {
		for _, v := range vs {
			m, _ := v.(map[string]any)
			id, _ := m["id"].(string)
			if m["id"], err = variantID(id); err != nil {
				return nil, err
			}
		}
	}
	definition, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	expanded, err := dsl.ExpandVariants(definition)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	// Each game as its variants' patches leave it: a patch is as hostile as the rest.
	for _, one := range expanded {
		if err := checkDefinition(one, p); err != nil {
			return nil, err
		}
	}

	web, err := readLook(files, name, p, variantID)
	if err != nil {
		return nil, err
	}

	p.Module = catalog.Module{ID: p.Home, Definition: definition, Assets: files, Web: web}
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(p.Module); err != nil {
		return nil, err
	}
	games := map[string]*catalog.CompiledGame{}
	for _, one := range expanded {
		def, err := dsl.Parse(one)
		if err != nil {
			return nil, err
		}
		cg, ok := cat.Get(def.ID)
		if !ok {
			return nil, fmt.Errorf("the game %q did not compile", def.ID)
		}
		games[def.ID] = cg
		p.Games = append(p.Games, def.ID)
	}
	if web != nil {
		if err := look.Check(web, p.Home, games); err != nil {
			return nil, fmt.Errorf("%s: %w", LookFile, err)
		}
	}
	for _, id := range p.Games {
		if err := trial(games[id]); err != nil {
			return nil, fmt.Errorf("the game %q: %w", id, err)
		}
	}
	return p, nil
}

// checkDefinition refuses what a pack may not ask for: anything that needs Go
// (ADR 0127), and anything but a colour where a colour goes.
func checkDefinition(doc []byte, p *Pack) error {
	def, err := dsl.Parse(doc)
	if err != nil {
		return fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	switch {
	case def.Engine != "":
		return fmt.Errorf("%s: %q says engine %q, and rules in Go need a build of their own: a pack is data", DefinitionFile, def.ID, def.Engine)
	case def.Callbacks != (dsl.CallbackRefs{}):
		return fmt.Errorf("%s: %q names callbacks, which are Go: a pack is data", DefinitionFile, def.ID)
	case def.WinCondition.Type == "callback" || def.WinCondition.Type == "ruleset":
		return fmt.Errorf("%s: %q has a win condition of type %q, which is Go: a pack is data", DefinitionFile, def.ID, def.WinCondition.Type)
	case len(def.Options) > 0:
		return fmt.Errorf("%s: %q has setup options, which need a Go ruleset to derive a game from: use variants", DefinitionFile, def.ID)
	case def.Family != p.Family || !def.Prototype || def.Version != p.Version:
		return fmt.Errorf("%s: a variant of %q changes the family, the version or prototype, which the server decides for a pack", DefinitionFile, p.Name)
	}
	var tree any
	if err := json.Unmarshal(doc, &tree); err != nil {
		return err
	}
	return checkColors(DefinitionFile, tree)
}

// readLook reads web.yaml, refuses what would run in a player's browser, and
// renames its variants for this version.
func readLook(files fs.FS, name string, p *Pack, variantID func(string) (string, error)) ([]byte, error) {
	web, err := look.LoadFile(files, LookFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", LookFile, err)
	}
	if web == nil {
		return nil, nil
	}
	var tree map[string]any
	if err := json.Unmarshal(web, &tree); err != nil {
		return nil, fmt.Errorf("%s: %w", LookFile, err)
	}
	if err := checkColors(LookFile, tree); err != nil {
		return nil, err
	}
	refuse := func(m map[string]any) error {
		for _, key := range []string{"script", "style"} {
			if _, set := m[key]; set {
				return fmt.Errorf("%s names a %s: a pack runs nothing in a player's browser, and its look is data (ADR 0128)", LookFile, key)
			}
		}
		return nil
	}
	if err := refuse(tree); err != nil {
		return nil, err
	}
	if vs, ok := tree["variants"].(map[string]any); ok {
		renamed := make(map[string]any, len(vs))
		for id, v := range vs {
			m, _ := v.(map[string]any)
			if err := refuse(m); err != nil {
				return nil, err
			}
			to, err := variantID(id)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", LookFile, err)
			}
			renamed[to] = v
		}
		tree["variants"] = renamed
	}
	return json.Marshal(tree)
}

// checkColors holds every "color" in a document to the form of a colour.
func checkColors(file string, node any) error {
	switch v := node.(type) {
	case map[string]any:
		for key, value := range v {
			if s, ok := value.(string); ok && (key == "color" || strings.HasSuffix(key, "_color")) && !colorForm.MatchString(s) {
				return fmt.Errorf("%s: %q is not a colour", file, s)
			}
			if err := checkColors(file, value); err != nil {
				return err
			}
		}
	case []any:
		for _, value := range v {
			if err := checkColors(file, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// trial plays one game at random, so a definition that compiles and then
// panics, or takes far too long to think, stops here and not in a player's game.
func trial(cg *catalog.CompiledGame) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("playing it failed: %v", r)
		}
	}()
	st, seat := state.InitialState(cg), state.FirstSeat(cg)
	rng := rand.New(rand.NewSource(1))
	deadline := time.Now().Add(trialTime)
	for range trialPlies {
		if time.Now().After(deadline) {
			return fmt.Errorf("a trial game took over %s: its moves are too slow to work out", trialTime)
		}
		legal := state.LegalSelections(cg, st, seat)
		if len(legal) == 0 {
			return nil
		}
		res, err := state.Apply(cg, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			return fmt.Errorf("a move it offered was refused: %w", err)
		}
		if res.GameEnded {
			return nil
		}
		st, seat = res.NewState, res.NextImpulseSeat
	}
	return nil
}
