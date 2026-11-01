package cthulhuwars

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
)

// nSeats is the most players at a table.
const nSeats = 5

// gameCfg is what a game's definition fixes about its table: how many play,
// which faction sits in each seat and which expansion is in the box. The
// definitions of the four layouts seat the first cults in order; a game derived
// from them (see derive.go) names the cults it wants in params.factions.
type gameCfg struct {
	n int
	// fac is the faction index of each seat (1..n), -1 beyond.
	fac [nSeats + 1]int
	// seatOf is the seat of each faction, 0 when it is not at the table.
	seatOf [nFactions]int
	// first is the seat of the first player of round one (see firstSeatOf).
	first int
	// hp says the High Priests expansion is in play.
	hp bool
	// az says the neutral content of the Azathoth faction is in play: the
	// neutral spellbooks (any cult may take one in place of its own) and the
	// neutral Monsters (Loyalty Cards bought with Doom).
	az bool
	// packs holds the packs of Loyalty Cards in the box besides the Azathoth
	// faction's: packDreamlands, packBeyond, packGOO1 (set when any Independent
	// is in the game); indep says which Independent Great Old Ones are: bit i is
	// cards[firstIndependent+i].
	packs int
	indep int
	// swap is, per faction, the neutral spellbook that replaces its timer
	// book (Black Goat's Ghroth, Crawling Chaos' The Thousand Forms), -1 when
	// it keeps its own. It needs no Azathoth faction in the game.
	swap [nFactions]int
	// sigils lists, per faction, the areas marked with its sigil: where
	// Windwalker may start and awaken Rhan-Tegoth, where Sleeper starts.
	sigils [nFactions][]string
}

// buildCfg reads the table of a definition.
func buildCfg(def *dsl.GameDefinition) *gameCfg {
	c := &gameCfg{n: len(def.Seats)}
	for i := range c.swap {
		c.swap[i] = -1
	}
	if c.n > nSeats {
		c.n = nSeats
	}
	for i := range c.fac {
		c.fac[i] = -1
	}
	var keys []string
	if l, ok := def.Params["factions"].([]any); ok {
		for _, v := range l {
			if s, ok := v.(string); ok {
				keys = append(keys, s)
			}
		}
	}
	for seat := 1; seat <= c.n; seat++ {
		f := seat - 1
		if seat-1 < len(keys) {
			if idx := factionIndex(keys[seat-1]); idx >= 0 {
				f = idx
			}
		}
		if f >= nFactions {
			f = nFactions - 1
		}
		c.fac[seat] = f
		if c.seatOf[f] == 0 {
			c.seatOf[f] = seat
		}
	}
	c.first = firstSeatOf(c.n, func(seat int) int { return c.fac[seat] })
	if b, ok := def.Params["high_priests"].(bool); ok {
		c.hp = b
	}
	if b, ok := def.Params["azathoth"].(bool); ok {
		c.az = b
	}
	for key, bit := range packParams {
		if b, ok := def.Params[key].(bool); ok && b {
			c.packs |= bit
		}
	}
	if l, ok := def.Params["independents"].([]any); ok {
		for _, v := range l {
			if name, ok := v.(string); ok {
				if i := igooIndex(name); i >= 0 {
					c.indep |= 1 << uint(i)
				}
			}
		}
		if c.indep != 0 {
			c.packs |= packGOO1
		}
	}
	if m, ok := def.Params["book_swaps"].(map[string]any); ok {
		for k, v := range m {
			f, n := factionIndex(k), -1
			if s, ok := v.(string); ok {
				n = neutralBookByKey(s)
			}
			if _, timer := timerBooks[f]; f >= 0 && timer && n >= 0 {
				c.swap[f] = n
			}
		}
	}
	if m, ok := def.Params["sigils"].(map[string]any); ok {
		for k, v := range m {
			idx := factionIndex(k)
			if idx < 0 {
				continue
			}
			if l, ok := v.([]any); ok {
				for _, a := range l {
					if s, ok := a.(string); ok {
						c.sigils[idx] = append(c.sigils[idx], s)
					}
				}
			}
		}
	}
	return c
}

// firstSeatOf is the seat that is the first player of round one. The rule book
// makes Great Cthulhu the first player; at a table without it, it suggests
// choosing "by alphabetic order per Faction name — i.e., Black Goat is before
// Yellow Sign", which is what this does. fac gives the faction index in a seat
// (-1 when unknown).
func firstSeatOf(n int, fac func(seat int) int) int {
	best := 0
	for seat := 1; seat <= n; seat++ {
		f := fac(seat)
		switch {
		case f < 0:
		case f == facGC:
			return seat
		case best == 0 || factions[f].Name < factions[fac(best)].Name:
			best = seat
		}
	}
	if best == 0 {
		return 1
	}
	return best
}

// cfgOf is the table of a compiled game.
func cfgOf(game *catalog.CompiledGame) *gameCfg { return mapOf(game).cfg }

// factionOfSeat is the faction index seat plays in game.
func factionOfSeat(game *catalog.CompiledGame, seat int) int {
	c := cfgOf(game)
	if seat < 1 || seat > nSeats {
		return -1
	}
	return c.fac[seat]
}

// keyOfSeat is the key (gc, cc, …) of the faction in seat, "?" when unknown.
func keyOfSeat(game *catalog.CompiledGame, seat int) string {
	if f := factionOfSeat(game, seat); f >= 0 {
		return factions[f].Key
	}
	return "?"
}

// seatOfKey is the seat of the faction named k, 0 when it is not at the table.
func seatOfKey(game *catalog.CompiledGame, k string) int {
	if f := factionIndex(k); f >= 0 {
		return cfgOf(game).seatOf[f]
	}
	return 0
}

// --- setup options and derived games ------------------------------------------------

// The options a game takes (declared in definition/cthulhuwars.yaml under "options").
const (
	optFactions    = "factions"
	optHighPriests = "high-priests"
	optAzathoth    = "azathoth"
	optDreamlands  = "dreamlands"
	optBeyond      = "beyond"
	optBookSwaps   = "book-swaps"
)

// packParams are the definition parameters of the packs of Loyalty Cards, the
// option that sets each, its short name in the id of a derived game and its
// title.
var packParams = map[string]int{"dreamlands": packDreamlands, "beyond": packBeyond}

var packOptions = []struct {
	bit               int
	option, key, flag string
	title             string
}{
	{packDreamlands, optDreamlands, "dreamlands", "dl", "Dreamlands Surface Monsters"},
	{packBeyond, optBeyond, "beyond", "bt", "Beyond Time and Space"},
}

// igooKeys are the options (and, after "g1", the letters of a derived game's
// id) that put an Independent Great Old One of Great Old One Pack 1 in the
// game, in the order of the cards.
var igooKeys = []struct{ option, letter, unit string }{
	{"igoo-abhoth", "a", uAbhoth},
	{"igoo-chaugnar", "c", uChaugnar},
	{"igoo-cthugha", "t", uCthugha},
	{"igoo-hydra", "h", uHydra},
	{"igoo-yig", "y", uYig},
}

// igooOption is the Independent an option names (-1: not one).
func igooOption(k string) int {
	for i, x := range igooKeys {
		if x.option == k {
			return i
		}
	}
	return -1
}

// igooSuffix is the part of a derived game's id that names the Independents
// ("g1ach"), "" for none.
func igooSuffix(indep int) string {
	out := ""
	for i, x := range igooKeys {
		if indep&(1<<uint(i)) != 0 {
			out += x.letter
		}
	}
	if out == "" {
		return ""
	}
	return "g1" + out
}

// igooFlag reads such a part (0 if it is no such part).
func igooFlag(p string) int {
	if len(p) < 3 || p[:2] != "g1" {
		return 0
	}
	n := 0
	for _, c := range p[2:] {
		found := false
		for i, x := range igooKeys {
			if x.letter == string(c) {
				n |= 1 << uint(i)
				found = true
			}
		}
		if !found {
			return 0
		}
	}
	return n
}

var _ catalog.Deriver = (*Ruleset)(nil)

// Derive implements catalog.Deriver: the options say which cults sit in the
// seats (as many as the layout has), in seat order, and whether the High
// Priests are in the box. The suffix names the combination ("gc-cc-bg-ys-ww",
// plus "-hp").
func (rs *Ruleset) Derive(base *dsl.GameDefinition, options map[string]any) (string, map[string]any, error) {
	n := len(base.Seats)
	keys := make([]string, 0, n)
	hp, az, packs, indep := false, false, 0, 0
	var swaps map[string]string
	for k, v := range options {
		switch k {
		case optFactions:
			list, ok := v.([]any)
			if !ok {
				if ss, ok2 := v.([]string); ok2 {
					for _, s := range ss {
						list = append(list, s)
					}
					ok = true
				}
			}
			if !ok {
				return "", nil, fmt.Errorf("cthulhuwars: option %q must be a list of faction keys", k)
			}
			for _, x := range list {
				s, ok := x.(string)
				if !ok {
					return "", nil, fmt.Errorf("cthulhuwars: option %q holds a value that is not a faction key", k)
				}
				keys = append(keys, strings.ToLower(strings.TrimSpace(s)))
			}
		case optHighPriests:
			b, ok := v.(bool)
			if !ok {
				return "", nil, fmt.Errorf("cthulhuwars: option %q must be true or false", k)
			}
			hp = b
		case optAzathoth:
			b, ok := v.(bool)
			if !ok {
				return "", nil, fmt.Errorf("cthulhuwars: option %q must be true or false", k)
			}
			az = b
		case optBookSwaps:
			m, err := swapOption(v)
			if err != nil {
				return "", nil, err
			}
			swaps = m
		case optDreamlands, optBeyond:
			b, ok := v.(bool)
			if !ok {
				return "", nil, fmt.Errorf("cthulhuwars: option %q must be true or false", k)
			}
			if b {
				for _, po := range packOptions {
					if po.option == k {
						packs |= po.bit
					}
				}
			}
		default:
			if i := igooOption(k); i >= 0 {
				b, ok := v.(bool)
				if !ok {
					return "", nil, fmt.Errorf("cthulhuwars: option %q must be true or false", k)
				}
				if b {
					indep |= 1 << uint(i)
				}
				break
			}
			return "", nil, fmt.Errorf("cthulhuwars: unknown option %q", k)
		}
	}
	if len(keys) == 0 {
		// Only the expansion was chosen: the layout's own cults.
		for i := 0; i < n; i++ {
			keys = append(keys, factions[defaultFaction(base, i+1)].Key)
		}
	}
	return deriveFrom(base, keys, hp, az, packs, indep, swaps)
}

// swapOption reads the book-swaps option: faction key → neutral spellbook key.
func swapOption(v any) (map[string]string, error) {
	out := map[string]string{}
	switch m := v.(type) {
	case map[string]any:
		for k, x := range m {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("cthulhuwars: option %q names neutral spellbooks as text", optBookSwaps)
			}
			out[k] = s
		}
	case map[string]string:
		for k, s := range m {
			out[k] = s
		}
	default:
		return nil, fmt.Errorf("cthulhuwars: option %q must map a faction (bg, cc) to a neutral spellbook", optBookSwaps)
	}
	return out, nil
}

var _ state.SeatCults = (*Ruleset)(nil)

// SeatCultOptions implements state.SeatCults: the options of the table with
// cult in seat. A cult played by another seat changes places with it, unless
// that seat is taken (a player or bot sits there).
func (rs *Ruleset) SeatCultOptions(game *catalog.CompiledGame, seat int, cult string, taken map[int]bool) (map[string]any, error) {
	c := cfgOf(game)
	want := factionIndex(strings.ToLower(strings.TrimSpace(cult)))
	if want < 0 {
		return nil, fmt.Errorf("cthulhuwars: unknown cult %q (use gc, cc, bg, ys, ww, sl, ow or tt)", cult)
	}
	if seat < 1 || seat > c.n {
		return nil, fmt.Errorf("cthulhuwars: this table has no seat %d", seat)
	}
	if taken[seat] {
		return nil, fmt.Errorf("cthulhuwars: seat %d is taken: its cult cannot change", seat)
	}
	keys := make([]any, c.n)
	for s := 1; s <= c.n; s++ {
		keys[s-1] = factions[c.fac[s]].Key
	}
	if c.fac[seat] == want {
		return nil, nil
	}
	if other := c.seatOf[want]; other != 0 {
		if taken[other] {
			return nil, fmt.Errorf("%w: %s is played from seat %d", state.ErrCultHeld, factions[want].Name, other)
		}
		keys[other-1] = factions[c.fac[seat]].Key
	}
	keys[seat-1] = factions[want].Key
	out := map[string]any{optFactions: keys, optHighPriests: c.hp, optAzathoth: c.az}
	for _, po := range packOptions {
		out[po.option] = c.packs&po.bit != 0
	}
	for i, x := range igooKeys {
		out[x.option] = c.indep&(1<<uint(i)) != 0
	}
	// A swapped book stays with its cult (a cult that moves seats takes it
	// along; one that was swapped out of the table loses it).
	swaps := map[string]any{}
	for f, n := range c.swap {
		if n < 0 {
			continue
		}
		for _, k := range keys {
			if k == any(factions[f].Key) {
				swaps[factions[f].Key] = neutralKeys[n]
			}
		}
	}
	if len(swaps) > 0 {
		out[optBookSwaps] = swaps
	}
	return out, nil
}

// defaultFaction is the faction a base definition seats in seat.
func defaultFaction(base *dsl.GameDefinition, seat int) int {
	c := buildCfg(base)
	if seat >= 1 && seat <= nSeats && c.fac[seat] >= 0 {
		return c.fac[seat]
	}
	return seat - 1
}

// FromSuffix implements catalog.Deriver.
func (rs *Ruleset) FromSuffix(base *dsl.GameDefinition, suffix string) (map[string]any, error) {
	parts := strings.Split(suffix, "-")
	hp, az, packs, indep := false, false, 0, 0
	var keys []string
	swaps := map[string]string{}
	for _, p := range parts {
		switch {
		case p == "hp":
			hp = true
		case p == "az":
			az = true
		case packFlag(p) != 0:
			packs |= packFlag(p)
		case igooFlag(p) != 0:
			indep |= igooFlag(p)
		case strings.Contains(p, "."):
			// "bg.shrivel": the cult and the neutral spellbook that replaces its timer book.
			k, n, _ := strings.Cut(p, ".")
			swaps[k] = n
		default:
			keys = append(keys, p)
		}
	}
	_, patch, err := deriveFrom(base, keys, hp, az, packs, indep, swaps)
	return patch, err
}

// deriveFrom validates a line-up and builds the suffix and merge patch.
func deriveFrom(base *dsl.GameDefinition, keys []string, hp, az bool, packs, indep int, swaps map[string]string) (string, map[string]any, error) {
	n := len(base.Seats)
	if len(keys) != n {
		return "", nil, fmt.Errorf("cthulhuwars: this layout seats %d players: name %d factions (got %d)", n, n, len(keys))
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if factionIndex(k) < 0 {
			return "", nil, fmt.Errorf("cthulhuwars: unknown faction %q (use gc, cc, bg, ys, ww, sl, ow or tt)", k)
		}
		if seen[k] {
			return "", nil, fmt.Errorf("cthulhuwars: faction %q is chosen twice", k)
		}
		seen[k] = true
	}
	// Windwalker and Sleeper need their sigil areas on the map.
	for _, k := range keys {
		if k != "ww" && k != "sl" {
			continue
		}
		sg, _ := base.Params["sigils"].(map[string]any)
		if l, _ := sg[k].([]any); len(l) == 0 {
			return "", nil, fmt.Errorf("cthulhuwars: this map has no area for %s", factions[factionIndex(k)].Name)
		}
	}
	// A neutral spellbook replaces a timer book (Ghroth, The Thousand Forms)
	// of a cult at the table; no neutral spellbook is set aside twice.
	swapKeys := make([]string, 0, len(swaps))
	used := map[string]bool{}
	for _, k := range []string{"bg", "cc"} {
		n, ok := swaps[k]
		if !ok || n == "" {
			continue
		}
		if indexOf(keys, k) < 0 {
			continue // that cult is not at the table
		}
		if neutralBookByKey(n) < 0 {
			return "", nil, fmt.Errorf("cthulhuwars: unknown neutral spellbook %q (use %s)", n, strings.Join(neutralKeys[:], ", "))
		}
		if used[n] {
			return "", nil, fmt.Errorf("cthulhuwars: the neutral spellbook %q replaces the book of two cults", n)
		}
		used[n] = true
		swapKeys = append(swapKeys, k+"."+n)
	}
	for k := range swaps {
		if k != "bg" && k != "cc" {
			return "", nil, fmt.Errorf("cthulhuwars: only Black Goat (bg) and Crawling Chaos (cc) can replace a spellbook with a neutral one, not %q", k)
		}
	}
	suffix := strings.Join(keys, "-")
	if hp {
		suffix += "-hp"
	}
	if az {
		suffix += "-az"
	}
	for _, po := range packOptions {
		if packs&po.bit != 0 {
			suffix += "-" + po.flag
		}
	}
	if g := igooSuffix(indep); g != "" {
		suffix += "-" + g
	}
	for _, s := range swapKeys {
		suffix += "-" + s
	}
	seats := make([]map[string]any, n)
	seatNums := make([]int, n)
	names := make([]string, n)
	badges := map[string]any{}
	for i, k := range keys {
		f := factions[factionIndex(k)]
		seats[i] = map[string]any{"seat": i + 1, "color": f.Color, "label": f.Name}
		seatNums[i] = i + 1
		names[i] = f.Name
		badges[fmt.Sprintf("gate-%d", i+1)] = map[string]any{
			"label": "Gate (" + f.Name + ")", "color": gateColor(f.Color), "glyph": "◉",
			"text": "A Gate controlled by " + f.Name + ". The figure on it is the unit that occupies it, apart from the others of its kind there: click yours to move it — that abandons the Gate.",
		}
	}
	// The seat that moves first leads the turn order, so that the platform
	// asks the right player: Windwalker, which sets up before anyone plays
	// (see stageSetup), else the first player of round one (Great Cthulhu, or
	// the cult first by name — see firstSeatOf) who chooses the direction of
	// play.
	lead := firstSeatOf(n, func(seat int) int { return factionIndex(keys[seat-1]) })
	// The Tcho-Tcho, the Windwalker and the Opener set up first of all, in that
	// order (see setupRank): the first of them leads.
	bestRank := 3
	for i, k := range keys {
		r := 3
		switch k {
		case "tt":
			r = 0
		case "ww":
			r = 1
		case "ow":
			r = 2
		}
		if r < bestRank {
			bestRank, lead = r, i+1
		}
	}
	order := []int{lead}
	for _, s := range seatNums {
		if s != lead {
			order = append(order, s)
		}
	}
	name := fmt.Sprintf("%s: %s", baseName(base.Name), strings.Join(names, ", "))
	if hp {
		name += " + High Priests"
	}
	if az {
		name += " + Azathoth"
	}
	for _, po := range packOptions {
		if packs&po.bit != 0 {
			name += " + " + po.title
		}
	}
	var indepNames, indepUnits []any
	for i, x := range igooKeys {
		if indep&(1<<uint(i)) != 0 {
			indepNames = append(indepNames, unitNameOf(x.unit))
			indepUnits = append(indepUnits, x.unit)
		}
	}
	if len(indepNames) > 0 {
		parts := make([]string, len(indepNames))
		for i, n := range indepNames {
			parts[i] = n.(string)
		}
		name += " + " + strings.Join(parts, ", ")
	}
	swapParam := map[string]any{}
	for _, s := range swapKeys {
		k, n, _ := strings.Cut(s, ".")
		swapParam[k] = n
	}
	params := map[string]any{
		"factions":     stringsToAny(keys),
		"high_priests": hp,
		"azathoth":     az,
		"book_swaps":   swapParam,
	}
	for _, po := range packOptions {
		params[po.key] = packs&po.bit != 0
	}
	params["independents"] = indepUnits
	patch := map[string]any{
		"name":       name,
		"seats":      seats,
		"turn_order": map[string]any{"type": "round-robin", "seats": order},
		"params":     params,
		"display":    map[string]any{"badges": badges},
	}
	return suffix, patch, nil
}

// packFlag is the pack a short name in the id of a derived game stands for
// (0 for any other part).
func packFlag(p string) int {
	for _, po := range packOptions {
		if po.flag == p {
			return po.bit
		}
	}
	return 0
}

func baseName(n string) string {
	if i := strings.Index(n, ":"); i > 0 {
		return n[:i]
	}
	return n
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// gateColor is the hex color of a faction's Gate badge.
func gateColor(color string) string {
	switch color {
	case "green":
		return "#3c9a4a"
	case "blue":
		return "#2f6fb5"
	case "red":
		return "#b3342b"
	case "yellow":
		return "#c9a227"
	case "white":
		return "#7f9bb0"
	case "orange":
		return "#d9822b"
	case "purple":
		return "#8a5bd0"
	case "brown":
		return "#8f6035"
	}
	return "#888888"
}
