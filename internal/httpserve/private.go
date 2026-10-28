package httpserve

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Private is bingle's second listener: the one that serves /metrics, and from
// later stages of docs/history/observability-plan.md the health page and the
// journal endpoints. It is separate from the public listener because the
// public site has no authentication of any kind, and none of this may be on it.
//
// Unlike Serve, it binds when it is built rather than when it starts serving,
// so a caller can ask what address it actually got. That is what lets a test
// pass "127.0.0.1:0" and be told the port instead of guessing one and racing
// the kernel for it (see the freeAddr comment in tests/httpserve).
type Private struct {
	ln  net.Listener
	srv *http.Server
	log *slog.Logger
}

// ListenPrivate binds addr and returns a listener ready to Serve. The handler
// is wrapped in RequireToken when token is not empty.
func ListenPrivate(addr, token string, handler http.Handler, lg *slog.Logger) (*Private, error) {
	if lg == nil {
		lg = slog.Default()
	}
	if token != "" {
		handler = RequireToken(handler, token)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("httpserve: the metrics listener could not bind %s: %w", addr, err)
	}
	return &Private{
		ln: ln,
		srv: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 20 * time.Second,
		},
		log: lg,
	}, nil
}

// Addr is the address the listener actually bound, port and all.
func (p *Private) Addr() string { return p.ln.Addr().String() }

// Serve answers on the bound listener until ctx is cancelled.
func (p *Private) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := p.srv.Serve(p.ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("httpserve: the metrics listener stopped: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		if err := p.srv.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("httpserve: shutting the metrics listener down: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

// Close releases the listener without having served, for a caller that binds
// and then fails before it gets as far as Serve.
func (p *Private) Close() error { return p.ln.Close() }

// RequireToken refuses every request that does not carry token as a bearer
// token, so the private listener can be put somewhere a network reaches it.
//
// The comparison is constant time over a hash of each side, so neither the
// length nor the content of the real token leaks through how long an answer
// takes.
func RequireToken(next http.Handler, token string) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offered, ok := bearer(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="bingle metrics"`)
			http.Error(w, "a bearer token is required", http.StatusUnauthorized)
			return
		}
		got := sha256.Sum256([]byte(offered))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			http.Error(w, "that is not the token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearer pulls the token out of an Authorization header.
func bearer(r *http.Request) (string, bool) {
	const prefix = "bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}
