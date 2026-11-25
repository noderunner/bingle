// Package wasmrules runs a game's rules from a WebAssembly module, the rules
// half of the install-pack proof of concept (ADR 0147).
//
// A Runtime compiles a module once (Load) and gives back a Module, which is a
// state.Ruleset: each of the Ruleset's nine methods is one call into the
// module, JSON in and JSON out (ABI v0), so everything above the engine plays
// the game unchanged. A module built with the guest glue also answers the bot
// call, so a built-in bot's search runs inside it and crosses once a decision
// (state.Searcher). The module is written against the guest package, which
// imports nothing of the engine. Native links the same rules into the binary
// instead.
//
// A module is sandboxed. It has no filesystem, no network, no environment and
// no arguments. Its clocks are counters and its randomness a fixed sequence,
// so it cannot tell one run from another. Its memory is capped and every call
// has a deadline. An instance runs one call at a time, so a module has a pool
// of them, and an instance that traps, runs out of time or replies with
// something that is no reply is thrown away and replaced.
package wasmrules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// The defaults of Options (ADR 0147).
const (
	DefaultMemoryPages = 2048 // 128 MiB
	DefaultDeadline    = 2 * time.Second
)

// Options are a Runtime's limits. A zero value takes its default.
type Options struct {
	// MemoryPages caps an instance's memory, in pages of 64 KiB.
	MemoryPages uint32
	// Deadline is how long one call may take, the wait for a free instance
	// included. A negative Deadline is none at all, which is for measuring
	// what the deadline costs and nothing else: a module that never returns
	// then holds its caller for good.
	Deadline time.Duration
	// Instances caps a module's pool: as many calls as that run at once, and
	// further calls wait. The default is the size of the bots' slots, twice
	// the number of processors.
	Instances int
	// Log gets a thrown-away instance at Warn and a module's own output at
	// Debug. The default is slog.Default.
	Log *slog.Logger
}

// Runtime runs rules modules: one WebAssembly runtime, with a module compiled
// when it is loaded and kept in memory.
type Runtime struct {
	rt  wazero.Runtime
	opt Options
}

// compiled is shared by every Runtime of the process: one that loads a
// module another still holds (a test's) does not compile it again. It is in
// memory only, and a module leaves it when the last Module made of it closes.
var compiled = wazero.NewCompilationCache()

// New returns a Runtime. Close it when its modules are no longer played.
func New(ctx context.Context, opt Options) (*Runtime, error) {
	if opt.MemoryPages == 0 {
		opt.MemoryPages = DefaultMemoryPages
	}
	if opt.Deadline == 0 {
		opt.Deadline = DefaultDeadline
	}
	if opt.Instances <= 0 {
		opt.Instances = 2 * runtime.NumCPU()
	}
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	// The compiler where there is one, the interpreter elsewhere. Closing a
	// module when its context ends is what gives a call its deadline: the
	// module's code then checks, at every loop, whether it has been closed.
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCompilationCache(compiled).
		WithMemoryLimitPages(opt.MemoryPages).
		WithCloseOnContextDone(opt.Deadline > 0))
	// WASI is the only thing a module may import. Nothing stands behind it
	// that an instance is not given: see instantiate.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("wasmrules: WASI: %w", err)
	}
	return &Runtime{rt: rt, opt: opt}, nil
}

// deadline gives one call its time, and extra on top: a bot call's budget.
// Without a deadline, a call still stops when ctx ends.
func (r *Runtime) deadline(ctx context.Context, extra time.Duration) (context.Context, context.CancelFunc) {
	if r.opt.Deadline < 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, r.opt.Deadline+max(extra, 0))
}

// Close closes the runtime and every module loaded into it.
func (r *Runtime) Close(ctx context.Context) error { return r.rt.Close(ctx) }

// wasiImports is what a module may import of WASI: what a Go module that
// reads no file and opens no connection imports, and no more.
var wasiImports = []string{
	"args_get", "args_sizes_get", "clock_time_get", "environ_get", "environ_sizes_get",
	"fd_close", "fd_fdstat_get", "fd_fdstat_set_flags", "fd_prestat_dir_name", "fd_prestat_get",
	"fd_read", "fd_write", "poll_oneoff", "proc_exit", "random_get", "sched_yield",
}

// exportPrefix starts the name of every export of ABI v0: bingle_abi,
// bingle_alloc and bingle_free, and one export for each call.
const exportPrefix = "bingle_"

var (
	i32, i64    = api.ValueTypeI32, api.ValueTypeI64
	callParams  = []api.ValueType{i32, i32}
	callResults = []api.ValueType{i64}
)

// Load compiles a rules module and checks that it speaks the ABI this server
// does. pack names it in errors and in the log. The games it plays are not
// named here: an instance is told of a game before the first call about it.
func (r *Runtime) Load(ctx context.Context, pack string, wasm []byte) (*Module, error) {
	code, err := r.rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, fmt.Errorf("wasmrules: %s is not a module that can be run: %w", pack, err)
	}
	m := &Module{
		rt: r, pack: pack, compiled: code,
		slots: make(chan struct{}, r.opt.Instances), closed: make(chan struct{}),
	}
	if err := m.check(ctx); err != nil {
		_ = code.Close(ctx)
		return nil, fmt.Errorf("wasmrules: %s: %w", pack, err)
	}
	return m, nil
}

// check refuses a module that is not a rules module of this ABI. The version
// is asked of the module first, so one of another version is told so rather
// than that it lacks an export.
func (m *Module) check(ctx context.Context) error {
	for _, f := range m.compiled.ImportedFunctions() {
		module, name, _ := f.Import()
		if module != wasi_snapshot_preview1.ModuleName || !slices.Contains(wasiImports, name) {
			return fmt.Errorf("the module imports %s.%s, which a rules module is not given", module, name)
		}
	}
	exported := m.compiled.ExportedFunctions()
	has := func(name string, params, results []api.ValueType) error {
		f, ok := exported[exportPrefix+name]
		if !ok {
			return fmt.Errorf("the module does not export %s%s", exportPrefix, name)
		}
		if !slices.Equal(f.ParamTypes(), params) || !slices.Equal(f.ResultTypes(), results) {
			return fmt.Errorf("the module's %s%s does not have the signature of ABI %d", exportPrefix, name, guest.ABI)
		}
		return nil
	}
	if err := has("abi", nil, []api.ValueType{i32}); err != nil {
		return fmt.Errorf("%w: it is not a rules module", err)
	}

	ctx, cancel := m.rt.deadline(ctx, 0)
	defer cancel()
	inst, err := m.instantiate(ctx)
	if err != nil {
		return err
	}
	version, err := inst.mod.ExportedFunction(exportPrefix + "abi").Call(ctx)
	if err != nil {
		inst.close()
		return fmt.Errorf("bingle_abi: %w", err)
	}
	if got := int32(version[0]); got != guest.ABI {
		inst.close()
		return fmt.Errorf("the module speaks ABI %d, and this server ABI %d", got, guest.ABI)
	}
	err = errors.Join(has("alloc", []api.ValueType{i32}, []api.ValueType{i32}), has("free", callParams, nil))
	for c := call(0); c < numRequired; c++ {
		err = errors.Join(err, has(c.String(), callParams, callResults))
	}
	if _, ok := exported[exportPrefix+callBot.String()]; ok {
		err = errors.Join(err, has(callBot.String(), callParams, callResults))
		m.bot = true
	}
	if err == nil {
		err = inst.bind()
	}
	if err != nil {
		inst.close()
		return err
	}
	m.idle, m.live = append(m.idle, inst), 1
	return nil
}
