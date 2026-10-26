package catalog_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// base is a small valid 4x4 placement game; each case patches one piece of it.
func base(extra string) string {
	// extra is spliced in as additional top-level members.
	return `{
		"id": "v", "name": "V",
		"board": {"generator": {"type":"grid","width":4,"height":4,"adjacency":"orthogonal4","labels":"algebraic"}},
		"seats": [{"seat":1,"color":"white"},{"seat":2,"color":"black"}],
		"pieces": {"p": {"tags":["t"]}, "q": {}},
		"turn_order": {"type":"round-robin","seats":[1,2]},
		"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}],
		"win_condition": {"type":"none"}
		` + extra + `}`
}

func TestRegister_ValidV2Definition(t *testing.T) {
	if err := catalog.NewCatalog().Register("v", []byte(base("")), nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRegister_AcceptsASemanticVersionAndRejectsOtherVersions(t *testing.T) {
	if err := catalog.NewCatalog().Register("v", []byte(base(`, "version": "0.12.3"`)), nil); err != nil {
		t.Fatalf("a major.minor.patch version: %v", err)
	}
	for _, bad := range []string{"1.0", "v1.0.0", "1.0.0-beta", "01.0.0", "latest"} {
		err := catalog.NewCatalog().Register("v", []byte(base(`, "version": "`+bad+`"`)), nil)
		if err == nil || !strings.Contains(err.Error(), "major.minor.patch") {
			t.Errorf("version %q: error %v, want one about major.minor.patch", bad, err)
		}
	}
}

func TestRegister_RejectsBadV2References(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string // substring of the error
	}{
		{"setup on unknown position", base(`, "setup": [{"position":"z9","piece":"p","color":"white"}]`), "unknown position"},
		{"setup unknown piece", base(`, "setup": [{"position":"a1","piece":"nope","color":"white"}]`), "unknown piece"},
		{"setup unknown color", base(`, "setup": [{"position":"a1","piece":"p","color":"green"}]`), "no seat plays"},
		{"reserve for unknown pool", base(`, "reserves": {"ghost": 3}`), "unknown pool"},
		{"negative reserve", base(`, "reserves": {"p": -1}`), "negative"},
		{"duplicate seat color", strings.Replace(base(""), `"color":"black"`, `"color":"white"`, 1), "duplicate seat color"},
		{"duplicate seat number", strings.Replace(base(""), `{"seat":2,"color":"black"}`, `{"seat":1,"color":"black"}`, 1), "duplicate seat number"},
		{"unknown action type", strings.Replace(base(""), `"type":"place"`, `"type":"teleport"`, 1), "unknown type"},
		{"place unknown piece", strings.Replace(base(""), `"pieces":["p"]`, `"pieces":["zzz"]`, 1), "unknown piece"},
		{"place unknown region", strings.Replace(base(""), `"source":"supply"`, `"source":"supply","region":"middle"`, 1), "unknown region"},
		{"place unknown source", strings.Replace(base(""), `"source":"supply"`, `"source":"bag"`, 1), "unknown source"},
		{"gravity unknown direction", strings.Replace(base(""), `"source":"supply"`, `"source":"supply","gravity":"UP"`, 1), "gravity"},
		{"duplicate action id", strings.Replace(base(""), `"actions": [`, `"actions": [{"id":"place","type":"place","pieces":["q"]},`, 1), "duplicate action"},
		{"stack move bad carry keyword", base(`, "actions2": 0`)[:0] + strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"mv","type":"move-stack","carry_limit":"huge"}]`, 1), "carry_limit"},
		{"opening unknown action", strings.Replace(base(""), `"seats":[1,2]}`, `"seats":[1,2],"opening":{"plies":1,"actions":["ghost"]}}`, 1), "unknown action"},
		{"opening opponent with 3 seats", strings.Replace(strings.Replace(base(""), `"seats":[1,2]}`, `"seats":[1,2,3],"opening":{"plies":1,"owner":"opponent"}}`, 1), `{"seat":2,"color":"black"}]`, `{"seat":2,"color":"black"},{"seat":3,"color":"red"}]`, 1), "exactly two seats"},
		{"connect unknown region", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"connect","regions":[["left","nowhere"]]}]}`, 1), "unknown region"},
		{"connect unknown tag", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"connect","tag":"ghost","regions":[["left","right"]]}]}`, 1), "tag"},
		{"slide without pieces", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide"}]`, 1), "must list its pieces"},
		{"slide unknown piece", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["ghost"]}]`, 1), "unknown piece"},
		{"slide bad occupancy", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"occupancy":"anywhere"}]`, 1), "occupancy"},
		{"slide negative distance", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"max_distance":-2}]`, 1), "negative"},
		{"slide unknown then type", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"then":{"type":"teleport","piece":"q"}}]`, 1), "then type"},
		{"slide unknown then piece", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"then":{"type":"shoot","piece":"ghost"}}]`, 1), "unknown piece"},
		{"slide bad direction", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"directions":["up-ish"]}]`, 1), "direction"},
		{"leap without pieces", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"l","type":"leap","max_distance":2}]`, 1), "must list its pieces"},
		{"leap unknown piece", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"l","type":"leap","pieces":["ghost"],"max_distance":2}]`, 1), "unknown piece"},
		{"leap without distance", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"l","type":"leap","pieces":["p"]}]`, 1), "max_distance"},
		{"leap clone beyond range", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"l","type":"leap","pieces":["p"],"max_distance":1,"clone_distance":2}]`, 1), "clone_distance"},
		{"custodian bad length", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"p","type":"place","pieces":["p"],"source":"supply","custodian":{"lengths":[0]}}]`, 1), "custodian lengths"},
		{"custodian unknown counter pool", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"p","type":"place","pieces":["p"],"source":"supply","custodian":{"count_in":"ghost"}}]`, 1), "unknown pool"},
		{"pool rule unknown pool", strings.Replace(strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"p","type":"place","pieces":["p"],"source":"supply"}]`, 1), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"pool","pool":"ghost","at_least":3}]}`, 1), "unknown pool"},
		{"pool rule needs a threshold", strings.Replace(base(`, "reserves": {"p": 3}`), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"pool","pool":"p"}]}`, 1), "at_least"},
		{"slide forbids unknown region", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"forbid_region":"moon"}]`, 1), "unknown region"},
		{"custodian anvil unknown region", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"slide","pieces":["p"],"custodian":{"lengths":[1],"anvils":["moon"]}}]`, 1), "anvil region"},
		{"absent unknown piece", strings.Replace(strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"p","type":"place","pieces":["p"],"source":"supply"}]`, 1), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"absent","piece":"ghost","seat":1}]}`, 1), "unknown piece"},
		{"absent needs a seat", strings.Replace(strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"p","type":"place","pieces":["p"],"source":"supply"}]`, 1), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"absent","piece":"p"}]}`, 1), "needs the seat"},
		{"swap negative ply", strings.Replace(base(""), `"actions": [{"id":"place","type":"place","pieces":["p"],"source":"supply"}]`, `"actions": [{"id":"s","type":"swap","plies":[-1]}]`, 1), "negative ply"},
		{"unknown result", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"line","length":3,"result":"tie"}]}`, 1), "result"},
		{"result on end rule", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"end","when":["board-full"],"result":"lose","decide":{"type":"draw"}}]}`, 1), "decide"},
		{"ring needs a hex board", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"ring"}]}`, 1), "hex board"},
		{"touch more than listed", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"connect","of":["left","right"],"touch":3}]}`, 1), "touch"},
		{"touch needs two", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"connect","of":["left","right"],"touch":1}]}`, 1), "touch"},
		{"of and regions together", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"connect","of":["left","right"],"touch":2,"regions":[["top","bottom"]]}]}`, 1), "not both"},
		{"of unknown region", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"connect","of":["left","moon"],"touch":2}]}`, 1), "unknown region"},
		{"line too short", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"line","length":1}]}`, 1), "length"},
		{"end unknown trigger", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"end","when":["sunrise"],"decide":{"type":"draw"}}]}`, 1), "trigger"},
		{"end without decide", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"end","when":["board-full"]}]}`, 1), "decide"},
		{"rules type without rules", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules"}`, 1), "at least one rule"},
		{"ptn needs algebraic grid", strings.Replace(strings.Replace(base(""), `,"labels":"algebraic"`, ``, 1), `"win_condition"`, `"notation":"ptn","win_condition"`, 1), "algebraic"},
		{"unknown notation", base(`, "notation": "xyz"`), "notation"},
		{"unknown board style", strings.Replace(base(""), `"board": {`, `"board": {"style":"hexes",`, 1), "style"},
		{"flattenable_by unknown piece", strings.Replace(base(""), `"q": {}`, `"q": {"stack":{"flattenable_by":["ghost"],"flatten_to":"p"}}`, 1), "flattenable_by"},
		{"step action without pieces", strings.Replace(base(""), `"actions": [`, `"actions": [{"id":"hop","type":"step"},`, 1), "list its pieces"},
		{"step action reserved id", strings.Replace(base(""), `"actions": [`, `"actions": [{"id":"step","type":"step","pieces":["p"]},`, 1), "reserved"},
		{"step action unknown piece", strings.Replace(base(""), `"actions": [`, `"actions": [{"id":"hop","type":"step","pieces":["ghost"]},`, 1), "unknown piece"},
		{"step action unknown occupancy", strings.Replace(base(""), `"actions": [`, `"actions": [{"id":"hop","type":"step","pieces":["p"],"occupancy":"swap"},`, 1), "occupancy"},
		{"relative direction without forward", strings.Replace(base(""), `"actions": [`, `"actions": [{"id":"hop","type":"step","pieces":["p"],"directions":["forward"]},`, 1), "forward"},
		{"invalid seat forward", strings.Replace(base(""), `{"seat":1,"color":"white"}`, `{"seat":1,"color":"white","forward":"NE"}`, 1), "forward direction"},
		{"promotion unknown region", strings.Replace(base(""), `"p": {"tags":["t"]}`, `"p": {"tags":["t"],"promote":[{"region":"nowhere","to":"q"}]}`, 1), "unknown region"},
		{"promotion unknown piece", strings.Replace(base(""), `"p": {"tags":["t"]}`, `"p": {"tags":["t"],"promote":[{"region":"top","to":"ghost"}]}`, 1), "promotes to unknown piece"},
		{"reach unknown region", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"reach","region":"nowhere"}]}`, 1), "unknown region"},
		{"ply-limit needs plies", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"end","when":["ply-limit"],"decide":{"type":"draw"}}]}`, 1), "plies"},
		{"rule for an unknown seat", strings.Replace(base(""), `"win_condition": {"type":"none"}`, `"win_condition": {"type":"rules","rules":[{"type":"reach","seat":9,"region":"top"}]}`, 1), "unknown seat"},
		{"flatten_to unknown piece", strings.Replace(base(""), `"q": {}`, `"q": {"stack":{"flattenable_by":["p"],"flatten_to":"ghost"}}`, 1), "flatten_to"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := catalog.NewCatalog()
			err := cat.Register("v", []byte(tc.doc), nil)
			if err == nil {
				t.Fatalf("Register succeeded, want an error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			if _, ok := cat.Get("v"); ok {
				t.Error("a rejected definition must not be registered")
			}
		})
	}
}

func TestRegisterModule_RegistersEveryVariant(t *testing.T) {
	doc := strings.Replace(base(""), `"win_condition"`, `"variants":[{"id":"v2","patch":{"board":{"generator":{"width":6}}}}],"win_condition"`, 1)
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(catalog.Module{ID: "v", Definition: []byte(doc)}); err != nil {
		t.Fatal(err)
	}
	a, okA := cat.Get("v")
	b, okB := cat.Get("v2")
	if !okA || !okB {
		t.Fatalf("registered: v=%v v2=%v", okA, okB)
	}
	if a.Board.Width() != 4 || b.Board.Width() != 6 {
		t.Errorf("widths = %d/%d, want 4/6", a.Board.Width(), b.Board.Width())
	}
	if a.Definition.Family != "v" || b.Definition.Family != "v" {
		t.Errorf("families = %q/%q, want both v", a.Definition.Family, b.Definition.Family)
	}
}
