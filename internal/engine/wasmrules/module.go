package wasmrules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	wsys "github.com/tetratelabs/wazero/sys"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Module is a loaded rules module and its pool of instances. Its Rules are
// the game's rules for a catalog.Module, safe to call from many goroutines.
//
// A call that fails never ends a game or loses a move. Start, Apply, Parse
// and Validate return the failure as their error. ImpulseSeat, Legal, Format,
// Gestures and Outcome answer with something that leaves the game as it
// stands: seat 0 (which the engine reads as nothing to say), no selections,
// no notation, no gestures, an outcome that is not over.
type Module struct {
	rt       *Runtime
	pack     string
	compiled wazero.CompiledModule
	games    games
	// bot says the module answers the bot call.
	bot bool

	// slots holds a token for every call that has an instance or waits for
	// one to be made, so there are never more instances than its size.
	slots  chan struct{}
	closed chan struct{}

	mu   sync.Mutex
	idle []*instance
	live int // instances that exist, idle or at work
	shut bool

	faults    atomic.Uint64         // instances thrown away, since the module was loaded
	lastFault atomic.Pointer[error] // why the last of them was
}

// Rules returns the module as a game's rules: a state.Searcher too when the
// module answers the bot call.
func (m *Module) Rules() state.SeededRuleset {
	a := &adapter{t: m, log: m.rt.opt.Log}
	if m.bot {
		return searcher{a}
	}
	return a
}

// Close throws the module's instances away, once the calls in flight have
// had their answers. Calls made after it fail.
func (m *Module) Close(ctx context.Context) error {
	m.mu.Lock()
	if m.shut {
		m.mu.Unlock()
		return nil
	}
	m.shut = true
	close(m.closed)
	m.mu.Unlock()
	// Every slot taken means no call has an instance any more.
	for range cap(m.slots) {
		m.slots <- struct{}{}
	}
	m.mu.Lock()
	idle := m.idle
	m.idle, m.live = nil, 0
	m.mu.Unlock()
	for _, inst := range idle {
		inst.close()
	}
	return m.compiled.Close(ctx)
}

// Instances is how many instances the module holds, idle or at work.
func (m *Module) Instances() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.live
}

// Memory is the linear memory of the module's idle instances, in bytes: what
// its pool holds between calls. An instance at work is not counted.
func (m *Module) Memory() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n uint64
	for _, inst := range m.idle {
		n += uint64(inst.mem.Size())
	}
	return n
}

// Faults is how many of the module's instances have failed and been thrown
// away since it was loaded (a trap, a deadline, memory it could not have, a
// reply that is none), and why the last of them did. A call answered with
// "nothing" because of one is told apart by it from a call that had nothing
// to say, which is how a pack's trial game knows its rules failed.
func (m *Module) Faults() (uint64, error) {
	n := m.faults.Load()
	if last := m.lastFault.Load(); last != nil {
		return n, *last
	}
	return n, nil
}

// instanceError is a failure of the instance that answered, not of the call:
// a trap, a deadline, memory it could not have, a reply that is none. The
// instance is thrown away, and the call asked once more of a new one.
type instanceError struct{ err error }

func (e *instanceError) Error() string { return e.err.Error() }
func (e *instanceError) Unwrap() error { return e.err }

// roundTrip makes one call on an instance of the pool. A call stopped
// because ctx ended is not tried again, and is no fault of the instance's
// though the instance is thrown away. Nor is a bot call tried again: its
// search had its time, and the bot searches without the module instead.
func (m *Module) roundTrip(ctx context.Context, c call, game *catalog.CompiledGame, req []byte, extra time.Duration, read func([]byte) error) error {
	told, err := m.games.of(game)
	if err != nil {
		return err
	}
	for try := 1; ; try++ {
		err := m.try(ctx, c, told, req, extra, read)
		var broken *instanceError
		if !errors.As(err, &broken) {
			return err
		}
		if ctx.Err() != nil {
			return fmt.Errorf("wasmrules: %s: %s of %s was stopped: %w", m.pack, c, told.id, ctx.Err())
		}
		m.lastFault.Store(&err)
		m.faults.Add(1)
		m.rt.opt.Log.Warn("wasmrules: an instance failed and was thrown away",
			"pack", m.pack, "call", c.String(), "game", told.id, "try", try, "error", err)
		if c == callBot {
			return fmt.Errorf("wasmrules: %s: %s of %s failed: %w", m.pack, c, told.id, err)
		}
		if try == 2 {
			return fmt.Errorf("wasmrules: %s: %s of %s failed twice: %w", m.pack, c, told.id, err)
		}
	}
}

// try makes the call once, within the deadline and extra: the wait for an
// instance, telling it of the game if it has not heard of it, and the call
// itself.
func (m *Module) try(ctx context.Context, c call, told *toldGame, req []byte, extra time.Duration, read func([]byte) error) error {
	ctx, cancel := m.rt.deadline(ctx, extra)
	defer cancel()
	inst, err := m.acquire(ctx)
	if err != nil {
		return err
	}
	if inst.told[told.id] != told {
		var own error
		err := inst.call(ctx, callInit, told.init, func(reply []byte) error {
			var env envelope[bool]
			if err := json.Unmarshal(reply, &env); err != nil {
				return err
			}
			if env.Error != nil {
				own = fmt.Errorf("wasmrules: %s: %s", m.pack, *env.Error)
			} else if env.OK == nil {
				return errors.New("the reply is neither ok nor an error")
			}
			return nil
		})
		if err != nil {
			m.discard(inst)
			return &instanceError{fmt.Errorf("init of %s: %w", told.id, err)}
		}
		if own != nil {
			// The rules do not play this game. That is no fault of the instance.
			m.release(inst)
			return own
		}
		inst.told[told.id] = told
	}
	if err := inst.call(ctx, c, req, read); err != nil {
		m.discard(inst)
		return &instanceError{err}
	}
	m.release(inst)
	return nil
}

// add counts an instance made or thrown away.
func (m *Module) add(n int) {
	m.mu.Lock()
	m.live += n
	m.mu.Unlock()
}

// acquire waits for a slot and takes an idle instance, or makes one.
func (m *Module) acquire(ctx context.Context) (*instance, error) {
	select {
	case m.slots <- struct{}{}:
	case <-m.closed:
		return nil, fmt.Errorf("wasmrules: %s is closed", m.pack)
	case <-ctx.Done():
		return nil, fmt.Errorf("wasmrules: %s: every one of its %d instances was busy for %s", m.pack, cap(m.slots), m.rt.opt.Deadline)
	}
	m.mu.Lock()
	if m.shut {
		m.mu.Unlock()
		<-m.slots
		return nil, fmt.Errorf("wasmrules: %s is closed", m.pack)
	}
	if n := len(m.idle); n > 0 {
		inst := m.idle[n-1]
		m.idle = m.idle[:n-1]
		m.mu.Unlock()
		return inst, nil
	}
	m.mu.Unlock()
	inst, err := m.instantiate(ctx)
	if err == nil {
		err = inst.bind()
		if err != nil {
			inst.close()
		}
	}
	if err != nil {
		<-m.slots
		return nil, &instanceError{err}
	}
	m.add(1)
	return inst, nil
}

// release gives an instance back to the pool, unless it or the module is
// done for.
func (m *Module) release(inst *instance) {
	m.mu.Lock()
	keep := !m.shut && !inst.spent
	if keep {
		m.idle = append(m.idle, inst)
	} else {
		m.live--
	}
	m.mu.Unlock()
	if !keep {
		inst.close()
	}
	<-m.slots
}

func (m *Module) discard(inst *instance) {
	inst.close()
	m.add(-1)
	<-m.slots
}

// instance is one instantiation of a module: its own memory, and one call at
// a time.
type instance struct {
	mod    api.Module
	mem    api.Memory
	alloc  api.Function
	free   api.Function
	calls  [numCalls]api.Function
	output *output
	clocks *clocks
	// req is the memory the host writes requests into, and reqCap its size.
	// It is kept from one call to the next: a guest keeps nothing of a request.
	req, reqCap uint32
	stack       [2]uint64
	// told is the games the instance knows, by id.
	told map[string]*toldGame
	// spent says the instance answered its last call and is not to answer
	// another.
	spent bool
}

// The fixed things an instance is given in place of the machine's.
var (
	// fakeEpoch is where an instance's wall clock starts: 2026-01-01 UTC.
	fakeEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	// randSeed seeds every instance's random_get, which Go's runtime reads
	// once at start, for the order of its maps among other things.
	randSeed = [32]byte([]byte("bingle wasmrules: a fixed seed.."))
)

// An instance's clocks do not run. They move a millisecond with every call
// made into it, and a nanosecond each time they are read, so that no two
// readings are the same. The time a call seems to take is then next to
// nothing, and nearly all of an instance's time passes between its calls.
//
// The other way round, with a clock that moves a good deal at every reading,
// starves the module: Go's collector reads the clock many times in a cycle
// and the code between cycles never does, so the collector finds itself using
// all of the processor, stops asking the code that allocates to help it, and
// the heap grows to the memory cap within some tens of thousands of calls.
//
// The one exception is the bot call, whose search must keep to a budget of
// time: while it runs, both clocks move as fast as the machine's, to the
// millisecond, from where they stand. The wall clock keeps its own date, so a
// module never learns the real one: it only sees time pass. (Both run because
// runtimes differ in which they read: Go's the monotonic clock, TinyGo's the
// wall clock.) A search reads the time only to stop, so with a budget of nodes
// or playouts its answer is the same however fast the machine is.
const (
	clockPerCall = int64(time.Millisecond)
	clockPerRead = 1
	clockRunning = time.Millisecond // the resolution of a running clock
)

// clocks are an instance's wall clock and monotonic clock, in nanoseconds.
type clocks struct {
	wall, mono int64
	// since is when the clocks began to run, and wallFrom and monoFrom where
	// they then stood; since is zero while they stand.
	since              time.Time
	wallFrom, monoFrom int64
}

func (c *clocks) call() {
	c.wall += clockPerCall
	c.mono += clockPerCall
}

// run sets the clocks running with the machine's, and stand stops them again
// where they have got to.
func (c *clocks) run()   { c.since, c.wallFrom, c.monoFrom = time.Now(), c.wall, c.mono }
func (c *clocks) stand() { c.since = time.Time{} }

// read moves a clock on for a reading: a nanosecond from the last one, or to
// where the running clock has got to from from if that is later.
func (c *clocks) read(clock *int64, from int64) int64 {
	*clock += clockPerRead
	if !c.since.IsZero() {
		*clock = max(*clock, from+int64(time.Since(c.since).Truncate(clockRunning)))
	}
	return *clock
}

func (c *clocks) nanotime() int64 { return c.read(&c.mono, c.monoFrom) }

func (c *clocks) walltime() (int64, int32) {
	wall := c.read(&c.wall, c.wallFrom)
	return wall / int64(time.Second), int32(wall % int64(time.Second))
}

// instantiate makes an instance and runs its initialisation. This is the
// whole of what a module is given: no filesystem, arguments or environment
// (wazero gives none unless asked), no standard input, clocks that count
// their own readings, a sleep and a yield that return at once, and
// randomness from a fixed seed. What it writes goes to the log.
func (m *Module) instantiate(ctx context.Context) (*instance, error) {
	out := &output{log: m.rt.opt.Log, pack: m.pack, left: outputLogged}
	clk := &clocks{wall: fakeEpoch}
	cfg := wazero.NewModuleConfig().
		WithName(""). // anonymous, so a module can be instantiated many times
		WithStartFunctions("_initialize").
		WithStdout(out).
		WithStderr(out).
		WithWalltime(clk.walltime, wsys.ClockResolution(clockPerRead)).
		WithNanotime(clk.nanotime, wsys.ClockResolution(clockPerRead)).
		WithNanosleep(func(int64) {}).
		WithOsyield(func() {}).
		WithRandSource(rand.NewChaCha8(randSeed))
	mod, err := m.rt.rt.InstantiateModule(ctx, m.compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("the module does not start: %w%s", err, out.said())
	}
	return &instance{mod: mod, output: out, clocks: clk, told: map[string]*toldGame{}}, nil
}

// bind looks up what a call needs of the instance.
func (inst *instance) bind() error {
	inst.mem = inst.mod.Memory()
	if inst.mem == nil {
		return errors.New("the module has no memory")
	}
	inst.alloc = inst.mod.ExportedFunction(exportPrefix + "alloc")
	inst.free = inst.mod.ExportedFunction(exportPrefix + "free")
	for c := range inst.calls {
		inst.calls[c] = inst.mod.ExportedFunction(exportPrefix + callNames[c])
	}
	return nil
}

func (inst *instance) close() { _ = inst.mod.Close(context.Background()) }

// maxReply is the longest reply the host reads.
const maxReply = 16 << 20

// call writes the request into the instance's memory, makes the call, gives
// the reply to read where it lies and frees it. Any error is the instance's,
// which is then of no more use.
func (inst *instance) call(ctx context.Context, c call, req []byte, read func([]byte) error) error {
	inst.output.head = inst.output.head[:0]
	inst.clocks.call()
	if c == callBot {
		inst.clocks.run()
		defer inst.clocks.stand()
	}
	fail := func(what string, err error) error {
		return fmt.Errorf("%s: %w%s", what, explain(err), inst.output.said())
	}
	if uint32(len(req)) > inst.reqCap {
		if inst.reqCap > 0 {
			inst.stack[0], inst.stack[1] = uint64(inst.req), uint64(inst.reqCap)
			if err := inst.free.CallWithStack(ctx, inst.stack[:]); err != nil {
				return fail("bingle_free", err)
			}
			inst.reqCap = 0
		}
		size := max(uint32(len(req)), 4096)
		inst.stack[0] = uint64(size)
		if err := inst.alloc.CallWithStack(ctx, inst.stack[:]); err != nil {
			return fail("bingle_alloc", err)
		}
		inst.req = uint32(inst.stack[0])
		if inst.req == 0 {
			return fmt.Errorf("bingle_alloc gave no memory for %d bytes", size)
		}
		inst.reqCap = size
	}
	if !inst.mem.Write(inst.req, req) {
		return errors.New("bingle_alloc gave memory the module does not have")
	}
	inst.stack[0], inst.stack[1] = uint64(inst.req), uint64(len(req))
	if err := inst.calls[c].CallWithStack(ctx, inst.stack[:]); err != nil {
		return fail("the call", err)
	}
	ptr, size := uint32(inst.stack[0]>>32), uint32(inst.stack[0])
	if size > maxReply {
		return fmt.Errorf("the reply is %d bytes long", size)
	}
	reply, ok := inst.mem.Read(ptr, size)
	if !ok {
		return errors.New("the reply lies outside the module's memory")
	}
	if err := read(reply); err != nil {
		return err
	}
	// The call has its answer. An instance that cannot free it (the deadline
	// may have passed meanwhile) is not asked again.
	inst.stack[0], inst.stack[1] = uint64(ptr), uint64(size)
	if err := inst.free.CallWithStack(ctx, inst.stack[:]); err != nil {
		inst.spent = true
	}
	return nil
}

// explain says what an error of a call was, in the proof of concept's words.
func explain(err error) error {
	var exit *wsys.ExitError
	if !errors.As(err, &exit) {
		// A trap: its first line says which, and the rest is the module's stack.
		line, _, _ := strings.Cut(err.Error(), "\n")
		return errors.New(line)
	}
	switch exit.ExitCode() {
	case wsys.ExitCodeDeadlineExceeded:
		return errors.New("it ran out of time")
	case wsys.ExitCodeContextCanceled:
		return errors.New("it was stopped")
	}
	return fmt.Errorf("the module exited with code %d", exit.ExitCode())
}

// How much of a module's output is kept for an error, and how much of an
// instance's is logged in all.
const (
	outputKept   = 240
	outputLogged = 16 << 10
)

// output is where an instance's standard output and error go: the log, at
// Debug and cut short, and the start of what a call wrote into the error of
// the call if it fails (a Go panic says there what it was).
type output struct {
	log  *slog.Logger
	pack string
	head []byte
	left int
}

func (o *output) Write(p []byte) (int, error) {
	if room := outputKept - len(o.head); room > 0 {
		o.head = append(o.head, p[:min(room, len(p))]...)
	}
	if o.left > 0 && o.log.Enabled(context.Background(), slog.LevelDebug) {
		cut := p[:min(o.left, len(p))]
		o.left -= len(cut)
		o.log.Debug("wasmrules: a module wrote", "pack", o.pack, "output", string(cut))
	}
	return len(p), nil
}

// said is what the instance wrote since the call began, for an error.
func (o *output) said() string {
	text := strings.Join(strings.Fields(string(o.head)), " ")
	if text == "" {
		return ""
	}
	return " (" + text + ")"
}
