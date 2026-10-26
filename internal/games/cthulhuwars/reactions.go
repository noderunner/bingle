package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Reactions are abilities a player may use on another player's turn: Great
// Cthulhu's Devolve ("after any player's Action"), Black Goat's Necrophagy,
// Windwalker's Cannibalism and Crawling Chaos' Madness in battles they are not
// part of, the Sleeper's Energy Nexus before any battle in an area with a
// Wizard, and the High Priest's Unspeakable Oath. The rulebook calls them
// interruptions: they happen after another player's action, and when two
// players want one, the first player in turn order goes first.
//
// In this engine a reaction is an ordinary decision of the reacting seat — a
// prompt ("react") queued right after the action, or the post-battle and
// retreat stages when the seat is not in the battle — so the generic session
// can recognise it (state.Reactor), ask the player — or, unless they asked to
// be asked on this occasion (the action was aimed at them, or was a chosen
// player's), decline for them.

var _ state.Reactor = (*Ruleset)(nil)

// acted records that the active seat just completed an action: before the
// next action the other players holding reactions are asked (see
// openReactions).
func (e *eng) acted() {
	d := e.d
	if d.Turn != nil && d.Phase == phaseAction {
		d.ReactOwed = d.Turn.Seat
		d.ReactAim = d.Aim.clone()
	}
}

// noteAim records what the decision is aimed at, for the reactions that follow
// the action (see Aim): a decision that begins an action starts the aim afresh
// and the steps of a Move in progress add their destinations. Only actions
// have an aim — the free actions and the effects of a battle do not.
func (e *eng) noteAim(seat int, mv state.Move) {
	d := e.d
	if d.Phase != phaseAction || (d.Turn != nil && d.Turn.Nexus) {
		return
	}
	switch d.Stage {
	case stageTurn:
		d.Aim = nil
	case stageMoving:
	default:
		return
	}
	if mv.Action == actLeave && mv.From == mv.To {
		return // stepping off a Gate is free: not an action
	}
	var seats []int
	var areas []string
	area := func() {
		if mv.To != "" {
			areas = append(areas, string(mv.To))
		}
	}
	switch mv.Action {
	case actRecruit, actSummon, actAwaken, actGate, actEmerge, actScream, actShriek, actHWINN, actIceAge, actWake, actSlumber, actGo, actLeave,
		actFilth, actGateOff:
		area()
	case actSting:
		// The Agony Sting hits the enemies that have Cultists in the ocean.
		area()
		if mv.To != "" {
			for _, o := range e.others(seat) {
				if e.cultists(e.areaIdx(string(mv.To)), o) > 0 {
					seats = append(seats, o)
				}
			}
		}
	case actBattle, actCapture, actDreams, actZingaya, actAvatar:
		seats = append(seats, mv.Count)
		area()
	case actGrant, actBestow, actSorcery:
		seats = append(seats, mv.Count)
	case actForms, actGhroth, actBoon, actBane:
		seats = append(seats, e.others(seat)...)
	case actDesecrate:
		if a, ok := e.gooArea(seat, uKing); ok {
			areas = append(areas, e.areaID(a))
		}
	default:
		return
	}
	if d.Aim == nil {
		d.Aim = &Aim{}
	}
	for _, s := range seats {
		if s != seat && !containsInt(d.Aim.Seats, s) {
			d.Aim.Seats = append(d.Aim.Seats, s)
		}
	}
	for _, a := range areas {
		if !containsStr(d.Aim.Areas, a) {
			d.Aim.Areas = append(d.Aim.Areas, a)
		}
	}
}

func containsInt(l []int, v int) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// present says seat has a presence in area a: a unit of its own there, or a
// Gate it controls.
func (e *eng) present(a, seat int) bool {
	return e.total(a, seat) > 0 || e.d.Gates[e.areaID(a)] == seat
}

// targetedBy says an action aimed at aim reached seat: it named the seat (a
// battle against it, a gift, Ghroth…) or touched an area where the seat has a
// presence (a unit, or a Gate it controls).
func (e *eng) targetedBy(seat int, aim *Aim) bool {
	if aim == nil {
		return false
	}
	if containsInt(aim.Seats, seat) {
		return true
	}
	for _, id := range aim.Areas {
		if e.present(e.areaIdx(id), seat) {
			return true
		}
	}
	return false
}

// devolveAvailable reports whether seat may Devolve now: it holds the
// spellbook, has a Deep One in its pool and an Acolyte to replace.
func (e *eng) devolveAvailable(seat int) bool {
	return e.is(seat, facGC) && e.d.hasBook(seat, bkDevolve) && e.pool(seat, uDeepOne) > 0 && e.onMap(seat, uCultist) > 0
}

// devolve replaces seat's Cultist in area a with a Deep One from its pool.
func (e *eng) devolve(seat, a int) {
	if !e.take(a, seat, uCultist) {
		return
	}
	e.setPool(seat, uCultist, e.pool(seat, uCultist)+1)
	e.place(a, seat, uDeepOne)
	e.say("Devolve: %s's Cultist in %s becomes a Deep One.", e.seatName(seat), e.areaName(a))
	// A Deep One cannot hold a Gate.
	e.syncGate(a)
}

// openReactions offers the reactions owed after an action: one prompt per
// player other than the actor who holds one, in turn order from the first
// player. Called once the action has fully resolved and play is back at a
// turn. then names what runs when the windows are closed ("react": the turn
// goes on; "endturn": the phase may now end).
func (e *eng) openReactions(then string) {
	d := e.d
	actor, aim := d.ReactOwed, d.ReactAim
	d.ReactOwed, d.ReactAim = 0, nil
	if actor == 0 {
		return
	}
	for _, s := range e.turnOrder(d.First, d.Dir) {
		// Walk Between Worlds: "after any Action (by any player)", the actor's
		// own included. It is always worth asking about.
		if e.walkAvailable(s) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "react", Seat: s, By: actor, Why: "walk", Targeted: true})
		}
		if s == actor {
			continue
		}
		targeted := e.targetedBy(s, aim)
		if e.devolveAvailable(s) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "react", Seat: s, By: actor, Why: "devolve", Targeted: targeted})
		}
		// Unspeakable Oath: "at the end of any player's Action" — asked after
		// every action of another player, while the High Priest is on the map.
		if e.oathAvailable(s) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "react", Seat: s, By: actor, Why: "oath", Targeted: targeted})
		}
	}
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts(then)
		e.beginPrompts(then)
	}
}

// reactMoves lists what the seat of a "react" prompt may do: use the ability
// (as often as it has something to use it on) or decline. It is empty when
// there is nothing left to react with, which closes the window.
func (e *eng) reactMoves(p Prompt) []state.Move {
	switch p.Why {
	case "devolve":
		var out []state.Move
		if e.d.hasBook(p.Seat, bkDevolve) && e.pool(p.Seat, uDeepOne) > 0 {
			for a := range e.m.ids {
				if p.Area != "" && e.areaID(a) != p.Area {
					continue
				}
				if e.count(a, p.Seat, uCultist) > 0 {
					out = append(out, state.Move{Action: actDevolve, To: e.m.ids[a]})
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return append(out, state.Move{Action: actDone})
	case "oath":
		if !e.oathAvailable(p.Seat) {
			return nil
		}
		return []state.Move{{Action: actOath}, {Action: actDone}}
	case "walk":
		return e.walkMoves(p.Seat)
	}
	return nil
}

// reactionName names the ability of a "react" prompt.
func reactionName(why string) string {
	switch why {
	case "devolve":
		return "Devolve"
	case "oath":
		return "Unspeakable Oath"
	case "walk":
		return "Walk Between Worlds"
	}
	return why
}

// PendingReaction implements state.Reactor: the position waits for an
// optional reaction when it is a "react" prompt, or a post-battle or retreat
// decision of a seat that is not part of the battle (Necrophagy, Cannibalism,
// Madness), or the Sleeper's Energy Nexus window.
func (rs *Ruleset) PendingReaction(game *catalog.CompiledGame, st *state.BoardState) *state.Reaction {
	d, err := dataOf(st)
	if err != nil {
		return nil
	}
	return rs.newEng(game, st, d).pendingReaction()
}

// pendingReaction is PendingReaction for a decoded position.
func (e *eng) pendingReaction() *state.Reaction {
	d := e.d
	if d.Phase == phaseOver {
		return nil
	}
	decline := state.Move{Action: actDone}
	switch d.Stage {
	case stagePrompt:
		if len(d.Prompts) > 0 {
			p := d.Prompts[0]
			switch p.Kind {
			case "react":
				return &state.Reaction{Seat: p.Seat, Trigger: p.By, Ability: reactionName(p.Why), Targeted: p.Targeted, Pass: decline}
			case "nexus":
				trigger := 0
				if d.Bat != nil {
					trigger = d.Bat.Att
				}
				return &state.Reaction{Seat: p.Seat, Trigger: trigger, Ability: "Energy Nexus", Targeted: e.inBattleArea(p.Seat), Pass: decline}
			}
		}
	case stagePost:
		if b := d.Bat; b != nil && b.Who != b.Att && b.Who != b.Def {
			name := "Necrophagy"
			if e.is(b.Who, facWW) {
				name = "Cannibalism"
			}
			return &state.Reaction{Seat: b.Who, Trigger: b.Att, Ability: name, Targeted: e.inBattleArea(b.Who), Pass: decline}
		}
	case stageRetreat:
		if b := d.Bat; b != nil && b.Retreater != 0 && b.Retreater != b.Att && b.Retreater != b.Def {
			return &state.Reaction{Seat: b.Retreater, Trigger: b.Att, Ability: "Madness", Targeted: e.inBattleArea(b.Retreater), Pass: decline}
		}
	}
	return nil
}

// inBattleArea says seat has a presence in the area of the battle under way:
// the battle is aimed at that area, so the seat is "targeted" by it.
func (e *eng) inBattleArea(seat int) bool {
	b := e.d.Bat
	return b != nil && e.present(e.areaIdx(b.Area), seat)
}

// ReactionAbilities implements state.Reactor: the reactions of seat's
// faction, each flagged with whether the cult has earned the spellbook yet.
// They are listed from the start of the game: the choice of when to be asked
// does not depend on holding the book. The Unspeakable Oath belongs to
// every cult while the High Priests are in the box.
func (rs *Ruleset) ReactionAbilities(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.ReactionAbility {
	d, err := dataOf(st)
	if err != nil || seat < 1 || seat > nSeats {
		return nil
	}
	e := rs.newEng(game, st, d)
	f := e.fac(seat)
	if f < 0 || !e.active(seat) {
		return nil
	}
	var out []state.ReactionAbility
	add := func(book int) {
		b := factions[f].Books[book]
		out = append(out, state.ReactionAbility{ID: b.ID, Name: b.Name, Text: b.Text, Held: d.hasBook(seat, book), AskAlways: book == bkMadness})
	}
	switch f {
	case facGC:
		add(bkDevolve)
	case facCC:
		add(bkMadness)
	case facBG:
		add(bkNecro)
	case facWW:
		add(bkCannibal)
	case facSL:
		add(bkNexus)
	}
	if d.loyal(seat, neutralUnitIndex(uShambler)) {
		out = append(out, state.ReactionAbility{
			ID: "walk-between-worlds", Name: "Walk Between Worlds", Held: true,
			Text: "After any Action — yours or another player's — put the Dimensional Shamblers waiting on your faction card in any areas. You are always asked while some wait.",
		})
	}
	if e.hasHP() {
		out = append(out, state.ReactionAbility{
			ID: "unspeakable-oath", Name: "Unspeakable Oath", Held: true,
			Text: "At the end of any player's Action — even when it is not your turn — sacrifice your High Priest to gain 2 Power. You are asked after every action of the others while he stands on the map.",
		})
	}
	return out
}
