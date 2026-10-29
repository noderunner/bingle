// Command replay reproduces a repro bundle: in process, deterministically,
// with no server, no database and no browser.
//
//	go run ./scripts/replay /var/lib/bingle/debug/20261028T193012Z-apply-rejected-a-legal-move-3f9a1c22d4e5.json
//
// It exits 0 when the bundle reproduced — the fault is still there — and 1
// when it did not, which means either that it is fixed or that the bundle no
// longer matches this build. The report says which.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/repro"
	"github.com/noderunner/bingle/internal/version"
)

func main() {
	board := flag.Bool("board", false, "print the position the bundle was written from")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(),
			"usage: replay [-board] <bundle.json>\n\nReproduces a repro bundle written by bingle (see docs/observability.md).\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	b, err := repro.Load(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Printf("bundle   %s\n", flag.Arg(0))
	fmt.Printf("kind     %s\n", b.Kind)
	fmt.Printf("game     %s\n", b.GameDefID)
	fmt.Printf("written  %s by bingle %s (this is %s)\n", b.Written.Format("2006-01-02 15:04:05 MST"), b.Bingle, version.Engine)
	if b.Bingle != "" && b.Bingle != version.Engine {
		fmt.Printf("         note: a different build wrote it, so a difference may be a change rather than a fix\n")
	}
	if b.Notation != "" {
		fmt.Printf("move     seat %d, %s\n", b.Seat, b.Notation)
	}
	if b.Note != "" {
		fmt.Printf("note     %s\n", b.Note)
	}
	if b.Error != "" {
		fmt.Printf("was      %s\n", b.Error)
	}
	if *board && b.Board != "" {
		fmt.Printf("\n%s\n", b.Board)
	}

	cat, err := buildCatalog()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	res, err := repro.Replay(cat, b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("\n%s\n", res)

	if !res.Reproduced {
		os.Exit(1)
	}
}

// buildCatalog registers every game this build ships with, which is what a
// bundle's game_def_id is looked up in.
func buildCatalog() (*catalog.Catalog, error) {
	cat := catalog.NewCatalog()
	for _, mod := range app.BuiltinModules() {
		if err := cat.RegisterModule(mod); err != nil {
			return nil, fmt.Errorf("replay: registering %q: %w", mod.ID, err)
		}
	}
	return cat, nil
}
