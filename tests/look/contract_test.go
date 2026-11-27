package look_test

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The paint contract of ADR 0154, decision 4, held for every look the page
// ships: within one game, the paints of all the colors in play (each seat's
// colors, and extra_colors) can be told apart, and the order of lightness is
// kept: where a game plays white, its paint is the lightest of the game's
// colors, and where it plays black, the darkest.
//
// A look is a set of paints, one for each color word of paint.js: the plain
// looks (wood, slate, green felt, paper: style.css's --piece-<word>-fill,
// which a board look changes nowhere, in either color scheme) and each
// illustrated theme of themes.js (a stone's main paint, "mid": its highlight
// and rim are decoration). A color the page does not know is lent the paint of
// a word its game does not play, in the order of paint.js's BY_SEAT, so the
// test resolves colors as paint.js does.
//
// The measure and the thresholds are bingle-3d's, so the two clients are held
// to one rule: the Euclidean distance in OKLab between two paints is at least
// minDistance, and white is at least minLightness of CIE L* lighter than every
// other color of its game and black at least as much darker.
const (
	minDistance  = 0.08
	minLightness = 4.0
)

// A boardLook: its name and the main paint of each color word.
type boardLook struct {
	name   string
	paints map[string]string
	// games says whether the look is offered to the game: the plain ones to
	// all, an illustrated theme to the games themes.js's stoneGame accepts.
	stones bool
}

// A game as the page sees it: its colors in play, in the order paint.js
// gives seats their lent paint.
type gameColors struct {
	id     string
	colors []string
	stones bool
}

// knownGaps lists pairs that the contract does not yet hold and that nobody
// has decided how to repaint: "look/game: a, b" for an indistinguishable pair,
// "look/game: white" or "look/game: black" for a broken order of lightness.
// A look is repainted in preference to adding to this list.
var knownGaps = map[string]bool{}

func TestEveryLookHoldsThePaintContract(t *testing.T) {
	looks := allLooks(t)
	games := allGameColors(t)
	order := byTheSeat(t)
	words := colorWords(t)
	if len(looks) < 7 {
		t.Fatalf("%d looks found", len(looks))
	}
	checked := 0
	for _, l := range looks {
		for _, g := range games {
			if l.stones && !g.stones {
				continue
			}
			if len(g.colors) < 2 {
				continue
			}
			checked++
			for _, f := range contractFailures(l.paints, paintWords(g.colors, order, words), g.colors) {
				key := l.name + "/" + g.id + ": " + f.key
				if knownGaps[key] {
					delete(knownGaps, key)
					continue
				}
				t.Errorf("the %s look, game %s: %s", l.name, g.id, f.msg)
			}
		}
	}
	if checked < 100 {
		t.Errorf("only %d looks x games checked", checked)
	}
	for key := range knownGaps {
		t.Errorf("knownGaps has %q, which the contract no longer fails: remove it", key)
	}
}

// The port of themes.js's themesFor reads a game as the page does, with its
// web.yaml over its definition: games drawn with chess pieces or as a map are
// not offered the illustrated themes, and games of stones are.
func TestWhichGamesAreOfferedThemes(t *testing.T) {
	offered := map[string]bool{}
	for _, g := range allGameColors(t) {
		offered[g.id] = g.stones
	}
	for id, want := range map[string]bool{"go-19x19": true, "hex-11": true, "tak-5x5": true, "blooms": true, "chess": false, "fist": false, "solitaire": false, "courtfall-1.0.0": false} {
		if got, ok := offered[id]; !ok || got != want {
			t.Errorf("%s: offered the illustrated themes = %v (known: %v), want %v", id, got, ok, want)
		}
	}
}

// The first four words paint.js lends to colors it does not know are told
// apart in every look: a game of four unknown colors is legible in any.
func TestTheLentPaintsAreToldApartInEveryLook(t *testing.T) {
	order := byTheSeat(t)
	for _, l := range allLooks(t) {
		colors := order[:4]
		for _, f := range contractFailures(l.paints, identity(colors), colors) {
			t.Errorf("the %s look, the first lent words %v: %s", l.name, colors, f.msg)
		}
	}
}

// The check itself: two paints made alike fail, and so does a white that is
// not the lightest or a black that is not the darkest.
func TestThePaintContractFailsWhenColorsAreAlike(t *testing.T) {
	good := map[string]string{"white": "#f7f4ec", "black": "#34312c", "red": "#d1543f", "blue": "#4a7fd6"}
	colors := []string{"white", "black", "red", "blue"}
	if f := contractFailures(good, identity(colors), colors); len(f) != 0 {
		t.Fatalf("good paints fail: %v", f)
	}
	alike := map[string]string{"white": "#f7f4ec", "black": "#34312c", "red": "#d1543f", "blue": "#d1543f"}
	if f := contractFailures(alike, identity(colors), colors); len(f) != 1 || f[0].key != "blue, red" {
		t.Errorf("red and blue alike: %v", f)
	}
	// Two names given one word's paint are alike, whatever the paints are.
	if f := contractFailures(good, map[string]string{"white": "white", "black": "black", "red": "red", "blue": "red"}, colors); len(f) != 1 {
		t.Errorf("two colors on one word: %v", f)
	}
	pinkWhite := map[string]string{"white": "#d1503f", "black": "#34312c", "red": "#d1543f", "blue": "#4a7fd6"}
	if f := contractFailures(pinkWhite, identity(colors), colors); len(f) == 0 {
		t.Error("a white as dark as red passes")
	}
	lightBlack := map[string]string{"white": "#f7f4ec", "black": "#7a8aa0", "red": "#6a2020", "blue": "#4a7fd6"}
	if f := contractFailures(lightBlack, identity(colors), colors); len(f) == 0 {
		t.Error("a black lighter than red passes")
	}
}

type failure struct{ key, msg string }

func identity(colors []string) map[string]string {
	m := map[string]string{}
	for _, c := range colors {
		m[c] = c
	}
	return m
}

// contractFailures checks one game's colors, each painted as the word word[c]
// says, in a look whose paints are paints[word].
func contractFailures(paints, word map[string]string, colors []string) []failure {
	var out []failure
	hexOf := func(c string) string { return paints[word[c]] }
	for i, a := range colors {
		for _, b := range colors[i+1:] {
			if d := distance(hexOf(a), hexOf(b)); d < minDistance {
				pair := []string{a, b}
				slices.Sort(pair)
				out = append(out, failure{strings.Join(pair, ", "), fmt.Sprintf("%s (%s as %s) and %s (%s as %s) are %.3f apart in OKLab, under %.2f", a, hexOf(a), word[a], b, hexOf(b), word[b], d, minDistance)})
			}
		}
	}
	for _, c := range []string{"white", "black"} {
		if !slices.Contains(colors, c) {
			continue
		}
		for _, o := range colors {
			if o == c {
				continue
			}
			gap := lightness(hexOf(c)) - lightness(hexOf(o))
			if c == "black" {
				gap = -gap
			}
			if gap < minLightness {
				side := map[string]string{"white": "lighter", "black": "darker"}[c]
				out = append(out, failure{c, fmt.Sprintf("%s (%s, L* %.1f) is %.1f L* %s than %s (%s, L* %.1f), under %.0f", c, hexOf(c), lightness(hexOf(c)), gap, side, o, hexOf(o), lightness(hexOf(o)), minLightness)})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Color math: sRGB to OKLab (Ottosson) and to CIE L* (D65).
// ---------------------------------------------------------------------------

func linear(hex string) [3]float64 {
	if len(hex) != 7 || hex[0] != '#' {
		panic("not a #rrggbb color: " + hex)
	}
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		panic("not a #rrggbb color: " + hex)
	}
	var out [3]float64
	for i, shift := range []uint{16, 8, 0} {
		v := float64((n>>shift)&255) / 255
		if v <= 0.04045 {
			out[i] = v / 12.92
		} else {
			out[i] = math.Pow((v+0.055)/1.055, 2.4)
		}
	}
	return out
}

func oklab(hex string) [3]float64 {
	c := linear(hex)
	l := math.Cbrt(0.4122214708*c[0] + 0.5363325363*c[1] + 0.0514459929*c[2])
	m := math.Cbrt(0.2119034982*c[0] + 0.6806995451*c[1] + 0.1073969566*c[2])
	s := math.Cbrt(0.0883024619*c[0] + 0.2817188376*c[1] + 0.6299787005*c[2])
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

func distance(a, b string) float64 {
	x, y := oklab(a), oklab(b)
	dl, da, db := x[0]-y[0], x[1]-y[1], x[2]-y[2]
	return math.Sqrt(dl*dl + da*da + db*db)
}

// lightness is CIE L*: 116 f(Y) - 16, Y the relative luminance.
func lightness(hex string) float64 {
	c := linear(hex)
	y := 0.2126729*c[0] + 0.7151522*c[1] + 0.0721750*c[2]
	if y > 216.0/24389 {
		return 116*math.Cbrt(y) - 16
	}
	return 24389.0 / 27 * y
}

// ---------------------------------------------------------------------------
// What the looks paint and what the games play
// ---------------------------------------------------------------------------

// byTheSeat reads paint.js's BY_SEAT, the order words are lent to colors.
func byTheSeat(t *testing.T) []string {
	t.Helper()
	m := regexp.MustCompile(`const BY_SEAT = \[([^\]]*)\]`).FindStringSubmatch(readStatic(t, "js/paint.js"))
	if m == nil {
		t.Fatal("paint.js has no BY_SEAT")
	}
	var out []string
	for _, q := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(m[1], -1) {
		out = append(out, q[1])
	}
	if len(out) < 4 {
		t.Fatalf("BY_SEAT = %v", out)
	}
	return out
}

var wordInClass = regexp.MustCompile(`[^a-z0-9]`)

// classWord is paint.js's classWord.
func classWord(name string) string {
	c := wordInClass.ReplaceAllString(strings.ToLower(name), "")
	if c == "" {
		return "white"
	}
	return c
}

// paintWords is paint.js's bySeat and paintWord for a game's colors, given in
// the order of its seats and then of its extra_colors: the color word each is
// painted as.
func paintWords(colors, order, words []string) map[string]string {
	known := map[string]bool{}
	for _, w := range words {
		known[w] = true
	}
	played := map[string]bool{}
	for _, c := range colors {
		if known[c] {
			played[c] = true
		}
	}
	var pool []string
	for _, w := range order {
		if !played[w] {
			pool = append(pool, w)
		}
	}
	if len(pool) == 0 {
		pool = order
	}
	out := map[string]string{}
	lent := 0
	for _, c := range colors {
		if known[c] {
			out[c] = c
		} else if _, ok := out[c]; !ok {
			out[c] = pool[lent%len(pool)]
			lent++
		}
	}
	return out
}

// allLooks is the plain looks and every illustrated theme.
func allLooks(t *testing.T) []boardLook {
	t.Helper()
	words := colorWords(t)
	css := readStatic(t, "style.css")

	// The plain looks share one set of paints: the stylesheet gives each word
	// one fill, and no board look or color scheme gives another.
	plain := map[string]string{}
	for _, m := range regexp.MustCompile(`--piece-([a-z]+)-fill:\s*(#[0-9a-fA-F]{6})`).FindAllStringSubmatch(css, -1) {
		if old, ok := plain[m[1]]; ok && old != m[2] {
			t.Errorf("style.css paints %s as %s and as %s: the plain looks must be told apart per look here", m[1], old, m[2])
		}
		plain[m[1]] = m[2]
	}
	if len(plain) != len(words) {
		t.Fatalf("style.css paints %d words, paint.js has %d", len(plain), len(words))
	}
	boards := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?s):root\[data-board="([a-z]+)"\]\s*\{([^}]*)\}`).FindAllStringSubmatch(css, -1) {
		boards[m[1]] = true
		if strings.Contains(m[2], "--piece-") {
			t.Errorf("style.css's %s board look paints a piece: the contract test reads the plain paints once", m[1])
		}
	}
	out := []boardLook{{name: "wood", paints: plain}}
	for _, id := range sortedKeys(boards) {
		out = append(out, boardLook{name: id, paints: plain})
	}

	src := readStatic(t, "js/themes.js")
	theme := regexp.MustCompile(`(?s)id: "([a-z]+)",.*?stones: \{(.*?)\n\t\},`)
	main := regexp.MustCompile(`(?m)^\t\t([a-z]+): stone\("(#[0-9a-fA-F]{6})"`)
	for _, th := range theme.FindAllStringSubmatch(src, -1) {
		paints := map[string]string{}
		for _, k := range main.FindAllStringSubmatch(th[2], -1) {
			paints[k[1]] = k[2]
		}
		if len(paints) != len(words) {
			t.Fatalf("the %s theme has %d stones with a #rrggbb main paint, want %d", th[1], len(paints), len(words))
		}
		out = append(out, boardLook{name: th[1], paints: paints, stones: true})
	}
	return out
}

// stoneShapes reads themes.js's STONE_SHAPES.
func stoneShapes(t *testing.T) map[string]bool {
	t.Helper()
	m := regexp.MustCompile(`const STONE_SHAPES = new Set\(\[([^\]]*)\]\)`).FindStringSubmatch(readStatic(t, "js/themes.js"))
	if m == nil {
		t.Fatal("themes.js has no STONE_SHAPES")
	}
	out := map[string]bool{}
	for _, q := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(m[1], -1) {
		out[q[1]] = true
	}
	if len(out) < 3 {
		t.Fatalf("STONE_SHAPES = %v", out)
	}
	return out
}

// mergePatch is look.js's mergePatch (RFC 7386).
func mergePatch(target, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	out := map[string]any{}
	if t, ok := target.(map[string]any); ok {
		for k, v := range t {
			out[k] = v
		}
	}
	for k, v := range p {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = mergePatch(out[k], v)
		}
	}
	return out
}

// offersThemes is themes.js's themesFor for a game: the page offers the
// illustrated themes to a game whose pieces are all stones, on a board that is
// no map of areas, free table or none. It reads the game as the page does,
// the game's info with its web.yaml laid over it (look.js's applyLook): the
// board's style and each piece's shape are mostly the look's, not the
// definition's (chess's pieces are chess pieces, Cthulhu Wars' board a map).
func offersThemes(t *testing.T, cg *catalog.CompiledGame, shapes map[string]bool) bool {
	t.Helper()
	def := cg.Definition
	style := def.Board.Style
	if style == "" {
		style = "squares"
	}
	shape := map[string]string{}
	for name, p := range def.Pieces {
		shape[name] = p.Shape
		if p.Shape == "" {
			shape[name] = "disc"
		}
	}
	if len(cg.Web) > 0 {
		var file map[string]any
		if err := json.Unmarshal(cg.Web, &file); err != nil {
			t.Fatalf("%s: web.yaml: %v", def.ID, err)
		}
		// look.js's resolveLook: the family's file with the game's variant
		// patched in; a derived game takes the one it was derived from.
		variants, _ := file["variants"].(map[string]any)
		delete(file, "variants")
		own, ok := variants[def.ID]
		if !ok {
			own = variants[strings.SplitN(def.ID, "~", 2)[0]]
		}
		look := file
		if own != nil {
			look, _ = mergePatch(file, own).(map[string]any)
		}
		if b, ok := look["board"].(map[string]any); ok {
			if v, ok := b["style"].(string); ok {
				style = v
			}
		}
		if ps, ok := look["pieces"].(map[string]any); ok {
			for name, p := range ps {
				if _, known := shape[name]; !known {
					continue
				}
				if pm, ok := p.(map[string]any); ok {
					if v, ok := pm["shape"].(string); ok && v != "" {
						shape[name] = v
					}
				}
			}
		}
	}
	if len(shape) == 0 || style == "areas" || style == "free" || style == "none" {
		return false
	}
	for _, sh := range shape {
		if !shapes[sh] {
			return false
		}
	}
	return true
}

// allGameColors is every shipped game's colors in play, as the page's
// resolver orders them (paint.js: seats in their order, each seat's colors in
// its order, then extra_colors), read from the shipped definitions.
func allGameColors(t *testing.T) []gameColors {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
	}
	shapes := stoneShapes(t)
	var out []gameColors
	for _, cg := range cat.List() {
		def := cg.Definition
		seats := slices.Clone(def.Seats)
		sort.SliceStable(seats, func(i, j int) bool { return seats[i].Seat < seats[j].Seat })
		var colors []string
		add := func(c string) {
			if c = classWord(c); !slices.Contains(colors, c) {
				colors = append(colors, c)
			}
		}
		for _, s := range seats {
			if len(s.Colors) > 0 {
				for _, c := range s.Colors {
					add(c)
				}
			} else {
				add(s.Color)
			}
		}
		for _, c := range def.ExtraColors {
			add(c)
		}
		stones := offersThemes(t, cg, shapes)
		out = append(out, gameColors{id: def.ID, colors: colors, stones: stones})
	}
	if len(out) < 25 {
		t.Fatalf("only %d games registered", len(out))
	}
	return out
}
