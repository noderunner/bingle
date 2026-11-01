package auth

import (
	"bytes"
	"context"
	_ "embed" // the built-in capabilities
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/noderunner/bingle/internal/store"
)

// Capabilities and modes (ADR 0071): one row per class and capability,
// seeded from bingle-capabilities.yaml on a fresh database, owned by the
// database and the admin page from then on.

// The capabilities.
const (
	CapPlay           = "play"
	CapJoin           = "join"
	CapCreateGame     = "create_game"
	CapChatGame       = "chat_game"
	CapChatGlobal     = "chat_global"
	CapVoiceGame      = "voice_game"
	CapVoiceDirect    = "voice_direct"
	CapDM             = "dm"
	CapNotify         = "notify"
	CapRunBatch       = "run_batch"
	CapViewMetrics    = "view_metrics"
	CapModerate       = "moderate"
	CapInstallPlugins = "install_plugins"
	CapAdmin          = "admin"
)

// AllCaps lists every capability, in the order the admin page shows them.
var AllCaps = []string{CapPlay, CapJoin, CapCreateGame, CapChatGame, CapChatGlobal, CapVoiceGame, CapVoiceDirect, CapDM, CapNotify,
	CapRunBatch, CapViewMetrics, CapModerate, CapInstallPlugins, CapAdmin}

// The classes. The admin group holds every capability and has no column.
const (
	ClassGuest = "guest"
	ClassUser  = "user"
	ClassAdmin = "admin"
)

// The rate limits' kinds.
const (
	LimitChat       = "chat"
	LimitCreateGame = "create_game"
	LimitDM         = "dm"
	LimitLogin      = "login"
)

var limitKinds = []string{LimitChat, LimitCreateGame, LimitDM, LimitLogin}

// Modes (BINGLE_AUTH_MODE).
const (
	ModeHybrid = "hybrid"
	ModeClosed = "closed"
)

//go:embed capabilities.yaml
var defaultCapsYAML []byte

// DefaultCapsYAML is the built-in seed, used when there is no file.
func DefaultCapsYAML() []byte { return bytes.Clone(defaultCapsYAML) }

// CapsSeed is a parsed bingle-capabilities.yaml.
type CapsSeed struct {
	Classes map[string][]string       `yaml:"classes"`
	Limits  map[string]map[string]int `yaml:"limits"`
}

// ParseCapsSeed reads a capabilities file, strictly: an unknown class,
// capability or limit is an error rather than something silently ignored.
func ParseCapsSeed(b []byte) (CapsSeed, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return CapsSeed{}, fmt.Errorf("auth: the capabilities file: %w", err)
	}
	for k := range raw {
		switch k {
		case "classes", "limits":
		case "mode", "signup":
			return CapsSeed{}, fmt.Errorf("auth: the capabilities file: %q is set with BINGLE_%s, not in the file", k, map[string]string{"mode": "AUTH_MODE", "signup": "SIGNUP"}[k])
		default:
			return CapsSeed{}, fmt.Errorf("auth: the capabilities file: unknown key %q", k)
		}
	}
	var s CapsSeed
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return CapsSeed{}, fmt.Errorf("auth: the capabilities file: %w", err)
	}
	for class, caps := range s.Classes {
		if class != ClassGuest && class != ClassUser {
			return CapsSeed{}, fmt.Errorf("auth: the capabilities file: class %q (guest and user only: the admin group has every capability)", class)
		}
		for _, c := range caps {
			if !slices.Contains(AllCaps, c) {
				return CapsSeed{}, fmt.Errorf("auth: the capabilities file: %s has an unknown capability %q", class, c)
			}
		}
	}
	for class, lim := range s.Limits {
		if class != ClassGuest && class != ClassUser {
			return CapsSeed{}, fmt.Errorf("auth: the capabilities file: limits for class %q", class)
		}
		for kind, n := range lim {
			if !slices.Contains(limitKinds, kind) || n < 0 {
				return CapsSeed{}, fmt.Errorf("auth: the capabilities file: %s's limit %s: %d", class, kind, n)
			}
		}
	}
	return s, nil
}

// rows is the seed as table rows: every capability of both classes, allowed
// or not.
func (s CapsSeed) rows() ([]store.CapRow, []store.LimitRow) {
	var caps []store.CapRow
	for _, class := range []string{ClassGuest, ClassUser} {
		for _, c := range AllCaps {
			caps = append(caps, store.CapRow{Class: class, Capability: c, Allowed: slices.Contains(s.Classes[class], c)})
		}
	}
	var limits []store.LimitRow
	for _, class := range []string{ClassGuest, ClassUser} {
		for kind, n := range s.Limits[class] {
			limits = append(limits, store.LimitRow{Class: class, Kind: kind, PerMinute: n})
		}
	}
	sort.Slice(limits, func(i, j int) bool { return limits[i].Class+limits[i].Kind < limits[j].Class+limits[j].Kind })
	return caps, limits
}

// ReadCapsFile reads the seed from path, or the built-in one when path is
// empty or names no file.
func ReadCapsFile(path string) (CapsSeed, []byte, error) {
	b := DefaultCapsYAML()
	if path != "" {
		got, err := os.ReadFile(path)
		switch {
		case err == nil:
			b = got
		case !errors.Is(err, os.ErrNotExist):
			return CapsSeed{}, nil, err
		}
	}
	s, err := ParseCapsSeed(b)
	return s, b, err
}

// Caps is the capability matrix, from the database, cached.
type Caps struct {
	st   *store.Store
	mode string
	file string // BINGLE_CAPS_FILE, for the drift badge and a reset

	mu      sync.RWMutex
	allowed map[string]bool // "class/capability"
	limits  map[string]int  // "class/kind"

	limiters sync.Map // "class/kind" → *windowLimiter
}

// NewCaps seeds the matrix from the file if the database has none yet, and
// loads it.
func NewCaps(ctx context.Context, st *store.Store, mode, file string) (*Caps, error) {
	c := &Caps{st: st, mode: mode, file: file}
	if c.mode == "" {
		c.mode = ModeHybrid
	}
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	// A fresh database: the file seeds it, this once.
	if len(c.allowed) == 0 {
		if err := c.Reset(ctx); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Reset re-applies the file (or the built-in seed): `bingle caps reset`.
func (c *Caps) Reset(ctx context.Context) error {
	seed, _, err := ReadCapsFile(c.file)
	if err != nil {
		return err
	}
	caps, limits := seed.rows()
	if err := c.st.ReplaceCaps(ctx, caps, limits); err != nil {
		return err
	}
	return c.load(ctx)
}

func (c *Caps) load(ctx context.Context) error {
	caps, limits, err := c.st.Caps(ctx)
	if err != nil {
		return err
	}
	allowed, lim := map[string]bool{}, map[string]int{}
	for _, r := range caps {
		allowed[r.Class+"/"+r.Capability] = r.Allowed
	}
	for _, l := range limits {
		lim[l.Class+"/"+l.Kind] = l.PerMinute
	}
	c.mu.Lock()
	c.allowed, c.limits = allowed, lim
	c.mu.Unlock()
	return nil
}

// Limit is a class's limit of a kind, per minute (0 for none).
func (c *Caps) Limit(class, kind string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.limits[class+"/"+kind]
}

// Mode is "hybrid" or "closed".
func (c *Caps) Mode() string { return c.mode }

// Allowed reports whether class may use capability.
func (c *Caps) Allowed(class, capability string) bool {
	if class == ClassAdmin {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allowed[class+"/"+capability]
}

// Set changes one class's permission, and keeps it.
func (c *Caps) Set(ctx context.Context, class, capability string, allowed bool) error {
	if (class != ClassGuest && class != ClassUser) || !slices.Contains(AllCaps, capability) {
		return fmt.Errorf("auth: no capability %q for class %q", capability, class)
	}
	if err := c.st.SetCap(ctx, class, capability, allowed); err != nil {
		return err
	}
	return c.load(ctx)
}

// Matrix is the matrix as the admin page shows it, and whether the database
// has drifted from the file.
func (c *Caps) Matrix(ctx context.Context) (rows []CapState, limits []store.LimitRow, drift bool, err error) {
	for _, cap := range AllCaps {
		rows = append(rows, CapState{Capability: cap, User: c.Allowed(ClassUser, cap), Guest: c.Allowed(ClassGuest, cap)})
	}
	_, limits, err = c.st.Caps(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	seed, _, err := ReadCapsFile(c.file)
	if err != nil {
		return rows, limits, true, nil
	}
	want, wantLimits := seed.rows()
	for _, r := range want {
		if c.Allowed(r.Class, r.Capability) != r.Allowed {
			drift = true
		}
	}
	if !slices.Equal(wantLimits, limits) {
		drift = true
	}
	return rows, limits, drift, nil
}

// CapState is one capability's row on the admin page.
type CapState struct {
	Capability string `json:"capability"`
	User       bool   `json:"user"`
	Guest      bool   `json:"guest"`
}

// allow takes one event of a rate-limited kind for key in class: false
// when the class's limit per minute is spent. A kind without a limit is
// free.
func (c *Caps) allow(class, kind, key string, now time.Time) bool {
	if class == ClassAdmin {
		return true
	}
	c.mu.RLock()
	n, ok := c.limits[class+"/"+kind]
	c.mu.RUnlock()
	if !ok || n == 0 {
		return true
	}
	v, _ := c.limiters.LoadOrStore(class+"/"+kind, &windowLimiter{window: time.Minute})
	l := v.(*windowLimiter)
	l.mu.Lock()
	l.per = n
	l.mu.Unlock()
	return l.allow(key, now)
}

// ---- which capability a request needs ---------------------------------------------

type route struct {
	method string // "" for any
	re     *regexp.Regexp
	cap    string
	limit  string // the rate limit it counts towards ("" for none)
}

// routes map a request to the capability it needs. The first match wins; a
// request that matches none needs nothing beyond the mode.
var routes = func() []route {
	r := func(method, pattern, cap, limit string) route {
		return route{method: method, re: regexp.MustCompile("^" + pattern + "$"), cap: cap, limit: limit}
	}
	return []route{
		r("POST", `/api/games`, CapCreateGame, LimitCreateGame),
		r("POST", `/api/games/import`, CapCreateGame, LimitCreateGame),
		r("POST", `/api/bot-matches`, CapCreateGame, LimitCreateGame),
		// A tutorial is the lowest barrier there is: playing is enough, but
		// it opens a room, so it counts towards the limit on making games.
		r("POST", `/api/tutorials`, CapPlay, LimitCreateGame),
		r("POST", `/api/games/[^/]+/join`, CapJoin, ""),
		r("GET", `/ws/[^/]+`, CapJoin, ""),
		r("POST", `/api/games/[^/]+/chat`, CapChatGame, LimitChat),
		r("POST", `/api/chat`, CapChatGlobal, LimitChat),
		r("", `/api/games/[^/]+/voice(/signal)?`, CapVoiceGame, ""),
		r("GET", `/api/rtc/config`, CapVoiceGame, ""),
		r("POST", `/api/dm/(call|signal)`, CapVoiceDirect, ""),
		r("POST", `/api/dm`, CapDM, LimitDM),
		r("POST", `/api/dm/(history|threads)`, CapDM, ""),
		r("GET", `/ws-dm`, CapDM, ""),
		r("", `/api/here`, CapDM, ""),
		r("POST", `/api/notify/.*`, CapNotify, ""),
		r("PUT", `/api/games/[^/]+/notify`, CapNotify, ""),
		r("POST", `/api/batches`, CapRunBatch, ""),
		r("POST", `/api/batches/[^/]+/cancel`, CapRunBatch, ""),
		r("DELETE", `/api/batches/[^/]+`, CapRunBatch, ""),
		r("POST", `/api/games/[^/]+/[^/]+`, CapPlay, ""),
		r("PUT", `/api/games/[^/]+/[^/]+`, CapPlay, ""),
		r("POST", `/api/(envs|matches)(/.*)?`, CapPlay, ""),
		r("", `/mcp(/.*)?`, CapPlay, ""),
	}
}()

// needs is the capability (and the rate limit) a request needs.
func needs(r *http.Request) (string, string) {
	for _, rt := range routes {
		if (rt.method == "" || rt.method == r.Method) && rt.re.MatchString(r.URL.Path) {
			return rt.cap, rt.limit
		}
	}
	return "", ""
}

// classOf is the class a principal acts as. A token never carries the
// admin group's powers.
func classOf(p Principal, ok bool) string {
	switch {
	case !ok || p.Account == nil:
		return ClassGuest
	case p.Account.Admin && p.Scopes == nil:
		return ClassAdmin
	}
	return ClassUser
}

// scopeCaps are what a token's scopes let it do (within its account's class).
var scopeCaps = map[string][]string{
	ScopePlay:        {CapPlay, CapJoin, CapChatGame, CapVoiceGame},
	ScopeCreateGame:  {CapCreateGame},
	ScopeReadMetrics: {CapViewMetrics},
}

// may reports whether p may use capability.
func (c *Caps) may(p Principal, ok bool, capability string) bool {
	if !c.Allowed(classOf(p, ok), capability) {
		return false
	}
	if ok && p.Scopes != nil {
		for _, s := range p.Scopes {
			if slices.Contains(scopeCaps[s], capability) {
				return true
			}
		}
		return false
	}
	return true
}

// Has is the capabilities p holds (for /auth/me, so a page can say what it
// will not offer).
func (c *Caps) Has(p Principal, ok bool) []string {
	var out []string
	for _, cap := range AllCaps {
		if c.may(p, ok, cap) {
			out = append(out, cap)
		}
	}
	return out
}

// check answers a request its principal may not make, and reports whether
// it did.
func (c *Caps) check(w http.ResponseWriter, r *http.Request, p Principal, ok bool, peer string, now time.Time) bool {
	// Closed: whoever is not signed in reaches the sign-in and nothing else.
	if c.mode == ModeClosed && (!ok || p.Account == nil) && !strings.HasPrefix(r.URL.Path, "/auth/") {
		writeError(w, http.StatusUnauthorized, "auth: this server is for its members: sign in first")
		return true
	}
	capability, limit := needs(r)
	if capability == "" {
		return false
	}
	if !c.may(p, ok, capability) {
		msg := fmt.Sprintf("auth: you may not do that here (it needs %q)", capability)
		if !ok || p.Account == nil {
			msg += ": sign in, or ask the server's administrator"
		}
		writeError(w, http.StatusForbidden, msg)
		return true
	}
	if limit != "" {
		key := peer
		if ok {
			key = p.ID
		}
		if !c.allow(classOf(p, ok), limit, key, now) {
			writeError(w, http.StatusTooManyRequests, "auth: slow down: too many of those in a minute")
			return true
		}
	}
	return false
}
