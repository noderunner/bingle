package cthulhuwars_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

func picksCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	return cat
}

func fourWith(extra map[string]any) map[string]any {
	o := map[string]any{"factions": []any{"gc", "cc", "bg", "ys"}}
	for k, v := range extra {
		o[k] = v
	}
	return o
}

// Single units and spellbooks name the game, and the id loads again by itself.
func TestSinglePicksNameTheGameAndLoadAgain(t *testing.T) {
	cat := picksCatalog(t)
	id, err := cat.Derive("cthulhu-wars", fourWith(map[string]any{
		"neutral-units": []any{"moonbeast", "hound-of-tindalos"},
		"neutral-books": []any{"shrivel", "mao"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Bits 4 and 9 of the cards, 0 and 3 of the spellbooks.
	if want := "cthulhu-wars~gc-cc-bg-ys-nu210-nb9"; id != want {
		t.Fatalf("id = %q, want %q", id, want)
	}
	fresh := picksCatalog(t)
	g, ok := fresh.Get(id)
	if !ok {
		t.Fatal("the id does not load in a new catalog")
	}
	for _, want := range []string{"Moonbeast", "Hound of Tindalos", "Shriveling", "The Mao Ceremony"} {
		if !strings.Contains(g.Definition.Name, want) {
			t.Errorf("the name %q does not say %s", g.Definition.Name, want)
		}
	}
	// The order they are named in makes no other game.
	again, err := cat.Derive("cthulhu-wars", fourWith(map[string]any{
		"neutral-units": []any{"hound-of-tindalos", "moonbeast"},
		"neutral-books": []any{"mao", "shrivel"},
	}))
	if err != nil || again != id {
		t.Errorf("another order = %q, %v", again, err)
	}
	// Changing a cult in the lobby keeps them.
	opts, ok, err := state.SeatCultOptions(g, 4, "ww", map[int]bool{})
	if err != nil || !ok {
		t.Fatalf("changing a cult: %v %v", ok, err)
	}
	moved, err := cat.Derive("cthulhu-wars", opts)
	if err != nil || moved != "cthulhu-wars~gc-cc-bg-ww-nu210-nb9" {
		t.Errorf("after the lobby change = %q, %v", moved, err)
	}
}

// One table has one id: a pack named card by card is the pack.
func TestATableHasOneId(t *testing.T) {
	cat := picksCatalog(t)
	for name, c := range map[string]struct {
		opts map[string]any
		want string
	}{
		"a whole pack":                {map[string]any{"neutral-units": []any{"moonbeast", "gnorri", "shantak"}}, "cthulhu-wars~gc-cc-bg-ys-dl"},
		"a card of a pack that is in": {map[string]any{"dreamlands": true, "neutral-units": []any{"gnorri"}}, "cthulhu-wars~gc-cc-bg-ys-dl"},
		"all of Azathoth": {map[string]any{
			"neutral-units": []any{"dimensional-shambler", "star-vampire", "elder-thing", "servitor"},
			"neutral-books": []any{"shrivel", "stars", "recrim", "mao", "umr", "undim"},
		}, "cthulhu-wars~gc-cc-bg-ys-az"},
		"Azathoth's Monsters without its books": {map[string]any{
			"neutral-units": []any{"dimensional-shambler", "star-vampire", "elder-thing", "servitor"},
		}, "cthulhu-wars~gc-cc-bg-ys-nuf"},
		"a book of Azathoth, which is in": {map[string]any{"azathoth": true, "neutral-books": []any{"mao"}}, "cthulhu-wars~gc-cc-bg-ys-az"},
	} {
		id, err := cat.Derive("cthulhu-wars", fourWith(c.opts))
		if err != nil || id != c.want {
			t.Errorf("%s: %q, %v; want %q", name, id, err, c.want)
		}
	}
}

func TestSinglePicksRefuseWhatIsNotOne(t *testing.T) {
	cat := picksCatalog(t)
	for name, opts := range map[string]map[string]any{
		"an unknown unit":   {"neutral-units": []any{"cthulhu"}},
		"an Independent":    {"neutral-units": []any{"abhoth"}},
		"an unknown book":   {"neutral-books": []any{"ghroth"}},
		"not a list":        {"neutral-units": "moonbeast"},
		"a list of numbers": {"neutral-books": []any{1}},
	} {
		if _, err := cat.Derive("cthulhu-wars", fourWith(opts)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A card chosen by itself can be bought; the rest of its pack cannot.
func TestOnlyTheChosenCardOfAPackIsForSale(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"neutral-units": []any{"gnorri", "star-vampire"}})
	st := doomPhase(tb, "gc", xp{
		units: []xu{{who: "gc", typ: "cultist", area: "south-pacific", n: 2}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 5}, doom: map[string]int{"gc": 6},
		pool: map[string]int{"gc|gnorri": 0, "gc|star-vampire": 0},
	})
	got := tb.can(st, "gc")
	for _, want := range []string{"buy:gnorri@south-pacific", "buy:star-vampire@south-pacific"} {
		if !has(got, want) {
			t.Errorf("%s is not offered: %v", want, got)
		}
	}
	for _, no := range []string{"buy:shantak@south-pacific", "buy:moonbeast", "buy:elder-thing@south-pacific", "buy:dimensional-shambler"} {
		if has(got, no) {
			t.Errorf("%s is offered: %v", no, got)
		}
	}
}

// A neutral spellbook chosen by itself can be taken when a requirement earns a
// book; the other five cannot.
func TestOnlyTheChosenNeutralSpellbookIsOffered(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"neutral-books": []any{"shrivel"}})
	st := tb.build(xp{stage: "prompt", edit: func(d *cthulhuwars.Data) {
		d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: tb.seat("gc")}}
		d.Resume = "turn"
		d.Turn = &cthulhuwars.TurnState{Seat: tb.seat("gc")}
	}})
	got := tb.can(st, "gc")
	if !has(got, "book:shriveling") || has(got, "book:the-mao-ceremony") || has(got, "book:undimensioned") {
		t.Errorf("book choices %v", got)
	}
}

// The Elder Thing's ability comes with its card, not with the whole of Azathoth.
func TestAnElderThingChosenAloneStillStopsDevour(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"neutral-units": []any{"elder-thing"}})
	bgSeat := tb.seat("bg")
	st := tb.build(xp{
		units: []xu{{"bg", "fungi", "europe", 1}, {"gc", "cthulhu", "europe", 1}, {"bg", "elder-thing", "europe", 1}},
		power: map[string]int{"bg": 4}, turn: "bg", rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1, 1),
		edit: func(d *cthulhuwars.Data) { d.Loyal[bgSeat] = 1 << cdElderThing },
	})
	st.SetReserve(tb.colour["bg"], "elder-thing", 2)
	st = tb.as(st, "bg", "battle@europe/gc")
	if tb.n(st, "bg", "fungi", "europe") != 1 {
		t.Errorf("Devour with an Elder Thing in the area: %d Fungi left", tb.n(st, "bg", "fungi", "europe"))
	}
}

// A random game with single picks plays through without an error.
func TestATableOfSinglePicksPlays(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{
		"neutral-units": []any{"gnorri", "hound-of-tindalos"},
		"neutral-books": []any{"shrivel"},
		"igoo-yig":      true,
	})
	rs, ok := state.RulesetOf(tb.g)
	if !ok {
		t.Fatal("no ruleset")
	}
	st, err := rs.Start(tb.g, 7)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		seat := rs.ToMove(tb.g, st)
		legal := rs.Legal(tb.g, st, seat)
		if len(legal) == 0 {
			break
		}
		res, err := rs.Apply(tb.g, st, seat, legal[i%len(legal)])
		if err != nil {
			t.Fatalf("move %d: %v", i, err)
		}
		st = res.NewState
	}
}

type catalogJSON struct {
	Tabs []struct {
		ID    string `json:"id"`
		Items []struct {
			ID        string `json:"id"`
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Option    string `json:"option"`
			Value     string `json:"value"`
			Group     string `json:"group"`
			Budget    bool   `json:"budget"`
			Text      string `json:"text"`
			Abilities []any  `json:"abilities"`
		} `json:"items"`
	} `json:"tabs"`
	Presets []struct {
		ID        string         `json:"id"`
		GameDefID string         `json:"game_def_id"`
		Options   map[string]any `json:"options"`
	} `json:"presets"`
}

// Everything the picker offers is an option the game takes, in a box and with
// a description; what the definition lists for other clients agrees with it.
func TestTheSetupCatalogAgreesWithTheOptions(t *testing.T) {
	cat := picksCatalog(t)
	g, ok := cat.Get("cthulhu-wars")
	if !ok {
		t.Fatal("no game")
	}
	raw, ok, err := state.SetupCatalogOf(g)
	if err != nil || !ok {
		t.Fatalf("catalog: %v %v", ok, err)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var c catalogJSON
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	declared := map[string]map[string]bool{}
	bools := map[string]bool{}
	for _, o := range g.Definition.Options {
		if o.Kind == "bool" {
			bools[o.ID] = true
		}
		declared[o.ID] = map[string]bool{}
		for _, ch := range o.Choices {
			declared[o.ID][ch.ID] = true
		}
	}
	seen := map[string]bool{}
	offered := map[string]int{}
	for _, tab := range c.Tabs {
		for _, it := range tab.Items {
			if seen[it.ID] {
				t.Errorf("%s is offered twice", it.ID)
			}
			seen[it.ID] = true
			if it.Value != "" {
				if !declared[it.Option][it.Value] {
					t.Errorf("%s: option %s does not list %q", it.ID, it.Option, it.Value)
				}
				offered[it.Option]++
			} else if !bools[it.Option] {
				t.Errorf("%s: %q is not a bool option", it.ID, it.Option)
			}
			if it.Text == "" && len(it.Abilities) == 0 {
				t.Errorf("%s has no text", it.ID)
			}
			if (tab.ID == "neutrals" || tab.ID == "independents") && (!it.Budget || it.Group == "") {
				t.Errorf("%s: budget %v, group %q", it.ID, it.Budget, it.Group)
			}
		}
	}
	for _, opt := range []string{"factions", "neutral-units", "neutral-books"} {
		if offered[opt] != len(declared[opt]) {
			t.Errorf("%s: %d offered, %d declared", opt, offered[opt], len(declared[opt]))
		}
	}
	// The ready-made setups make games.
	if len(c.Presets) == 0 {
		t.Fatal("no presets")
	}
	for _, p := range c.Presets {
		if _, err := cat.Derive(p.GameDefID, p.Options); err != nil {
			t.Errorf("preset %s: %v", p.ID, err)
		}
	}
}
