package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("BINGLE_ADDR", "")
	t.Setenv("BINGLE_DB_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Addr != defaultAddr {
		t.Errorf("Addr = %q, want %q", cfg.Addr, defaultAddr)
	}
	if cfg.DBPath != defaultDBPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, defaultDBPath)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("BINGLE_ADDR", ":9090")
	t.Setenv("BINGLE_DB_PATH", "/tmp/custom.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/custom.db")
	}
}

func TestLoad_PublicURL(t *testing.T) {
	for _, c := range []struct {
		env, want string
		bad       bool
	}{
		{"", "", false},
		{"https://games.example.com", "https://games.example.com", false},
		{"https://games.example.com/", "https://games.example.com", false},
		{"http://203.0.113.7:8080/bingle/", "http://203.0.113.7:8080/bingle", false},
		{"games.example.com", "", true},
		{"ftp://games.example.com", "", true},
		{"https://games.example.com/?a=1", "", true},
	} {
		t.Setenv("BINGLE_PUBLIC_URL", c.env)
		cfg, err := Load()
		if c.bad {
			if err == nil {
				t.Errorf("BINGLE_PUBLIC_URL=%q: want an error", c.env)
			}
			continue
		}
		if err != nil || cfg.PublicURL != c.want {
			t.Errorf("BINGLE_PUBLIC_URL=%q: got %q, %v; want %q", c.env, cfg.PublicURL, err, c.want)
		}
	}
}

func clearTLSEnv(t *testing.T) {
	for _, k := range []string{"BINGLE_ADDR", "BINGLE_DB_PATH", "BINGLE_TLS_CERT", "BINGLE_TLS_KEY", "BINGLE_TLS_DOMAIN", "BINGLE_TLS_EMAIL",
		"BINGLE_TLS_CACHE", "BINGLE_HTTP_ADDR", "BINGLE_TLS_ACME_URL", "BINGLE_TLS_HSTS_SECONDS", "BINGLE_PUBLIC_URL"} {
		t.Setenv(k, "")
	}
}

func TestLoad_TLSOffByDefault(t *testing.T) {
	clearTLSEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSEnabled() || len(cfg.TLSDomains) != 0 || cfg.HTTPAddr != "" || cfg.Addr != defaultAddr || cfg.PublicURL != "" {
		t.Errorf("TLS settings leak into the default config: %+v", cfg)
	}
}

func TestLoad_TLSDomain(t *testing.T) {
	clearTLSEnv(t)
	t.Setenv("BINGLE_TLS_DOMAIN", " Games.Example.com , play.example.com,games.example.com ")
	t.Setenv("BINGLE_DB_PATH", "/data/bingle.db")
	t.Setenv("BINGLE_TLS_EMAIL", "Me <me@example.com>")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.TLSDomains; len(got) != 2 || got[0] != "games.example.com" || got[1] != "play.example.com" {
		t.Errorf("domains = %v", got)
	}
	if !cfg.TLSEnabled() || cfg.Addr != ":443" || cfg.HTTPAddr != ":80" || cfg.TLSCacheDir != "/data/certs" || cfg.TLSEmail != "me@example.com" {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.PublicURL != "https://games.example.com" {
		t.Errorf("public URL = %q, want it to default to the first domain", cfg.PublicURL)
	}

	// Explicit settings win.
	t.Setenv("BINGLE_ADDR", ":8443")
	t.Setenv("BINGLE_HTTP_ADDR", ":8081")
	t.Setenv("BINGLE_TLS_CACHE", "/var/certs")
	t.Setenv("BINGLE_PUBLIC_URL", "https://games.example.com:8443")
	t.Setenv("BINGLE_TLS_ACME_URL", "https://acme-staging-v02.api.letsencrypt.org/directory")
	t.Setenv("BINGLE_TLS_HSTS_SECONDS", "3600")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8443" || cfg.HTTPAddr != ":8081" || cfg.TLSCacheDir != "/var/certs" || cfg.PublicURL != "https://games.example.com:8443" ||
		cfg.ACMEURL == "" || cfg.HSTSSeconds != 3600 {
		t.Errorf("config = %+v", cfg)
	}
}

func TestLoad_TLSDomainErrors(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"with certificate files": {"BINGLE_TLS_DOMAIN": "games.example.com", "BINGLE_TLS_CERT": "/c.pem", "BINGLE_TLS_KEY": "/k.pem"},
		"an IP address":          {"BINGLE_TLS_DOMAIN": "203.0.113.7"},
		"a single label":         {"BINGLE_TLS_DOMAIN": "localhost"},
		"a URL":                  {"BINGLE_TLS_DOMAIN": "https://games.example.com"},
		"a port":                 {"BINGLE_TLS_DOMAIN": "games.example.com:443"},
		"a wildcard":             {"BINGLE_TLS_DOMAIN": "*.example.com"},
		"a bad label":            {"BINGLE_TLS_DOMAIN": "-games.example.com"},
		"the same address twice": {"BINGLE_TLS_DOMAIN": "games.example.com", "BINGLE_ADDR": ":80"},
		"a bad email":            {"BINGLE_TLS_DOMAIN": "games.example.com", "BINGLE_TLS_EMAIL": "nope"},
		"a bad ACME URL":         {"BINGLE_TLS_DOMAIN": "games.example.com", "BINGLE_TLS_ACME_URL": "letsencrypt"},
		"a bad HSTS":             {"BINGLE_TLS_HSTS_SECONDS": "-1"},
	} {
		clearTLSEnv(t)
		for k, v := range env {
			t.Setenv(k, v)
		}
		if _, err := Load(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestLoad_CertFilesStillWork(t *testing.T) {
	clearTLSEnv(t)
	t.Setenv("BINGLE_TLS_CERT", "/c.pem")
	t.Setenv("BINGLE_TLS_KEY", "/k.pem")
	cfg, err := Load()
	if err != nil || !cfg.TLSEnabled() || len(cfg.TLSDomains) != 0 || cfg.Addr != defaultAddr {
		t.Errorf("cert files: %+v, %v", cfg, err)
	}
	t.Setenv("BINGLE_TLS_KEY", "")
	cfg, _ = Load()
	if cfg.TLSEnabled() {
		t.Error("a certificate without a key turned TLS on")
	}
}

func TestLoad_RTC(t *testing.T) {
	for _, k := range []string{"BINGLE_RTC_STUN", "BINGLE_RTC_TURN_URLS", "BINGLE_RTC_TURN_SECRET"} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil || len(cfg.RTCStun) != 1 || cfg.RTCStun[0] != DefaultRTCStun || len(cfg.RTCTurnURLs) != 0 {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}
	t.Setenv("BINGLE_RTC_STUN", "none")
	if cfg, err = Load(); err != nil || len(cfg.RTCStun) != 0 {
		t.Errorf("none: %+v, %v", cfg, err)
	}
	t.Setenv("BINGLE_RTC_STUN", "stun:a.example:3478, stun:b.example:3478")
	t.Setenv("BINGLE_RTC_TURN_URLS", "turn:t.example:3478,turns:t.example:5349")
	t.Setenv("BINGLE_RTC_TURN_SECRET", "x")
	if cfg, err = Load(); err != nil || len(cfg.RTCStun) != 2 || len(cfg.RTCTurnURLs) != 2 || cfg.RTCTurnSecret != "x" {
		t.Errorf("set: %+v, %v", cfg, err)
	}
	for name, env := range map[string]map[string]string{
		"a STUN url with the wrong scheme": {"BINGLE_RTC_STUN": "http://x"},
		"a TURN url without a scheme":      {"BINGLE_RTC_TURN_URLS": "t.example:3478", "BINGLE_RTC_TURN_SECRET": "x"},
		"TURN without a secret":            {"BINGLE_RTC_TURN_URLS": "turn:t.example:3478"},
		"a secret without TURN":            {"BINGLE_RTC_TURN_SECRET": "x"},
	} {
		for _, k := range []string{"BINGLE_RTC_STUN", "BINGLE_RTC_TURN_URLS", "BINGLE_RTC_TURN_SECRET"} {
			t.Setenv(k, env[k])
		}
		if _, err := Load(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestLoad_Logging(t *testing.T) {
	t.Setenv("BINGLE_LOG_LEVEL", "")
	t.Setenv("BINGLE_LOG_FORMAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != defaultLogFormat {
		t.Errorf("by default LogLevel = %v and LogFormat = %q, want info and %q", cfg.LogLevel, cfg.LogFormat, defaultLogFormat)
	}

	for _, c := range []struct {
		env  string
		want slog.Level
		bad  bool
	}{
		{env: "debug", want: slog.LevelDebug},
		{env: "DEBUG", want: slog.LevelDebug},
		{env: "info", want: slog.LevelInfo},
		{env: "warn", want: slog.LevelWarn},
		{env: "error", want: slog.LevelError},
		{env: "verbose", bad: true},
		{env: "7", bad: true},
	} {
		t.Setenv("BINGLE_LOG_LEVEL", c.env)
		cfg, err := Load()
		switch {
		case c.bad && err == nil:
			t.Errorf("BINGLE_LOG_LEVEL=%q was accepted", c.env)
		case !c.bad && err != nil:
			t.Errorf("BINGLE_LOG_LEVEL=%q: %v", c.env, err)
		case !c.bad && cfg.LogLevel != c.want:
			t.Errorf("BINGLE_LOG_LEVEL=%q gave %v, want %v", c.env, cfg.LogLevel, c.want)
		}
	}
	t.Setenv("BINGLE_LOG_LEVEL", "")

	for _, c := range []struct {
		env, want string
		bad       bool
	}{
		{env: "json", want: "json"},
		{env: "JSON", want: "json"},
		{env: "text", want: "text"},
		{env: "logfmt", bad: true},
	} {
		t.Setenv("BINGLE_LOG_FORMAT", c.env)
		cfg, err := Load()
		switch {
		case c.bad && err == nil:
			t.Errorf("BINGLE_LOG_FORMAT=%q was accepted", c.env)
		case !c.bad && err != nil:
			t.Errorf("BINGLE_LOG_FORMAT=%q: %v", c.env, err)
		case !c.bad && cfg.LogFormat != c.want:
			t.Errorf("BINGLE_LOG_FORMAT=%q gave %q, want %q", c.env, cfg.LogFormat, c.want)
		}
	}
}

func TestLoad_Metrics(t *testing.T) {
	t.Setenv("BINGLE_METRICS_ADDR", "")
	t.Setenv("BINGLE_METRICS_TOKEN", "")

	// Unset is not the same as empty here — empty turns the listener off —
	// so this really has to remove it. t.Setenv above registered the cleanup
	// that puts it back.
	if err := os.Unsetenv("BINGLE_METRICS_ADDR"); err != nil {
		t.Fatalf("unsetting BINGLE_METRICS_ADDR: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.MetricsAddr != defaultMetricsAddr {
		t.Errorf("MetricsAddr = %q, want %q", cfg.MetricsAddr, defaultMetricsAddr)
	}
	if cfg.MetricsToken.Set() {
		t.Error("a token was invented from nowhere")
	}

	const goodToken = "a-token-long-enough-to-be-worth-having"
	for _, c := range []struct {
		name, addr, token string
		bad               bool
	}{
		{name: "off", addr: ""},
		{name: "loopback, no token", addr: "127.0.0.1:9095"},
		{name: "loopback by name", addr: "localhost:9095"},
		{name: "IPv6 loopback", addr: "[::1]:9095"},
		{name: "loopback with a token", addr: "127.0.0.1:9095", token: goodToken},
		{name: "every interface needs a token", addr: ":9095", bad: true},
		{name: "a routable address needs a token", addr: "10.0.0.5:9095", bad: true},
		{name: "0.0.0.0 needs a token", addr: "0.0.0.0:9095", bad: true},
		{name: "every interface with a token", addr: ":9095", token: goodToken},
		{name: "a routable address with a token", addr: "10.0.0.5:9095", token: goodToken},
		{name: "not host:port", addr: "9095", bad: true},
		{name: "no port", addr: "127.0.0.1:", bad: true},
		{name: "a token that is too short", addr: "127.0.0.1:9095", token: "short", bad: true},
		{name: "a token with no listener", addr: "", token: goodToken, bad: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("BINGLE_METRICS_ADDR", c.addr)
			t.Setenv("BINGLE_METRICS_TOKEN", c.token)
			cfg, err := Load()
			switch {
			case c.bad && err == nil:
				t.Errorf("addr %q token %q was accepted", c.addr, c.token)
			case !c.bad && err != nil:
				t.Errorf("addr %q token %q: %v", c.addr, c.token, err)
			case !c.bad && cfg.MetricsAddr != c.addr:
				t.Errorf("MetricsAddr = %q, want %q", cfg.MetricsAddr, c.addr)
			}
		})
	}
}

// The token is a Secret, so no way of printing the configuration can show it.
func TestLoad_MetricsTokenIsNeverPrinted(t *testing.T) {
	const token = "a-token-long-enough-to-be-worth-having"
	t.Setenv("BINGLE_METRICS_ADDR", "127.0.0.1:9095")
	t.Setenv("BINGLE_METRICS_TOKEN", token)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.MetricsToken.Reveal() != token {
		t.Fatalf("the token did not survive loading")
	}
	for _, printed := range []string{
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
		fmt.Sprint(cfg.MetricsToken),
		cfg.MetricsToken.LogValue().String(),
	} {
		if strings.Contains(printed, token) {
			t.Errorf("the token was printed: %s", printed)
		}
	}
}

func TestLoad_Journal(t *testing.T) {
	for _, v := range []string{"BINGLE_METRICS_DB", "BINGLE_SLOW_REQUEST"} {
		t.Setenv(v, "")
		if err := os.Unsetenv(v); err != nil {
			t.Fatalf("unsetting %s: %v", v, err)
		}
	}
	t.Setenv("BINGLE_DB_PATH", "/var/lib/bingle/bingle.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	// The metrics database sits beside the game database, so a backup of the
	// directory takes both.
	if want := "/var/lib/bingle/" + defaultMetricsDBName; cfg.MetricsDB != want {
		t.Errorf("MetricsDB = %q, want %q", cfg.MetricsDB, want)
	}
	if cfg.SlowRequest != defaultSlowRequest {
		t.Errorf("SlowRequest = %v, want %v", cfg.SlowRequest, defaultSlowRequest)
	}

	// Empty keeps no history; the journal then only logs.
	t.Setenv("BINGLE_METRICS_DB", "")
	if cfg, err = Load(); err != nil || cfg.MetricsDB != "" {
		t.Errorf("an empty BINGLE_METRICS_DB gave %q, %v", cfg.MetricsDB, err)
	}

	for _, c := range []struct {
		env  string
		want time.Duration
		bad  bool
	}{
		{env: "500ms", want: 500 * time.Millisecond},
		{env: "2s", want: 2 * time.Second},
		{env: "0", want: 0}, // off
		{env: "soon", bad: true},
		{env: "-1s", bad: true},
		{env: "250", bad: true}, // a duration needs a unit
	} {
		t.Setenv("BINGLE_SLOW_REQUEST", c.env)
		cfg, err := Load()
		switch {
		case c.bad && err == nil:
			t.Errorf("BINGLE_SLOW_REQUEST=%q was accepted", c.env)
		case !c.bad && err != nil:
			t.Errorf("BINGLE_SLOW_REQUEST=%q: %v", c.env, err)
		case !c.bad && cfg.SlowRequest != c.want:
			t.Errorf("BINGLE_SLOW_REQUEST=%q gave %v, want %v", c.env, cfg.SlowRequest, c.want)
		}
	}
}

func TestLoad_MetricsHistory(t *testing.T) {
	for _, v := range []string{"BINGLE_METRICS_SAMPLE", "BINGLE_METRICS_RETAIN_DAYS"} {
		t.Setenv(v, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.MetricsSample != defaultMetricsSample || cfg.MetricsRetainDays != defaultMetricsRetainDays {
		t.Errorf("by default the history is every %v for %d days, want %v and %d",
			cfg.MetricsSample, cfg.MetricsRetainDays, defaultMetricsSample, defaultMetricsRetainDays)
	}

	for _, c := range []struct {
		env  string
		want time.Duration
		bad  bool
	}{
		{env: "5m", want: 5 * time.Minute},
		{env: "1h", want: time.Hour},
		{env: "30s", want: 30 * time.Second},
		{env: "0", want: 0},    // no history
		{env: "1s", bad: true}, // too often to be history
		{env: "often", bad: true},
		{env: "-5m", bad: true},
	} {
		t.Setenv("BINGLE_METRICS_SAMPLE", c.env)
		cfg, err := Load()
		switch {
		case c.bad && err == nil:
			t.Errorf("BINGLE_METRICS_SAMPLE=%q was accepted", c.env)
		case !c.bad && err != nil:
			t.Errorf("BINGLE_METRICS_SAMPLE=%q: %v", c.env, err)
		case !c.bad && cfg.MetricsSample != c.want:
			t.Errorf("BINGLE_METRICS_SAMPLE=%q gave %v, want %v", c.env, cfg.MetricsSample, c.want)
		}
	}
	t.Setenv("BINGLE_METRICS_SAMPLE", "")

	for _, c := range []struct {
		env  string
		want int
		bad  bool
	}{
		{env: "14", want: 14},
		{env: "0", want: 0}, // everything at full resolution
		{env: "-1", bad: true},
		{env: "a fortnight", bad: true},
	} {
		t.Setenv("BINGLE_METRICS_RETAIN_DAYS", c.env)
		cfg, err := Load()
		switch {
		case c.bad && err == nil:
			t.Errorf("BINGLE_METRICS_RETAIN_DAYS=%q was accepted", c.env)
		case !c.bad && err != nil:
			t.Errorf("BINGLE_METRICS_RETAIN_DAYS=%q: %v", c.env, err)
		case !c.bad && cfg.MetricsRetainDays != c.want:
			t.Errorf("BINGLE_METRICS_RETAIN_DAYS=%q gave %d, want %d", c.env, cfg.MetricsRetainDays, c.want)
		}
	}
}

func TestLoad_ClientErrors(t *testing.T) {
	t.Setenv("BINGLE_CLIENT_ERRORS", "")

	// On by default: the UI is where there was no visibility at all.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !cfg.ClientErrors {
		t.Error("the reporting endpoint is off by default")
	}

	for _, c := range []struct {
		env  string
		want bool
		bad  bool
	}{
		{env: "off", want: false},
		{env: "0", want: false},
		{env: "false", want: false},
		{env: "on", want: true},
		{env: "1", want: true},
		{env: "sometimes", bad: true},
	} {
		t.Setenv("BINGLE_CLIENT_ERRORS", c.env)
		cfg, err := Load()
		switch {
		case c.bad && err == nil:
			t.Errorf("BINGLE_CLIENT_ERRORS=%q was accepted", c.env)
		case !c.bad && err != nil:
			t.Errorf("BINGLE_CLIENT_ERRORS=%q: %v", c.env, err)
		case !c.bad && cfg.ClientErrors != c.want:
			t.Errorf("BINGLE_CLIENT_ERRORS=%q gave %v, want %v", c.env, cfg.ClientErrors, c.want)
		}
	}
}

func TestLoad_Debug(t *testing.T) {
	for _, v := range []string{"BINGLE_DEBUG_GAMES", "BINGLE_DEBUG_ROOM", "BINGLE_DEBUG_SECRETS", "BINGLE_PARANOID", "BINGLE_DEBUG_DIR"} {
		t.Setenv(v, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(cfg.Debug.Games) != 0 || cfg.Debug.Room != "" || cfg.Debug.Secrets || cfg.Debug.Paranoid || cfg.Debug.Dir != "" {
		t.Errorf("nothing is set and Debug = %+v", cfg.Debug)
	}

	t.Setenv("BINGLE_DEBUG_GAMES", " Courtfall , nyet ,, ")
	t.Setenv("BINGLE_DEBUG_ROOM", "ab12cd")
	if cfg, err = Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if strings.Join(cfg.Debug.Games, ",") != "courtfall,nyet" {
		t.Errorf("Games = %v, want the names lower-cased and trimmed", cfg.Debug.Games)
	}
	if cfg.Debug.Room != "AB12CD" {
		t.Errorf("Room = %q, want it upper-cased", cfg.Debug.Room)
	}

	t.Setenv("BINGLE_DEBUG_DIR", "/var/lib/bingle/debug")
	if cfg, err = Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.Debug.Dir != "/var/lib/bingle/debug" {
		t.Errorf("Dir = %q", cfg.Debug.Dir)
	}
	t.Setenv("BINGLE_DEBUG_DIR", "")
	if cfg, err = Load(); err != nil || cfg.Debug.Dir != "" {
		t.Errorf("an empty BINGLE_DEBUG_DIR gave %q, %v", cfg.Debug.Dir, err)
	}

	t.Setenv("BINGLE_DEBUG_ROOM", "not a room code")
	if _, err := Load(); err == nil {
		t.Error("BINGLE_DEBUG_ROOM was accepted with a space in it")
	}
	t.Setenv("BINGLE_DEBUG_ROOM", "")

	// The two development-only switches are refused rather than ignored when
	// they say something ambiguous, because switching one on by accident is
	// what their documentation warns about.
	for _, name := range []string{"BINGLE_DEBUG_SECRETS", "BINGLE_PARANOID"} {
		for _, on := range []string{"1", "on", "true", "yes"} {
			t.Setenv(name, on)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("%s=%q: %v", name, on, err)
			}
			got := cfg.Debug.Secrets
			if name == "BINGLE_PARANOID" {
				got = cfg.Debug.Paranoid
			}
			if !got {
				t.Errorf("%s=%q did not turn it on", name, on)
			}
		}
		t.Setenv(name, "maybe")
		if _, err := Load(); err == nil {
			t.Errorf("%s=maybe was accepted", name)
		}
		t.Setenv(name, "")
	}
}
