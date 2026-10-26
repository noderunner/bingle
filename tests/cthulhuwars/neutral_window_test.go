package cthulhuwars_test

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The window of the neutral items that are still there to take.

type neutralDoc struct {
	Items []struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Name   string `json:"name"`
		Obtain string `json:"obtain"`
		How    string `json:"how"`
		Timing string `json:"timing"`
		Text   string `json:"text"`
		Cost   int    `json:"cost"`
		Combat string `json:"combat"`
		Awaken []string
		Book   *struct {
			Name string `json:"name"`
		} `json:"book"`
		Req string `json:"req"`
	} `json:"items"`
}

// neutralOf is the neutral part of the public view of st (nil when absent).
func neutralOf(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, viewer int) *neutralDoc {
	t.Helper()
	raw, err := json.Marshal(state.Public(g, st, viewer))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Ext struct {
			View struct {
				Neutral *neutralDoc `json:"neutral"`
			} `json:"view"`
		} `json:"ext"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Ext.View.Neutral
}

func (n *neutralDoc) ids(kind string) []string {
	var out []string
	for _, it := range n.Items {
		if it.Kind == kind {
			out = append(out, it.ID)
		}
	}
	sort.Strings(out)
	return out
}

func TestNoNeutralWindowWithoutNeutralContent(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", fourCults, true)
	if n := neutralOf(t, tb.g, tb.build(xp{}), 1); n != nil {
		t.Errorf("a table without neutral content has a window: %+v", n)
	}
	// A spellbook swapped in at setup alone is not content left to take.
	tb = azTable(t, "cthulhu-wars", fourCults, false, map[string]string{"cc": "mao"})
	if n := neutralOf(t, tb.g, tb.build(xp{}), 1); n != nil {
		t.Errorf("a swapped book alone opens the window: %+v", n)
	}
}

func TestTheNeutralWindowListsWhatIsLeftToTake(t *testing.T) {
	g := packTable(t, "cthulhu-wars", fourCults, allCards)
	st, err := state.InitialStateSeeded(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	n := neutralOf(t, g, st, 1)
	if n == nil {
		t.Fatal("no window with every pack in the box")
	}
	if got := len(n.ids("book")); got != 6 {
		t.Errorf("%d neutral spellbooks, want 6", got)
	}
	if got := strings.Join(n.ids("monster"), " "); got != "dimensional-shambler elder-thing gnorri moonbeast servitor shantak star-vampire voonith wamp" {
		t.Errorf("monsters: %s", got)
	}
	if got := strings.Join(n.ids("terror"), " "); got != "hound-of-tindalos" {
		t.Errorf("terrors: %s", got)
	}
	if got := strings.Join(n.ids("goo"), " "); got != "abhoth chaugnar-faugn cthugha mother-hydra yig" {
		t.Errorf("Great Old Ones: %s", got)
	}
	for _, it := range n.Items {
		switch it.Kind {
		case "book":
			if it.Timing == "" || it.Text == "" {
				t.Errorf("%s: no rules", it.Name)
			}
		case "goo":
			if len(it.Awaken) == 0 || it.Book == nil || it.Req == "" || it.Cost == 0 {
				t.Errorf("%s: awaken %v book %v req %q cost %d", it.Name, it.Awaken, it.Book, it.Req, it.Cost)
			}
		default:
			if it.Obtain == "" || it.How == "" || it.Combat == "" {
				t.Errorf("%s: obtain %q how %q combat %q", it.Name, it.Obtain, it.How, it.Combat)
			}
		}
	}
	for _, it := range n.Items {
		if it.ID == "hound-of-tindalos" && it.Obtain != "2 Doom and 2 Power" {
			t.Errorf("the Terror costs %q", it.Obtain)
		}
	}
}

func TestTakenNeutralItemsLeaveTheWindow(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, map[string]string{"cc": "mao"})
	bg := tb.seat("bg")
	st := tb.build(xp{edit: func(d *cthulhuwars.Data) {
		d.Loyal[bg] = 1 << 1        // the Star Vampire's card is Black Goat's
		d.Neutral[bg] = 1 << 0      // and so is Shriveling
		d.Books[bg] |= 1 << (6 + 0) // (the book number says it is held)
	}})
	n := neutralOf(t, tb.g, st, 1)
	if n == nil {
		t.Fatal("no window")
	}
	for _, id := range n.ids("monster") {
		if id == "star-vampire" {
			t.Errorf("the bought card is still on offer: %v", n.ids("monster"))
		}
	}
	books := strings.Join(n.ids("book"), " ")
	if strings.Contains(books, "shriveling") {
		t.Errorf("the held spellbook is still on offer: %s", books)
	}
	if strings.Contains(books, "mao-ceremony") {
		t.Errorf("the book set aside for the Crawling Chaos is on offer: %s", books)
	}
	if got := len(n.ids("book")); got != 4 {
		t.Errorf("%d books left, want 4: %s", got, books)
	}
	if got := len(n.ids("monster")); got != 3 {
		t.Errorf("%d monsters left, want 3", got)
	}
}
