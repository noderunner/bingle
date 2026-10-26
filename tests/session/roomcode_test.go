// Tests of room codes: their length and alphabet, with no ambiguous characters.

package session_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/session/roomcode"
)

const wantLen = 6

// codeSampleSize is the number of codes drawn for the length/alphabet
// membership checks (AC 1 & 2 in the task brief).
const codeSampleSize = 1000

// TestNew_AlwaysReturnsSixCharacters calls New() many times and checks that
// every single result is exactly 6 characters long (bytes; the alphabet is
// pure ASCII so byte length == rune length here).
func TestNew_AlwaysReturnsSixCharacters(t *testing.T) {
	for i := 0; i < codeSampleSize; i++ {
		code := roomcode.New()
		if len(code) != wantLen {
			t.Fatalf("call %d: New() = %q, len=%d, want len=%d", i, code, len(code), wantLen)
		}
	}
}

// TestNew_OnlyUsesAlphabetCharacters checks that every character produced
// across many calls is a member of roomcode.Alphabet — never anything
// outside that set.
func TestNew_OnlyUsesAlphabetCharacters(t *testing.T) {
	for i := 0; i < codeSampleSize; i++ {
		code := roomcode.New()
		for _, r := range code {
			if !strings.ContainsRune(roomcode.Alphabet, r) {
				t.Fatalf("call %d: New() = %q contains character %q not in Alphabet %q", i, code, r, roomcode.Alphabet)
			}
		}
	}
}

// TestAlphabet_NoDuplicateCharacters ensures the alphabet constant itself
// contains each character exactly once. A duplicate would silently skew the
// distribution of generated codes toward the repeated character.
func TestAlphabet_NoDuplicateCharacters(t *testing.T) {
	seen := make(map[rune]int, len(roomcode.Alphabet))
	for _, r := range roomcode.Alphabet {
		seen[r]++
	}
	for r, count := range seen {
		if count > 1 {
			t.Errorf("Alphabet %q contains %q %d times, want exactly once", roomcode.Alphabet, r, count)
		}
	}
}

// TestAlphabet_ExcludesAmbiguousCharacters ensures the easily-confused
// characters (0/O, 1/I/L) are explicitly absent, in both cases where a case
// distinction is meaningful (the alphabet is documented as digits +
// uppercase letters, so lowercase variants are checked too in case a future
// edit accidentally widens the character set).
func TestAlphabet_ExcludesAmbiguousCharacters(t *testing.T) {
	excluded := []rune{'0', 'O', 'o', '1', 'I', 'i', 'L', 'l'}
	for _, r := range excluded {
		if strings.ContainsRune(roomcode.Alphabet, r) {
			t.Errorf("Alphabet %q unexpectedly contains ambiguous character %q", roomcode.Alphabet, r)
		}
	}
}

// TestAlphabet_IsExactlyDigitsAndUppercaseMinusAmbiguous pins the alphabet
// down to precisely digits + uppercase letters with 0, O, 1, I, L removed —
// not just "excludes some ambiguous chars" but "is exactly this set, no
// more, no less" (guards against e.g. accidentally including punctuation or
// lowercase letters).
func TestAlphabet_IsExactlyDigitsAndUppercaseMinusAmbiguous(t *testing.T) {
	ambiguous := map[rune]bool{'0': true, 'O': true, '1': true, 'I': true, 'L': true}
	var want strings.Builder
	for r := '0'; r <= '9'; r++ {
		if !ambiguous[r] {
			want.WriteRune(r)
		}
	}
	for r := 'A'; r <= 'Z'; r++ {
		if !ambiguous[r] {
			want.WriteRune(r)
		}
	}

	if roomcode.Alphabet != want.String() {
		t.Errorf("Alphabet = %q, want %q", roomcode.Alphabet, want.String())
	}
}

// TestNew_StatisticalUniquenessAmongManyDraws is a statistical sanity check,
// not a cryptographic proof: with a 32-character alphabet and 6 positions
// there are 32^6 = 1,073,741,824 possible codes. Drawing 5000 codes and
// expecting zero exact duplicates is justified by the birthday bound —
// P(any collision) ≈ 1 - exp(-n^2 / (2*N)) with n=1000, N≈1.07e9 gives
// P ≈ 1 - exp(-1e6 / 2.15e9) ≈ 0.0005, i.e. roughly a 0.05% chance of ANY
// collision at all (not per-pair — across the whole sample) if New() draws
// uniformly at random. (n=5000 would be about 1.2%, which CI did hit.)
// Asserting zero duplicates is therefore the correct expectation at this
// sample size, not an arbitrary tolerance, and a real bug (e.g. a broken
// RNG seed, or an alphabet index collapsing outcomes) would produce
// duplicates far more often than this.
func TestNew_StatisticalUniquenessAmongManyDraws(t *testing.T) {
	const n = 1000
	seen := make(map[string]int, n)
	for i := 0; i < n; i++ {
		code := roomcode.New()
		seen[code]++
	}

	dupes := 0
	for code, count := range seen {
		if count > 1 {
			dupes++
			t.Logf("duplicate code %q seen %d times", code, count)
		}
	}
	if dupes != 0 {
		t.Errorf("got %d duplicate code(s) among %d draws, want 0 (see test comment for why 0 is expected at this sample size)", dupes, n)
	}
}

// TestNew_ConsecutiveCallsDiffer is a trivial non-determinism smoke check,
// distinct from the statistical test above: two back-to-back calls should
// not produce the exact same code (this would only spuriously fail if New()
// somehow always returned a constant value, which the alphabet/length tests
// above wouldn't otherwise catch).
func TestNew_ConsecutiveCallsDiffer(t *testing.T) {
	a := roomcode.New()
	b := roomcode.New()
	if a == b {
		t.Errorf("two consecutive New() calls both returned %q; want them to differ", a)
	}
}
