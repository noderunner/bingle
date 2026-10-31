package qr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// The worked example every QR tutorial uses: "HELLO WORLD" at 1-M.
func TestReedSolomonMatchesTheStandardsExample(t *testing.T) {
	data := []byte{32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17}
	want := []byte{196, 35, 39, 119, 235, 215, 231, 226, 93, 23}
	if got := remainder(data, generator(10)); string(got) != string(want) {
		t.Errorf("error correction = %v, want %v", got, want)
	}
}

// Format and version information, against the standard's tables.
func TestFormatAndVersionBits(t *testing.T) {
	if got := formatBits(0); got != 0b101010000010010 {
		t.Errorf("format bits for M, mask 0 = %015b", got)
	}
	if got := formatBits(7); got != 0b100101010100000 {
		t.Errorf("format bits for M, mask 7 = %015b", got)
	}
	c := newCode(7)
	bits := 0
	for i := 0; i < 18; i++ {
		if c.dark[i/3][c.Size-11+i%3] {
			bits |= 1 << i
		}
	}
	if bits != 0x07C94 {
		t.Errorf("version 7 information = %#x, want 0x7c94", bits)
	}
}

// Every version leaves exactly the standard's number of modules for data,
// and the timing patterns run unbroken between the finders.
func TestFunctionPatterns(t *testing.T) {
	for v := 1; v <= 10; v++ {
		c := newCode(v)
		free := 0
		for y := range c.fn {
			for x := range c.fn[y] {
				if !c.fn[y][x] {
					free++
				}
			}
		}
		if free != rawModules(v) {
			t.Errorf("version %d: %d modules for data, want %d", v, free, rawModules(v))
		}
		for i := 8; i < c.Size-8; i++ {
			if c.dark[6][i] != (i%2 == 0) || c.dark[i][6] != (i%2 == 0) {
				t.Errorf("version %d: the timing patterns are broken at %d", v, i)
			}
		}
	}
}

func TestEncodeChoosesTheSmallestVersion(t *testing.T) {
	for _, tc := range []struct{ n, version int }{{1, 1}, {14, 1}, {15, 2}, {106, 6}, {107, 7}, {180, 9}, {181, 10}, {MaxBytes, 10}} {
		c, err := Encode([]byte(strings.Repeat("a", tc.n)))
		if err != nil || c.Version != tc.version || c.Size != 4*tc.version+17 {
			t.Errorf("%d bytes: version %v, %v; want %d", tc.n, c, err, tc.version)
		}
	}
	if _, err := Encode([]byte(strings.Repeat("a", MaxBytes+1))); !errors.Is(err, ErrTooLong) {
		t.Errorf("%d bytes: %v", MaxBytes+1, err)
	}
}

// A setup link drawn by this package and read back by OpenCV's decoder when
// it was written. The fingerprint only changes if the drawing does: if it
// must, check the new symbol with a real reader before updating it.
func TestASetupLinkIsDrawnAsBefore(t *testing.T) {
	const link = "otpauth://totp/Bingle%3A%40miles_admin?algorithm=SHA1&digits=6&issuer=Bingle&period=30&secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	c, err := Encode([]byte(link))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if c.Dark(x, y) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	if got := hex.EncodeToString(sum[:8]); got != golden {
		t.Errorf("version %d mask %d drew %s, want %s", c.Version, c.Mask, got, golden)
	}
}

const golden = "3f5f14df0f3502b6"

func TestSVG(t *testing.T) {
	c, _ := Encode([]byte("x"))
	s := c.SVG()
	if !strings.HasPrefix(s, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 29 29"`) || !strings.HasSuffix(s, "</svg>") || strings.Contains(s, "<script") {
		t.Errorf("svg = %.120s", s)
	}
}
