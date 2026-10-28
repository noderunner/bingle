// Tests of the logger itself and of the redaction helpers: the level and the
// format do what they say, and nothing a helper returns carries the thing it
// was given.

package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/logging"
)

func TestNew_LevelKeepsDebugLinesOutUntilAsked(t *testing.T) {
	var buf bytes.Buffer
	lg := logging.New(&buf, slog.LevelInfo, logging.FormatText)
	lg.Debug("a quiet line")
	lg.Info("a loud line")
	if got := buf.String(); strings.Contains(got, "quiet") {
		t.Errorf("at info the log holds the debug line: %q", got)
	} else if !strings.Contains(got, "loud") {
		t.Errorf("at info the log is missing the info line: %q", got)
	}

	buf.Reset()
	logging.New(&buf, slog.LevelDebug, logging.FormatText).Debug("a quiet line")
	if !strings.Contains(buf.String(), "quiet") {
		t.Errorf("at debug the log is missing the debug line: %q", buf.String())
	}
}

func TestNew_JSONFormatIsOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelInfo, logging.FormatJSON).Info("hello", "games", 2)
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("the json format did not write an object: %v (%q)", err, buf.String())
	}
	if line["msg"] != "hello" || line["games"] != float64(2) {
		t.Errorf("the object = %v", line)
	}
}

// A room code is the only thing between a stranger and a game, so a log line
// may carry two characters of it and a hash, and never the code.
func TestRoom_KeepsTwoCharactersAndHidesTheRest(t *testing.T) {
	const code = "AB12CD"
	ref := logging.RoomRef(code)
	if strings.Contains(ref, code) {
		t.Fatalf("RoomRef(%q) = %q, which carries the whole code", code, ref)
	}
	if !strings.HasPrefix(ref, "AB-") {
		t.Errorf("RoomRef(%q) = %q, want it to start with the first two characters", code, ref)
	}
	if rest := strings.TrimPrefix(ref, "AB-"); len(rest) != 12 {
		t.Errorf("RoomRef(%q) = %q, want twelve hex characters after the prefix", code, ref)
	}
	// Two lines about one room have to match, whatever case the code reached
	// each of them in, and two rooms must not.
	if logging.RoomRef("ab12cd") != ref {
		t.Errorf("RoomRef is case sensitive: %q vs %q", logging.RoomRef("ab12cd"), ref)
	}
	if logging.RoomRef("AB12CE") == ref {
		t.Error("two different room codes give the same reference")
	}
	if logging.RoomRef("") != "" {
		t.Errorf("RoomRef(\"\") = %q, want empty", logging.RoomRef(""))
	}
	if a := logging.Room(code); a.Key != "room" || a.Value.String() != ref {
		t.Errorf("Room(%q) = %v, want the room key and %q", code, a, ref)
	}
}

func TestAddrAndName_AreOnlyEverHashed(t *testing.T) {
	const address = "Player@example.com"
	a := logging.Addr(address)
	if a.Key != "addr" {
		t.Errorf("Addr key = %q, want addr", a.Key)
	}
	if v := a.Value.String(); strings.Contains(strings.ToLower(v), "player") || strings.Contains(v, "example.com") {
		t.Fatalf("Addr(%q) = %q, which carries the address", address, v)
	}
	// The same address in another case is the same person.
	if logging.Addr("player@example.com").Value.String() != a.Value.String() {
		t.Error("Addr is case sensitive, so the same address gives two references")
	}

	n := logging.Name("Ana")
	if n.Key != "player" {
		t.Errorf("Name key = %q, want player", n.Key)
	}
	if v := n.Value.String(); strings.Contains(strings.ToLower(v), "ana") {
		t.Fatalf("Name(%q) = %q, which carries the name", "Ana", v)
	}
	if logging.Name("Bo").Value.String() == n.Value.String() {
		t.Error("two different names give the same reference")
	}
}
