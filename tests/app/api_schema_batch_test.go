// Tests of the batch routes' part of docs/api.schema.json (ADR 0134, ADR
// 0161): what the server sends for a batch fits it, and the schema written out
// for batch.Spec and batch.Report, which have JSON forms of their own, says
// what their code reads and writes.

package app_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/batch"
)

// schemaProps are the names of the properties of the schema's definition def.
func schemaProps(t *testing.T, schema map[string]any, def string) []string {
	t.Helper()
	d, ok := schema["$defs"].(map[string]any)[def].(map[string]any)
	if !ok {
		t.Fatalf("no definition %s", def)
	}
	var names []string
	for k := range d["properties"].(map[string]any) {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

func TestAPISchema_BatchSpecSaysWhatTheDecoderReads(t *testing.T) {
	schema := apiSchema(t)
	for _, def := range []string{"BatchSpec", "StartBatchRequest"} {
		props := schemaProps(t, schema, def)

		// Every field the schema names is one the decoder reads: setting it
		// changes the spec. (name is the request's, read beside the spec.)
		values := map[string]any{
			"base": "x", "options": []string{"hp"}, "players": []any{map[string]any{"bot": "random"}},
			"games": 3, "seed": 4, "rotate": true, "max_plies": 5, "event_games": 6, "workers": 7,
			"limits": map[string]any{"games": 8}, "high_priests": true,
		}
		for _, p := range props {
			if p == "name" {
				continue
			}
			v, ok := values[p]
			if !ok {
				t.Fatalf("%s names %q, which this test does not know", def, p)
			}
			var got batch.Spec
			raw, _ := json.Marshal(map[string]any{p: v})
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("%s: %s: %v", def, p, err)
			}
			if reflect.DeepEqual(got, batch.Spec{}) {
				t.Errorf("%s names %q, which the decoder does not read", def, p)
			}
		}

		// Every field the decoder reads is named: the spec's JSON tags are
		// the schema's properties, but for the old high_priests and the request's name.
		var tags []string
		st := reflect.TypeFor[batch.Spec]()
		for i := range st.NumField() {
			tags = append(tags, strings.Split(st.Field(i).Tag.Get("json"), ",")[0])
		}
		tags = append(tags, "high_priests")
		if def == "StartBatchRequest" {
			tags = append(tags, "name")
		}
		slices.Sort(tags)
		if !slices.Equal(tags, props) {
			t.Errorf("%s names %v, the spec reads %v", def, props, tags)
		}
	}
}

func TestAPISchema_ASpecTheDecoderAcceptsFitsTheSchema(t *testing.T) {
	schema := apiSchema(t)
	for name, spec := range map[string]string{
		"full":      `{"base":"cthulhu-wars-5p","options":["hp"],"players":[{"faction":"gc","bot":"mcts","mcts_iterations":50,"tuning":{"a":1.5}},{"bot":"random"}],"games":9,"seed":3,"rotate":true,"max_plies":100,"event_games":2,"workers":2,"limits":{"games":20000,"minutes":30}}`,
		"minimal":   `{"base":"hex-7","players":[{"bot":"random"},{"bot":"random"}],"games":1}`,
		"with name": `{"name":"a run","base":"hex-7","players":[{"bot":"random"},{"bot":"random"}],"games":1}`,
		"old":       `{"base":"hex-7","players":[{"bot":"random"},{"bot":"random"}],"games":1,"high_priests":true}`,
	} {
		var decoded batch.Spec
		if err := json.Unmarshal([]byte(spec), &decoded); err != nil {
			t.Fatalf("%s: the decoder refuses it: %v", name, err)
		}
		var sent any
		_ = json.Unmarshal([]byte(spec), &sent)
		for _, problem := range checkSchema(schema, map[string]any{"$ref": "#/$defs/StartBatchRequest"}, sent, "") {
			t.Errorf("%s: %s", name, problem)
		}

		// What the server then stores and sends back fits too (without the name).
		again, _ := json.Marshal(decoded)
		var echoed any
		_ = json.Unmarshal(again, &echoed)
		for _, problem := range checkSchema(schema, map[string]any{"$ref": "#/$defs/BatchSpec"}, echoed, "") {
			t.Errorf("%s as sent back: %s", name, problem)
		}
	}
	// A spec without what Validate cannot do without does not fit a request.
	var bad any
	_ = json.Unmarshal([]byte(`{"base":"hex-7"}`), &bad)
	if problems := checkSchema(schema, map[string]any{"$ref": "#/$defs/StartBatchRequest"}, bad, ""); len(problems) == 0 {
		t.Error("a request with no players or games fits the schema")
	}
}

func TestAPISchema_BatchReportSaysWhatItWrites(t *testing.T) {
	schema := apiSchema(t)
	rep := batch.Report{
		Games: 2, Decided: 1, EndReasons: map[string]int{"win": 1},
		PlyBuckets: batch.Histogram{Counts: []int{1}},
		Seats:      []batch.SeatReport{{Seat: 1}},
		Bots:       []batch.BotReport{{Bot: "random"}},
		Game:       map[string]json.RawMessage{"factions": json.RawMessage(`{"gc":1}`)},
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]any
	_ = json.Unmarshal(raw, &written)
	var fields []string
	for k := range written {
		if k != "factions" { // the game's own part
			fields = append(fields, k)
		}
	}
	slices.Sort(fields)
	if props := schemaProps(t, schema, "BatchReport"); !slices.Equal(props, fields) {
		t.Errorf("BatchReport names %v, a report writes %v", props, fields)
	}
	for _, problem := range checkSchema(schema, map[string]any{"$ref": "#/$defs/BatchReport"}, written, "") {
		t.Error(problem)
	}
}

// A batch started through the API, its listing, report, games, events and
// cancellation, and a refusal, fit the schema.
func TestAPISchema_DescribesTheBatchRoutes(t *testing.T) {
	a, base := membersServer(t)
	schema := apiSchema(t)
	ana := nativeMember(t, a, base, "ana", schema)

	spec := map[string]any{"name": "Hex", "base": "hex-7", "games": 3, "workers": 1, "seed": 1, "event_games": 1,
		"players": []map[string]any{{"bot": "random"}, {"bot": "random"}}}
	started := ana.do("POST /api/batches", "/api/batches", spec, 201)
	id, _ := started["id"].(float64)
	path := fmt.Sprintf("/api/batches/%d", int64(id))
	if id == 0 {
		t.Fatalf("the started batch: %v", started)
	}

	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if info := ana.do("GET /api/batches/{id}", path, nil, 200); info["status"] == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the batch did not finish")
		}
	}
	list := ana.do("GET /api/batches", "/api/batches", nil, 200)
	if batches, _ := list["batches"].([]any); len(batches) != 1 {
		t.Errorf("the listing: %v", list)
	}
	rep := ana.do("GET /api/batches/{id}/report", path+"/report", nil, 200)
	if r, _ := rep["report"].(map[string]any); r["games"] != float64(3) {
		t.Errorf("the report: %v", rep)
	}
	games := ana.do("GET /api/batches/{id}/games", path+"/games", nil, 200)
	if g, _ := games["games"].([]any); len(g) != 3 {
		t.Errorf("the games: %v", games)
	}
	if ev := ana.do("GET /api/batches/{id}/games/{n}/events", path+"/games/0/events", nil, 200); ev["events"] == nil {
		t.Errorf("the events: %v", ev)
	}
	ana.do("POST /api/batches/{id}/cancel", path+"/cancel", nil, 200)

	// A spec over a ceiling is refused with the limit named, which x-error says.
	spec["games"] = batch.MaxGames + 1
	refused := ana.do("POST /api/batches", "/api/batches", spec, 400)
	if refused["limit"] != "games" || refused["max"] != float64(batch.MaxGames) {
		t.Errorf("the refusal: %v", refused)
	}

	// Deleting answers 204 with no body, as the schema's entry says (it has none).
	if code := doJSONAs(t, ana.key, "DELETE", base+path, nil, nil); code != 204 {
		t.Errorf("DELETE %s: %d", path, code)
	}
	if e := schema["x-endpoints"].(map[string]any)["DELETE /api/batches/{id}"].(map[string]any); len(e) != 0 {
		t.Errorf("DELETE /api/batches/{id}: %v", e)
	}
}
