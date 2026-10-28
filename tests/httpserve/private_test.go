// Tests of the private listener: where it binds, and the token it can be
// asked for when it is not on the loopback interface.

package httpserve_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/httpserve"
)

// startPrivate binds a private listener on a port the kernel picks, serves it
// until the test ends, and returns its base URL. Nothing here guesses a port:
// ListenPrivate binds before it serves and says what it got, which is the
// whole reason it is shaped that way.
func startPrivate(t *testing.T, token string, h http.Handler) string {
	t.Helper()
	priv, err := httpserve.ListenPrivate("127.0.0.1:0", token, h, nil)
	if err != nil {
		t.Fatalf("ListenPrivate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- priv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("the private listener returned %v, want nil", err)
			}
		case <-time.After(shutdownBudget):
			t.Errorf("the private listener did not stop within %s", shutdownBudget)
		}
	})
	return "http://" + priv.Addr()
}

// get returns the status and body of a GET, with an optional bearer token.
func get(t *testing.T, url, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestListenPrivate_SaysWhichPortItGot(t *testing.T) {
	priv, err := httpserve.ListenPrivate("127.0.0.1:0", "", okHandler, nil)
	if err != nil {
		t.Fatalf("ListenPrivate: %v", err)
	}
	t.Cleanup(func() { _ = priv.Close() })

	host, port, err := net.SplitHostPort(priv.Addr())
	if err != nil {
		t.Fatalf("Addr() = %q: %v", priv.Addr(), err)
	}
	if host != "127.0.0.1" {
		t.Errorf("bound %s, want the loopback interface", host)
	}
	if port == "0" || port == "" {
		t.Errorf("Addr() = %q, want the port it actually got", priv.Addr())
	}
}

// Nothing else on the machine can reach a listener on 127.0.0.1, which is how
// the metrics stay off the public site by default.
func TestListenPrivate_WithoutATokenAnswersOnLoopback(t *testing.T) {
	base := startPrivate(t, "", okHandler)
	if status, body := get(t, base+"/", ""); status != http.StatusOK || body != "ok" {
		t.Errorf("GET / = %d %q, want 200 ok", status, body)
	}
}

func TestRequireToken_RefusesWithoutTheRightToken(t *testing.T) {
	const token = "a-token-long-enough-to-be-worth-having"
	base := startPrivate(t, token, okHandler)

	for _, c := range []struct {
		name, offered string
		want          int
	}{
		{name: "no token at all", offered: "", want: http.StatusUnauthorized},
		{name: "the wrong token", offered: "not-the-token-but-the-same-length-ish", want: http.StatusForbidden},
		{name: "a prefix of it", offered: token[:10], want: http.StatusForbidden},
		{name: "the token", offered: token, want: http.StatusOK},
	} {
		t.Run(c.name, func(t *testing.T) {
			if status, _ := get(t, base+"/", c.offered); status != c.want {
				t.Errorf("status = %d, want %d", status, c.want)
			}
		})
	}
}

// A 401 has to say how to authenticate, or a scraper cannot be configured
// from the error alone.
func TestRequireToken_SaysWhatItWants(t *testing.T) {
	base := startPrivate(t, "a-token-long-enough-to-be-worth-having", okHandler)
	req, err := http.NewRequest(http.MethodGet, base+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("WWW-Authenticate"); got == "" {
		t.Error("a 401 with no WWW-Authenticate header")
	}
}

// A malformed Authorization header is a missing one, not a crash.
func TestRequireToken_HandlesAStrangeHeader(t *testing.T) {
	base := startPrivate(t, "a-token-long-enough-to-be-worth-having", okHandler)
	for _, header := range []string{"Bearer", "Bearer ", "Basic abc", "bearer", "", "Bearer  "} {
		req, err := http.NewRequest(http.MethodGet, base+"/", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("GET with %q: %v", header, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Authorization: %q was accepted", header)
		}
	}
}
