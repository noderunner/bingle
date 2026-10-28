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
	defaultAddr   = ":8080"
	defaultDBPath = "./bingle.db"

	// With automatic certificates bingle takes the standard ports: the
	// certificate authority validates over 80 and 443.
	defaultTLSAddr  = ":443"
	defaultHTTPAddr = ":80"

	// defaultLogFormat and the format it is checked against are the formats
	// internal/logging builds a handler for.
	defaultLogFormat = "text"
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

// TLSEnabled reports whether bingle serves HTTPS, by automatic certificates
// or by a certificate and key from files.
func (c Config) TLSEnabled() bool {
	return len(c.TLSDomains) > 0 || (c.TLSCert != "" && c.TLSKey != "")
}

// Load reads configuration from environment variables, falling back to
// defaults when unset.
func Load() (Config, error) {
	cfg := Config{
		Addr:      defaultAddr,
		DBPath:    defaultDBPath,
		LogFormat: defaultLogFormat,
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
		cfg.TLSCacheDir = filepath.Join(filepath.Dir(cfg.DBPath), "certs")
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

	if v := os.Getenv("BINGLE_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("config: BINGLE_RETENTION_DAYS must be a non-negative integer, got %q", v)
		}
		cfg.RetentionDays = n
	}

	return cfg, nil
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
