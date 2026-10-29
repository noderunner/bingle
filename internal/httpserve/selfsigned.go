package httpserve

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A certificate bingle makes for itself.
//
// BINGLE_TLS_SELFSIGNED exists so that "the connection must be secure" does
// not make development and CI painful: a laptop, a container on a private
// network, a machine reached over a tunnel. It is **generated once and
// cached**, because a certificate that changes on every restart is one every
// browser warns about again every restart, and nobody would leave it on.
//
// It is not a substitute for a real certificate. A browser will not trust it,
// and HSTS is deliberately not sent with it (see withHSTS): HSTS plus an
// untrusted certificate is a page nobody can click through to.

// selfSignedLife is how long a generated certificate lasts. Long enough not
// to be a chore, short enough that an abandoned one expires.
const selfSignedLife = 2 * 365 * 24 * time.Hour

// selfSignedRenewWithin regenerates a cached certificate that is about to
// expire, so a long-lived server does not wake up one morning unable to serve.
const selfSignedRenewWithin = 30 * 24 * time.Hour

// SelfSigned is a generated certificate and the files it lives in.
type SelfSigned struct {
	CertFile string
	KeyFile  string
	// Hosts are the names and addresses it is valid for.
	Hosts []string
	// Fingerprint is the SHA-256 of the certificate, in the colon-separated
	// form a browser shows, so an operator can check what they are trusting.
	Fingerprint string
	// Generated says whether this run made it, as opposed to reusing the
	// cached one.
	Generated bool
	// NotAfter is when it expires.
	NotAfter time.Time
}

// EnsureSelfSigned returns a certificate for host, generating and caching one
// in dir if there is not already a usable one there.
//
// host is the name the server is reached by, which may be empty for a bind
// like ":8443"; localhost, 127.0.0.1 and ::1 are always included, because the
// reason to want this is usually to reach the server from the machine it is on.
func EnsureSelfSigned(dir, host string) (SelfSigned, error) {
	if dir == "" {
		return SelfSigned{}, fmt.Errorf("httpserve: a self-signed certificate needs a cache directory")
	}
	hosts := selfSignedHosts(host)
	out := SelfSigned{
		CertFile: filepath.Join(dir, "selfsigned.crt"),
		KeyFile:  filepath.Join(dir, "selfsigned.key"),
		Hosts:    hosts,
	}

	// Reuse what is there, if it still covers what is wanted and is not about
	// to expire. Anything wrong with it is a reason to replace it, not to
	// fail: the cache is bingle's own and holds nothing a person chose.
	if cert, err := x509.ParseCertificate(cachedDER(out.CertFile)); err == nil {
		if usableSelfSigned(cert, hosts) {
			out.Fingerprint = fingerprint(cert.Raw)
			out.NotAfter = cert.NotAfter
			return out, nil
		}
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return SelfSigned{}, fmt.Errorf("httpserve: making the certificate cache %s: %w", dir, err)
	}
	der, keyPEM, err := generateSelfSigned(hosts)
	if err != nil {
		return SelfSigned{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(out.CertFile, certPEM, 0o600); err != nil {
		return SelfSigned{}, fmt.Errorf("httpserve: writing %s: %w", out.CertFile, err)
	}
	// 0600: it is a private key, and the directory is bingle's own.
	if err := os.WriteFile(out.KeyFile, keyPEM, 0o600); err != nil {
		return SelfSigned{}, fmt.Errorf("httpserve: writing %s: %w", out.KeyFile, err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return SelfSigned{}, fmt.Errorf("httpserve: the certificate just written will not parse: %w", err)
	}
	out.Fingerprint = fingerprint(der)
	out.NotAfter = cert.NotAfter
	out.Generated = true
	return out, nil
}

// selfSignedHosts is what the certificate is valid for: the configured name
// plus the local ones, without duplicates.
func selfSignedHosts(host string) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	host = strings.TrimSpace(host)
	// A bind like ":8443" or "0.0.0.0:8443" names no host.
	if host != "" && host != "0.0.0.0" && host != "::" {
		seen := false
		for _, h := range hosts {
			if strings.EqualFold(h, host) {
				seen = true
			}
		}
		if !seen {
			hosts = append([]string{host}, hosts...)
		}
	}
	return hosts
}

// usableSelfSigned reports whether a cached certificate still covers hosts and
// is not about to expire.
func usableSelfSigned(cert *x509.Certificate, hosts []string) bool {
	if time.Now().Add(selfSignedRenewWithin).After(cert.NotAfter) {
		return false
	}
	for _, h := range hosts {
		if err := cert.VerifyHostname(h); err != nil {
			return false
		}
	}
	return true
}

// cachedDER reads a PEM certificate file and returns its DER bytes, or nil.
func cachedDER(path string) []byte {
	raw, err := os.ReadFile(path) //nolint:gosec // bingle's own cache directory
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}
	return block.Bytes
}

// generateSelfSigned makes a P-256 certificate for hosts.
func generateSelfSigned(hosts []string) (der []byte, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("httpserve: generating a key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("httpserve: generating a serial number: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hosts[0], Organization: []string{"bingle (self-signed)"}},
		NotBefore:             time.Now().Add(-time.Hour), // a little slack for a wrong clock
		NotAfter:              time.Now().Add(selfSignedLife),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("httpserve: creating the certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("httpserve: encoding the key: %w", err)
	}
	return der, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

// fingerprint is the SHA-256 of a certificate in the form a browser shows it,
// so what is printed at startup can be compared with what the browser warns
// about.
func fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	hexed := strings.ToUpper(hex.EncodeToString(sum[:]))
	var b strings.Builder
	for i := 0; i < len(hexed); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hexed[i : i+2])
	}
	return b.String()
}
