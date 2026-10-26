package cthulhuwars

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// GuideTuning shifts the scores the bot guide gives to the kinds of decision
// it weighs (see guide.go). Every field is an offset added to the guide's own
// number, so the zero value is the guide as written. The scores only need to be
// ordered sensibly, so a shift of a few points changes which kind of action the
// bot prefers: that is what the tuner (internal/tune) searches. Shipped values
// per cult live in tuned.json and are applied to every bot of that cult.
type GuideTuning struct {
	// Gate, Recruit, Awaken and Capture shift the value of building a Gate,
	// recruiting a Cultist, awakening a Great Old One and capturing a unit.
	Gate    float64 `json:"gate,omitempty"`
	Recruit float64 `json:"recruit,omitempty"`
	Awaken  float64 `json:"awaken,omitempty"`
	Capture float64 `json:"capture,omitempty"`
	// SummonUrgent is the value of a protector where an enemy already stands,
	// SummonWatch where one could walk in, SummonIdle of any other summon.
	SummonUrgent float64 `json:"summon_urgent,omitempty"`
	SummonWatch  float64 `json:"summon_watch,omitempty"`
	SummonIdle   float64 `json:"summon_idle,omitempty"`
	// OpeningMove is a Cultist walking to a free area to build a Gate, GuardMove
	// a monster walking to guard what is exposed, HuntMove one walking to
	// capture Cultists.
	OpeningMove float64 `json:"opening_move,omitempty"`
	GuardMove   float64 `json:"guard_move,omitempty"`
	HuntMove    float64 `json:"hunt_move,omitempty"`
	// Battle shifts every battle the guide would fight: higher is bolder.
	Battle float64 `json:"battle,omitempty"`
	// EndTurn shifts the value of ending the turn after the main action: higher
	// means fewer extra actions (saving Power), lower means spending it.
	EndTurn float64 `json:"end_turn,omitempty"`
	// Reserve shifts the penalty for spending the Power kept to answer a threat.
	Reserve float64 `json:"reserve,omitempty"`
	// Req shifts the reward for a step toward a spellbook requirement (see
	// guide_req.go): higher chases the six books harder.
	Req float64 `json:"req,omitempty"`
	// Slack shifts how far below the best score a move may be and still be
	// considered: higher plays less predictably.
	Slack float64 `json:"slack,omitempty"`
}

// Knob describes one field of GuideTuning for a search: its name, the range
// worth searching and a pointer to the field in a given tuning.
type Knob struct {
	Name     string
	Min, Max float64
	Ptr      func(*GuideTuning) *float64
}

// GuideKnobs lists the fields of GuideTuning with their search ranges.
var GuideKnobs = []Knob{
	{"gate", -40, 40, func(t *GuideTuning) *float64 { return &t.Gate }},
	{"recruit", -40, 40, func(t *GuideTuning) *float64 { return &t.Recruit }},
	{"awaken", -40, 40, func(t *GuideTuning) *float64 { return &t.Awaken }},
	{"capture", -40, 40, func(t *GuideTuning) *float64 { return &t.Capture }},
	{"summon_urgent", -40, 40, func(t *GuideTuning) *float64 { return &t.SummonUrgent }},
	{"summon_watch", -40, 40, func(t *GuideTuning) *float64 { return &t.SummonWatch }},
	{"summon_idle", -30, 50, func(t *GuideTuning) *float64 { return &t.SummonIdle }},
	{"opening_move", -40, 40, func(t *GuideTuning) *float64 { return &t.OpeningMove }},
	{"guard_move", -40, 40, func(t *GuideTuning) *float64 { return &t.GuardMove }},
	{"hunt_move", -40, 40, func(t *GuideTuning) *float64 { return &t.HuntMove }},
	{"battle", -40, 40, func(t *GuideTuning) *float64 { return &t.Battle }},
	{"end_turn", -45, 45, func(t *GuideTuning) *float64 { return &t.EndTurn }},
	{"reserve", -35, 35, func(t *GuideTuning) *float64 { return &t.Reserve }},
	{"req", -12, 40, func(t *GuideTuning) *float64 { return &t.Req }},
	{"slack", -3, 10, func(t *GuideTuning) *float64 { return &t.Slack }},
}

//go:embed tuned.json
var tunedJSON []byte

var (
	tunedOnce sync.Once
	tuned     map[string]GuideTuning
)

// ShippedTuning is the tuning bots of the cult with the given key (gc, cc, bg,
// ys, ww, sl) play with unless told otherwise: the zero tuning for a cult with
// no entry in tuned.json.
func ShippedTuning(cult string) GuideTuning {
	tunedOnce.Do(func() {
		tuned = map[string]GuideTuning{}
		if err := json.Unmarshal(tunedJSON, &tuned); err != nil {
			panic(fmt.Sprintf("cthulhuwars: tuned.json: %v", err))
		}
	})
	return tuned[cult]
}

// GuideMovesTuned is GuideMoves with an explicit tuning for the seat (nil: the
// shipped tuning of its cult).
func (rs *Ruleset) GuideMovesTuned(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move, t *GuideTuning) []state.Move {
	return rs.guideMoves(game, st, seat, legal, t)
}
