// Package fakesmtp is a small SMTP server for tests: it speaks just enough of
// the protocol (EHLO, STARTTLS or implicit TLS, AUTH PLAIN and LOGIN, MAIL,
// RCPT, DATA) to receive what the mailer sends, with a self-signed certificate
// the test hands to the mailer as its CA file.
package fakesmtp

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Mail is one received message.
type Mail struct {
	From, Data    string
	To            []string
	User, Pass    string
	AuthedInClear bool // AUTH was accepted before TLS
}

// Options change how the server behaves.
type Options struct {
	Implicit   bool   // implicit TLS (like port 465)
	NoSTARTTLS bool   // do not offer STARTTLS
	AuthFail   bool   // answer every AUTH with 535
	RcptReply  string // when set, the reply to RCPT TO, such as "550 5.1.1 no such user"
	LoginOnly  bool   // offer AUTH LOGIN but not PLAIN
}

// Server is a running fake SMTP server.
type Server struct {
	Addr    string // host:port
	Host    string
	Port    int
	CAFile  string // PEM of the certificate, for BINGLE_SMTP_CA_FILE
	opts    Options
	ln      net.Listener
	tlsCfg  *tls.Config
	mu      sync.Mutex
	mails   []Mail
	started int // connections accepted
	auths   int // AUTH commands received
}

// Start runs a server until the test ends.
func Start(t *testing.T, opts Options) *Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "fake smtp"},
		NotBefore:    time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:        true, BasicConstraintsValid: true,
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	s := &Server{
		Addr: ln.Addr().String(), Host: "127.0.0.1", Port: port, CAFile: caFile, opts: opts, ln: ln,
		tlsCfg: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12},
	}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

// Mails returns what has been received so far.
func (s *Server) Mails() []Mail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Mail(nil), s.mails...)
}

// AuthAttempts is how many AUTH commands arrived (a client that refuses to
// send in the clear sends none).
func (s *Server) AuthAttempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.auths
}

// Connections is how many connections were accepted.
func (s *Server) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

func (s *Server) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.started++
		s.mu.Unlock()
		go s.handle(c)
	}
}

func (s *Server) handle(raw net.Conn) {
	defer func() { _ = raw.Close() }()
	_ = raw.SetDeadline(time.Now().Add(20 * time.Second))
	conn := raw
	secure := false
	if s.opts.Implicit {
		conn = tls.Server(raw, s.tlsCfg)
		secure = true
	}
	r := bufio.NewReader(conn)
	say := func(f string, a ...any) { _, _ = fmt.Fprintf(conn, f+"\r\n", a...) }
	say("220 fake.smtp ESMTP")
	var cur Mail
	authed := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		if strings.HasPrefix(cmd, "AUTH ") {
			s.mu.Lock()
			s.auths++
			s.mu.Unlock()
		}
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250-fake.smtp")
			if !secure && !s.opts.NoSTARTTLS && !s.opts.Implicit {
				say("250-STARTTLS")
			}
			if s.opts.LoginOnly {
				say("250-AUTH LOGIN")
			} else {
				say("250-AUTH PLAIN LOGIN")
			}
			say("250 8BITMIME")
		case cmd == "STARTTLS":
			if s.opts.NoSTARTTLS {
				say("502 not offered")
				continue
			}
			say("220 go ahead")
			tc := tls.Server(raw, s.tlsCfg)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, secure = tc, true
			r = bufio.NewReader(tc)
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			parts := strings.Fields(line)
			if len(parts) < 3 {
				say("334 ")
				l, _ := r.ReadString('\n')
				parts = append(parts, strings.TrimSpace(l))
			}
			raw, _ := base64.StdEncoding.DecodeString(parts[2])
			f := strings.Split(string(raw), "\x00")
			if s.opts.AuthFail || len(f) != 3 {
				say("535 5.7.8 authentication failed")
				continue
			}
			cur.User, cur.Pass, cur.AuthedInClear, authed = f[1], f[2], !secure, true
			say("235 ok")
		case cmd == "AUTH LOGIN":
			say("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
			u, _ := r.ReadString('\n')
			say("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
			p, _ := r.ReadString('\n')
			ub, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(u))
			pb, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(p))
			if s.opts.AuthFail {
				say("535 5.7.8 authentication failed")
				continue
			}
			cur.User, cur.Pass, cur.AuthedInClear, authed = string(ub), string(pb), !secure, true
			say("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			if !authed {
				say("530 authentication required")
				continue
			}
			cur.From = addressOf(line[len("MAIL FROM:"):])
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if s.opts.RcptReply != "" {
				say("%s", s.opts.RcptReply)
				continue
			}
			cur.To = append(cur.To, addressOf(line[len("RCPT TO:"):]))
			say("250 ok")
		case cmd == "DATA":
			say("354 end with .")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				b.WriteString(strings.TrimPrefix(l, "."))
			}
			cur.Data = b.String()
			s.mu.Lock()
			s.mails = append(s.mails, cur)
			s.mu.Unlock()
			cur = Mail{User: cur.User, Pass: cur.Pass, AuthedInClear: cur.AuthedInClear}
			say("250 queued")
		case cmd == "RSET":
			say("250 ok")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("502 unknown")
		}
	}
}

// addressOf takes the address out of "<a@b.c> BODY=8BITMIME".
func addressOf(arg string) string {
	arg = strings.TrimSpace(arg)
	if i := strings.Index(arg, ">"); i >= 0 {
		arg = arg[:i]
	}
	return strings.TrimPrefix(arg, "<")
}
