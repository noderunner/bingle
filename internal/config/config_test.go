package config

import (
	"log/slog"
	"testing"
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
