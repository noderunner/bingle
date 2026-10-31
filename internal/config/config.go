// Package config loads bingle's runtime configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// The default bind is **loopback**, not every interface.
	//
	// It used to be ":8080". The secure-connection rule
	// (docs/history/auth-and-accounts-plan.md, section 2) exempts a bind only
	// this machine can reach, and the one bind nobody chose must be the one
	// the rule exempts — otherwise `./bin/bingle` with nothing configured
	// would refuse to start, which is the opposite of what the plan is for.
	//
	// So bingle out of the box is a server for the machine it is on. Making
	// it reachable is now a deliberate act, and that act comes with a
	// certificate: see loadTransport.
	defaultAddr   = "127.0.0.1:8080"
	defaultDBPath = "./bingle.db"

	// With automatic certificates bingle takes the standard ports: the
	// certificate authority validates over 80 and 443.
	defaultTLSAddr  = ":443"
	defaultHTTPAddr = ":80"

	// defaultLogFormat and the format it is checked against are the formats
	// internal/logging builds a handler for.
	defaultLogFormat = "text"

	// The private listener for metrics: loopback, and a port in the range
	// Prometheus' own exporters use.
	defaultMetricsAddr = "127.0.0.1:9095"

	// The metrics database, beside the game database.
	defaultMetricsDBName = "bingle-metrics.db"

	// A quarter of a second: past what anybody calls instant, and well short
	// of what anybody calls broken.
	defaultSlowRequest = 250 * time.Millisecond
	defaultGuestIdle   = 90 * 24 * time.Hour

	// Five minutes: history that survives into the backup, not a time-series
	// database. A scraper reads /metrics as often as it likes regardless.
	defaultMetricsSample = 5 * time.Minute
	// A fortnight at five-minute resolution is enough to see what happened
	// and short enough to stay small.
	defaultMetricsRetainDays = 14
	// The shortest interval worth allowing: below this the history stops
	// being history and starts being a second scrape loop.
	minMetricsSample = 10 * time.Second
)

// Config holds bingle's runtime configuration.
type Config struct {
	Addr    string
	DBPath  string
	TLSCert string
	TLSKey  string
	// RetentionDays deletes games nobody has touched for that many days
	// (0 = keep everything, the default).
	RetentionDays int
	// PublicURL is the base URL players on other networks reach the server
	// by, such as "https://games.example.com" (BINGLE_PUBLIC_URL). Invite
	// links are built from it; empty means the address a request came to is
	// used.
	PublicURL string

	// TLSDomains are the host names bingle gets and renews a Let's Encrypt
	// certificate for (BINGLE_TLS_DOMAIN, comma separated). Empty means no
	// automatic certificates. Cannot be combined with TLSCert and TLSKey.
	TLSDomains []string
	// TLSEmail (BINGLE_TLS_EMAIL, optional) is the contact address given to
	// the certificate authority, and also the address email notifications
	// are sent from.
	TLSEmail string
	// TLSCacheDir keeps the certificates and the account key between runs
	// (BINGLE_TLS_CACHE, default "certs" next to the database).
	TLSCacheDir string
	// HTTPAddr is the plain-HTTP listener that answers certificate
	// challenges and redirects to HTTPS (BINGLE_HTTP_ADDR, default ":80"; only
	// used with TLSDomains).
	HTTPAddr string
	// ACMEURL replaces the certificate authority's directory
	// (BINGLE_TLS_ACME_URL): the Let's Encrypt staging directory while
	// trying things out, which issues untrusted certificates but has far
	// higher limits.
	ACMEURL string
	// TLSSelfSigned generates and caches a certificate at startup
	// (BINGLE_TLS_SELFSIGNED), so "the connection must be secure" does not
	// make development and CI painful. A browser will not trust it.
	TLSSelfSigned bool
	// TrustProxy says a terminating proxy is in front of bingle
	// (BINGLE_TRUST_PROXY): bingle then serves plain HTTP, believes the
	// proxy's X-Forwarded-Proto and X-Forwarded-For, refuses any request that
	// arrives as http — and applies **no loopback exemption at all**, because
	// a proxy on the same host makes every request look local.
	TrustProxy bool
	// Dev turns on the deterministic development conveniences that later
	// stages of the auth plan add: a known admin password and API token
	// (BINGLE_DEV). It is refused unless the bind is loopback, because it is
	// a published admin account on anything else.
	Dev bool
	// GuestIdle is how long a guest key may go unused before it stops working
	// (BINGLE_GUEST_IDLE, "90d" by default; ADR 0067). The guest's history
	// stays, under its ID.
	GuestIdle time.Duration

	// Accounts (ADR 0068). Signup is "open" or "closed" (BINGLE_SIGNUP);
	// open still needs mail, without which nobody can sign up.
	Signup string
	// SessionIdle and SessionMax end a session unused that long, and any
	// session that old (BINGLE_SESSION_IDLE, BINGLE_SESSION_MAX: 14d, 90d).
	SessionIdle, SessionMax time.Duration
	// AdminHandle, AdminEmail and AdminPassword make the first administrator
	// of an empty server (BINGLE_ADMIN_HANDLE, BINGLE_ADMIN_EMAIL,
	// BINGLE_ADMIN_PASSWORD). The address defaults to the handle at the
	// public host; the password to bingle123 under BINGLE_DEV or on a
	// loopback bind (never behind a trusted proxy) and to a random one
	// printed once otherwise. It must be changed at the first sign-in.
	AdminHandle   string
	AdminEmail    string
	AdminPassword Secret
	// OIDC are the sign-in providers (BINGLE_OIDC_PROVIDERS and a
	// BINGLE_OIDC_<ID>_* block each; ADR 0070).
	OIDC []OIDCProvider
	// AuthMode is "hybrid" (guests play) or "closed" (members only)
	// (BINGLE_AUTH_MODE; ADR 0071).
	AuthMode string
	// CapsFile seeds the capabilities of a fresh database
	// (BINGLE_CAPS_FILE, bingle-capabilities.yaml by default; the built-in
	// defaults when there is no such file).
	CapsFile string

	// HSTSSeconds, when above zero, adds a Strict-Transport-Security header of
	// that many seconds to HTTPS responses (BINGLE_TLS_HSTS_SECONDS). Off by
	// default: browsers refuse plain HTTP for the host while it is in force.
	HSTSSeconds int

	// LogLevel is how much the server says (BINGLE_LOG_LEVEL: debug, info,
	// warn or error; info by default). debug is the verbose mode: it adds a
	// line per request, per WebSocket and per move.
	LogLevel slog.Level
	// LogFormat is "text" (the default: key=value lines) or "json", one
	// object per line for a log collector (BINGLE_LOG_FORMAT).
	LogFormat string

	// MetricsAddr is the address of the private listener that serves
	// /metrics, and from later stages the health page and the journal
	// endpoints (BINGLE_METRICS_ADDR, default "127.0.0.1:9095"). Empty turns
	// all of it off. It is deliberately not the public listener: the public
	// site has no authentication of any kind, and metrics must not be on it.
	MetricsAddr string
	// MetricsDB is the database the slow-request journal (and, from Stage 5
	// of the observability plan, the metric history) is kept in
	// (BINGLE_METRICS_DB, default "bingle-metrics.db" beside the game
	// database). Empty keeps no history: the journal then logs its WARN line
	// and writes no row. It is a separate file so that measuring the server
	// never writes to the database being measured.
	MetricsDB string
	// MetricsSample is how often the metric history is written
	// (BINGLE_METRICS_SAMPLE, default 5m). Zero keeps no history, and the
	// metrics are still served live.
	MetricsSample time.Duration
	// MetricsRetainDays is how long five-minute samples are kept at full
	// resolution before they are compacted to hourly ones, then to daily
	// after a year (BINGLE_METRICS_RETAIN_DAYS, default 14). Zero keeps
	// everything at full resolution, which grows without bound.
	MetricsRetainDays int

	// SlowRequest is how slow a request has to be before the journal writes
	// it down (BINGLE_SLOW_REQUEST, default 250ms). Zero turns the journal
	// off, collector and all. A request the server failed is written down
	// however fast it was.
	SlowRequest time.Duration

	// ClientErrors mounts POST /api/client-error, where a browser reports
	// what went wrong in it (BINGLE_CLIENT_ERRORS, on by default; "off"
	// removes the route). It is the one observability route on the public
	// listener, because a browser has to be able to reach it.
	ClientErrors bool

	// Debug narrows the debug firehose and turns on the development-only
	// checks. See Debug.
	Debug Debug

	// MetricsToken is the bearer token the private listener requires
	// (BINGLE_METRICS_TOKEN). It is required when MetricsAddr is not on the
	// loopback interface, and optional when it is.
	MetricsToken Secret

	// Mail is the setup for email notifications (see Mail).
	Mail Mail

	// Changelog is the text of CHANGELOG.md, whose newest release the help
	// window shows. It is not read from the environment: main sets it from the
	// copy built into the binary.
	Changelog string

	// RTCStun lists the STUN servers voice chat uses to find a path between
	// players (BINGLE_RTC_STUN, comma separated "stun:host:port" URLs). The
	// default is Google's public one; "none" means no STUN server.
	RTCStun []string
	// RTCTurnURLs and RTCTurnSecret describe a TURN relay (coturn) for players
	// whose network blocks direct connections (BINGLE_RTC_TURN_URLS, comma
	// separated "turn:host:port" / "turns:host:port" URLs, and
	// BINGLE_RTC_TURN_SECRET, coturn's static-auth-secret). Bingle then hands
	// each player time-limited credentials for it. Unset: no relay.
	RTCTurnURLs   []string
	RTCTurnSecret string
}

// Debug holds the settings that make a verbose server readable, and the two
// that are for development only.
type Debug struct {
	// Games limits the per-move debug lines to these game families
	// (BINGLE_DEBUG_GAMES, comma separated). Empty means every game: debug
	// across two dozen games is a firehose, which is what this narrows.
	Games []string
	// Room logs everything about one room whatever the level
	// (BINGLE_DEBUG_ROOM), for chasing a single report.
	Room string
	// Secrets puts full positions in the debug lines instead of the public
	// view (BINGLE_DEBUG_SECRETS). **Development only**: a position carries
	// committed dice, hidden hands and RNG state.
	Secrets bool
	// Paranoid makes the move funnel check the engine against itself after
	// every move (BINGLE_PARANOID). **Development and CI only**: it runs
	// Legal a second time on every move.
	Paranoid bool
	// Dir is where repro bundles are written (BINGLE_DEBUG_DIR); empty
	// writes none. A bundle is the full position, secrets and all, which is
	// what makes it reproduce — so it goes to a directory and never to the
	// log stream.
	Dir string
}

// Secret is a credential that must never be printed: every way of formatting
// it (String, %v, %+v, %#v, JSON, slog) gives "[redacted]". Only Reveal returns
// the text, and only the code that has to use the credential calls it.
type Secret struct{ v string }

// NewSecret wraps a credential.
func NewSecret(v string) Secret { return Secret{v: v} }

// Reveal returns the credential.
func (s Secret) Reveal() string { return s.v }

// Set reports whether there is a credential.
func (s Secret) Set() bool { return s.v != "" }

// String implements fmt.Stringer.
func (Secret) String() string { return "[redacted]" }

// GoString implements fmt.GoStringer (%#v).
func (Secret) GoString() string { return `"[redacted]"` }

// MarshalJSON implements json.Marshaler.
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

// MarshalText implements encoding.TextMarshaler.
func (Secret) MarshalText() ([]byte, error) { return []byte("[redacted]"), nil }

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// Mail is the outgoing mail setup for email notifications. The feature is on
// only when Host, Password and From are all set (Enabled).
type Mail struct {
	// Host and Port are the SMTP server (BINGLE_SMTP_HOST, BINGLE_SMTP_PORT,
	// default 587).
	Host string
	Port int
	// User is the login name (BINGLE_SMTP_USER; defaults to From).
	User string
	// Password is read from the file BINGLE_SMTP_PASSWORD_FILE, which must not
	// be readable by group or others. There is deliberately no plain
	// environment variable for it.
	Password Secret
	// From is the sender address: BINGLE_TLS_EMAIL.
	From string
	// Mode is "starttls" (the default: port 587 style) or "tls" (implicit TLS,
	// port 465 style) (BINGLE_SMTP_TLS). Mail is never sent in the clear.
	Mode string
	// CAFile is a PEM file with the certificate that signs the server's
	// (BINGLE_SMTP_CA_FILE), for a server with a certificate of its own such as
	// Proton Mail Bridge on this machine. Empty means the system's roots.
	CAFile string
	// HourlyLimit and DailyLimit cap all mail bingle sends (BINGLE_MAIL_HOURLY_LIMIT,
	// BINGLE_MAIL_DAILY_LIMIT; 20 and 150).
	HourlyLimit int
	DailyLimit  int
	// AwayUnit is the length of a "minute" in the away setting; zero means a
	// real minute (tests shorten it).
	AwayUnit time.Duration
}

// Enabled reports whether email notifications are set up.
func (m Mail) Enabled() bool { return m.Host != "" && m.Password.Set() && m.From != "" }

// DefaultRTCStun is the STUN server used when BINGLE_RTC_STUN is not set.
const DefaultRTCStun = "stun:stun.l.google.com:19302"

// TLSEnabled reports whether bingle serves HTTPS, by automatic certificates,
// by a certificate and key from files, or by one it generates for itself.
func (c Config) TLSEnabled() bool {
	return len(c.TLSDomains) > 0 || (c.TLSCert != "" && c.TLSKey != "") || c.TLSSelfSigned
}

// LoopbackBind reports whether the public listener is reachable only from this
// machine. It is what makes `./bin/bingle` with nothing configured, and every
// in-process test, legal under the secure-connection rule.
func (c Config) LoopbackBind() bool {
	local, err := LoopbackAddr(c.Addr)
	return err == nil && local
}

// SecureConnection reports whether a client's connection to bingle will be
// secure: bingle terminates TLS, or a trusted proxy does.
func (c Config) SecureConnection() bool {
	return c.TLSEnabled() || c.TrustProxy
}

// Load reads configuration from environment variables, falling back to
// defaults when unset.
func Load() (Config, error) {
	cfg := Config{
		Addr:        defaultAddr,
		DBPath:      defaultDBPath,
		LogFormat:   defaultLogFormat,
		MetricsAddr: defaultMetricsAddr,
		SlowRequest: defaultSlowRequest,

		MetricsSample:     defaultMetricsSample,
		MetricsRetainDays: defaultMetricsRetainDays,
		ClientErrors:      true,
	}

	// slog.Level parses "debug", "info", "warn" and "error" itself, in any
	// case, and its zero value is already info.
	if v := strings.TrimSpace(os.Getenv("BINGLE_LOG_LEVEL")); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("config: BINGLE_LOG_LEVEL must be debug, info, warn or error, got %q", v)
		}
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("BINGLE_LOG_FORMAT"))); v != "" {
		if v != "text" && v != "json" {
			return Config{}, fmt.Errorf("config: BINGLE_LOG_FORMAT must be text or json, got %q", v)
		}
		cfg.LogFormat = v
	}

	if v := os.Getenv("BINGLE_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("BINGLE_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("BINGLE_TLS_CERT"); v != "" {
		cfg.TLSCert = v
	}
	if v := os.Getenv("BINGLE_TLS_KEY"); v != "" {
		cfg.TLSKey = v
	}

	addrSet := os.Getenv("BINGLE_ADDR") != ""

	if e := strings.TrimSpace(os.Getenv("BINGLE_TLS_EMAIL")); e != "" {
		a, err := mail.ParseAddress(e)
		if err != nil {
			return Config{}, fmt.Errorf("config: BINGLE_TLS_EMAIL: %q is not an email address", e)
		}
		cfg.TLSEmail = a.Address
	}

	if v := os.Getenv("BINGLE_TLS_DOMAIN"); strings.TrimSpace(v) != "" {
		domains, err := ParseTLSDomains(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: BINGLE_TLS_DOMAIN: %w", err)
		}
		if cfg.TLSCert != "" || cfg.TLSKey != "" {
			return Config{}, fmt.Errorf("config: BINGLE_TLS_DOMAIN (automatic certificates) and BINGLE_TLS_CERT/BINGLE_TLS_KEY (certificate files) cannot both be set")
		}
		cfg.TLSDomains = domains
		if !addrSet {
			cfg.Addr = defaultTLSAddr
		}
		cfg.HTTPAddr = defaultHTTPAddr
		if a := os.Getenv("BINGLE_HTTP_ADDR"); a != "" {
			cfg.HTTPAddr = a
		}
		if cfg.HTTPAddr == cfg.Addr {
			return Config{}, fmt.Errorf("config: BINGLE_HTTP_ADDR and BINGLE_ADDR are both %q", cfg.Addr)
		}
		cfg.TLSCacheDir = defaultCacheDir(cfg.DBPath)
		if c := strings.TrimSpace(os.Getenv("BINGLE_TLS_CACHE")); c != "" {
			cfg.TLSCacheDir = c
		}
		if u := strings.TrimSpace(os.Getenv("BINGLE_TLS_ACME_URL")); u != "" {
			if _, err := ParsePublicURL(u); err != nil {
				return Config{}, fmt.Errorf("config: BINGLE_TLS_ACME_URL: %w", err)
			}
			cfg.ACMEURL = u
		}
	}
	if err := loadTransport(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadMail(&cfg); err != nil {
		return Config{}, err
	}

	stun, err := parseICEList("BINGLE_RTC_STUN", os.Getenv("BINGLE_RTC_STUN"), []string{"stun:", "stuns:"})
	if err != nil {
		return Config{}, err
	}
	if len(stun) == 0 && strings.TrimSpace(os.Getenv("BINGLE_RTC_STUN")) == "" {
		stun = []string{DefaultRTCStun}
	}
	cfg.RTCStun = stun
	turn, err := parseICEList("BINGLE_RTC_TURN_URLS", os.Getenv("BINGLE_RTC_TURN_URLS"), []string{"turn:", "turns:"})
	if err != nil {
		return Config{}, err
	}
	cfg.RTCTurnURLs = turn
	cfg.RTCTurnSecret = os.Getenv("BINGLE_RTC_TURN_SECRET")
	if len(turn) > 0 && cfg.RTCTurnSecret == "" {
		return Config{}, fmt.Errorf("config: BINGLE_RTC_TURN_URLS needs BINGLE_RTC_TURN_SECRET (coturn's static-auth-secret)")
	}
	if len(turn) == 0 && cfg.RTCTurnSecret != "" {
		return Config{}, fmt.Errorf("config: BINGLE_RTC_TURN_SECRET needs BINGLE_RTC_TURN_URLS")
	}

	if v := os.Getenv("BINGLE_TLS_HSTS_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("config: BINGLE_TLS_HSTS_SECONDS must be a non-negative integer, got %q", v)
		}
		cfg.HSTSSeconds = n
	}

	if v := strings.TrimSpace(os.Getenv("BINGLE_PUBLIC_URL")); v != "" {
		u, err := ParsePublicURL(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: BINGLE_PUBLIC_URL: %w", err)
		}
		cfg.PublicURL = u
	}

	// Invite links point at the HTTPS name unless told otherwise.
	if cfg.PublicURL == "" && len(cfg.TLSDomains) > 0 {
		cfg.PublicURL = "https://" + cfg.TLSDomains[0]
	}

	if err := loadMetrics(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadJournal(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadDebug(&cfg); err != nil {
		return Config{}, err
	}

	if v := os.Getenv("BINGLE_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("config: BINGLE_RETENTION_DAYS must be a non-negative integer, got %q", v)
		}
		cfg.RetentionDays = n
	}

	return cfg, nil
}

// loadMetrics reads the private listener's settings. The listener is on the
// loopback interface by default; putting it anywhere a network can reach
// means a token, because everything it serves — which games are being played,
// how busy the server is, and from later stages the health page — would
// otherwise be readable by anyone who can reach the port.
func loadMetrics(cfg *Config) error {
	if v, ok := os.LookupEnv("BINGLE_METRICS_ADDR"); ok {
		cfg.MetricsAddr = strings.TrimSpace(v)
	}
	if tok := strings.TrimSpace(os.Getenv("BINGLE_METRICS_TOKEN")); tok != "" {
		if len(tok) < minMetricsToken {
			return fmt.Errorf("config: BINGLE_METRICS_TOKEN must be at least %d characters", minMetricsToken)
		}
		cfg.MetricsToken = NewSecret(tok)
	}
	if cfg.MetricsAddr == "" {
		if cfg.MetricsToken.Set() {
			return fmt.Errorf("config: BINGLE_METRICS_TOKEN needs BINGLE_METRICS_ADDR (there is no listener to protect)")
		}
		return nil
	}
	local, err := LoopbackAddr(cfg.MetricsAddr)
	if err != nil {
		return fmt.Errorf("config: BINGLE_METRICS_ADDR: %w", err)
	}
	if !local && !cfg.MetricsToken.Set() {
		return fmt.Errorf("config: BINGLE_METRICS_ADDR is %q, which is not on this machine's loopback interface, so BINGLE_METRICS_TOKEN must be set: everything that listener serves would otherwise be readable by anyone who can reach the port", cfg.MetricsAddr)
	}
	return nil
}

// minMetricsToken is the shortest bearer token worth having; a short one is a
// guessable one.
const minMetricsToken = 16

// loadJournal reads the slow-request journal's settings. The journal is on by
// default, because a request nobody can explain is the thing it exists to
// explain; what it costs is one pooled slice per request.
func loadJournal(cfg *Config) error {
	cfg.MetricsDB = filepath.Join(filepath.Dir(cfg.DBPath), defaultMetricsDBName)
	if v, ok := os.LookupEnv("BINGLE_METRICS_DB"); ok {
		cfg.MetricsDB = strings.TrimSpace(v)
	}
	cfg.Signup = "open"
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("BINGLE_SIGNUP"))); v != "" {
		if v != "open" && v != "invite" && v != "closed" {
			return fmt.Errorf("config: BINGLE_SIGNUP must be open, invite or closed, got %q", v)
		}
		cfg.Signup = v
	}
	cfg.SessionIdle, cfg.SessionMax = 14*24*time.Hour, 90*24*time.Hour
	for _, d := range []struct {
		name string
		to   *time.Duration
	}{{"BINGLE_SESSION_IDLE", &cfg.SessionIdle}, {"BINGLE_SESSION_MAX", &cfg.SessionMax}} {
		if v := strings.TrimSpace(os.Getenv(d.name)); v != "" {
			got, err := parseDays(v)
			if err != nil || got < time.Hour {
				return fmt.Errorf("config: %s must be a duration of an hour or more, such as 14d or 36h, got %q", d.name, v)
			}
			*d.to = got
		}
	}
	if cfg.SessionIdle > cfg.SessionMax {
		return fmt.Errorf("config: BINGLE_SESSION_IDLE (%s) is longer than BINGLE_SESSION_MAX (%s)", cfg.SessionIdle, cfg.SessionMax)
	}
	cfg.AuthMode = "hybrid"
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("BINGLE_AUTH_MODE"))); v != "" {
		if v != "hybrid" && v != "closed" {
			return fmt.Errorf("config: BINGLE_AUTH_MODE must be hybrid or closed, got %q", v)
		}
		cfg.AuthMode = v
	}
	cfg.CapsFile = "bingle-capabilities.yaml"
	if v, ok := os.LookupEnv("BINGLE_CAPS_FILE"); ok {
		cfg.CapsFile = strings.TrimSpace(v)
	}
	cfg.AdminHandle = "admin"
	if v := strings.TrimSpace(os.Getenv("BINGLE_ADMIN_HANDLE")); v != "" {
		cfg.AdminHandle = v
	}
	cfg.AdminEmail = strings.TrimSpace(os.Getenv("BINGLE_ADMIN_EMAIL"))
	cfg.AdminPassword = NewSecret(os.Getenv("BINGLE_ADMIN_PASSWORD"))
	oidc, err := loadOIDC(cfg.PublicURL)
	if err != nil {
		return err
	}
	cfg.OIDC = oidc
	cfg.GuestIdle = defaultGuestIdle
	if v := strings.TrimSpace(os.Getenv("BINGLE_GUEST_IDLE")); v != "" {
		d, err := parseDays(v)
		if err != nil || d < time.Hour {
			return fmt.Errorf("config: BINGLE_GUEST_IDLE must be a duration of an hour or more, such as 90d or 720h, got %q", v)
		}
		cfg.GuestIdle = d
	}
	if v := strings.TrimSpace(os.Getenv("BINGLE_SLOW_REQUEST")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return fmt.Errorf("config: BINGLE_SLOW_REQUEST must be a duration such as 250ms or 2s (0 turns the journal off), got %q", v)
		}
		cfg.SlowRequest = d
	}
	if v := strings.TrimSpace(os.Getenv("BINGLE_METRICS_SAMPLE")); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil || d < 0:
			return fmt.Errorf("config: BINGLE_METRICS_SAMPLE must be a duration such as 5m (0 keeps no history), got %q", v)
		case d > 0 && d < minMetricsSample:
			return fmt.Errorf("config: BINGLE_METRICS_SAMPLE is %s, which is less than %s: the history is written to disk, and below that it stops being history and becomes a second scrape loop", d, minMetricsSample)
		}
		cfg.MetricsSample = d
	}
	if v, ok := os.LookupEnv("BINGLE_CLIENT_ERRORS"); ok && strings.TrimSpace(v) != "" {
		on, err := parseBool("BINGLE_CLIENT_ERRORS", v)
		if err != nil {
			return err
		}
		cfg.ClientErrors = on
	}
	if v := strings.TrimSpace(os.Getenv("BINGLE_METRICS_RETAIN_DAYS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("config: BINGLE_METRICS_RETAIN_DAYS must be a non-negative integer (0 keeps everything at full resolution), got %q", v)
		}
		cfg.MetricsRetainDays = n
	}
	return nil
}

// LoopbackAddr reports whether a listen address is reachable only from this
// machine. A bare port (":9095") or an unspecified host ("0.0.0.0:9095") is
// not: both accept from anywhere.
func LoopbackAddr(addr string) (bool, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false, fmt.Errorf("%q must be host:port, such as 127.0.0.1:9095: %w", addr, err)
	}
	if port == "" {
		return false, fmt.Errorf("%q has no port", addr)
	}
	if host == "" {
		return false, nil // ":9095" listens on every interface
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// A name: "localhost" is the only one worth treating as loopback
		// without resolving it, and resolution belongs nowhere near config.
		return strings.EqualFold(host, "localhost"), nil
	}
	return ip.IsLoopback(), nil
}

// loadDebug reads the debug settings. The two development-only ones are
// refused rather than ignored when they are set to something that is not a
// plain on or off, because switching them on by accident in production is
// exactly what their documentation warns about.
func loadDebug(cfg *Config) error {
	for _, part := range strings.Split(os.Getenv("BINGLE_DEBUG_GAMES"), ",") {
		if g := strings.ToLower(strings.TrimSpace(part)); g != "" {
			cfg.Debug.Games = append(cfg.Debug.Games, g)
		}
	}
	cfg.Debug.Dir = strings.TrimSpace(os.Getenv("BINGLE_DEBUG_DIR"))
	if v := strings.TrimSpace(os.Getenv("BINGLE_DEBUG_ROOM")); v != "" {
		if !roomCodeish.MatchString(v) {
			return fmt.Errorf("config: BINGLE_DEBUG_ROOM: %q is not a room code", v)
		}
		cfg.Debug.Room = strings.ToUpper(v)
	}
	for name, dst := range map[string]*bool{
		"BINGLE_DEBUG_SECRETS": &cfg.Debug.Secrets,
		"BINGLE_PARANOID":      &cfg.Debug.Paranoid,
	} {
		on, err := parseBool(name, os.Getenv(name))
		if err != nil {
			return err
		}
		*dst = on
	}
	return nil
}

// roomCodeish is what a room code looks like: letters and digits, and short.
var roomCodeish = regexp.MustCompile(`^[A-Za-z0-9]{4,12}$`)

// parseBool reads an on/off setting, refusing anything ambiguous.
func parseBool(name, raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "0", "off", "false", "no":
		return false, nil
	case "1", "on", "true", "yes":
		return true, nil
	default:
		return false, fmt.Errorf("config: %s must be on or off (1/0, on/off, true/false), got %q", name, raw)
	}
}

// ParsePublicURL checks a public base URL and returns it without a trailing
// slash: an absolute http or https URL with a host, and no query or fragment.
func ParsePublicURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%q must be an absolute http or https URL such as https://games.example.com", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q must not have a query or a fragment", raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

var hostLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ParseTLSDomains reads a comma separated list of host names to get
// certificates for: plain DNS names (lower-cased), with no scheme, port, path
// or wildcard, and not IP addresses — a public certificate authority issues
// for names it can reach, one by one.
func ParseTLSDomains(raw string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		d := strings.ToLower(strings.TrimSpace(part))
		if d == "" {
			continue
		}
		if strings.ContainsAny(d, "/:*@ ") {
			return nil, fmt.Errorf("%q must be a bare host name such as games.example.com (no scheme, port or wildcard)", d)
		}
		d = strings.TrimSuffix(d, ".")
		labels := strings.Split(d, ".")
		if net.ParseIP(d) != nil || len(labels) < 2 || len(d) > 253 {
			return nil, fmt.Errorf("%q must be a public DNS name such as games.example.com (not an IP address or a single label)", d)
		}
		for _, l := range labels {
			if len(l) > 63 || !hostLabel.MatchString(l) {
				return nil, fmt.Errorf("%q is not a valid host name", d)
			}
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no host name given")
	}
	return out, nil
}

// parseICEList reads a comma separated list of STUN or TURN URLs, each starting
// with one of the schemes. "none" (alone) gives an empty list.
func parseICEList(name, raw string, schemes []string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		u := strings.TrimSpace(part)
		if u == "" || strings.EqualFold(u, "none") {
			continue
		}
		ok := false
		for _, sc := range schemes {
			if strings.HasPrefix(strings.ToLower(u), sc) && len(u) > len(sc) {
				ok = true
			}
		}
		if !ok || strings.ContainsAny(u, " \t") {
			return nil, fmt.Errorf("config: %s: %q must be a URL starting with %s", name, u, strings.Join(schemes, " or "))
		}
		out = append(out, u)
	}
	return out, nil
}

// loadTransport reads the settings that decide how a client reaches bingle,
// and enforces the rule the whole of docs/history/auth-and-accounts-plan.md
// rests on: **the connection must be secure**.
//
// Secure means TLS at bingle — certificate files, ACME, or one it generates —
// or plain HTTP behind a proxy that terminates TLS and says so. The one
// exemption is a bind only this machine can reach, which is what keeps
// `./bin/bingle` on a laptop and every in-process test working with nothing
// configured.
//
// It is enforced here, at startup, rather than at the first request: a server
// that would hand out passwords over plain HTTP should not come up at all.
func loadTransport(cfg *Config) error {
	on, err := parseBool("BINGLE_TLS_SELFSIGNED", os.Getenv("BINGLE_TLS_SELFSIGNED"))
	if err != nil {
		return err
	}
	cfg.TLSSelfSigned = on
	if cfg.TLSSelfSigned && len(cfg.TLSDomains) > 0 {
		return fmt.Errorf("config: BINGLE_TLS_SELFSIGNED and BINGLE_TLS_DOMAIN (automatic certificates) cannot both be set: pick which certificate to serve")
	}
	if cfg.TLSSelfSigned && (cfg.TLSCert != "" || cfg.TLSKey != "") {
		return fmt.Errorf("config: BINGLE_TLS_SELFSIGNED and BINGLE_TLS_CERT/BINGLE_TLS_KEY cannot both be set: pick which certificate to serve")
	}
	// A generated certificate needs somewhere to live across restarts.
	if cfg.TLSSelfSigned && cfg.TLSCacheDir == "" {
		cfg.TLSCacheDir = defaultCacheDir(cfg.DBPath)
		if c := strings.TrimSpace(os.Getenv("BINGLE_TLS_CACHE")); c != "" {
			cfg.TLSCacheDir = c
		}
	}

	if on, err = parseBool("BINGLE_TRUST_PROXY", os.Getenv("BINGLE_TRUST_PROXY")); err != nil {
		return err
	}
	cfg.TrustProxy = on
	// BINGLE_TRUST_PROXY together with TLS is allowed, and deliberately not
	// refused. It is unusual — a proxy forwarding to bingle over TLS — but it
	// is not wrong: a request that arrives over TLS is secure whatever the
	// headers say, and the forwarded scheme is only consulted when it does
	// not. Refusing the combination would make the container image, which
	// generates a certificate for itself, impossible to put behind a proxy
	// without first knowing to unset that.

	if on, err = parseBool("BINGLE_DEV", os.Getenv("BINGLE_DEV")); err != nil {
		return err
	}
	cfg.Dev = on
	if cfg.Dev && !cfg.LoopbackBind() {
		return fmt.Errorf("config: BINGLE_DEV is on and bingle is listening on %q, which is not loopback. BINGLE_DEV turns on deterministic development credentials; on anything a network can reach, that is a published administrator account", cfg.Addr)
	}
	if cfg.Dev && cfg.TrustProxy {
		return fmt.Errorf("config: BINGLE_DEV and BINGLE_TRUST_PROXY cannot both be set: behind a proxy every request looks local, so the loopback exemptions BINGLE_DEV relies on do not hold")
	}

	if !cfg.SecureConnection() && !cfg.LoopbackBind() {
		return fmt.Errorf("config: bingle is listening on %q, which a network can reach, with no secure connection. "+
			"Set BINGLE_TLS_DOMAIN for automatic certificates, BINGLE_TLS_CERT and BINGLE_TLS_KEY for your own, "+
			"BINGLE_TLS_SELFSIGNED=1 for a generated one, or BINGLE_TRUST_PROXY=1 if a proxy in front of bingle terminates TLS. "+
			"A loopback address such as 127.0.0.1:8080 needs none of these", cfg.Addr)
	}
	return nil
}

// defaultCacheDir is where certificates are kept: beside the database, so a
// backup of that directory takes them.
func defaultCacheDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "certs")
}

// loadMail reads the BINGLE_SMTP_* and BINGLE_MAIL_* settings. Without
// BINGLE_SMTP_HOST the feature is simply off.
func loadMail(cfg *Config) error {
	m := Mail{Port: 587, Mode: "starttls", HourlyLimit: 20, DailyLimit: 150, From: cfg.TLSEmail}
	m.Host = strings.TrimSpace(os.Getenv("BINGLE_SMTP_HOST"))
	if m.Host == "" {
		if os.Getenv("BINGLE_SMTP_PASSWORD_FILE") != "" {
			return fmt.Errorf("config: BINGLE_SMTP_PASSWORD_FILE needs BINGLE_SMTP_HOST")
		}
		return nil
	}
	if strings.ContainsAny(m.Host, "/ :@") {
		return fmt.Errorf("config: BINGLE_SMTP_HOST: %q must be a bare host name", m.Host)
	}
	if v := os.Getenv("BINGLE_SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("config: BINGLE_SMTP_PORT must be a port number, got %q", v)
		}
		m.Port = n
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("BINGLE_SMTP_TLS"))); v != "" {
		if v != "starttls" && v != "tls" {
			return fmt.Errorf("config: BINGLE_SMTP_TLS must be starttls or tls, got %q", v)
		}
		m.Mode = v
	}
	m.User = strings.TrimSpace(os.Getenv("BINGLE_SMTP_USER"))
	if m.User == "" {
		m.User = m.From
	}
	m.CAFile = strings.TrimSpace(os.Getenv("BINGLE_SMTP_CA_FILE"))
	for name, dst := range map[string]*int{"BINGLE_MAIL_HOURLY_LIMIT": &m.HourlyLimit, "BINGLE_MAIL_DAILY_LIMIT": &m.DailyLimit} {
		if v := os.Getenv(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return fmt.Errorf("config: %s must be a positive integer, got %q", name, v)
			}
			*dst = n
		}
	}
	if m.From == "" {
		return fmt.Errorf("config: BINGLE_SMTP_HOST needs BINGLE_TLS_EMAIL, the address the mail is sent from")
	}
	file := os.Getenv("BINGLE_SMTP_PASSWORD_FILE")
	if file == "" {
		return fmt.Errorf("config: BINGLE_SMTP_HOST needs BINGLE_SMTP_PASSWORD_FILE (the password is never read from the environment)")
	}
	pw, err := readSecretFile(file)
	if err != nil {
		return fmt.Errorf("config: BINGLE_SMTP_PASSWORD_FILE: %w", err)
	}
	m.Password = NewSecret(pw)
	cfg.Mail = m
	return nil
}

// readSecretFile reads a one-line secret from a file that only its owner may
// read, and strips the trailing newline.
func readSecretFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s is readable by group or others (mode %04o): run chmod 600 on it", path, st.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimRight(string(b), "\r\n")
	if v == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return v, nil
}

// parseDays is time.ParseDuration with days as well: "90d", or "36h".
func parseDays(v string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(v, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days < 0 {
			return 0, fmt.Errorf("not a number of days: %q", v)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(v)
}

// SeedPasswordDeterministic reports whether the first administrator may get
// the well-known development password: under BINGLE_DEV, or on a loopback
// bind — and never behind a trusted proxy, where every request looks local.
func (c Config) SeedPasswordDeterministic() bool {
	return !c.TrustProxy && (c.Dev || c.LoopbackBind())
}

// SeedAdminEmail is the first administrator's address: BINGLE_ADMIN_EMAIL, or
// the handle at the public URL's host, or at localhost.
func (c Config) SeedAdminEmail() string {
	if c.AdminEmail != "" {
		return c.AdminEmail
	}
	handle := c.AdminHandle
	if handle == "" {
		handle = "admin"
	}
	host := "localhost.localdomain"
	if u, err := url.Parse(c.PublicURL); err == nil && u.Hostname() != "" && strings.Contains(u.Hostname(), ".") {
		host = u.Hostname()
	}
	return handle + "@" + host
}

// OIDCProvider is one OpenID Connect provider people may sign in with.
type OIDCProvider struct {
	// ID is the provider's name in the URLs (/auth/oidc/<id>/…) and in the
	// variables' names, lower case.
	ID string
	// Issuer is the provider's issuer URL, whose discovery document says
	// the rest.
	Issuer       string
	ClientID     string
	ClientSecret Secret
	// Scopes are asked for; "openid email profile" by default.
	Scopes []string
	// Label is the button's: "Sign in with <Label>".
	Label string
}

var oidcIDRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,23}$`)

// loadOIDC reads BINGLE_OIDC_PROVIDERS ("google,corp") and, for each,
// BINGLE_OIDC_<ID>_ISSUER, _CLIENT_ID, _CLIENT_SECRET (or
// _CLIENT_SECRET_FILE), _SCOPES and _LABEL. A provider needs a public URL:
// it sends people back to it.
func loadOIDC(publicURL string) ([]OIDCProvider, error) {
	list := strings.TrimSpace(os.Getenv("BINGLE_OIDC_PROVIDERS"))
	if list == "" {
		return nil, nil
	}
	if publicURL == "" {
		return nil, fmt.Errorf("config: BINGLE_OIDC_PROVIDERS needs BINGLE_PUBLIC_URL: a provider sends people back to it, at /auth/oidc/<id>/callback")
	}
	var out []OIDCProvider
	seen := map[string]bool{}
	for _, raw := range strings.Split(list, ",") {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if !oidcIDRe.MatchString(id) || seen[id] {
			return nil, fmt.Errorf("config: BINGLE_OIDC_PROVIDERS: %q is not a provider name (a–z and digits, a letter first) or is there twice", raw)
		}
		seen[id] = true
		env := "BINGLE_OIDC_" + strings.ToUpper(id) + "_"
		p := OIDCProvider{ID: id, Issuer: strings.TrimSpace(os.Getenv(env + "ISSUER")), ClientID: strings.TrimSpace(os.Getenv(env + "CLIENT_ID")),
			Label: strings.TrimSpace(os.Getenv(env + "LABEL")), Scopes: strings.Fields(os.Getenv(env + "SCOPES"))}
		secret := os.Getenv(env + "CLIENT_SECRET")
		if file := os.Getenv(env + "CLIENT_SECRET_FILE"); file != "" {
			if secret != "" {
				return nil, fmt.Errorf("config: set %sCLIENT_SECRET or %sCLIENT_SECRET_FILE, not both", env, env)
			}
			s, err := readSecretFile(file)
			if err != nil {
				return nil, fmt.Errorf("config: %sCLIENT_SECRET_FILE: %w", env, err)
			}
			secret = s
		}
		p.ClientSecret = NewSecret(secret)
		if u, err := url.Parse(p.Issuer); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("config: %sISSUER must be the provider's issuer URL, got %q", env, p.Issuer)
		}
		if p.ClientID == "" || !p.ClientSecret.Set() {
			return nil, fmt.Errorf("config: %sCLIENT_ID and %sCLIENT_SECRET are needed", env, env)
		}
		if len(p.Scopes) == 0 {
			p.Scopes = []string{"openid", "email", "profile"}
		}
		if p.Label == "" {
			p.Label = strings.ToUpper(id[:1]) + id[1:]
		}
		out = append(out, p)
	}
	return out, nil
}
