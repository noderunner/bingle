package session

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// A reaction is an ability a player may use on another player's turn (see
// state.Reactor). The game asks its holder after the other player's action —
// which stops the game until they answer. So that a game never waits on a
// person, the decline is played for them by default; they choose, per game,
// the occasions on which they want to be asked instead: when an action is
// aimed at them ("targeted"), after any other player's action, or after the
// actions of chosen players. Bots, AI clients and a person who plays several
// seats themselves are asked every time.

// ReactionSeat is another seat a person may choose to be asked about.
type ReactionSeat struct {
	Seat  int    `json:"seat"`
	Color string `json:"color"`
	Label string `json:"label,omitempty"`
	Name  string `json:"name,omitempty"`
}

// ReactionSettings is the reaction side of one person's seat: the reactions
// of their faction (and whether they hold them yet), the occasions on which
// they want to be asked, and the seats those may name.
type ReactionSettings struct {
	Seat      int                     `json:"seat"`
	Abilities []state.ReactionAbility `json:"abilities"`
	// Targeted: ask when an action is aimed at the player — at the player
	// itself, at one of its units or at an area where it has a unit or a Gate.
	Targeted bool `json:"targeted"`
	// AskAll: ask after any other player's action. AskSeats: ask after the
	// actions of these seats.
	AskAll   bool  `json:"ask_all"`
	AskSeats []int `json:"ask_seats"`
	// Choices is the choice in force for each reaction, by ability id: the
	// reactions can differ (Madness is asked about every time at first, the
	// Unspeakable Oath only when targeted). The three fields above are the
	// choice for any reaction not set apart.
	Choices map[string]store.AbilityPref `json:"choices"`
	Others  []ReactionSeat               `json:"others"`
	// Pending is set while a reaction waits for this player's answer.
	Pending *state.Reaction `json:"pending,omitempty"`
}

// ReactionUpdate is a change to a player's choice: the fields that are set
// replace the current ones, the others stay. Ability names the reaction (an
// ability id) the change is for; without it the change is made to every
// reaction.
type ReactionUpdate struct {
	Ability  string
	Targeted *bool
	AskAll   *bool
	AskSeats *[]int
}

// ErrNoReactions is returned for a game whose ruleset has no reactions.
var ErrNoReactions = fmt.Errorf("session: this game has no reactions")

// ReactionSettings returns the reaction abilities, the choice of when to be
// asked and the pending reaction (if any) of the player called name.
func (m *Manager) ReactionSettings(ctx context.Context, roomCode, name string) (ReactionSettings, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return ReactionSettings{}, err
	}
	return m.reactionSettings(ctx, game, name)
}

// soloSeat says the person at seat plays other seats of the game too (a game
// against oneself).
func soloSeat(players []store.Player, seat int) bool {
	var me *store.Player
	for i := range players {
		if players[i].Seat == seat {
			me = &players[i]
		}
	}
	if me == nil || me.Bot != "" {
		return false
	}
	for _, p := range players {
		if p.Seat != seat && p.Bot == "" && sameOwner(p.Name, me.Name) {
			return true
		}
	}
	return false
}

// reactionPrefs is seat's effective choice: what it saved, converted when the
// row is from the earlier opposite choice, else the defaults — a person is
// asked when targeted and otherwise passed for, except for the reactions that
// ask every time until the person says otherwise (Madness). abilities are the
// seat's reactions.
func (m *Manager) reactionPrefs(ctx context.Context, game store.Game, compiled *catalog.CompiledGame, seat int, abilities []state.ReactionAbility) (store.ReactionPrefs, error) {
	p, found, err := m.store.ReactionPrefs(ctx, game.ID, seat)
	if err != nil {
		return store.ReactionPrefs{}, err
	}
	if !found {
		// A person who plays several seats of a game against themselves is the
		// only one who could be held up, and the page offers no settings for
		// them: every reaction is put to them, as to a bot.
		if players, err := m.store.ListPlayers(ctx, game.ID); err == nil && soloSeat(players, seat) {
			return store.ReactionPrefs{V: 2, Targeted: true, AskAll: true, AskSeats: []int{}}, nil
		}
		out := store.ReactionPrefs{V: 2, Targeted: true, AskSeats: []int{}}
		for _, ab := range abilities {
			if ab.AskAlways {
				if out.Abilities == nil {
					out.Abilities = map[string]store.AbilityPref{}
				}
				out.Abilities[ab.ID] = store.AbilityPref{Targeted: true, AskAll: true, AskSeats: []int{}}
			}
		}
		return out, nil
	}
	if !p.Legacy() {
		return p, nil
	}
	return legacyChoice(compiled, seat, p.All, p.Seats), nil
}

// choiceOf is the choice in force for the reaction with id: the one set apart
// for it, else the general one.
func choiceOf(prefs store.ReactionPrefs, id string) store.AbilityPref {
	c, ok := prefs.Abilities[id]
	if !ok {
		c = store.AbilityPref{Targeted: prefs.Targeted, AskAll: prefs.AskAll, AskSeats: prefs.AskSeats}
	}
	c.AskSeats = append([]int{}, c.AskSeats...)
	return c
}

// abilityID is the id of the reaction called name among abilities ("" if none).
func abilityID(abilities []state.ReactionAbility, name string) string {
	for _, ab := range abilities {
		if strings.EqualFold(ab.Name, name) {
			return ab.ID
		}
	}
	return ""
}

// legacyChoice translates the earlier, opposite choice — which reactions to
// pass: everything (so nothing is asked), those seats' (the others are asked),
// or nothing (all are asked) — into the occasions to be asked on.
func legacyChoice(compiled *catalog.CompiledGame, seat int, all bool, seats []int) store.ReactionPrefs {
	out := store.ReactionPrefs{V: 2, AskSeats: []int{}}
	switch {
	case all:
	case len(seats) == 0:
		out.AskAll = true
	default:
		out.Targeted = true
		passed := map[int]bool{}
		for _, s := range seats {
			passed[s] = true
		}
		for _, sd := range compiled.Definition.Seats {
			if sd.Seat != seat && !passed[sd.Seat] {
				out.AskSeats = append(out.AskSeats, sd.Seat)
			}
		}
	}
	return out
}

// SetReactionsLegacy is SetReactions for a client that still sends the earlier
// choice (all, seats: what to pass): it is translated into the occasions to be
// asked on.
func (m *Manager) SetReactionsLegacy(ctx context.Context, roomCode, name string, all bool, seats []int) (ReactionSettings, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return ReactionSettings{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return ReactionSettings{}, fmt.Errorf("session: game definition %q not found", game.GameDefID)
	}
	cur, err := m.reactionSettings(ctx, game, name)
	if err != nil {
		return ReactionSettings{}, err
	}
	c := legacyChoice(compiled, cur.Seat, all, seats)
	return m.SetReactions(ctx, roomCode, name, ReactionUpdate{Targeted: &c.Targeted, AskAll: &c.AskAll, AskSeats: &c.AskSeats})
}

func (m *Manager) reactionSettings(ctx context.Context, game store.Game, name string) (ReactionSettings, error) {
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return ReactionSettings{}, err
	}
	var me *store.Player
	byseat := map[int]store.Player{}
	for i := range players {
		byseat[players[i].Seat] = players[i]
		if players[i].Bot == "" && strings.EqualFold(players[i].Name, name) {
			me = &players[i]
		}
	}
	if me == nil {
		return ReactionSettings{}, ErrNotAPlayer
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return ReactionSettings{}, fmt.Errorf("session: game definition %q not found", game.GameDefID)
	}
	out := ReactionSettings{Seat: me.Seat, Abilities: []state.ReactionAbility{}, Targeted: true, AskSeats: []int{}, Choices: map[string]store.AbilityPref{}, Others: []ReactionSeat{}}
	for _, sd := range compiled.Definition.Seats {
		if sd.Seat == me.Seat {
			continue
		}
		rs := ReactionSeat{Seat: sd.Seat, Color: sd.Color, Label: sd.Label}
		if p, ok := byseat[sd.Seat]; ok {
			rs.Name = p.Name
		}
		out.Others = append(out.Others, rs)
	}
	if game.Status == "waiting" {
		return out, nil
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return ReactionSettings{}, err
	}
	if ab := state.ReactionAbilities(compiled, bs, me.Seat); len(ab) > 0 {
		out.Abilities = ab
	}
	prefs, err := m.reactionPrefs(ctx, game, compiled, me.Seat, out.Abilities)
	if err != nil {
		return ReactionSettings{}, err
	}
	out.Targeted, out.AskAll, out.AskSeats = prefs.Targeted, prefs.AskAll, prefs.AskSeats
	for _, ab := range out.Abilities {
		out.Choices[ab.ID] = choiceOf(prefs, ab.ID)
	}
	if game.Status == "active" {
		if r := state.PendingReaction(compiled, bs); r != nil && r.Seat == me.Seat {
			out.Pending = r
		}
	}
	return out, nil
}

// SetReactions stores the occasions on which the player called name wants to
// be asked about their reactions (the rest are declined for them). Fields the
// update leaves unset keep their value. When a reaction of theirs is waiting
// right now it is declined at once, unless it is one they asked for.
func (m *Manager) SetReactions(ctx context.Context, roomCode, name string, upd ReactionUpdate) (ReactionSettings, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return ReactionSettings{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return ReactionSettings{}, fmt.Errorf("session: game definition %q not found", game.GameDefID)
	}
	cur, err := m.reactionSettings(ctx, game, name)
	if err != nil {
		return ReactionSettings{}, err
	}
	abilities := cur.Abilities
	if game.Status == "waiting" {
		abilities = nil
	}
	prefs, err := m.reactionPrefs(ctx, game, compiled, cur.Seat, abilities)
	if err != nil {
		return ReactionSettings{}, err
	}
	valid := map[int]bool{}
	for _, o := range cur.Others {
		valid[o.Seat] = true
	}
	cleanSeats := func(in []int) []int {
		clean := []int{}
		seen := map[int]bool{}
		for _, s := range in {
			if valid[s] && !seen[s] {
				seen[s] = true
				clean = append(clean, s)
			}
		}
		sort.Ints(clean)
		return clean
	}
	apply := func(c *store.AbilityPref) {
		if upd.Targeted != nil {
			c.Targeted = *upd.Targeted
		}
		if upd.AskAll != nil {
			c.AskAll = *upd.AskAll
		}
		if upd.AskSeats != nil {
			c.AskSeats = cleanSeats(*upd.AskSeats)
		}
	}
	if upd.Ability != "" {
		known := false
		for _, ab := range cur.Abilities {
			known = known || ab.ID == upd.Ability
		}
		if !known {
			return ReactionSettings{}, fmt.Errorf("session: no reaction %q to choose for", upd.Ability)
		}
		c := choiceOf(prefs, upd.Ability)
		apply(&c)
		if prefs.Abilities == nil {
			prefs.Abilities = map[string]store.AbilityPref{}
		}
		prefs.Abilities[upd.Ability] = c
	} else {
		// Every reaction: the general choice and each one set apart.
		g := store.AbilityPref{Targeted: prefs.Targeted, AskAll: prefs.AskAll, AskSeats: prefs.AskSeats}
		apply(&g)
		prefs.Targeted, prefs.AskAll, prefs.AskSeats = g.Targeted, g.AskAll, g.AskSeats
		for id, c := range prefs.Abilities {
			apply(&c)
			prefs.Abilities[id] = c
		}
	}
	if err := m.store.SetReactionPrefs(ctx, game.ID, cur.Seat, prefs); err != nil {
		return ReactionSettings{}, err
	}
	// A window that is already open may now be passed.
	if game.Status == "active" {
		m.runBots(ctx, roomCode)
		if game, err = m.loadGame(ctx, roomCode, true); err != nil {
			return ReactionSettings{}, err
		}
	}
	return m.reactionSettings(ctx, game, name)
}

// AskEveryReaction makes the game ask the player called name about every
// reaction, as it asks a bot: an AI client answers each window itself, so
// nothing is passed for it. It leaves a choice the player already made alone.
func (m *Manager) AskEveryReaction(ctx context.Context, roomCode, name string) error {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return err
	}
	for _, p := range players {
		if p.Bot != "" || !strings.EqualFold(p.Name, name) {
			continue
		}
		if _, found, err := m.store.ReactionPrefs(ctx, game.ID, p.Seat); err != nil || found {
			return err
		}
		return m.store.SetReactionPrefs(ctx, game.ID, p.Seat, store.ReactionPrefs{Targeted: true, AskAll: true})
	}
	return nil
}

// autoPass returns the declining move to play for a person whose reaction is
// pending and who did not ask to be asked about it: asking is by occasion —
// the action was aimed at them, it was any other player's, or it was a chosen
// player's.
func (m *Manager) autoPass(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, bs *state.BoardState, player store.Player) (state.Move, bool) {
	r := state.PendingReaction(compiled, bs)
	if r == nil || r.Seat != player.Seat {
		return state.Move{}, false
	}
	abilities := state.ReactionAbilities(compiled, bs, player.Seat)
	prefs, err := m.reactionPrefs(ctx, game, compiled, player.Seat, abilities)
	if err != nil {
		return state.Move{}, false
	}
	choice := choiceOf(prefs, abilityID(abilities, r.Ability))
	ask := choice.AskAll || (choice.Targeted && r.Targeted)
	for _, s := range choice.AskSeats {
		if s == r.Trigger {
			ask = true
		}
	}
	if ask {
		return state.Move{}, false
	}
	for _, mv := range state.LegalMoves(compiled, bs, player.Seat) {
		if mv.Action == r.Pass.Action && mv.Piece == r.Pass.Piece && mv.To == r.Pass.To && mv.Count == r.Pass.Count {
			return mv, true
		}
	}
	return state.Move{}, false
}
