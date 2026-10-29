// Tests of what the metrics database does with a browser's faults: one row per
// distinct fault, counted, and dropped once nobody has hit it for a while.

package journal_test

import (
	"context"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/journal"
)

// A bad release is one row with four hundred hits, not four hundred rows.
func TestClientErrors_AreOneRowPerFault(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	const (
		message = "Cannot read properties of null (reading 'board')"
		stack   = "at draw (/static/js/game.js:120:7)"
		version = "0.16.0"
	)
	hash := journal.ClientErrorHash(message, stack, version)
	report := func(page string) int64 {
		t.Helper()
		n, err := db.RecordClientError(ctx, journal.ClientError{
			Hash: hash, Message: message, Stack: stack, Version: version,
			Page: page, Browser: "Firefox 142 desktop", LastSeen: time.Now(),
		})
		if err != nil {
			t.Fatalf("recording: %v", err)
		}
		return n
	}

	if got := report("/game/{code}"); got != 1 {
		t.Errorf("the first report counted %d, want 1", got)
	}
	if got := report("/game/{code}"); got != 2 {
		t.Errorf("the second report counted %d, want 2", got)
	}
	for i := 0; i < 398; i++ {
		report("/game/{code}")
	}

	rows, err := db.ClientErrors(ctx, 10)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("four hundred reports made %d rows, want 1", len(rows))
	}
	if rows[0].Count != 400 {
		t.Errorf("the row counts %d, want 400", rows[0].Count)
	}
	if rows[0].Message != message || rows[0].Page != "/game/{code}" {
		t.Errorf("the row = %+v", rows[0])
	}
	if rows[0].FirstSeen.After(rows[0].LastSeen) {
		t.Errorf("first seen %v is after last seen %v", rows[0].FirstSeen, rows[0].LastSeen)
	}
	if got := counts(t, db)["client_errors"]; got != 1 {
		t.Errorf("the table holds %d rows", got)
	}
}

// The release is in the key, so the same message on a new release is a new
// fault — which is what makes "this started with 0.17" answerable.
func TestClientErrors_TheVersionIsPartOfTheFault(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	const message = "x is not a function"
	for _, version := range []string{"0.16.0", "0.17.0"} {
		hash := journal.ClientErrorHash(message, "", version)
		if _, err := db.RecordClientError(ctx, journal.ClientError{
			Hash: hash, Message: message, Version: version, Page: "/", LastSeen: time.Now(),
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	rows, err := db.ClientErrors(ctx, 10)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("the same message on two releases made %d rows, want 2", len(rows))
	}
}

// Newest first: after a release that is the order that matters.
func TestClientErrors_ComeBackNewestFirst(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	now := time.Now()

	for i, age := range []time.Duration{3 * time.Hour, time.Hour, 2 * time.Hour} {
		message := string(rune('a' + i))
		if _, err := db.RecordClientError(ctx, journal.ClientError{
			Hash:     journal.ClientErrorHash(message, "", "0.16.0"),
			Message:  message,
			Version:  "0.16.0",
			Page:     "/",
			LastSeen: now.Add(-age),
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	rows, err := db.ClientErrors(ctx, 10)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	if rows[0].Message != "b" || rows[2].Message != "a" {
		t.Errorf("the order is %q, %q, %q", rows[0].Message, rows[1].Message, rows[2].Message)
	}
}

// A fault nobody has hit for a fortnight has been fixed or has stopped
// mattering.
func TestClientErrors_OldOnesArePurged(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

	for i, age := range []time.Duration{30 * 24 * time.Hour, time.Hour} {
		message := string(rune('a' + i))
		if _, err := db.RecordClientError(ctx, journal.ClientError{
			Hash: journal.ClientErrorHash(message, "", "0.16.0"), Message: message,
			Version: "0.16.0", Page: "/", LastSeen: now.Add(-age),
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	n, err := db.PurgeClientErrors(ctx, now.Add(-14*24*time.Hour))
	if err != nil {
		t.Fatalf("purging: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d rows, want 1", n)
	}
	rows, err := db.ClientErrors(ctx, 10)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(rows) != 1 || rows[0].Message != "b" {
		t.Errorf("the wrong row survived: %+v", rows)
	}
}
