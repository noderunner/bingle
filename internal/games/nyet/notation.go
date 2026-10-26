package nyet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation:
//
//	nyet:trump:red      cross off a box of the board (nyet:first:wolf, nyet:discard:passleft,
//	                    nyet:super:none, nyet:points:-2 …)
//	team:crane+tiger    the first player's team (team:alone, or the partners' names)
//	bonus:tiger         the seat that takes the x2 bonus card
//	discard:blue-9      a card to discard      pass:blue-9   a card to pass left
//	blue-9              a card played to the trick (colour-value)

// seatName is a seat's character, lower case ("wolf").
func seatName(game *catalog.CompiledGame, seat int) string {
	for _, s := range game.Definition.Seats {
		if s.Seat == seat {
			l := s.Label
			if l == "" {
				l = s.Color
			}
			return strings.ToLower(l)
		}
	}
	return strconv.Itoa(seat)
}

func seatByName(game *catalog.CompiledGame, name string) (int, bool) {
	for _, s := range game.Definition.Seats {
		if seatName(game, s.Seat) == name {
			return s.Seat, true
		}
	}
	return 0, false
}

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	switch mv.Action {
	case actCross:
		line, option, _ := strings.Cut(mv.Piece, ":")
		if line == lineFirst {
			s, _ := strconv.Atoi(option)
			option = seatName(game, s)
		}
		return "nyet:" + line + ":" + option
	case actTeam:
		if mv.Piece == "alone" {
			return "team:alone"
		}
		var names []string
		for _, p := range strings.Split(mv.Piece, ",") {
			s, _ := strconv.Atoi(p)
			names = append(names, seatName(game, s))
		}
		return "team:" + strings.Join(names, "+")
	case actBonus:
		s, _ := strconv.Atoi(mv.Piece)
		return "bonus:" + seatName(game, s)
	case actDiscard:
		return "discard:" + mv.Piece
	case actGive:
		return "pass:" + mv.Piece
	}
	return mv.Piece
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("nyet: cannot read %q as a move (%s)", text, why)
	}
	known := func(id string) bool { return game.Cards.Types[id] != nil }
	switch {
	case strings.HasPrefix(t, "nyet:"):
		line, option, ok := strings.Cut(t[5:], ":")
		if !ok {
			return bad("expected nyet:line:option")
		}
		if line == lineFirst {
			s, ok := seatByName(game, option)
			if !ok {
				return bad("unknown character " + option)
			}
			option = strconv.Itoa(s)
		}
		return state.Move{Action: actCross, Piece: line + ":" + option}, nil
	case strings.HasPrefix(t, "team:"):
		who := t[5:]
		if who == "alone" {
			return state.Move{Action: actTeam, Piece: "alone"}, nil
		}
		var seats []string
		for _, name := range strings.Split(who, "+") {
			s, ok := seatByName(game, name)
			if !ok {
				return bad("unknown character " + name)
			}
			seats = append(seats, strconv.Itoa(s))
		}
		return state.Move{Action: actTeam, Piece: strings.Join(seats, ",")}, nil
	case strings.HasPrefix(t, "bonus:"):
		s, ok := seatByName(game, t[6:])
		if !ok {
			return bad("unknown character " + t[6:])
		}
		return state.Move{Action: actBonus, Piece: strconv.Itoa(s)}, nil
	case strings.HasPrefix(t, "discard:"):
		if !known(t[8:]) {
			return bad("unknown card " + t[8:])
		}
		return state.Move{Action: actDiscard, Piece: t[8:]}, nil
	case strings.HasPrefix(t, "pass:"):
		if !known(t[5:]) {
			return bad("unknown card " + t[5:])
		}
		return state.Move{Action: actGive, Piece: t[5:]}, nil
	case known(t):
		return state.Move{Action: actPlay, Piece: t}, nil
	}
	return bad("not a card, a team or a box of the board")
}

// Steps: one action per decision (a card is card-<type>).
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	switch mv.Action {
	case actCross:
		return []state.Step{{Kind: state.StepAction, Value: "cross-" + strings.Replace(mv.Piece, ":", "-", 1)}}
	case actTeam:
		return []state.Step{{Kind: state.StepAction, Value: "team-" + strings.ReplaceAll(mv.Piece, ",", "-")}}
	case actBonus:
		return []state.Step{{Kind: state.StepAction, Value: "bonus-" + mv.Piece}}
	}
	return []state.Step{{Kind: state.StepAction, Value: "card-" + mv.Piece}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			fmt.Sprintf("The game lasts %d rounds (the number depends on the players); each round every player scores the points their team earns, and the player with the most points at the end wins (a tie is a draw).", game.ParamInt("rounds", 8)),
			"A team scores, for each trick it won and each 1 it captured as loot, the points the round's last box says (1, 2, 3, 4 or −2); the bonus card doubles its owner's points.",
		},
		NotationHelp: []string{
			"Crossing a box of the board: nyet:trump:red (hearts), nyet:super:none, nyet:discard:passleft, nyet:points:-2, nyet:first:wolf (the box left empty on each line sets the rule of the round).",
			"The first player's team: team:crane (a partner), team:crane+tiger (two), team:alone; bonus:tiger gives the x2 bonus card.",
			"Cards are colour-value, the four colours being the four suits (red hearts, blue spades, yellow diamonds, green clubs): blue-9 is the 9 of spades. Playing a card is just its name; discard:blue-9 discards one and pass:blue-9 passes it to the left.",
		},
	}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, rs.ToMove(game, st)); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	return info
}
