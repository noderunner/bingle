package wasmrules_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
	"github.com/noderunner/bingle/tests/testdata/draughts/faulty"
)

// syncBuffer is a log that calls on several goroutines may write to.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// faultyFixture is the faulty module with small limits, so that a call that
// never returns or wants all the memory there is fails quickly, the game of
// draughts it plays, and the log.
type faultyFixture struct {
	mod  *wasmrules.Module
	game *catalog.CompiledGame
	ref  *catalog.CompiledGame
	log  *syncBuffer
}

func newFaulty(t *testing.T, opt wasmrules.Options) *faultyFixture {
	t.Helper()
	f := &faultyFixture{log: &syncBuffer{}}
	opt.Log = slog.New(slog.NewTextHandler(f.log, nil))
	if opt.Deadline == 0 {
		opt.Deadline = 400 * time.Millisecond
	}
	if opt.MemoryPages == 0 {
		opt.MemoryPages = 1024 // 64 MiB
	}
	if opt.Instances == 0 {
		opt.Instances = 2
	}
	f.mod = load(t, newRuntime(t, opt), testdata.FaultyWasm)
	f.game = games(t, f.mod.Rules())[draughts.ID]
	f.ref = nativeGames(t)[draughts.ID]
	return f
}

// start is the start of a game, with the counter that asks every call about
// it for a fault.
func (f *faultyFixture) start(t *testing.T, kind int) *state.BoardState {
	t.Helper()
	st, err := state.InitialStateSeeded(f.ref, 100)
	if err != nil {
		t.Fatal(err)
	}
	st.SetCounter(faulty.Counter, kind)
	return st
}

// healthy fails the test unless the module still plays as the linked rules do.
func (f *faultyFixture) healthy(t *testing.T) {
	t.Helper()
	want := playOut(t, f.ref, 100, 5, 30)
	got := playOut(t, f.game, 100, 5, 30)
	if len(got) != len(want) {
		t.Fatalf("after the failure the module plays %d selections, the linked rules %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after the failure the module's selection %d is %+v, the linked rules' %+v", i, got[i], want[i])
		}
	}
}

// A call into an instance that panics fails, and fails the way its method
// promises: an error where the method has one, and otherwise an answer that
// leaves the game as it stands. The pool is usable after each.
func TestAPanicFailsOnlyItsCall(t *testing.T) {
	f := newFaulty(t, wasmrules.Options{})
	bad := f.start(t, faulty.Panic)
	mv := state.Selection{Action: "move", From: "a3", To: "b4", Count: faulty.Panic}

	t.Run("start", func(t *testing.T) {
		_, err := state.InitialStateSeeded(f.game, faulty.Panic)
		if err == nil || !strings.Contains(err.Error(), faulty.PanicText) {
			t.Fatalf("Start = %v, want an error that says what the module said", err)
		}
		f.healthy(t)
	})
	t.Run("apply", func(t *testing.T) {
		before, _ := bad.MarshalJSON()
		res, err := state.Apply(f.game, bad, 1, state.Selection{Action: "move", From: "a3", To: "b4"})
		if err == nil || res != nil {
			t.Fatalf("Apply = %v, %v, want an error and no result", res, err)
		}
		if after, _ := bad.MarshalJSON(); !bytes.Equal(before, after) {
			t.Fatal("the failed Apply changed the position it was given")
		}
		f.healthy(t)
	})
	t.Run("parse", func(t *testing.T) {
		if _, err := state.ParseMove(f.game, faulty.ParseText+"1"); err == nil {
			t.Fatal("Parse returned no error")
		}
		f.healthy(t)
	})
	t.Run("validate", func(t *testing.T) {
		if err := state.Validate(f.game, bad); err == nil {
			t.Fatal("Validate returned no error")
		}
		f.healthy(t)
	})
	t.Run("impulse seat", func(t *testing.T) {
		rs, _ := state.RulesetOf(f.game)
		if seat := rs.ImpulseSeat(f.game, bad); seat != 0 {
			t.Fatalf("ImpulseSeat = %d, want 0", seat)
		}
		// Which the engine reads as nothing to say, not as the end of the game.
		if seat := state.StartSeat(f.game, bad); seat != 1 {
			t.Fatalf("StartSeat = %d, want the first seat", seat)
		}
		if seat, err := state.ImpulseAfter(f.game, bad, 2); err != nil || seat != 2 {
			t.Fatalf("ImpulseAfter = %d, %v, want the seat that moved", seat, err)
		}
		f.healthy(t)
	})
	t.Run("legal", func(t *testing.T) {
		if legal := state.LegalSelections(f.game, bad, 1); len(legal) != 0 {
			t.Fatalf("Legal = %v, want none", legal)
		}
		f.healthy(t)
	})
	t.Run("format", func(t *testing.T) {
		if text := state.FormatMove(f.game, mv); text != "" {
			t.Fatalf("Format = %q, want nothing", text)
		}
		f.healthy(t)
	})
	t.Run("gestures", func(t *testing.T) {
		if gestures := state.Gestures(f.game, mv); len(gestures) != 0 {
			t.Fatalf("Gestures = %v, want none", gestures)
		}
		f.healthy(t)
	})
	t.Run("outcome", func(t *testing.T) {
		out, err := state.Evaluate(f.game, bad, 1)
		if err != nil || out.Ended || out.Winner != nil {
			t.Fatalf("Outcome = %+v, %v, want a game that is not over", out, err)
		}
		f.healthy(t)
	})

	// Every failure was logged with the pack, the call and the game.
	logged := f.log.String()
	for _, want := range []string{"pack=fixture", "call=legal", "call=outcome", "game=" + draughts.ID, "try=2", faulty.PanicText} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %q:\n%s", want, logged)
		}
	}
}

// A call that never returns is stopped at its deadline, and one that wants
// more memory than an instance may have is refused it. Both fail cleanly.
func TestALoopAndAMemoryHogFailCleanly(t *testing.T) {
	f := newFaulty(t, wasmrules.Options{})
	t.Run("loop", func(t *testing.T) {
		began := time.Now()
		_, err := state.InitialStateSeeded(f.game, faulty.Loop)
		if err == nil || !strings.Contains(err.Error(), "ran out of time") {
			t.Fatalf("Start = %v, want an error that says it ran out of time", err)
		}
		// Twice the deadline: the call is asked once more of a new instance.
		if took := time.Since(began); took < 800*time.Millisecond || took > 20*time.Second {
			t.Fatalf("the call took %s, want about two deadlines of 400ms", took)
		}
		f.healthy(t)
	})
	t.Run("memory", func(t *testing.T) {
		if _, err := state.InitialStateSeeded(f.game, faulty.Grow); err == nil {
			t.Fatal("Start returned no error")
		} else {
			t.Log(err)
		}
		f.healthy(t)
	})
	if n := f.mod.Instances(); n > 2 {
		t.Fatalf("the module holds %d instances, more than its 2", n)
	}
}

// An instance that fails is replaced, and the call asked once more of the new
// one: a call that only a used instance fails succeeds.
func TestAFailedCallIsAskedOnceMoreOfANewInstance(t *testing.T) {
	f := newFaulty(t, wasmrules.Options{Instances: 1})
	// Tire the only instance. Format takes its fault from the selection.
	state.FormatMove(f.game, state.Selection{Action: "move", From: "a3", To: "b4", Count: faulty.Tire})
	st := f.start(t, faulty.IfTired)
	legal := state.LegalSelections(f.game, st, 1)
	want := state.LegalSelections(f.ref, f.start(t, 0), 1)
	if len(legal) == 0 || len(legal) != len(want) {
		t.Fatalf("Legal = %d selections, want the %d of the linked rules", len(legal), len(want))
	}
	logged := f.log.String()
	if !strings.Contains(logged, "try=1") || strings.Contains(logged, "try=2") {
		t.Fatalf("want one instance thrown away, at the first try:\n%s", logged)
	}
	f.healthy(t)
}

// An instance lives through any number of calls: its memory does not grow
// with them. A clock that moved at every reading once starved the module's
// collector, and an instance reached its memory cap after some tens of
// thousands of calls. Each call here makes two megabytes of garbage, in an
// instance that may hold sixteen.
func TestAnInstanceDoesNotGrowWithItsCalls(t *testing.T) {
	f := newFaulty(t, wasmrules.Options{Instances: 1, MemoryPages: 256, Deadline: 5 * time.Second})
	mv := state.Selection{Action: "move", From: "a3", To: "b4", Count: faulty.Churn}
	// The clock that starved the collector lost its instance at the 110th.
	calls := 600
	if testing.Short() {
		calls = 300
	}
	for i := range calls {
		if text := state.FormatMove(f.game, mv); text != "a3-b4" {
			t.Fatalf("call %d: Format = %q\n%s", i, text, f.log.String())
		}
		if logged := f.log.String(); logged != "" {
			t.Fatalf("an instance was thrown away at call %d:\n%s", i, logged)
		}
	}
}

// A reply that makes no sense for the game counts as no reply.
func TestANextSeatTheGameDoesNotHaveIsRefused(t *testing.T) {
	f := newFaulty(t, wasmrules.Options{})
	st := f.start(t, 0)
	_, err := state.Apply(f.game, st, 1, state.Selection{Action: "move", From: "a3", To: "b4", Count: faulty.BadSeat})
	if err == nil || !strings.Contains(err.Error(), "seat 9") {
		t.Fatalf("Apply = %v, want an error about seat 9", err)
	}
	f.healthy(t)
}

// Calls wait for a free instance no longer than their deadline: with one
// instance and four calls that never return, every call is back within its
// two tries.
func TestCallsWaitWithinTheirDeadline(t *testing.T) {
	f := newFaulty(t, wasmrules.Options{Instances: 1, Deadline: 300 * time.Millisecond})
	bad := f.start(t, faulty.Loop)
	began := time.Now()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if legal := state.LegalSelections(f.game, bad, 1); len(legal) != 0 {
				t.Errorf("Legal = %v, want none", legal)
			}
		})
	}
	wg.Wait()
	if took := time.Since(began); took > 20*time.Second {
		t.Fatalf("four calls took %s", took)
	}
	f.healthy(t)
}

// The smallest modules that are not rules modules of this ABI, by hand.
var (
	wasmHeader = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	// (module (func (export "bingle_abi") (result i32) i32.const 99))
	wasmABI99 = append(append([]byte{}, wasmHeader...),
		0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f, // one type: () → i32
		0x03, 0x02, 0x01, 0x00, // one function of it
		0x07, 0x0e, 0x01, 0x0a, 'b', 'i', 'n', 'g', 'l', 'e', '_', 'a', 'b', 'i', 0x00, 0x00,
		0x0a, 0x07, 0x01, 0x05, 0x00, 0x41, 0xe3, 0x00, 0x0b) // i32.const 99
	// (module (import "env" "foo" (func)))
	wasmImport = append(append([]byte{}, wasmHeader...),
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00, // one type: () → ()
		0x02, 0x0b, 0x01, 0x03, 'e', 'n', 'v', 0x03, 'f', 'o', 'o', 0x00, 0x00)
)

// The host refuses a module whose ABI it does not speak, and anything else
// that is not a rules module, with an error that says why. The runtime goes
// on loading modules after each.
func TestOnlyARulesModuleOfThisABILoads(t *testing.T) {
	rt := newRuntime(t, wasmrules.Options{})
	ctx := context.Background()
	for _, c := range []struct {
		name string
		wasm []byte
		want string
	}{
		{"another ABI", wasmABI99, "speaks ABI 99"},
		{"no ABI", wasmHeader, "not a rules module"},
		{"another import", wasmImport, "imports env.foo"},
		{"not a module", []byte("rules.wasm"), "not a module"},
		{"nothing", nil, "not a module"},
	} {
		t.Run(c.name, func(t *testing.T) {
			mod, err := rt.Load(ctx, "bad", c.wasm)
			if err == nil {
				_ = mod.Close(ctx)
				t.Fatal("the module was loaded")
			}
			if !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "bad") {
				t.Fatalf("Load = %v, want an error that names the pack and says %q", err, c.want)
			}
		})
	}
	mod := load(t, rt, testdata.DraughtsWasm)
	game := games(t, mod.Rules())[draughts.ID]
	if len(playOut(t, game, 1, 1, 10)) != 11 {
		t.Fatal("the runtime does not play a good module after refusing bad ones")
	}
}

// Rules that cannot play a game say so when they are told of it, and that is
// the game's failure alone: the module's other games play on.
func TestAGameTheRulesRefuseFailsAlone(t *testing.T) {
	mod := draughtsModule(t)
	all := games(t, mod.Rules())
	// The same rules given a definition for three seats.
	cat := catalog.NewCatalog()
	def := bytes.Replace(testdata.Draughts().Definition, []byte(`"seats":[`), []byte(`"seats":[{"seat":3,"color":"red","label":"Red"},`), 1)
	def = bytes.Replace(def, []byte(`"id":"`+draughts.ID+`"`), []byte(`"id":"test-draughts-three"`), 1)
	if err := cat.RegisterWith("test-draughts-three", def, nil, mod.Rules()); err != nil {
		t.Fatal(err)
	}
	three, _ := cat.Get("test-draughts-three")
	if _, err := state.InitialStateSeeded(three, 1); err == nil || !strings.Contains(err.Error(), "two seats") {
		t.Fatalf("Start = %v, want the rules' own refusal", err)
	}
	if len(playOut(t, all[draughts.SmallID], 1, 1, 10)) != 11 {
		t.Fatal("the module's other games do not play")
	}
}

// A closed module fails its calls instead of hanging.
func TestAClosedModuleFailsItsCalls(t *testing.T) {
	rt := newRuntime(t, wasmrules.Options{})
	mod := load(t, rt, testdata.DraughtsWasm)
	game := games(t, mod.Rules())[draughts.ID]
	if _, err := state.InitialStateSeeded(game, 1); err != nil {
		t.Fatal(err)
	}
	if err := mod.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InitialStateSeeded(game, 1); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Start on a closed module = %v", err)
	}
}
