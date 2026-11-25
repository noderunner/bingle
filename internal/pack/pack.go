// Package pack reads install packs (ADR 0147): a game whose rules are a
// WebAssembly module, as a directory that a server named in BINGLE_PACKS
// reads once at start-up. A pack is meant to be handed from one server to
// another, so Read treats one as hostile, wherever it came from: nothing in
// it is offered until every check here has passed, and what is served is
// what was checked.
//
// The layout:
//
//	manifest.yaml   name, version, abi, private
//	game.yaml       the definition (engine: ruleset), and its variants
//	web.yaml        optional: how the web page draws it (ADR 0124)
//	rules.wasm      the rules (package wasmrules)
//	guide/          the guide's Markdown and pictures, which the definition
//	                and the look name by their paths inside it
//	clients/        optional: every other client's files, not read here
//
// Each version of a pack is a game of its own, pack-name-version, on ADR
// 0098's terms, and a prototype, so it is not rated.
package pack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/internal/web/look"
)

// Prefix begins the id of every pack's game. A built-in game may not use it
// (tests/registry), so a pack can never shadow one.
const Prefix = "pack-"

// The files and folders of a pack.
const (
	ManifestFile   = "manifest.yaml"
	DefinitionFile = "game.yaml"
	LookFile       = look.FileName
	RulesFile      = "rules.wasm"
	GuideDir       = "guide"
	// ClientsDir holds the files of clients other than the web page. It is
	// left alone here: listing and serving it is phase 5 of ADR 0147.
	ClientsDir = "clients"
)

// The limits of a pack, enforced on what is really read.
const (
	MaxFileBytes  = 8 << 20  // one file of the pack but its rules
	MaxRulesBytes = 64 << 20 // rules.wasm
	MaxTotalBytes = 40 << 20 // every file but rules.wasm
	MaxFiles      = 300
	// MaxDefinitionBytes limits the definition with its aliases expanded
	// (dsl.MaxVariants limits its variants).
	MaxDefinitionBytes = 1 << 20
	maxNameLen         = 40
)

// The trial game each of a pack's games is played before it is offered.
const (
	trialPlies = 300
	trialTime  = 10 * time.Second
	trialSeed  = 0x5eed_0f_7e57
)

// Manifest is manifest.yaml.
type Manifest struct {
	// Name is the pack's name: lower-case words joined by dashes.
	Name string `json:"name"`
	// Version is its version, major.minor.patch.
	Version string `json:"version"`
	// ABI is the version of the calls its rules speak (wasmrules, ADR 0147).
	ABI *int `json:"abi"`
	// Private marks a commercial game as the overlay's are (ADR 0140).
	Private bool `json:"private,omitempty"`
}

// Pack is a pack that has passed every check.
type Pack struct {
	// Dir is the directory it was read from.
	Dir string
	// Name and Version are the manifest's, Title the game's name for people.
	Name, Version, Title string
	// Family is pack-name, and Home the id of this version, pack-name-version.
	Family, Home string
	// Games are the ids it registers: Home and its variants, in order.
	Games []string
	// Module is what to register in a catalog.
	Module catalog.Module

	rules *wasmrules.Module
}

// Close throws its rules' instances away.
func (p *Pack) Close(ctx context.Context) error { return p.rules.Close(ctx) }

var (
	nameForm    = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	wordForm    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`) // a variant's, after the pack's name
	versionForm = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)
	// A colour reaches a style attribute, so it may only ever be a colour: not
	// url(...), which would have a player's browser call somebody's server, and
	// not a semicolon and a rule of its own.
	colorForm = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{1,30}|(rgb|rgba|hsl|hsla)\([0-9., %/deg-]{1,40}\))$`)
	// What each kind of picture must really be (http.DetectContentType).
	pictureTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}
)

// Load reads every pack in dir, each a directory of its own. A pack that
// fails a check is logged with its reason and left out, and so is anything
// in dir that is not a directory: the server starts without them. Only a dir
// that cannot be read at all is an error.
func Load(ctx context.Context, rt *wasmrules.Runtime, dir string, log *slog.Logger) ([]*Pack, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("pack: the packs directory: %w", err)
	}
	var packs []*Pack
	loaded := map[string]string{} // Home → the directory it came from
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		at := filepath.Join(dir, e.Name())
		// The directory is the operator's, so a link to a pack is followed.
		if info, err := os.Stat(at); err != nil || !info.IsDir() {
			log.Error("pack: left out: not a directory", "dir", at)
			continue
		}
		start := time.Now()
		p, err := Read(ctx, rt, at)
		if err == nil {
			if from, twice := loaded[p.Home]; twice {
				err = fmt.Errorf("pack: %s %s was loaded already, from %s", p.Name, p.Version, from)
				_ = p.Close(ctx)
			}
		}
		if err != nil {
			log.Error("pack: left out", "dir", at, "error", err)
			continue
		}
		loaded[p.Home] = at
		packs = append(packs, p)
		log.Info("pack: loaded", "dir", at, "games", p.Games, "private", p.Module.Private, "took", time.Since(start).Round(time.Millisecond))
	}
	return packs, nil
}

// Read checks the pack in dir and returns it. Its error says what is wrong,
// in words for whoever made the pack.
func Read(ctx context.Context, rt *wasmrules.Runtime, dir string) (*Pack, error) {
	// Every file is read through a root confined to the pack, so neither a
	// path nor a link can reach outside it.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	defer func() { _ = root.Close() }()
	files, wasm, err := readFiles(root.FS())
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	p, err := build(ctx, rt, files, wasm)
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	p.Dir = dir
	return p, nil
}

// readFiles reads and checks every file of the pack but its clients' (which
// are left alone), and returns them with the rules apart.
func readFiles(fsys fs.FS) (memFS, []byte, error) {
	files := memFS{}
	var wasm []byte
	seen := map[string]bool{}
	count, total := 0, int64(0)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		// A file a system keeps beside others (.DS_Store, .git) is not the
		// pack's, and is neither read nor served.
		if strings.HasPrefix(path.Base(name), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if lower := strings.ToLower(name); seen[lower] {
			return fmt.Errorf("%q is in the pack twice", name)
		} else {
			seen[lower] = true
		}
		top, _, nested := strings.Cut(name, "/")
		switch d.Type() {
		case fs.ModeDir:
			switch {
			case name == ClientsDir:
				return fs.SkipDir
			case top == GuideDir:
				return nil
			}
			return fmt.Errorf("%q: a pack holds %s and %s and no other folder", name, GuideDir, ClientsDir)
		case 0:
		case fs.ModeSymlink:
			return fmt.Errorf("%q is a link: a pack holds plain files only", name)
		default:
			return fmt.Errorf("%q is not a plain file", name)
		}
		switch {
		case top == GuideDir:
		case nested:
			return fmt.Errorf("%q: a pack holds %s and %s and no other folder", name, GuideDir, ClientsDir)
		case name == ClientsDir:
			return fmt.Errorf("%q is a file, where a pack keeps a folder of its clients' files", name)
		case name != ManifestFile && name != DefinitionFile && name != LookFile && name != RulesFile:
			return fmt.Errorf("%q: a pack holds %s, %s, %s and %s beside its folders, and no other file", name, ManifestFile, DefinitionFile, LookFile, RulesFile)
		}
		if count++; count > MaxFiles {
			return fmt.Errorf("over %d files", MaxFiles)
		}
		limit := int64(MaxFileBytes)
		if name == RulesFile {
			limit = MaxRulesBytes
		}
		content, err := readLimited(fsys, name, limit)
		if err != nil {
			return err
		}
		if name == RulesFile {
			wasm = content
			return nil
		}
		if total += int64(len(content)); total > MaxTotalBytes {
			return fmt.Errorf("its files come to over %d bytes", MaxTotalBytes)
		}
		if err := checkContent(name, content); err != nil {
			return err
		}
		files[name] = content
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if wasm == nil {
		return nil, nil, fmt.Errorf("no %s: a pack's rules are a module", RulesFile)
	}
	return files, wasm, nil
}

// readLimited reads one file, and no more than limit bytes of it whatever
// its size said when it was listed.
func readLimited(fsys fs.FS, name string, limit int64) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	// It may have changed since it was listed.
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a plain file", name)
	}
	content, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%q: %w", name, err)
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("%q is over %d bytes", name, limit)
	}
	return content, nil
}

// checkContent holds a file to its kind: a pack's own files are text and
// pictures, and a picture has to be one.
func checkContent(name string, content []byte) error {
	ext := strings.ToLower(path.Ext(name))
	isText := utf8.Valid(content) && bytes.IndexByte(content, 0) < 0
	if !strings.HasPrefix(name, GuideDir+"/") {
		// The manifest, the definition and the look.
		if !isText {
			return fmt.Errorf("%q is not text", name)
		}
		return nil
	}
	switch ext {
	case ".md":
		if !isText {
			return fmt.Errorf("%q is not text", name)
		}
		return nil
	case ".svg":
		if !isText || !bytes.Contains(content, []byte("<svg")) {
			return fmt.Errorf("%q is not an SVG picture", name)
		}
		return nil
	}
	want, ok := pictureTypes[ext]
	if !ok {
		return fmt.Errorf("%q: a guide holds Markdown and pictures (PNG, JPEG, GIF, WebP, SVG), and no other kind of file", name)
	}
	if got := http.DetectContentType(content); got != want {
		return fmt.Errorf("%q is %s, not %s", name, got, want)
	}
	return nil
}

// readManifest reads and checks manifest.yaml.
func readManifest(files fs.FS) (*Manifest, error) {
	raw, err := dsl.LoadAs(files, ManifestFile, reflect.TypeFor[Manifest]())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no %s", ManifestFile)
		}
		return nil, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	switch {
	case !nameForm.MatchString(m.Name) || len(m.Name) > maxNameLen:
		return nil, fmt.Errorf("%s: the name %q has to be lower-case words joined by dashes, of at most %d characters", ManifestFile, m.Name, maxNameLen)
	case strings.HasPrefix(m.Name, Prefix):
		return nil, fmt.Errorf("%s: the name %q has to be the game's own: the server adds %q", ManifestFile, m.Name, Prefix)
	case !versionForm.MatchString(m.Version):
		return nil, fmt.Errorf("%s: the version %q has to be major.minor.patch", ManifestFile, m.Version)
	case m.ABI == nil:
		return nil, fmt.Errorf("%s: no abi: say which version of the calls its rules speak (%d)", ManifestFile, guest.ABI)
	case *m.ABI != guest.ABI:
		return nil, fmt.Errorf("%s: its rules speak ABI %d, and this server ABI %d", ManifestFile, *m.ABI, guest.ABI)
	}
	return &m, nil
}

// build reads the definition and the look, names them for this version,
// loads the rules, compiles every game and plays it.
func build(ctx context.Context, rt *wasmrules.Runtime, files memFS, wasm []byte) (*Pack, error) {
	m, err := readManifest(files)
	if err != nil {
		return nil, err
	}
	p := &Pack{Name: m.Name, Version: m.Version, Family: Prefix + m.Name, Home: Prefix + m.Name + "-" + m.Version}
	definition, expanded, err := readDefinition(files, p)
	if err != nil {
		return nil, err
	}
	web, err := readLook(files, p)
	if err != nil {
		return nil, err
	}

	p.rules, err = rt.Load(ctx, p.Name+" "+p.Version, wasm)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = p.rules.Close(ctx)
		}
	}()
	// The guide is the game's files, as a built-in game's guide folder is:
	// what a definition's guide and a look's pictures name is a path in it.
	// Nothing else of the pack is served.
	guide := memFS{}
	for name, content := range files {
		if rest, ok := strings.CutPrefix(name, GuideDir+"/"); ok {
			guide[rest] = content
		}
	}
	p.Module = catalog.Module{ID: p.Home, Definition: definition, Rules: p.rules.Rules(), Assets: guide, Web: web, Private: m.Private}

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
		cg, found := cat.Get(def.ID)
		if !found {
			return nil, fmt.Errorf("the game %q did not compile", def.ID)
		}
		games[def.ID] = cg
		p.Games = append(p.Games, def.ID)
	}
	p.Title = games[p.Home].Definition.Name
	if web != nil {
		if err := look.Check(web, p.Home, games); err != nil {
			return nil, fmt.Errorf("%s: %w", LookFile, err)
		}
	}
	for _, id := range p.Games {
		if err := trial(games[id], p.rules); err != nil {
			return nil, fmt.Errorf("the game %q: %w", id, err)
		}
	}
	ok = true
	return p, nil
}

// readDefinition reads game.yaml and names it and its variants for this
// version. It returns the definition and each game as its variant's patch
// leaves it, each checked: a patch is as hostile as the rest.
func readDefinition(files fs.FS, p *Pack) ([]byte, [][]byte, error) {
	raw, err := dsl.Load(files, DefinitionFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("no %s", DefinitionFile)
		}
		return nil, nil, fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	if len(raw) > MaxDefinitionBytes {
		return nil, nil, fmt.Errorf("%s comes to %d bytes, over the limit of %d", DefinitionFile, len(raw), MaxDefinitionBytes)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	for _, key := range []string{"id", "family", "version", "prototype"} {
		if _, set := doc[key]; set {
			return nil, nil, fmt.Errorf("%s sets %q, which the server sets from %s", DefinitionFile, key, ManifestFile)
		}
	}
	// Each version is a game of its own (ADR 0098), and a variant's id says
	// the version too.
	doc["id"], doc["family"], doc["version"], doc["prototype"] = p.Home, p.Family, p.Version, true
	if vs, ok := doc["variants"].([]any); ok {
		if len(vs) > dsl.MaxVariants {
			return nil, nil, fmt.Errorf("%s: %d variants, over the limit of %d", DefinitionFile, len(vs), dsl.MaxVariants)
		}
		for _, v := range vs {
			m, _ := v.(map[string]any)
			id, _ := m["id"].(string)
			if m["id"], err = p.variantID(id); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", DefinitionFile, err)
			}
		}
	}
	definition, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, err
	}
	expanded, err := dsl.ExpandVariants(definition)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	for _, one := range expanded {
		if err := checkDefinition(one, p); err != nil {
			return nil, nil, err
		}
	}
	return definition, expanded, nil
}

// variantID is the id of the variant named-word in this version.
func (p *Pack) variantID(id string) (string, error) {
	word, ok := strings.CutPrefix(id, p.Name+"-")
	if !ok || !wordForm.MatchString(word) || len(word) > maxNameLen {
		return "", fmt.Errorf("the variant %q has to be named %q, a dash and a word", id, p.Name)
	}
	return p.Home + "-" + word, nil
}

// checkDefinition refuses what a pack may not ask for: rules that are not
// its module's, and anything but a colour where a colour goes.
func checkDefinition(doc []byte, p *Pack) error {
	def, err := dsl.Parse(doc)
	if err != nil {
		return fmt.Errorf("%s: %w", DefinitionFile, err)
	}
	switch {
	case def.Engine != "ruleset":
		return fmt.Errorf("%s: %q has to say engine: ruleset, since its rules are the pack's module", DefinitionFile, def.ID)
	case def.Callbacks != (dsl.CallbackRefs{}):
		return fmt.Errorf("%s: %q names callbacks, which are Go in a build of the server", DefinitionFile, def.ID)
	case def.WinCondition.Type == "callback":
		return fmt.Errorf("%s: %q has a win condition of type callback, which is Go in a build of the server", DefinitionFile, def.ID)
	case len(def.Options) > 0:
		return fmt.Errorf("%s: %q has setup options, which a pack's rules cannot derive a game from yet: use variants", DefinitionFile, def.ID)
	case def.Family != p.Family || !def.Prototype || def.Version != p.Version:
		return fmt.Errorf("%s: a variant of %q changes the family, the version or prototype, which the server sets", DefinitionFile, p.Name)
	}
	var tree any
	if err := json.Unmarshal(doc, &tree); err != nil {
		return err
	}
	return checkColors(DefinitionFile, tree)
}

// readLook reads web.yaml, refuses what would run in a player's browser, and
// renames its variants for this version.
func readLook(files fs.FS, p *Pack) ([]byte, error) {
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
				return fmt.Errorf("%s names a %s: a pack runs nothing in a player's browser, and its look is data", LookFile, key)
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
			to, err := p.variantID(id)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", LookFile, err)
			}
			renamed[to] = v
		}
		tree["variants"] = renamed
	}
	return json.Marshal(tree)
}

// checkColors holds every colour in a document to the form of a colour.
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

// trial plays one game at random, so rules that load and then fail, panic
// or take far too long to think stop here and not in a player's game. A call
// into the module that fails answers with nothing rather than an error, so
// the module's own count of failures is what tells.
func trial(cg *catalog.CompiledGame, rules *wasmrules.Module) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("playing it failed: %v", r)
		}
	}()
	before, _ := rules.Faults()
	failed := func() error {
		if n, why := rules.Faults(); n != before {
			return fmt.Errorf("its rules failed while it was played: %w", why)
		}
		return nil
	}
	deadline := time.Now().Add(trialTime)
	st, err := state.InitialStateSeeded(cg, trialSeed)
	if err != nil {
		return fmt.Errorf("its rules could not start it: %w", err)
	}
	seat := state.StartSeat(cg, st)
	rng := rand.New(rand.NewSource(trialSeed)) //nolint:gosec // a repeatable trial, not a secret
	for range trialPlies {
		if time.Now().After(deadline) {
			return fmt.Errorf("a trial game took over %s: its moves are too slow to work out", trialTime)
		}
		legal := state.LegalSelections(cg, st, seat)
		if err := failed(); err != nil {
			return err
		}
		if len(legal) == 0 {
			return nil
		}
		res, err := state.Apply(cg, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			return fmt.Errorf("a selection its rules offered was refused: %w", err)
		}
		if err := failed(); err != nil {
			return err
		}
		if res.GameEnded {
			return nil
		}
		st, seat = res.NewState, res.NextImpulseSeat
	}
	return nil
}
