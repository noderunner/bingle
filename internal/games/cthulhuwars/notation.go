package cthulhuwars

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation (no spaces; areas are their ids, factions gc cc bg ys ww sl):
//
//	recruit@asia              recruit a Cultist (recruit:high-priest@asia: the High Priest)
//	summon:deep-one@asia      summon a monster at a Gate you control
//	awaken:cthulhu@asia       awaken a Great Old One
//	gate@asia                 create a Gate
//	move: go:cultist@asia>europe   move one unit (the first begins a Move action); done ends it
//	battle@asia/cc            battle the faction cc in an area
//	capture@asia/cc           capture a Cultist (capture:serpent-man@asia/cc: a
//	                          Monster, with Capture Monster)
//	control:cultist@asia      put a unit that stands in the area onto its Gate
//	                          (free): take control of an abandoned Gate, or
//	                          replace the unit on your own (a Cultist arriving
//	                          at an abandoned Gate takes it by itself)
//	leave:cultist@asia>europe the unit that stands on a Gate moves away — the
//	                          Gate is abandoned (a Move step); leave:cultist@
//	                          asia>asia steps it off where it is (free, once
//	                          in a turn per Gate). The other units of its kind
//	                          move with go.
//	reveal                    turn all your Elder Signs face up (public)
//	pay/4                     Crawling Chaos: pay 4 Power for a spellbook
//	pass | done               drop to 0 Power | end your turn
//	absorb:cultist abduct invisible:cultist seek@asia necro   battle abilities
//	invisible:cultist/cc      Invisibility on a unit of another faction
//	harbinger:power | harbinger:signs   what a Great Old One that was hit pays
//	bring:deep-one | bring:all   a unit that sinks with Cthulhu / walks with the
//	                          King (then done); bring:byakhee@asia for Shriek
//	assign:deep-one           the unit that takes a Kill / Pain result
//	retreat:deep-one>europe   where a pained unit retreats (…/gc: Madness
//	                          sends another faction's unit)
//	lose:cultist@asia         the unit you eliminate
//	avatar@asia/cc            Shub-Niggurath swaps with a unit of cc there;
//	                          swap:cultist is the unit its owner gives
//	place@asia/gc             Ghroth: put a Cultist of gc there
//	grant:cc                  Yellow Sign: cc gains 3 Doom
//	book:absorb               the spellbook you earn
//	ritual | skip | blood@asia   Doom phase decisions
//	cw | ccw | first:gc       direction / first player
//
// plus the spellbook actions: submerge, emerge@area, dreams@area/key,
// devolve@area, forms, ghroth, scream@area, shriek@area, zingaya@area/key,
// desecrate (rolls the die at once; then desecrate:unit places the Monster or
// Cultist of cost 2 or less), hwinn@area, sacrifice — and for the expansion cults:
// start@area, hibernate, iceage@area, claim, howl, cannibal:unit, eternal
// (Windwalker); lethargy, boon, bestow:key, bane, sorcery@area/key,
// restore@area, slumber@area, wake@area, death@area, demand:signs|pains, nexus
// (Sleeper); oath (the High Priest); and for the Loyalty Cards: buy:unit[@area|/key],
// moon:book/key, ransom:book, crypt:key | crypt@area, filth@area, sting@area |
// sting:unit>area, zygote | zygote@area, discard:NN, gateoff@area, spare:unit/key,
// pick:N, donate:give|refuse, awaken:cthugha@area/unit.

// bookActions are the action ids that exist because of a spellbook: the
// ability is the book's, so the button wears a spellbook once it is earned.
var bookActions = map[string]bool{
	actSubmerge: true, actEmerge: true, actDreams: true, actDevolve: true, actAbsorb: true, // Great Cthulhu
	actForms: true, actAbduct: true, actInvisible: true, actSeek: true, // Crawling Chaos
	actBlood: true, actGhroth: true, actNecro: true, // Black Goat
	actScream: true, actShriek: true, actZingaya: true, actHWINN: true, // Yellow Sign
	actIceAge: true, actHowl: true, actCannibal: true, // Windwalker
	actSorcery: true, actRestore: true, actSlumber: true, actWake: true, actNexus: true, // Sleeper
	actShrivel: true, actRecrim: true, actUndim: true, actMao: true, // neutral spellbooks
	actCurse: true, actAscend: true, actChannel: true, actUpgrade: true, actTerror: true, actIdolatry: true, // Opener, Tcho-Tcho
	actZygote: true, // Mother Hydra's spellbook
}

// requirementActions are the action ids that meet a spellbook requirement
// (pay Power, sacrifice, give Doom or Power, take the free book).
var requirementActions = map[string]bool{
	actPay: true, actSacrifice: true, actGrant: true, actBoon: true, actBane: true, actBestow: true, actClaim: true, actDismantle: true,
	actDiscard: true, actGateOff: true,
}

// actionSource is the Source of a button: "book", "requirement" or "".
func actionSource(id string) string {
	switch {
	case bookActions[id], strings.HasPrefix(id, "recriminate-"):
		return "book"
	case requirementActions[id], strings.HasPrefix(id, "pay-"), strings.HasPrefix(id, "grant-"), strings.HasPrefix(id, "bestow-"):
		return "requirement"
	}
	return ""
}

// abilityText is the rule of a unit's (first) named ability, with its timing:
// "Action: Cost 2 — If the King…".
func abilityText(id string) string {
	for _, f := range factions {
		if u, ok := f.unitSpec(id); ok && len(u.Abilities) > 0 {
			a := u.Abilities[0]
			return a.Name + " (" + a.Timing + "): " + a.Text
		}
	}
	return ""
}

// highPriestRules sums up the High Priests option for the card of its button.
const highPriestRules = "The High Priests option gives every cult one High Priest: a Cultist for every purpose — he gives 1 Power in Gather Power, " +
	"builds and holds a Gate and can be captured. Recruit him in an area where you already have a unit. " +
	"Unspeakable Oath: at the end of any action — even another player's —, in Gather Power or in the Doom phase you may sacrifice him: " +
	"he returns to your pool and you gain 2 Power. He can be recruited again."

// onGate prefixes the piece value of the unit that stands on a Gate: apart
// from the rest of its kind in the same area, it is picked up by its own click.
const onGate = "gate:"

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	at := func() string { return "@" + string(mv.To) }
	vs := func() string { return "/" + keyOfSeat(game, mv.Count) }
	switch mv.Action {
	case actRecruit:
		if mv.Piece != "" && mv.Piece != uCultist {
			return mv.Action + ":" + mv.Piece + at()
		}
		return mv.Action + at()
	case actGate, actAbandon, actEmerge, actScream, actShriek, actHWINN, actDevolve, actBlood,
		actStart, actIceAge, actRestore, actSlumber, actWake, actDeath:
		return mv.Action + at()
	case actControl:
		if mv.Piece != "" {
			return mv.Action + ":" + mv.Piece + at()
		}
		return mv.Action + at()
	case actSeek:
		if mv.To != "" {
			return mv.Action + at()
		}
		return mv.Action
	case actSummon:
		return mv.Action + ":" + mv.Piece + at()
	case actAwaken:
		if g := replacedBy(mv); g != "" {
			return mv.Action + ":" + mv.Piece + at() + "/" + g
		}
		return mv.Action + ":" + mv.Piece + at()
	case actMoon:
		if mv.To != "" {
			return mv.Action + at()
		}
		return mv.Action + ":" + mv.Piece + vs()
	case actRansom, actDiscard:
		if mv.Piece != "" {
			return mv.Action + ":" + mv.Piece
		}
		return fmt.Sprintf("%s:%d", mv.Action, mv.Count)
	case actCrypt:
		if mv.To != "" {
			return mv.Action + at()
		}
		return mv.Action + ":" + keyOfSeat(game, mv.Count)
	case actFilth, actGateOff:
		return mv.Action + at()
	case actSting:
		if mv.Piece != "" {
			return fmt.Sprintf("%s:%s>%s", mv.Action, mv.Piece, mv.To)
		}
		return mv.Action + at()
	case actZygote:
		if mv.To != "" {
			return mv.Action + at()
		}
		return mv.Action
	case actSpare:
		return mv.Action + ":" + mv.Piece + vs()
	case actPick:
		return fmt.Sprintf("%s:%d", mv.Action, mv.Count)
	case actDonate:
		return mv.Action + ":" + mv.Piece
	case actGo, actLeave, actBeyond:
		return fmt.Sprintf("%s:%s@%s>%s", mv.Action, mv.Piece, mv.From, mv.To)
	case actAscend:
		return mv.Action + ":" + keyOfSeat(game, mv.Count)
	case actUpgrade:
		if mv.Count != 0 {
			return fmt.Sprintf("%s:%s/%d", mv.Action, mv.Piece, mv.Count)
		}
		return mv.Action + ":" + mv.Piece
	case actCurse:
		if mv.To != "" {
			return mv.Action + at()
		}
		return mv.Action + ":" + keyOfSeat(game, mv.Count)
	case actSyco:
		return mv.Action + ":" + mv.Piece + vs()
	case actTerror:
		return mv.Action + ":" + mv.Piece
	case actRitual:
		if mv.Piece != "" {
			return mv.Action + ":" + mv.Piece
		}
		return mv.Action
	case actHiero, actIdolatry, actDismantle:
		return mv.Action + at()
	case actCapture:
		if mv.Piece != "" {
			return mv.Action + ":" + mv.Piece + at() + vs()
		}
		return mv.Action + at() + vs()
	case actBattle, actDreams, actZingaya, actAvatar, actPlace, actSorcery:
		return mv.Action + at() + vs()
	case actWalk:
		return mv.Action + at()
	case actShrivel:
		return mv.Action + ":" + mv.Piece
	case actRecrim:
		return mv.Action + ":" + mv.Piece
	case actRearrange:
		return fmt.Sprintf("%s:%s@%s>%s", mv.Action, mv.Piece, mv.From, mv.To)
	case actMao:
		return fmt.Sprintf("%s:%s%s", mv.Action, mv.Piece, at())
	case actBuy:
		s := mv.Action + ":" + mv.Piece
		if mv.To != "" {
			s += at()
		}
		if mv.Count != 0 {
			s += vs()
		}
		return s
	case actPay:
		return fmt.Sprintf("%s/%d", mv.Action, mv.Count)
	case actAbsorb, actSwap, actCannibal:
		return mv.Action + ":" + mv.Piece
	case actInvisible, actAssign:
		if mv.Count != 0 {
			return mv.Action + ":" + mv.Piece + vs()
		}
		return mv.Action + ":" + mv.Piece
	case actBring:
		if mv.From != "" {
			return fmt.Sprintf("bring:%s@%s", mv.Piece, mv.From)
		}
		return mv.Action + ":" + mv.Piece
	case actGrant, actBestow:
		return mv.Action + ":" + keyOfSeat(game, mv.Count)
	case actHarbinger, actDemand:
		return mv.Action + ":" + mv.Piece
	case actRetreat:
		if mv.Count != 0 {
			return fmt.Sprintf("retreat:%s>%s%s", mv.Piece, mv.To, vs())
		}
		return fmt.Sprintf("retreat:%s>%s", mv.Piece, mv.To)
	case actLose:
		return fmt.Sprintf("lose:%s%s", mv.Piece, at())
	case actBook:
		return "book:" + mv.Piece
	case actDesecrate:
		if mv.Piece != "" {
			return "desecrate:" + mv.Piece
		}
		return "desecrate"
	case "first":
		return "first:" + keyOfSeat(game, mv.Count)
	}
	return mv.Action
}

var plainActions = map[string]bool{
	actPass: true, actDone: true, actSacrifice: true, actSubmerge: true, actForms: true, actReveal: true,
	actGhroth: true, actAbduct: true, actSeek: true, actNecro: true, actRitual: true, actSkip: true,
	actCW: true, actCCW: true, actDesecrate: true,
	actHibernate: true, actClaim: true, actHowl: true, actEternal: true,
	actLethargy: true, actBoon: true, actBane: true, actNexus: true, actOath: true, actUndim: true, actChannel: true,
	actZygote: true,
}

// Parse reads notation produced by Format.
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return state.Move{}, fmt.Errorf("cthulhuwars: empty move")
	}
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("cthulhuwars: cannot read %q as a move (%s)", text, why)
	}
	m := mapOf(game)
	area := func(s string) (board.PositionID, bool) {
		id := board.PositionID(s)
		_, ok := m.index[id]
		return id, ok
	}
	unit := func(s string) bool { _, ok := unitTypeIndex[s]; return ok }
	if plainActions[t] {
		return state.Move{Action: t}, nil
	}
	// "action/N"
	if i := strings.IndexByte(t, '/'); i > 0 && !strings.ContainsAny(t, "@:") {
		act, arg := t[:i], t[i+1:]
		n, err := strconv.Atoi(arg)
		if act == actPay && err == nil && n >= 1 {
			return state.Move{Action: act, Count: n}, nil
		}
		if act == actReveal && err == nil && n >= 1 {
			// The old "reveal/N" turned one sign over; revealing is now
			// all-or-nothing, so any N means "reveal".
			return state.Move{Action: actReveal}, nil
		}
		return bad("expected reveal or pay/N")
	}
	if i := strings.IndexByte(t, ':'); i > 0 {
		act, rest := t[:i], t[i+1:]
		switch act {
		case actSummon, actAwaken, actRecruit, actControl:
			parts := strings.SplitN(rest, "@", 2)
			if len(parts) != 2 || !unit(parts[0]) {
				return bad("expected " + act + ":unit@area")
			}
			where, replaced, hasReplaced := strings.Cut(parts[1], "/")
			id, ok := area(where)
			if !ok {
				return bad("unknown area " + where)
			}
			mv := state.Move{Action: act, Piece: parts[0], To: id}
			if hasReplaced {
				// Cthugha: the Great Old One he replaces.
				idx, known := unitTypeIndex[replaced]
				if act != actAwaken || parts[0] != uCthugha || !known {
					return bad("expected awaken:cthugha@area/great-old-one")
				}
				mv.Count = idx + 1
			}
			return mv, nil
		case actMoon:
			name, key, _ := strings.Cut(rest, "/")
			_, ok := bookByID(name)
			s := seatOfKey(game, key)
			if !ok || s == 0 {
				return bad("expected moon:spellbook/faction")
			}
			return state.Move{Action: actMoon, Piece: name, Count: s}, nil
		case actRansom:
			if _, ok := bookByID(rest); !ok {
				return bad("unknown spellbook " + rest)
			}
			return state.Move{Action: actRansom, Piece: rest}, nil
		case actCrypt:
			s := seatOfKey(game, rest)
			if s == 0 {
				return bad("name the enemy faction")
			}
			return state.Move{Action: actCrypt, Count: s}, nil
		case actDiscard, actPick:
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 {
				return bad("expected " + act + ":N")
			}
			return state.Move{Action: act, Count: n}, nil
		case actDonate:
			if rest != "give" && rest != "refuse" {
				return bad("expected donate:give or donate:refuse")
			}
			return state.Move{Action: actDonate, Piece: rest}, nil
		case actSpare:
			name, key, _ := strings.Cut(rest, "/")
			if !unit(name) {
				return bad("unknown unit " + name)
			}
			mv := state.Move{Action: actSpare, Piece: name}
			if key != "" {
				s := seatOfKey(game, key)
				if s == 0 {
					return bad("unknown faction " + key)
				}
				mv.Count = s
			}
			return mv, nil
		case actSting:
			parts := strings.SplitN(rest, ">", 2)
			if len(parts) != 2 || !unit(parts[0]) {
				return bad("expected sting:unit>area")
			}
			id, ok := area(parts[1])
			if !ok {
				return bad("unknown area " + parts[1])
			}
			return state.Move{Action: actSting, Piece: parts[0], To: id}, nil
		case actCapture:
			parts := strings.SplitN(rest, "@", 2)
			// capture:gate:cultist is the Cultist that holds the Gate, not one beside it.
			if len(parts) != 2 || (!unit(parts[0]) && parts[0] != onGate+uCultist) {
				return bad("expected capture:unit@area/faction")
			}
			where, key, _ := strings.Cut(parts[1], "/")
			id, ok := area(where)
			if !ok {
				return bad("unknown area " + where)
			}
			seat := seatOfKey(game, key)
			if seat == 0 {
				return bad("name the enemy faction: gc, cc, bg, ys, ww, sl, ow or tt")
			}
			return state.Move{Action: actCapture, Piece: parts[0], To: id, Count: seat}, nil
		case actAscend:
			s := seatOfKey(game, rest)
			if s == 0 {
				return bad("name the enemy faction")
			}
			return state.Move{Action: actAscend, Count: s}, nil
		case actUpgrade:
			name, cnt, hasCnt := strings.Cut(rest, "/")
			if !unit(name) {
				return bad("unknown unit " + name)
			}
			mv := state.Move{Action: actUpgrade, Piece: name}
			if hasCnt {
				n, err := strconv.Atoi(cnt)
				if err != nil || n < 1 {
					return bad("expected upgrade:spawn-of-yog-sothoth/N")
				}
				mv.Count = n
			}
			return mv, nil
		case actCurse:
			s := seatOfKey(game, rest)
			if s == 0 {
				return bad("name the enemy faction")
			}
			return state.Move{Action: actCurse, Count: s}, nil
		case actSyco:
			name, key, _ := strings.Cut(rest, "/")
			s := seatOfKey(game, key)
			if (name != "give" && name != "less") || s == 0 {
				return bad("expected syco:give/tt or syco:less/tt")
			}
			return state.Move{Action: actSyco, Piece: name, Count: s}, nil
		case actTerror:
			if rest != "foe" && rest != "self" {
				return bad("expected terror:foe or terror:self")
			}
			return state.Move{Action: actTerror, Piece: rest}, nil
		case actRitual:
			if rest != "descend" {
				return bad("expected ritual or ritual:descend")
			}
			return state.Move{Action: actRitual, Piece: rest}, nil
		case actShrivel:
			if !unit(rest) {
				return bad("unknown unit " + rest)
			}
			return state.Move{Action: actShrivel, Piece: rest}, nil
		case actRecrim:
			b, ok := bookByID(rest)
			if !ok {
				return bad("unknown spellbook " + rest)
			}
			return state.Move{Action: actRecrim, Piece: rest, Count: b + 1}, nil
		case actMao:
			parts := strings.SplitN(rest, "@", 2)
			if len(parts) != 2 || !unit(parts[0]) {
				return bad("expected mao:unit@area")
			}
			id, ok := area(parts[1])
			if !ok {
				return bad("unknown area " + parts[1])
			}
			return state.Move{Action: actMao, Piece: parts[0], To: id}, nil
		case actBuy:
			name, where, hasWhere := strings.Cut(rest, "@")
			key := ""
			if !hasWhere {
				name, key, _ = strings.Cut(rest, "/")
			} else if w, k, ok := strings.Cut(where, "/"); ok {
				where, key = w, k
			}
			if !isCardUnit(name) || isIndependent(name) {
				return bad("expected buy:unit for a Loyalty Card")
			}
			mv := state.Move{Action: actBuy, Piece: name}
			if hasWhere {
				id, ok := area(where)
				if !ok {
					return bad("unknown area " + where)
				}
				mv.To = id
			}
			if key != "" {
				s := seatOfKey(game, key)
				if s == 0 {
					return bad("unknown faction " + key)
				}
				mv.Count = s
			}
			return mv, nil
		case actGo, actLeave, actRearrange, actBeyond:
			parts := strings.SplitN(rest, "@", 2)
			if len(parts) != 2 || !unit(parts[0]) {
				return bad("expected " + act + ":unit@from>to")
			}
			ft := strings.SplitN(parts[1], ">", 2)
			if len(ft) != 2 {
				return bad("expected " + act + ":unit@from>to")
			}
			from, ok1 := area(ft[0])
			to, ok2 := area(ft[1])
			if !ok1 || !ok2 {
				return bad("unknown area")
			}
			return state.Move{Action: act, Piece: parts[0], From: from, To: to}, nil
		case actAbsorb, actInvisible, actAssign, actSwap, actCannibal:
			name, key := rest, ""
			if j := strings.IndexByte(rest, '/'); j >= 0 {
				name, key = rest[:j], rest[j+1:]
			}
			if !unit(name) {
				return bad("unknown unit " + name)
			}
			mv := state.Move{Action: act, Piece: name}
			if key != "" {
				s := seatOfKey(game, key)
				if s == 0 {
					return bad("unknown faction " + key)
				}
				mv.Count = s
			}
			return mv, nil
		case actBring:
			if rest == bringAll {
				return state.Move{Action: actBring, Piece: bringAll}, nil
			}
			name, from, hasFrom := strings.Cut(rest, "@")
			if !unit(name) {
				return bad("unknown unit " + name)
			}
			mv := state.Move{Action: actBring, Piece: name}
			if hasFrom {
				id, ok := area(from)
				if !ok {
					return bad("unknown area " + from)
				}
				mv.From = id
			}
			return mv, nil
		case actGrant, actBestow:
			s := seatOfKey(game, rest)
			if s == 0 {
				return bad("unknown faction " + rest)
			}
			return state.Move{Action: act, Count: s}, nil
		case actDemand:
			if rest != "signs" && rest != "pains" {
				return bad("expected demand:signs or demand:pains")
			}
			return state.Move{Action: actDemand, Piece: rest}, nil
		case actHarbinger:
			if rest != "power" && rest != "signs" {
				return bad("expected harbinger:power or harbinger:signs")
			}
			return state.Move{Action: actHarbinger, Piece: rest}, nil
		case actRetreat:
			parts := strings.SplitN(rest, ">", 2)
			if len(parts) != 2 || !unit(parts[0]) {
				return bad("expected retreat:unit>area")
			}
			where, key := parts[1], ""
			if j := strings.IndexByte(where, '/'); j >= 0 {
				where, key = where[:j], where[j+1:]
			}
			id, ok := area(where)
			if !ok {
				return bad("unknown area " + where)
			}
			mv := state.Move{Action: actRetreat, Piece: parts[0], To: id}
			if key != "" {
				s := seatOfKey(game, key)
				if s == 0 {
					return bad("unknown faction " + key)
				}
				mv.Count = s
			}
			return mv, nil
		case actLose:
			parts := strings.SplitN(rest, "@", 2)
			if len(parts) != 2 || !unit(parts[0]) {
				return bad("expected lose:unit@area")
			}
			id, ok := area(parts[1])
			if !ok {
				return bad("unknown area " + parts[1])
			}
			return state.Move{Action: actLose, Piece: parts[0], To: id}, nil
		case actBook:
			if b, ok := bookByID(rest); ok {
				return state.Move{Action: actBook, Piece: rest, Count: b + 1}, nil
			}
			return bad("unknown spellbook " + rest)
		case actDesecrate:
			if !unit(rest) {
				return bad("unknown unit " + rest)
			}
			return state.Move{Action: actDesecrate, Piece: rest}, nil
		case "first":
			s := seatOfKey(game, rest)
			if s == 0 {
				return bad("unknown faction " + rest)
			}
			return state.Move{Action: "first", Count: s}, nil
		}
		return bad("unknown action " + act)
	}
	// "action@area" or "action@area/key"
	if i := strings.IndexByte(t, '@'); i > 0 {
		act, rest := t[:i], t[i+1:]
		key := ""
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			rest, key = rest[:j], rest[j+1:]
		}
		id, ok := area(rest)
		if !ok {
			return bad("unknown area " + rest)
		}
		switch act {
		case actRecruit, actGate, actControl, actAbandon, actEmerge, actScream, actShriek, actHWINN, actDevolve, actBlood, actSeek,
			actStart, actIceAge, actRestore, actSlumber, actWake, actDeath, actWalk, actHiero, actIdolatry, actDismantle,
			actMoon, actCrypt, actFilth, actGateOff, actSting, actZygote:
			return state.Move{Action: act, To: id}, nil
		case actCurse:
			return state.Move{Action: actCurse, To: id}, nil
		case actBattle, actCapture, actDreams, actZingaya, actAvatar, actPlace, actSorcery:
			s := seatOfKey(game, key)
			if s == 0 {
				return bad("name the enemy faction: gc, cc, bg, ys, ww, sl, ow or tt")
			}
			return state.Move{Action: act, To: id, Count: s}, nil
		}
		return bad("unknown action " + act)
	}
	return bad("not a Cthulhu Wars move")
}

// ownedPiece is the piece value of a click on a unit that is not the deciding
// player's own: the type, then "@" and the color of its owner (Count is the
// owner's seat), so a click on the right token is unambiguous.
func ownedPiece(game *catalog.CompiledGame, mv state.Move) string {
	if f := factionOfSeat(game, mv.Count); f >= 0 {
		return mv.Piece + "@" + factions[f].Color
	}
	return mv.Piece
}

// Steps decomposes a move into clicks.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	pos := func(id board.PositionID) state.Step { return state.Step{Kind: state.StepPos, Value: string(id)} }
	act := func(id string) state.Step { return state.Step{Kind: state.StepAction, Value: id} }
	piece := func(p string) state.Step { return state.Step{Kind: state.StepPiece, Value: p} }
	switch mv.Action {
	case actRecruit:
		// The High Priest has a button of his own: "Recruit High Priest".
		if mv.Piece == uHighPriest {
			return []state.Step{act("recruit-" + uHighPriest), pos(mv.To)}
		}
		if mv.Piece != "" && mv.Piece != uCultist {
			return []state.Step{act(mv.Action), piece(mv.Piece), pos(mv.To)}
		}
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actGate, actAbandon, actEmerge, actScream, actShriek, actHWINN, actDevolve, actBlood,
		actStart, actIceAge, actRestore, actSlumber, actWake, actDeath:
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actControl:
		// A unit that stands in the area is picked up and put on the Gate,
		// which is clicked: the same clicks as a move that goes nowhere.
		if mv.Piece != "" {
			return []state.Step{pos(mv.To), piece(mv.Piece), pos(mv.To)}
		}
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actSeek:
		if mv.To != "" {
			return []state.Step{act(mv.Action), pos(mv.To)}
		}
		return []state.Step{act(mv.Action)}
	case actSummon:
		// Every monster has a button of its own: "Summon Deep One (1)".
		return []state.Step{act("summon-" + mv.Piece), pos(mv.To)}
	case actAwaken:
		// Every Great Old One has a button of its own: "Awaken Cthulhu (10)".
		// Cthugha also says which Great Old One he replaces.
		if g := replacedBy(mv); g != "" {
			return []state.Step{act("awaken-" + mv.Piece), pos(mv.To), piece(g)}
		}
		return []state.Step{act("awaken-" + mv.Piece), pos(mv.To)}
	case actGo:
		return []state.Step{pos(mv.From), piece(mv.Piece), pos(mv.To)}
	case actLeave:
		// The unit that stands on a Gate is a piece of its own, apart from
		// the rest of its kind: it is picked up from the Gate.
		return []state.Step{pos(mv.From), piece(onGate + mv.Piece), pos(mv.To)}
	case actCapture:
		if mv.Piece == uCultist || mv.Piece == onGate+uCultist {
			// Which of the Cultists of an area: the stack or the Gate's.
			return []state.Step{act(mv.Action), pos(mv.To), piece(mv.Piece), act("vs-" + keyOfSeat(game, mv.Count))}
		}
		if mv.Piece != "" {
			return []state.Step{act(mv.Action), piece(mv.Piece), pos(mv.To), act("vs-" + keyOfSeat(game, mv.Count))}
		}
		return []state.Step{act(mv.Action), pos(mv.To), act("vs-" + keyOfSeat(game, mv.Count))}
	case actBattle, actDreams, actZingaya, actAvatar, actSorcery:
		return []state.Step{act(mv.Action), pos(mv.To), act("vs-" + keyOfSeat(game, mv.Count))}
	case actPlace:
		return []state.Step{act("place-" + keyOfSeat(game, mv.Count)), pos(mv.To)}
	case actPay:
		return []state.Step{act("pay-" + strconv.Itoa(mv.Count))}
	case actBeyond:
		return []state.Step{act(mv.Action), pos(mv.From), piece(mv.Piece), pos(mv.To)}
	case actAscend:
		return []state.Step{act("ascend-" + keyOfSeat(game, mv.Count))}
	case actUpgrade:
		if mv.Count != 0 {
			return []state.Step{act(mv.Action), piece(mv.Piece), act("mutants-" + strconv.Itoa(mv.Count))}
		}
		return []state.Step{act(mv.Action), piece(mv.Piece)}
	case actCurse:
		if mv.To != "" {
			return []state.Step{act(mv.Action), pos(mv.To)}
		}
		return []state.Step{act("curse-" + keyOfSeat(game, mv.Count))}
	case actSyco:
		return []state.Step{act("syco-" + mv.Piece)}
	case actTerror:
		return []state.Step{act("terror-" + mv.Piece)}
	case actRitual:
		if mv.Piece != "" {
			return []state.Step{act("ritual-" + mv.Piece)}
		}
		return []state.Step{act(mv.Action)}
	case actHiero, actIdolatry, actDismantle:
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actAbsorb, actCannibal, actShrivel:
		return []state.Step{act(mv.Action), piece(mv.Piece)}
	case actWalk:
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actRecrim:
		return []state.Step{act("recriminate-" + mv.Piece)}
	case actRearrange:
		return []state.Step{pos(mv.From), piece(mv.Piece), pos(mv.To)}
	case actMao:
		return []state.Step{act(actMao), piece(mv.Piece), pos(mv.To)}
	case actBuy:
		steps := []state.Step{act("buy-" + mv.Piece)}
		if mv.To != "" {
			steps = append(steps, pos(mv.To))
		}
		if mv.Count != 0 {
			steps = append(steps, act("to-"+keyOfSeat(game, mv.Count)))
		}
		return steps
	case actMoon:
		if mv.To != "" {
			return []state.Step{pos(mv.To)}
		}
		return []state.Step{act("moon-" + mv.Piece + "-" + keyOfSeat(game, mv.Count))}
	case actRansom:
		return []state.Step{act("ransom-" + mv.Piece)}
	case actCrypt:
		if mv.To != "" {
			return []state.Step{pos(mv.To)}
		}
		return []state.Step{act("crypt-" + keyOfSeat(game, mv.Count))}
	case actFilth, actGateOff:
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actSting:
		if mv.Piece != "" {
			return []state.Step{piece(mv.Piece), pos(mv.To)}
		}
		return []state.Step{act(mv.Action), pos(mv.To)}
	case actZygote:
		if mv.To != "" {
			return []state.Step{pos(mv.To)}
		}
		return []state.Step{act(mv.Action)}
	case actDiscard:
		return []state.Step{act("discard-" + strconv.Itoa(mv.Count))}
	case actSpare:
		return []state.Step{piece(ownedPiece(game, mv))}
	case actPick:
		return []state.Step{act("pick-" + strconv.Itoa(mv.Count))}
	case actDonate:
		return []state.Step{act("donate-" + mv.Piece)}
	case actInvisible:
		return []state.Step{act(mv.Action), piece(ownedPiece(game, mv))}
	case actAssign:
		return []state.Step{piece(ownedPiece(game, mv))}
	case actSwap:
		return []state.Step{piece(mv.Piece)}
	case actBring:
		switch {
		case mv.Piece == bringAll:
			return []state.Step{act("bring-all")}
		case mv.From != "":
			return []state.Step{piece(mv.Piece), pos(mv.From)}
		}
		return []state.Step{piece(mv.Piece)}
	case actGrant:
		return []state.Step{act("grant-" + keyOfSeat(game, mv.Count))}
	case actBestow:
		return []state.Step{act("bestow-" + keyOfSeat(game, mv.Count))}
	case actHarbinger:
		return []state.Step{act("harbinger-" + mv.Piece)}
	case actDemand:
		return []state.Step{act("demand-" + mv.Piece)}
	case actRetreat:
		return []state.Step{piece(ownedPiece(game, mv)), pos(mv.To)}
	case actLose:
		return []state.Step{act(actLose), piece(mv.Piece), pos(mv.To)}
	case actBook:
		return []state.Step{act("book-" + mv.Piece)}
	case actDesecrate:
		// The action rolls the die; the unit it places is chosen after it.
		if mv.Piece != "" {
			return []state.Step{piece(mv.Piece)}
		}
		return []state.Step{act(actDesecrate)}
	case "first":
		return []state.Step{act("first-" + keyOfSeat(game, mv.Count))}
	}
	return []state.Step{act(mv.Action)}
}

// ActionNotations lists every move notation in a stable order: the fixed
// action space of the game (what an agent's policy head ranges over).
func (rs *Ruleset) ActionNotations(game *catalog.CompiledGame) []string {
	m := mapOf(game)
	var out []string
	seen := map[string]bool{}
	add := func(mv state.Move) {
		s := rs.Format(game, mv)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, id := range m.ids {
		for _, a := range []string{actRecruit, actGate, actControl, actAbandon, actEmerge, actScream, actShriek, actHWINN, actDevolve, actBlood,
			actStart, actIceAge, actRestore, actSlumber, actWake, actDeath} {
			add(state.Move{Action: a, To: id})
		}
		for _, a := range []string{actBattle, actCapture, actDreams, actZingaya, actAvatar, actPlace, actSorcery} {
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: a, To: id, Count: s})
			}
		}
		add(state.Move{Action: actRecruit, Piece: uHighPriest, To: id})
		add(state.Move{Action: actSeek, To: id})
	}
	for _, typ := range unitTypes {
		if lateUnit(typ) {
			continue // added at the end: the earlier numbers do not move
		}
		for _, id := range m.ids {
			switch unitClassOf(typ) {
			case classGOO:
				add(state.Move{Action: actAwaken, Piece: typ, To: id})
			case classMonster:
				add(state.Move{Action: actSummon, Piece: typ, To: id})
			}
			add(state.Move{Action: actLose, Piece: typ, To: id})
			add(state.Move{Action: actRetreat, Piece: typ, To: id})
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: actRetreat, Piece: typ, To: id, Count: s})
			}
		}
		// Moves reach two areas away (Flight), so every pair within two
		// steps is part of the space.
		for a := range m.ids {
			near := map[int]bool{}
			for _, b := range m.adj[a] {
				near[b] = true
				for _, c := range m.adj[b] {
					near[c] = true
				}
			}
			for c := range m.ids {
				if c != a && near[c] {
					add(state.Move{Action: actGo, Piece: typ, From: m.ids[a], To: m.ids[c]})
				}
			}
		}
		for _, a := range []string{actAbsorb, actInvisible, actAssign, actBring, actSwap} {
			add(state.Move{Action: a, Piece: typ})
		}
		for s := 1; s <= nSeats; s++ {
			add(state.Move{Action: actInvisible, Piece: typ, Count: s})
			add(state.Move{Action: actAssign, Piece: typ, Count: s})
			for _, id := range m.ids {
				add(state.Move{Action: actCapture, Piece: typ, To: id, Count: s})
			}
		}
		for _, id := range m.ids {
			add(state.Move{Action: actBring, Piece: typ, From: id})
		}
		add(state.Move{Action: actDesecrate, Piece: typ})
		add(state.Move{Action: actCannibal, Piece: typ})
	}
	add(state.Move{Action: actDesecrate})
	for s := 1; s <= nSeats; s++ {
		add(state.Move{Action: "first", Count: s})
		add(state.Move{Action: actGrant, Count: s})
		add(state.Move{Action: actBestow, Count: s})
	}
	for _, f := range factions {
		for _, b := range f.Books {
			add(state.Move{Action: actBook, Piece: b.ID})
		}
	}
	add(state.Move{Action: actDemand, Piece: "signs"})
	add(state.Move{Action: actDemand, Piece: "pains"})
	add(state.Move{Action: actBring, Piece: bringAll})
	add(state.Move{Action: actHarbinger, Piece: "power"})
	add(state.Move{Action: actHarbinger, Piece: "signs"})
	for _, n := range []int{4, 6, 10} {
		add(state.Move{Action: actPay, Count: n})
	}
	for _, a := range []string{actPass, actDone, actSkip, actRitual, actReveal, actSubmerge, actForms, actGhroth, actSacrifice, actAbduct, actSeek, actNecro, actCW, actCCW,
		actHibernate, actClaim, actHowl, actEternal, actLethargy, actBoon, actBane, actNexus, actOath} {
		add(state.Move{Action: a})
	}
	// A unit that occupies a Gate steps off it by "moving" to its own area
	// (added last, so the numbers of the earlier moves stay as they were).
	for _, typ := range []string{uCultist, uHighPriest, uYoung} {
		for _, id := range m.ids {
			add(state.Move{Action: actGo, Piece: typ, From: id, To: id})
		}
	}
	// The unit on a Gate leaves it with leave (the same area included), and a
	// unit beside the Gate is put on it with control: both added after the
	// rest, so the numbers of the earlier moves stay as they were.
	for _, typ := range []string{uCultist, uHighPriest, uYoung} {
		for a := range m.ids {
			near := map[int]bool{a: true}
			for _, b := range m.adj[a] {
				near[b] = true
				for _, c := range m.adj[b] {
					near[c] = true
				}
			}
			for c := range m.ids {
				if near[c] {
					add(state.Move{Action: actLeave, Piece: typ, From: m.ids[a], To: m.ids[c]})
				}
			}
		}
		for _, id := range m.ids {
			add(state.Move{Action: actControl, Piece: typ, To: id})
		}
	}
	// The Azathoth faction's neutral spellbooks and Monsters (added last).
	for i, b := range neutralBooks {
		add(state.Move{Action: actBook, Piece: b.ID})
		add(state.Move{Action: actRecrim, Piece: b.ID, Count: bookSlots + i + 1})
	}
	for _, f := range factions {
		for i, b := range f.Books {
			add(state.Move{Action: actRecrim, Piece: b.ID, Count: i + 1})
		}
	}
	add(state.Move{Action: actUndim})
	for _, u := range neutralUnits {
		typ := u.ID
		add(state.Move{Action: actShrivel, Piece: typ})
		add(state.Move{Action: actBuy, Piece: typ})
		for s := 1; s <= nSeats; s++ {
			add(state.Move{Action: actBuy, Piece: typ, Count: s})
		}
		for _, id := range m.ids {
			add(state.Move{Action: actSummon, Piece: typ, To: id})
			add(state.Move{Action: actBuy, Piece: typ, To: id})
			add(state.Move{Action: actLose, Piece: typ, To: id})
			add(state.Move{Action: actRetreat, Piece: typ, To: id})
			add(state.Move{Action: actMao, Piece: typ, To: id})
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: actRetreat, Piece: typ, To: id, Count: s})
			}
		}
		for a := range m.ids {
			for c := range m.ids {
				if c != a {
					add(state.Move{Action: actGo, Piece: typ, From: m.ids[a], To: m.ids[c]})
					add(state.Move{Action: actRearrange, Piece: typ, From: m.ids[a], To: m.ids[c]})
				}
			}
		}
	}
	for _, typ := range []string{uCultist, uHighPriest} {
		for _, id := range m.ids {
			add(state.Move{Action: actMao, Piece: typ, To: id})
		}
	}
	for _, id := range m.ids {
		add(state.Move{Action: actWalk, To: id})
	}
	// The Opener of the Way and the Tcho-Tcho (added after everything else).
	for _, f := range factions[facOW:] {
		for _, u := range f.Units {
			if u.ID == uCultist {
				continue
			}
			typ := u.ID
			add(state.Move{Action: actShrivel, Piece: typ})
			add(state.Move{Action: actUpgrade, Piece: typ})
			for _, id := range m.ids {
				switch u.Class {
				case classGOO:
					add(state.Move{Action: actAwaken, Piece: typ, To: id})
				default:
					add(state.Move{Action: actSummon, Piece: typ, To: id})
				}
				add(state.Move{Action: actLose, Piece: typ, To: id})
				add(state.Move{Action: actRetreat, Piece: typ, To: id})
				add(state.Move{Action: actMao, Piece: typ, To: id})
				for s := 1; s <= nSeats; s++ {
					add(state.Move{Action: actRetreat, Piece: typ, To: id, Count: s})
				}
			}
			for a := range m.ids {
				for c := range m.ids {
					if c != a {
						add(state.Move{Action: actGo, Piece: typ, From: m.ids[a], To: m.ids[c]})
						add(state.Move{Action: actRearrange, Piece: typ, From: m.ids[a], To: m.ids[c]})
					}
				}
			}
			for _, k := range []string{"foe", "self"} {
				add(state.Move{Action: actTerror, Piece: k})
			}
			for _, id := range m.ids {
				add(state.Move{Action: actBring, Piece: typ, From: id})
			}
			add(state.Move{Action: actAssign, Piece: typ})
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: actAssign, Piece: typ, Count: s})
			}
		}
	}
	for n := 1; n <= 4; n++ {
		add(state.Move{Action: actUpgrade, Piece: uSpawn, Count: n})
	}
	// The Beyond One: units that cost 3 or more, from any area to any other.
	for _, typ := range unitTypes {
		if cost := costOfType(typ); cost >= 3 {
			for a := range m.ids {
				for c := range m.ids {
					if c != a {
						add(state.Move{Action: actBeyond, Piece: typ, From: m.ids[a], To: m.ids[c]})
					}
				}
			}
		}
	}
	for s := 1; s <= nSeats; s++ {
		add(state.Move{Action: actAscend, Count: s})
		add(state.Move{Action: actCurse, Count: s})
		for _, k := range []string{"give", "less"} {
			add(state.Move{Action: actSyco, Piece: k, Count: s})
		}
	}
	add(state.Move{Action: actRitual, Piece: "descend"})
	add(state.Move{Action: actChannel})
	for _, id := range m.ids {
		add(state.Move{Action: actCurse, To: id})
		add(state.Move{Action: actHiero, To: id})
		add(state.Move{Action: actIdolatry, To: id})
		add(state.Move{Action: actDismantle, To: id})
	}
	// The Loyalty Cards after the first neutral Monsters (added last).
	cardNotations(m, add)
	// Capturing the Cultist that holds a Gate (added after everything else).
	for _, id := range m.ids {
		for s := 1; s <= nSeats; s++ {
			add(state.Move{Action: actCapture, Piece: onGate + uCultist, To: id, Count: s})
		}
	}
	return out
}

// cardNotations adds the moves of the Dreamlands and Beyond Time and Space
// cards and of the Independent Great Old Ones to the action space.
func cardNotations(m *mapData, add func(state.Move)) {
	for _, c := range cards[len(neutralUnits):] {
		typ := c.Unit.ID
		add(state.Move{Action: actShrivel, Piece: typ})
		for _, id := range m.ids {
			switch c.Kind {
			case cardIndependent:
				add(state.Move{Action: actAwaken, Piece: typ, To: id})
			default:
				add(state.Move{Action: actSummon, Piece: typ, To: id})
				add(state.Move{Action: actBuy, Piece: typ, To: id})
			}
			add(state.Move{Action: actLose, Piece: typ, To: id})
			add(state.Move{Action: actRetreat, Piece: typ, To: id})
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: actRetreat, Piece: typ, To: id, Count: s})
			}
		}
		if c.Kind != cardIndependent {
			add(state.Move{Action: actBuy, Piece: typ})
		}
		for a := range m.ids {
			for b := range m.ids {
				if a != b {
					add(state.Move{Action: actGo, Piece: typ, From: m.ids[a], To: m.ids[b]})
				}
			}
		}
		for _, id := range m.ids {
			add(state.Move{Action: actBring, Piece: typ, From: id})
		}
		add(state.Move{Action: actAssign, Piece: typ})
		for s := 1; s <= nSeats; s++ {
			add(state.Move{Action: actAssign, Piece: typ, Count: s})
			add(state.Move{Action: actSpare, Piece: typ, Count: s})
		}
	}
	for _, typ := range []string{uFilth} {
		for _, id := range m.ids {
			add(state.Move{Action: actLose, Piece: typ, To: id})
			add(state.Move{Action: actRetreat, Piece: typ, To: id})
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: actRetreat, Piece: typ, To: id, Count: s})
			}
		}
		add(state.Move{Action: actAssign, Piece: typ})
		add(state.Move{Action: actShrivel, Piece: typ})
		for s := 1; s <= nSeats; s++ {
			add(state.Move{Action: actAssign, Piece: typ, Count: s})
			add(state.Move{Action: actSpare, Piece: typ, Count: s})
		}
	}
	for i := range cards {
		if cards[i].Unit.ID == uCthugha {
			for _, id := range m.ids {
				for _, g := range unitTypes {
					if unitClassOf(g) == classGOO {
						add(state.Move{Action: actAwaken, Piece: uCthugha, To: id, Count: unitTypeIndex[g] + 1})
					}
				}
			}
		}
	}
	for _, id := range m.ids {
		add(state.Move{Action: actMoon, To: id})
		add(state.Move{Action: actCrypt, To: id})
		add(state.Move{Action: actFilth, To: id})
		add(state.Move{Action: actSting, To: id})
		add(state.Move{Action: actZygote, To: id})
		add(state.Move{Action: actGateOff, To: id})
		for _, typ := range []string{uCultist, uHighPriest} {
			add(state.Move{Action: actSting, Piece: typ, To: id})
		}
	}
	add(state.Move{Action: actZygote})
	for _, f := range factions {
		for _, b := range f.Books {
			for s := 1; s <= nSeats; s++ {
				add(state.Move{Action: actMoon, Piece: b.ID, Count: s})
			}
			add(state.Move{Action: actRansom, Piece: b.ID})
		}
	}
	for _, b := range neutralBooks {
		for s := 1; s <= nSeats; s++ {
			add(state.Move{Action: actMoon, Piece: b.ID, Count: s})
		}
		add(state.Move{Action: actRansom, Piece: b.ID})
	}
	for s := 1; s <= nSeats; s++ {
		add(state.Move{Action: actCrypt, Count: s})
	}
	for a := 1; a <= 3; a++ {
		for b := a; b <= 3; b++ {
			add(state.Move{Action: actDiscard, Count: 10*a + b})
		}
		add(state.Move{Action: actPick, Count: a})
	}
	add(state.Move{Action: actDonate, Piece: "give"})
	add(state.Move{Action: actDonate, Piece: "refuse"})
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"Be first to reach 30 Doom with all six spellbooks; when the world is brought to the brink, the cult with the most Doom wins (see the Overview and The Round).",
		},
		CountLabels: map[string]string{"": "Choose"},
		NotationHelp: []string{
			"Areas are named by id: north-america, south-pacific, middle-east…; factions are gc (Great Cthulhu), cc (Crawling Chaos), bg (Black Goat), ys (Yellow Sign), ww (Windwalker), sl (Sleeper).",
			"recruit@asia · gate@asia · summon:deep-one@south-pacific · awaken:cthulhu@south-pacific — the common actions.",
			"go:cultist@asia>europe — move a unit (1 Power each; the first starts a Move action, then more units may follow); done — finish moving.",
			"battle@asia/cc · capture@asia/cc — attack or capture an enemy faction in an area.",
			"control:cultist@asia — put a unit that stands in the area onto its Gate (free: it takes an abandoned Gate, or replaces the unit on your own; a Cultist arriving at an abandoned Gate takes it by itself); leave:cultist@asia>europe — the unit that stands on a Gate moves away, abandoning it (a Move step); leave:cultist@asia>asia steps it off where it is (free, once in a turn per Gate); the other units of its kind move with go. reveal — turn all your Elder Signs face up; pass — drop to 0 Power; done — end your turn.",
			"Battle: absorb:cultist · abduct · invisible:cultist · seek · necro, then assign:deep-one for each result and retreat:deep-one>europe.",
			"Submerge: submerge sinks Cthulhu, then bring:deep-one for each unit that goes with him and done when finished; emerge@area returns them.",
			"The game begins with the first player — Great Cthulhu, else the first cult by name — choosing the direction of play: cw (down the player list) or ccw (up it); the Windwalker, when it is at the table, sets up before that (start@area).",
			"Doom phase: ritual · skip · blood@area · death@area · oath; choices: book:absorb · cw / ccw · first:gc · lose:cultist@asia · desecrate:undead (after desecrate has rolled its die: the unit it places).",
			"Loyalty Cards: buy:gnorri@asia · buy:wamp (a card, in the Doom phase) · moon:frenzy/bg (a Moonbeast on a spellbook) · ransom:frenzy · crypt:cc then crypt@europe (Wamps) · filth@asia · sting@north-atlantic then sting:cultist>europe · zygote then zygote@asia · discard:12 · gateoff@asia · spare:nyarlathotep/cc · pick:2 · donate:give|refuse · awaken:abhoth@asia (an Independent; awaken:cthugha@asia/cthulhu names the Great Old One he replaces).",
			"Expansion cults: start@area · hibernate · iceage@area · claim · howl · cannibal:wendigo · eternal (Windwalker); lethargy · boon · bestow:cc · bane · sorcery@area/cc · restore@area · slumber@area · wake@area · demand:signs · nexus (Sleeper); recruit:high-priest@area · oath (High Priests).",
		},
	}
	if st, err := rs.Start(game, 1); err == nil {
		// The first decision may be another seat's (the Windwalker begins).
		if moves := rs.Legal(game, st, rs.ToMove(game, st)); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	add := func(id, label, hint string) {
		info.Actions = append(info.Actions, state.RulesetAction{ID: id, Label: label, Hint: hint, Source: actionSource(id)})
	}
	// The rules text of a spellbook ("Name (timing): what it does"), of a
	// requirement or of a unit, taken from the faction cards.
	book := func(key, id string) string {
		if fi := factionIndex(key); fi >= 0 {
			for _, b := range factions[fi].Books {
				if b.ID == id {
					return b.Timing + " — " + b.Text
				}
			}
		}
		return ""
	}
	req := func(key, id string) string {
		if fi := factionIndex(key); fi >= 0 {
			for _, r := range factions[fi].Reqs {
				if r.ID == id {
					return r.Text
				}
			}
		}
		return ""
	}
	ability := func(key string) string {
		if fi := factionIndex(key); fi >= 0 {
			return "Faction ability — " + factions[fi].Ability
		}
		return ""
	}
	add(actRecruit, "Recruit Cultist", "Place a Cultist from your pool in an area where you already have a unit — anywhere, when you have no units on the map. A Cultist gives 1 Power in Gather Power and builds and holds Gates.")
	add("recruit-"+uHighPriest, "Recruit High Priest", highPriestRules)
	add(actSummon, "Summon Monster", "Pay the monster's cost: place it at a Gate you control.")
	// One button per monster; the page adds the cost it has just now.
	seenMonster := map[string]bool{}
	for _, f := range factions {
		for _, u := range f.Units {
			if u.Class != classMonster || seenMonster[u.ID] {
				continue
			}
			seenMonster[u.ID] = true
			add("summon-"+u.ID, "Summon "+u.Name, "Place a "+u.Name+" at a Gate you control. "+u.Text)
		}
	}
	add(actAwaken, "Awaken Great Old One", "Pay its cost at the place its card requires.")
	// One button per Great Old One; the page adds the cost it has just now.
	seenGOO := map[string]bool{}
	for _, f := range factions {
		for _, u := range f.Units {
			if u.Class != classGOO || seenGOO[u.ID] {
				continue
			}
			seenGOO[u.ID] = true
			add("awaken-"+u.ID, "Awaken "+u.Name, "How to awaken "+u.Name+": "+awakenText(u.ID))
		}
	}
	add(actGate, "Build Gate", enterText(actGate))
	add(actBattle, "Battle", "Fight one enemy cult in an area where you both have units. Each unit rolls as many dice as its Combat: a 6 is a Kill, a 4 or a 5 a Pain; whoever keeps units wins. With all six spellbooks Battle is an unlimited action, once per area in an Action phase.")
	add(actCapture, "Capture Cultist", "A Great Old One takes an enemy Cultist in its area when the enemy has no Great Old One there; a monster only when the enemy has neither a monster nor a Great Old One there. The Cultist goes home in Gather Power and its captor gains 1 Power. When the enemy has Cultists beside the one that holds a Gate you choose which; capturing the one on the Gate leaves it abandoned and your own Cultist in the area takes it.")
	add(actControl, "Put a unit on the Gate", "Free, and it does not end your turn: a unit of yours that stands in the area of a Gate goes onto it — taking control of an abandoned Gate, or replacing the unit on a Gate of your own. Select the unit, then click the Gate.")
	add(actPass, "Pass (drop to 0 Power)", "Give up your remaining Power: you take no more actions in this Action phase.")
	add(actDone, "Done", "Finish this part of the turn.")
	add(actReveal, "Reveal Elder Signs", "Turn all your Elder Signs face up: their total is added to your Doom and everyone sees them. Signs you gain later stay hidden until you reveal again.")
	add("pay-4", "Pay 4 Power", "Crawling Chaos requirement: "+req("cc", "pay-4")+" It earns a spellbook of your choice.")
	add("pay-6", "Pay 6 Power", "Crawling Chaos requirement: "+req("cc", "pay-6")+" It earns a spellbook of your choice.")
	add("pay-10", "Pay 10 Power", "Crawling Chaos: both spellbook requirements in one action — two spellbooks of your choice.")
	add(actSacrifice, "Sacrifice 2 Cultists", "Black Goat requirement: "+req("bg", "sacrifice")+" It earns a spellbook of your choice.")
	add(actSubmerge, "Submerge", book("gc", "submerge"))
	add(actBring, "Take along", "A unit that goes along: sinks with Cthulhu, walks with the King in Yellow, flies with the Byakhee.")
	add("bring-all", "Take them all", "Every unit that can go along does.")
	add(actSwap, "Swap", "The unit that takes Shub-Niggurath's place.")
	add(actAvatar, "Avatar", abilityText(uShub))
	add(actGrant, "Grant Doom", "Yellow Sign requirement: "+req("ys", "gift")+" It earns a spellbook of your choice.")
	add("harbinger-power", "Take the Power", "The Harbinger: half of that Great Old One's awakening cost in Power.")
	add("harbinger-signs", "Take 2 Elder Signs", "The Harbinger: two Elder Signs instead of the Power.")
	add(actEmerge, "Emerge", "Submerge, later (0 Power): put the units that sank with Cthulhu into any one area.")
	add(actDreams, "Dreams", book("gc", "dreams"))
	add(actDevolve, "Devolve", book("gc", "devolve"))
	add(actForms, "The Thousand Forms", book("cc", "forms"))
	add(actGhroth, "Ghroth", book("bg", "ghroth"))
	add(actScream, "The Screaming Dead", book("ys", "screaming"))
	add(actShriek, "Shriek of the Byakhee", book("ys", "shriek"))
	add(actZingaya, "Zingaya", book("ys", "zingaya"))
	add(actDesecrate, "Desecrate", abilityText(uKing)+" The die is rolled at once and cannot be taken back; the unit is chosen after it. A token on a glyph area meets that requirement, and with The Third Eye and Hastur in play it also earns an Elder Sign.")
	add(actHWINN, "He Who is Not to be Named", book("ys", "hwinn"))
	add(actRitual, "Ritual of Annihilation", "Pay the cost shown on the track: 1 Doom per Gate you control and 1 Elder Sign per Great Old One you have in play; the marker moves one space. Once per Doom phase.")
	add(actSkip, "Finish", "End your part of the Doom phase.")
	add(actBlood, "Blood Sacrifice", book("bg", "blood"))
	add(actAbsorb, "Absorb", book("gc", "absorb"))
	add(actAbduct, "Abduct", book("cc", "abduct"))
	add(actInvisible, "Invisibility", book("cc", "invisibility"))
	add(actSeek, "Seek and Destroy", book("cc", "seek"))
	add(actNecro, "Necrophagy", book("bg", "necro"))
	add(actLose, "Eliminate", "Choose the unit that is eliminated.")
	add(actCW, "Clockwise", "Play passes down the player list, from the last cult back to the first.")
	add(actCCW, "Counter-clockwise", "Play passes up the player list, from the first cult back to the last.")
	add(actStart, "Begin here", "Windwalker: the area marked with its sigil where the cult sets up its Gate and Acolytes.")
	add(actHibernate, "Hibernate", ability("ww"))
	add(actIceAge, "Ice Age", book("ww", "ice-age"))
	add(actClaim, "Take the free spellbook", "Windwalker requirement, available at any time: "+req("ww", "take")+" So it pays to wait.")
	add(actHowl, "Howl", book("ww", "howl"))
	add(actCannibal, "Cannibalism", book("ww", "cannibalism"))
	add(actEternal, "Eternal", abilityText(uRhan))
	add(actLethargy, "Lethargy", abilityText(uTsath)+" Offered only while another player still has Power.")
	add(actBoon, "Boon", "Sleeper requirement: "+req("sl", "boon")+" It earns a spellbook of your choice.")
	add(actBane, "Bane", "Sleeper requirement: "+req("sl", "bane")+" It earns a spellbook of your choice.")
	add(actSorcery, "Ancient Sorcery", book("sl", "ancient-sorcery"))
	add(actRestore, "Return the Serpent Man", "Ancient Sorcery: place the Serpent Man anywhere on the map.")
	add(actSlumber, "Cursed Slumber", book("sl", "cursed-slumber"))
	add(actWake, "Wake", "Cursed Slumber (action, 1 Power): put the sleeping Gate and its Cultist into any area that has no Gate.")
	add(actDeath, "Death from Below", ability("sl"))
	add(actNexus, "Energy Nexus", book("sl", "energy-nexus"))
	add(actOath, "Unspeakable Oath", "The High Priest: sacrifice him at the end of any action — even another player's —, in Gather Power or in the Doom phase: he returns to your pool and you gain 2 Power. He can be recruited again for 3.")
	add("demand-signs", "Give an Elder Sign", "Demand Sacrifice: the Sleeper gains 1 Elder Sign.")
	add("demand-pains", "Kills become Pains", "Demand Sacrifice: all your Kills against the Sleeper's units count as Pains.")
	// The Opener of the Way and the Tcho-Tcho.
	add(actBeyond, "The Beyond One", ability("ow"))
	add(actCurse, "Dread Curse of Azathoth", book("ow", "dread-curse"))
	add(actChannel, "Channel Power", book("ow", "channel-power"))
	add(actUpgrade, "Promote a unit", book("ow", "million-favored"))
	for n := 1; n <= 4; n++ {
		add("mutants-"+strconv.Itoa(n), plural(n, "Mutant", "Mutants"), "The Million Favored Ones: the Spawn of Yog-Sothoth is replaced by this many Mutants from your pool.")
	}
	add("ritual-descend", "Ritual of Annihilation (Dragon Descending)", book("ow", "dragon-descending")+" The Ritual is paid as usual.")
	add("terror-foe", "Lower the enemy's Combat", book("tt", "terror")+" Here: the enemy's Combat is reduced by 1 per Proto-Shoggoth in the battle.")
	add("terror-self", "Raise your Combat", book("tt", "terror")+" Here: your Combat is increased by 1 per Proto-Shoggoth in the battle.")
	add("syco-give", "Give the Tcho-Tcho 1 Doom", ability("tt")+" Here: the Tcho-Tcho gains 1 Doom.")
	add("syco-less", "Earn 1 less Doom", ability("tt")+" Here: your Ritual earns 1 Doom fewer.")
	add(actHiero, "Place the High Priest", book("tt", "hierophants"))
	add(actIdolatry, "Idolatry", book("tt", "idolatry"))
	add(actDismantle, "Remove your Gate", "Tcho-Tcho requirement: "+req("tt", "dismantle")+" It earns a spellbook of your choice.")
	for _, f := range factions {
		add("ascend-"+f.Key, "Dragon Ascending: copy "+f.Name, book("ow", "dragon-ascending"))
		add("curse-"+f.Key, "Curse "+f.Name, "Dread Curse of Azathoth: this result falls on "+f.Name+" in the area.")
	}
	// The Azathoth faction: neutral spellbooks and Monsters.
	nb := func(n int) string { return neutralBooks[n].Timing + "\n" + neutralBooks[n].Text }
	add(actShrivel, "Shriveling", nb(nbShrivel))
	add(actUndim, "Undimensioned", nb(nbUndim))
	add(actMao, "Sacrifice a Cultist", nb(nbMao))
	add(actWalk, "Walk Between Worlds", "Place a Dimensional Shambler from your faction card in any area.")
	for _, b := range neutralBooks {
		add("book-"+b.ID, b.Name, b.Timing+"\n"+b.Text)
	}
	for _, f := range factions {
		for _, b := range f.Books {
			add("recriminate-"+b.ID, "Set aside "+b.Name, "Recriminations: remove "+b.Name+" from your faction card and take another available spellbook instead.")
		}
	}
	for _, b := range neutralBooks {
		add("recriminate-"+b.ID, "Set aside "+b.Name, "Recriminations: remove "+b.Name+" from your faction card and take another available spellbook instead.")
	}
	for _, u := range neutralUnits {
		add("summon-"+u.ID, "Summon "+u.Name, "Place a "+u.Name+" at a Gate you control. "+u.Text)
		add("buy-"+u.ID, "Buy the "+u.Name+" card (2 Doom)", loyaltyText[u.ID]+" "+u.Text+" One card a Doom phase; only you may summon and control its Monsters from then on.")
	}
	for _, f := range factions {
		add("to-"+f.Key, "Give the card to "+f.Name, "The Servitor of the Outer Gods card goes to "+f.Name+": Adulation is theirs.")
	}
	// The Dreamlands and Beyond Time and Space cards, and the Independents.
	for _, c := range cards[len(neutralUnits):] {
		u := c.Unit
		switch c.Kind {
		case cardIndependent:
			add("awaken-"+u.ID, "Awaken "+u.Name, "How to awaken the Independent "+u.Name+": "+numbered(c.Awaken)+" "+u.Text)
		default:
			add("summon-"+u.ID, "Summon "+u.Name, "Place a "+u.Name+" at a Gate you control. "+u.Text)
			add("buy-"+u.ID, "Buy the "+u.Name+" card ("+cardCostText(c)+")", c.Buy+" "+u.Text+" One Loyalty Card a Doom phase; only you may summon and control it from then on.")
		}
	}
	add(actMoon, "Lay the Moonbeast", abilityText(uMoonbeast))
	add(actRansom, "Pay 1 Doom: free the spellbook", "A Moonbeast lies on your spellbook: pay 1 Doom (any time it is your turn, no Action) and it goes back to the map.")
	add(actCrypt, "Crypt Dweller", abilityText(uWamp))
	add(actFilth, "Filth", abilityText(uAbhoth))
	add(actSting, "The Agony Sting", abilityText(uHydra))
	add(actZygote, "The Zygote", cards[cardIndex(uHydra)].Book.Timing+" — "+cards[cardIndex(uHydra)].Book.Text)
	add(actDiscard, "Discard 2 Elder Signs", "Chaugnar Faugn's requirement: "+cards[cardIndex(uChaugnar)].Req+" It earns his spellbook.")
	add(actGateOff, "Remove a Gate", "Yig's requirement: "+cards[cardIndex(uYig)].Req+" It earns his spellbook.")
	add(actSpare, "Spare", abilityText(uCthugha))
	add(actPick, "Choose the Elder Sign", cards[cardIndex(uChaugnar)].Book.Text)
	add(actDonate, "Messenger of Yig", cards[cardIndex(uYig)].Book.Text)
	add("donate-give", "Give 1 Power", "Messenger of Yig: give 1 Power to Yig's controller.")
	add("donate-refuse", "Refuse", "Messenger of Yig: refuse, and Yig's controller gains 1 Doom.")
	for a := 1; a <= 3; a++ {
		for b := a; b <= 3; b++ {
			add("discard-"+strconv.Itoa(10*a+b), "Discard a "+strconv.Itoa(a)+" and a "+strconv.Itoa(b), "Chaugnar Faugn's requirement: discard these two Elder Signs (no Doom for them).")
		}
		add("pick-"+strconv.Itoa(a), "Give a "+strconv.Itoa(a), "Curse of Chaugnar Faugn: the Elder Sign received is worth "+strconv.Itoa(a)+".")
	}
	for _, f := range factions {
		add("crypt-"+f.Key, "Let "+f.Name+" place the Wamps", "Crypt Dweller: "+f.Name+" places all the Wamps of your pool in areas without a Gate.")
		for _, b := range f.Books {
			add("moon-"+b.ID+"-"+f.Key, "Lay it on "+f.Name+"'s "+b.Name, "Blasphemous Obeisance: "+f.Name+" cannot use "+b.Name+" while the Moonbeast lies there.")
		}
		for _, b := range neutralBooks {
			add("moon-"+b.ID+"-"+f.Key, "Lay it on "+f.Name+"'s "+b.Name, "Blasphemous Obeisance: "+f.Name+" cannot use "+b.Name+" while the Moonbeast lies there.")
		}
	}
	for _, f := range factions {
		for _, b := range f.Books {
			add("ransom-"+b.ID, "Pay 1 Doom: free "+b.Name, "The Moonbeast on "+b.Name+" goes back to the map.")
		}
	}
	for _, b := range neutralBooks {
		add("ransom-"+b.ID, "Pay 1 Doom: free "+b.Name, "The Moonbeast on "+b.Name+" goes back to the map.")
	}
	for _, f := range factions {
		add("vs-"+f.Key, f.Name, "Against "+f.Name+".")
		add("place-"+f.Key, "Place a "+f.Name+" Cultist", "Ghroth puts a "+f.Name+" Acolyte anywhere on the map.")
		add("grant-"+f.Key, "Give 3 Doom to "+f.Name, "Yellow Sign's requirement: "+f.Name+" gains 3 Doom.")
		add("bestow-"+f.Key, "Give 3 Power to "+f.Name, "Sleeper's requirement (3 Power): "+f.Name+" gains 3 Power.")
		add("first-"+f.Key, f.Name, "Make "+f.Name+" the first player.")
		for _, b := range f.Books {
			// The timing, a line break, then the rule: the page shows them apart.
			add("book-"+b.ID, b.Name, b.Timing+"\n"+b.Text)
		}
	}
	return info
}

// numbered writes steps as one numbered paragraph: "1. … 2. …".
func numbered(steps []string) string {
	out := make([]string, len(steps))
	for i, st := range steps {
		out[i] = fmt.Sprintf("%d. %s", i+1, st)
	}
	return strings.Join(out, " ")
}
