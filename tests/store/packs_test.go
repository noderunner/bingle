package store_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// An installed pack is kept whole, one row a version: a version already
// installed is refused rather than replaced, and removing one leaves the others.
func TestPacks_SaveListDelete(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "packs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	v1 := store.StoredPack{Name: "lines", Version: "1.0.0", Title: "Lines", Games: []string{"pack-lines-1.0.0", "pack-lines-1.0.0-big"},
		SHA256: "aa", Zip: []byte("PK one"), InstalledBy: 7}
	v2 := store.StoredPack{Name: "lines", Version: "1.1.0", Title: "Lines", Games: []string{"pack-lines-1.1.0"}, SHA256: "bb", Zip: []byte("PK two")}
	for _, p := range []store.StoredPack{v1, v2} {
		if err := s.SavePack(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	again := v1
	again.Zip, again.SHA256 = []byte("PK other"), "cc"
	if err := s.SavePack(ctx, again); !errors.Is(err, store.ErrPackInstalled) {
		t.Fatalf("installing 1.0.0 twice: %v, want ErrPackInstalled", err)
	}

	got, err := s.Packs(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("Packs: %+v, %v", got, err)
	}
	if g := got[0]; g.Version != "1.0.0" || !bytes.Equal(g.Zip, v1.Zip) || g.SHA256 != "aa" || g.InstalledBy != 7 ||
		len(g.Games) != 2 || g.Games[1] != "pack-lines-1.0.0-big" || g.Title != "Lines" || g.InstalledAt == "" {
		t.Errorf("1.0.0 came back as %+v", g)
	}
	if g := got[1]; g.Version != "1.1.0" || g.InstalledBy != 0 {
		t.Errorf("1.1.0 came back as %+v", g)
	}

	if err := s.DeletePack(ctx, "lines", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePack(ctx, "lines", "1.0.0"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting it twice: %v, want ErrNotFound", err)
	}
	if got, _ := s.Packs(ctx); len(got) != 1 || got[0].Version != "1.1.0" {
		t.Errorf("after deleting 1.0.0: %+v", got)
	}
}
