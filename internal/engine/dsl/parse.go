package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// requiredTopLevelFields lists the JSON keys that must be present in a
// game-definition document. Order determines which missing field is
// reported first.
var requiredTopLevelFields = []string{
	"id", "name", "board", "seats", "pieces", "turn_order", "win_condition",
}

// Parse decodes a game-definition JSON document into a GameDefinition.
//
// Parse validates that the required top-level fields (id, name, board,
// seats, pieces, turn_order, win_condition) are present and non-empty,
// returning a clear error naming the first offending field if not. It
// intentionally does NOT validate that primitive/generator names referenced
// inside the definition (e.g. a piece's movement "primitive": "step") are
// actually known to the engine — that check belongs to the future catalog
// package, which resolves a GameDefinition against the primitive registry.
// Performing that check here would require dsl to import catalog (or
// primitives), creating an import cycle, since catalog depends on dsl.
func Parse(data []byte) (*GameDefinition, error) {
	// Decoded separately (rather than relying solely on zero-values in def)
	// so that a field explicitly set to its zero value in the JSON is still
	// reported as "present"; only fields entirely absent from the document
	// are flagged by the presence check below.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("dsl: parse game definition: %w", err)
	}

	// Unknown fields are refused: a misspelled field would otherwise be
	// ignored without a word. Load reports the same mistake with its file
	// and line; this catches definitions that reach the engine another way.
	var def GameDefinition
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&def); err != nil {
		return nil, fmt.Errorf("dsl: parse game definition: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("dsl: parse game definition: text after the definition")
	}

	if err := validateRequiredFields(raw, &def); err != nil {
		return nil, err
	}

	return &def, nil
}

func validateRequiredFields(raw map[string]json.RawMessage, def *GameDefinition) error {
	for _, field := range requiredTopLevelFields {
		if _, present := raw[field]; !present {
			return missingFieldError(field)
		}
	}

	switch {
	case strings.TrimSpace(def.ID) == "":
		return missingFieldError("id")
	case strings.TrimSpace(def.Name) == "":
		return missingFieldError("name")
	case def.Board.Generator == nil && len(def.Board.Positions) == 0:
		return missingFieldError("board")
	case len(def.Seats) == 0:
		return missingFieldError("seats")
	case len(def.Pieces) == 0:
		return missingFieldError("pieces")
	case strings.TrimSpace(def.TurnOrder.Type) == "":
		return missingFieldError("turn_order")
	case strings.TrimSpace(def.WinCondition.Type) == "":
		return missingFieldError("win_condition")
	}

	return nil
}

func missingFieldError(field string) error {
	return fmt.Errorf("dsl: game definition missing required field %q", field)
}

// ExpandVariants turns one document that may carry a "variants" array into
// the complete, standalone definition documents it stands for: the base
// document first (with its "variants" key removed), followed by one
// document per variant, each being the base with the variant's RFC 7386
// merge patch applied, its "id" replaced by the variant's ID and, when the
// variant gives one, its "name" replaced too.
//
// A document without variants expands to itself unchanged.
func ExpandVariants(data []byte) ([][]byte, error) {
	var base map[string]any
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("dsl: expand variants: %w", err)
	}

	rawVariants, hasVariants := base["variants"]
	if !hasVariants {
		return [][]byte{data}, nil
	}

	var variants []VariantDef
	vbytes, err := json.Marshal(rawVariants)
	if err != nil {
		return nil, fmt.Errorf("dsl: expand variants: %w", err)
	}
	if err := json.Unmarshal(vbytes, &variants); err != nil {
		return nil, fmt.Errorf("dsl: expand variants: %w", err)
	}

	delete(base, "variants")
	family, _ := base["id"].(string)
	if _, set := base["family"]; !set && family != "" {
		base["family"] = family
	}

	docs := make([][]byte, 0, len(variants)+1)
	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("dsl: expand variants: %w", err)
	}
	docs = append(docs, baseBytes)

	for _, v := range variants {
		if strings.TrimSpace(v.ID) == "" {
			return nil, fmt.Errorf("dsl: expand variants: variant missing id")
		}
		// Re-decode the base per variant so patches never alias each other.
		var doc map[string]any
		if err := json.Unmarshal(baseBytes, &doc); err != nil {
			return nil, fmt.Errorf("dsl: expand variants: %w", err)
		}
		if len(v.Patch) > 0 {
			var patch any
			if err := json.Unmarshal(v.Patch, &patch); err != nil {
				return nil, fmt.Errorf("dsl: expand variants: variant %q patch: %w", v.ID, err)
			}
			merged, ok := mergePatch(doc, patch).(map[string]any)
			if !ok {
				return nil, fmt.Errorf("dsl: expand variants: variant %q patch must be a JSON object", v.ID)
			}
			doc = merged
		}
		doc["id"] = v.ID
		if v.Name != "" {
			doc["name"] = v.Name
		}
		out, err := json.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("dsl: expand variants: %w", err)
		}
		docs = append(docs, out)
	}

	return docs, nil
}

// MergePatch applies an RFC 7386 JSON merge patch to target and returns the
// result: objects merge recursively, null deletes a key, anything else
// replaces.
func MergePatch(target, patch any) any { return mergePatch(target, patch) }

// mergePatch applies an RFC 7386 JSON merge patch to target and returns the
// result: objects merge recursively, null deletes a key, anything else
// replaces.
func mergePatch(target, patch any) any {
	patchObj, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	targetObj, ok := target.(map[string]any)
	if !ok {
		targetObj = map[string]any{}
	}
	for k, v := range patchObj {
		if v == nil {
			delete(targetObj, k)
			continue
		}
		targetObj[k] = mergePatch(targetObj[k], v)
	}
	return targetObj
}
