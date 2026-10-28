// Tests of the two things that made this package flaky in CI: the ports the
// tests bind, and how long a graceful shutdown is allowed to take.

package httpserve_test

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/httpserve"
)

// freeAddr's whole purpose is to hand out a port the kernel will not hand to
// somebody else between the check and Serve's bind, so: every port is inside
// the window, outside the ephemeral range, and no two calls agree.
func TestFreeAddr_PortsAreOutsideTheEphemeralRangeAndDoNotRepeat(t *testing.T) {
	const want = 300
	seen := make(map[int]bool, want)
	for i := 0; i < want; i++ {
		addr := freeAddr(t)
		_, portText, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatalf("freeAddr returned %q: %v", addr, err)
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			t.Fatalf("freeAddr returned %q: %v", addr, err)
		}
		if port < portWindowBase || port >= portWindowBase+portWindowSize {
			t.Fatalf("freeAddr returned port %d, outside the window %d-%d",
				port, portWindowBase, portWindowBase+portWindowSize-1)
		}
		if seen[port] {
			t.Fatalf("freeAddr returned port %d twice in %d calls", port, want)
		}
		seen[port] = true
	}
}

// And the window really is below what this kernel uses for outgoing
// connections. If someone has tuned the range down, this says so plainly
// rather than leaving the package to flake now and then.
func TestFreeAddr_TheWindowIsBelowThisMachinesEphemeralRange(t *testing.T) {
	const path = "/proc/sys/net/ipv4/ip_local_port_range"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no %s on this system: %v", path, err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		t.Skipf("%s says %q, which is not a range", path, raw)
	}
	low, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Skipf("%s says %q: %v", path, raw, err)
	}
	if top := portWindowBase + portWindowSize - 1; top >= low {
		t.Fatalf("the test port window %d-%d reaches into this machine's ephemeral range (%s), "+
			"so the kernel can take a port a test is about to bind: lower portWindowBase or the window size",
			portWindowBase, top, strings.TrimSpace(string(raw)))
	}
}

// A connection that is accepted and then says nothing — which is what the
// certificate warm-up and the refusal test leave behind — must not stop the
// server shutting down. net/http needs five seconds to decide such a
// connection can be abandoned, so this is really a test that the budget the
// helpers allow clears that.
func TestServe_ShutsDownWithASilentConnectionInFlight(t *testing.T) {
	cfg := acmeConfig(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- httpserve.Serve(ctx, cfg, okHandler) }()

	waitForHTTP(t, noRedirect(), "http://"+cfg.HTTPAddr+"/")

	// Connected, and never a ClientHello.
	raw, err := net.Dial("tcp", cfg.Addr)
	if err != nil {
		t.Fatalf("dialling the TLS port: %v", err)
	}
	defer func() { _ = raw.Close() }()

	start := time.Now()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil", err)
		}
		t.Logf("shutdown took %s of the %s allowed", time.Since(start).Round(time.Millisecond), shutdownBudget)
	case <-time.After(shutdownBudget):
		t.Fatalf("Serve did not return within %s with a silent connection open", shutdownBudget)
	}
}
