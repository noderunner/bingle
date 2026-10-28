//go:build ignore

// Command gendocs writes docs/games.md, the catalog of shipped games, from
// the registered definitions so the table never drifts from the JSON:
//
//	go run scripts/gendocs/main.go > docs/games.md
package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

func main() {
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			log.Fatal(err)
		}
	}
	families := map[string][]*catalog.CompiledGame{}
	for _, g := range cat.List() {
		fam := g.Definition.Family
		if fam == "" {
			fam = g.Definition.ID
		}
		families[fam] = append(families[fam], g)
	}
	var names []string
	for f := range families {
		names = append(names, f)
	}
	sort.Strings(names)

	fmt.Println("# Game catalog")
	fmt.Println()
	fmt.Println("Generated from the registered definitions by `go run scripts/gendocs/main.go`;")
	fmt.Println("each game is one JSON document under `internal/games`. See [dsl.md](dsl.md) for the")
	fmt.Println("vocabulary and [mcp.md](mcp.md) for the training side (planes, action IDs).")
	fmt.Println()
	fmt.Println("| Game | Version | Board | Seats | Actions | Ends by | Variants |")
	fmt.Println("|---|---|---|---|---|---|---|")
	for _, fam := range names {
		list := families[fam]
		sort.Slice(list, func(i, j int) bool { return list[i].Definition.ID < list[j].Definition.ID })
		g := list[0]
		for _, c := range list {
			if c.Definition.ID == fam {
				g = c
			}
		}
		def := g.Definition
		board := boardText(g)
		actions := map[string]bool{}
		for _, a := range g.Actions {
			actions[a.Type] = true
		}
		var as []string
		for a := range actions {
			as = append(as, a)
		}
		sort.Strings(as)
		ends := map[string]bool{}
		if g.Rules != nil {
			// A Go ruleset: its rules are not declarative actions and rules.
			as = []string{"ruleset (Go)"}
			ends["victory points"] = true
			if def.Board.Style == "areas" {
				ends = map[string]bool{"doom": true}
			}
		}
		for _, r := range def.WinCondition.Rules {
			t := r.Type
			if t == "end" && r.Decide != nil {
				t = "end/" + r.Decide.Type
			}
			if r.Result == "lose" {
				t += " (lose)"
			}
			ends[t] = true
		}
		var es []string
		for e := range ends {
			es = append(es, e)
		}
		sort.Strings(es)
		var vs []string
		if len(list) > 1 {
			for _, c := range list {
				vs = append(vs, "`"+c.Definition.ID+"`")
			}
		}
		name := def.Name
		if len(list) > 1 {
			name = strings.TrimSpace(strings.SplitN(name, "(", 2)[0])
		}
		fmt.Printf("| **%s** (`%s`) | %s | %s | %d | %s | %s | %s |\n", name, def.ID, def.Version, board, len(def.Seats),
			strings.Join(as, ", "), strings.Join(es, ", "), strings.Join(vs, " "))
	}
	fmt.Println()
	fmt.Println("Variants are separate games derived from the base document by a JSON merge patch.")
	fmt.Println()
	fmt.Println("**Version** is the game implementation's semantic version (`version` in its JSON). Raise it with")
	fmt.Println("every change to the game's definition or Go code; a version below 1.0.0 is shown as a Beta in the")
	fmt.Println("list of games. The server records each version it has run in the `game_versions` table.")
}

func boardText(g *catalog.CompiledGame) string {
	b := g.Board
	if g.Definition.Board.Style == "areas" {
		return fmt.Sprintf("area map, %d areas", b.Len())
	}
	switch b.Kind() {
	case "hex":
		if g.Definition.Board.Generator != nil && g.Definition.Board.Generator.Type == "hexhex" {
			if s2 := g.Definition.Board.Generator.Size2; s2 > 0 && s2 != g.Definition.Board.Generator.Size {
				return fmt.Sprintf("hexagon, %d and %d per side", g.Definition.Board.Generator.Size, s2)
			}
			return fmt.Sprintf("hexagon, %d per side", g.Definition.Board.Generator.Size)
		}
		return fmt.Sprintf("hex %d×%d", b.Width(), b.Height())
	case "grid":
		style := "squares"
		if g.Definition.Board.Style == "points" {
			style = "points"
		}
		return fmt.Sprintf("%d×%d %s", b.Width(), b.Height(), style)
	}
	return fmt.Sprintf("graph, %d nodes", b.Len())
}
