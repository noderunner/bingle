package dsl_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

func TestExpandVariants_NoVariantsIsIdentity(t *testing.T) {
	doc := []byte(`{"id":"g","name":"G"}`)
	out, err := dsl.ExpandVariants(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || string(out[0]) != string(doc) {
		t.Errorf("out = %q, want the input unchanged", out)
	}
}

func TestExpandVariants_AppliesMergePatchPerVariant(t *testing.T) {
	doc := []byte(`{
		"id": "base", "name": "Base",
		"board": {"generator": {"type":"grid","width":5,"height":5,"adjacency":"orthogonal4"}},
		"reserves": {"stone": 21, "cap": 1},
		"variants": [
			{"id": "small", "name": "Small", "patch": {"board":{"generator":{"width":3,"height":3}}, "reserves":{"stone":10,"cap":null}}},
			{"id": "wide", "patch": {"board":{"generator":{"width":9}}}}
		]
	}`)
	out, err := dsl.ExpandVariants(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("docs = %d, want base + 2 variants", len(out))
	}

	type gen struct {
		Width, Height int
	}
	type parsed struct {
		ID, Name, Family string
		Board            struct{ Generator gen }
		Reserves         map[string]int
		Variants         []any
	}
	var base, small, wide parsed
	for i, p := range []*parsed{&base, &small, &wide} {
		if err := json.Unmarshal(out[i], p); err != nil {
			t.Fatal(err)
		}
	}

	if base.ID != "base" || base.Board.Generator.Width != 5 || base.Variants != nil || base.Family != "base" {
		t.Errorf("base = %+v", base)
	}
	if small.ID != "small" || small.Name != "Small" || small.Board.Generator.Width != 3 || small.Board.Generator.Height != 3 {
		t.Errorf("small = %+v", small)
	}
	if small.Reserves["stone"] != 10 {
		t.Errorf("small stone reserve = %d, want 10", small.Reserves["stone"])
	}
	if _, has := small.Reserves["cap"]; has {
		t.Errorf("null in the patch should delete the key; reserves = %v", small.Reserves)
	}
	// A variant without a name keeps the base name; untouched siblings stay.
	if wide.ID != "wide" || wide.Name != "Base" || wide.Board.Generator.Width != 9 || wide.Board.Generator.Height != 5 {
		t.Errorf("wide = %+v", wide)
	}
	if wide.Reserves["cap"] != 1 || wide.Family != "base" || small.Family != "base" {
		t.Errorf("variants must not leak into each other: wide=%+v small=%+v", wide, small)
	}
}

func TestExpandVariants_RejectsVariantWithoutID(t *testing.T) {
	_, err := dsl.ExpandVariants([]byte(`{"id":"b","variants":[{"name":"x"}]}`))
	if err == nil {
		t.Error("want an error for a variant with no id")
	}
}

func TestQuantity_AcceptsNumberOrKeyword(t *testing.T) {
	var a struct {
		Q dsl.Quantity `json:"q"`
	}
	if err := json.Unmarshal([]byte(`{"q": 4}`), &a); err != nil || a.Q.N != 4 || a.Q.Keyword != "" {
		t.Errorf("number: %+v err=%v", a.Q, err)
	}
	if err := json.Unmarshal([]byte(`{"q": "board-size"}`), &a); err != nil || a.Q.Keyword != "board-size" {
		t.Errorf("keyword: %+v err=%v", a.Q, err)
	}
	if err := json.Unmarshal([]byte(`{"q": [1]}`), &a); err == nil {
		t.Error("an array should be rejected")
	}
}
