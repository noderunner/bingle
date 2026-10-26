package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/config"
)

// ErrNoSTARTTLS is returned when the server does not offer STARTTLS: nothing,
// least of all the password, is sent over an unencrypted connection.
var ErrNoSTARTTLS = errors.New("mail: the SMTP server does not offer STARTTLS, refusing to send in the clear")

// Sender delivers messages through an SMTP server with authentication.
type Sender struct {
	host     string
	port     int
	user     string
	password config.Secret
	from     string
	fromName string
	implicit bool // implicit TLS instead of STARTTLS
	roots    *x509.CertPool
	timeout  time.Duration
	now      func() time.Time
}

// NewSender builds the Sender of the mail settings.
func NewSender(m config.Mail) (*Sender, error) {
	s := &Sender{
		host: m.Host, port: m.Port, user: m.User, password: m.Password, from: m.From,
		fromName: "Bingle", implicit: m.Mode == "tls", timeout: 45 * time.Second, now: time.Now,
	}
	if m.CAFile != "" {
		pem, err := os.ReadFile(m.CAFile)
		if err != nil {
			return nil, fmt.Errorf("mail: reading BINGLE_SMTP_CA_FILE: %w", err)
		}
		s.roots = x509.NewCertPool()
		if !s.roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("mail: BINGLE_SMTP_CA_FILE has no certificate")
		}
	}
	return s, nil
}

// From is the sender address.
func (s *Sender) From() string { return s.from }

// Send delivers one message. The error never contains the password.
func (s *Sender) Send(ctx context.Context, msg Message) error {
	data, err := msg.Build(s.from, s.fromName, s.now())
	if err != nil {
		return err
	}
	to, _ := ValidAddress(msg.To)

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(s.host, strconv.Itoa(s.port)))
	if err != nil {
		return fmt.Errorf("mail: connecting to the SMTP server: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	tlsCfg := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12, RootCAs: s.roots}
	if s.implicit {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("mail: TLS handshake: %w", err)
		}
		conn = tc
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("mail: SMTP greeting: %w", err)
	}
	defer func() { _ = c.Close() }()
	if !s.implicit {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return ErrNoSTARTTLS
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mail: STARTTLS: %w", err)
		}
	}
	// Only now, inside TLS, is the password sent.
	ok, params := c.Extension("AUTH")
	if !ok {
		return errors.New("mail: the SMTP server offers no authentication")
	}
	var auth smtp.Auth
	switch {
	case strings.Contains(strings.ToUpper(params), "PLAIN"):
		auth = smtp.PlainAuth("", s.user, s.password.Reveal(), s.host)
	case strings.Contains(strings.ToUpper(params), "LOGIN"):
		auth = loginAuth{user: s.user, password: s.password.Reveal()}
	default:
		return fmt.Errorf("mail: no supported authentication (server offers %q)", params)
	}
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("mail: authentication: %w", err)
	}
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("mail: writing the message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: sending the message: %w", err)
	}
	_ = c.Quit()
	return nil
}

// loginAuth is the AUTH LOGIN mechanism, for servers that offer no PLAIN.
type loginAuth struct{ user, password string }

func (a loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }

func (a loginAuth) Next(from []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(from))) {
	case "username:":
		return []byte(a.user), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, errors.New("mail: unexpected AUTH LOGIN prompt")
}

// Failure is what the sender should do about an error.
type Failure int

const (
	// Transient: a problem of this message or moment (a refused recipient, a timeout).
	Transient Failure = iota
	// BadCredentials: the server rejected the login. Retrying risks a lockout.
	BadCredentials
	// RateLimited: the server asks to slow down or the account hit a quota.
	RateLimited
)

// Classify says what kind of failure err is.
func Classify(err error) Failure {
	var pe *textproto.Error
	if !errors.As(err, &pe) {
		return Transient
	}
	switch {
	case pe.Code == 535 || pe.Code == 534 || pe.Code == 530 || pe.Code == 538:
		return BadCredentials
	case pe.Code == 421 || pe.Code == 450 || pe.Code == 451 || pe.Code == 452 || strings.Contains(pe.Msg, "5.4.5") || strings.Contains(pe.Msg, "4.7.0"):
		return RateLimited
	}
	return Transient
}
