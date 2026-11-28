package store_test

// Announcements (ADR 0155): who is offered which, and what seeing one does.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/store"
)

func waiting(t *testing.T, s interface {
	AnnouncementsWaiting(context.Context, string) ([]int64, error)
}, id string) []int64 {
	t.Helper()
	got, err := s.AnnouncementsWaiting(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Nobody is offered what was announced before they existed; somebody who
// was there is offered it until they close it, and then never again.
func TestAnnouncements_OnlyThoseMadeSinceAPrincipalWas(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	before, err := s.CreateAnnouncement(ctx, "Before", "Old news", nil, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGuest(ctx, "g1", "hash-1"); err != nil {
		t.Fatal(err)
	}
	after1, _ := s.CreateAnnouncement(ctx, "After", "News", nil, now.Add(time.Hour))
	after2, _ := s.CreateAnnouncement(ctx, "Later", "More news", nil, now.Add(2*time.Hour))
	if got := waiting(t, s, "g1"); !slices.Equal(got, []int64{after1.ID, after2.ID}) {
		t.Fatalf("waiting: %v, want %d and %d (not %d, from before the guest)", got, after1.ID, after2.ID, before.ID)
	}
	// Closing the card marks all it showed; an id that is no announcement is passed over.
	if err := s.MarkAnnouncementsSeen(ctx, "g1", []int64{after1.ID, after2.ID, 999}, now); err != nil {
		t.Fatal(err)
	}
	if got := waiting(t, s, "g1"); len(got) != 0 {
		t.Errorf("after closing: %v still waiting", got)
	}
	if err := s.MarkAnnouncementsSeen(ctx, "g1", []int64{after1.ID}, now); err != nil {
		t.Errorf("marking one seen again: %v", err)
	}
	// Everything is still listed for looking back, oldest first.
	all, err := s.Announcements(ctx, false)
	if err != nil || len(all) != 3 || all[0].ID != before.ID || all[2].ID != after2.ID {
		t.Errorf("listed: %+v %v", all, err)
	}
}

// A withdrawn announcement is offered and listed no more.
func TestAnnouncements_AWithdrawnOneIsNotOffered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateGuest(ctx, "g1", "hash-1"); err != nil {
		t.Fatal(err)
	}
	a, _ := s.CreateAnnouncement(ctx, "Soon gone", "Never mind", nil, time.Now().Add(time.Hour))
	if got := waiting(t, s, "g1"); len(got) != 1 {
		t.Fatalf("waiting: %v", got)
	}
	if err := s.WithdrawAnnouncement(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if got := waiting(t, s, "g1"); len(got) != 0 {
		t.Errorf("a withdrawn one is waiting: %v", got)
	}
	if list, _ := s.Announcements(ctx, false); len(list) != 0 {
		t.Errorf("a withdrawn one is listed: %+v", list)
	}
	if list, _ := s.Announcements(ctx, true); len(list) != 1 || !list[0].Withdrawn {
		t.Errorf("the admin's list: %+v", list)
	}
	if err := s.WithdrawAnnouncement(ctx, 999); err == nil {
		t.Error("withdrawing no announcement succeeded")
	}
}

// A guest's seen marks go where the guest goes.
func TestAnnouncements_SeenMarksFollowAMergedGuest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"phone", "laptop"} {
		if err := s.CreateGuest(ctx, id, "hash-"+id); err != nil {
			t.Fatal(err)
		}
	}
	a1, _ := s.CreateAnnouncement(ctx, "One", "1", nil, time.Now().Add(time.Hour))
	a2, _ := s.CreateAnnouncement(ctx, "Two", "2", nil, time.Now().Add(2*time.Hour))
	_ = s.MarkAnnouncementsSeen(ctx, "phone", []int64{a1.ID}, time.Now())
	_ = s.MarkAnnouncementsSeen(ctx, "laptop", []int64{a1.ID, a2.ID}, time.Now())
	if err := s.MergePrincipal(ctx, "laptop", "phone"); err != nil {
		t.Fatal(err)
	}
	if got := waiting(t, s, "phone"); len(got) != 0 {
		t.Errorf("after the merge the phone still has %v waiting", got)
	}
}

// Deleting an announcement takes its pictures and who has seen it with it.
func TestAnnouncements_PicturesGoWithTheirAnnouncement(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateGuest(ctx, "g1", "hash-g1"); err != nil {
		t.Fatal(err)
	}
	pics := []store.Picture{{Name: "a.png", ContentType: "image/png", Data: []byte("a")}, {Name: "b.jpg", ContentType: "image/jpeg", Data: []byte("b")}}
	a, err := s.CreateAnnouncement(ctx, "Pictures", "![a](a.png)", pics, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	kept, _ := s.CreateAnnouncement(ctx, "Kept", "![a](a.png)", pics[:1], time.Now().Add(time.Hour))
	if got, err := s.AnnouncementByID(ctx, a.ID); err != nil || len(got.Pictures) != 2 || got.Pictures[0] != "a.png" {
		t.Fatalf("by id: %+v %v", got, err)
	}
	if p, err := s.AnnouncementPicture(ctx, a.ID, "b.jpg"); err != nil || string(p.Data) != "b" || p.ContentType != "image/jpeg" {
		t.Fatalf("b.jpg: %+v %v", p, err)
	}
	_ = s.MarkAnnouncementsSeen(ctx, "g1", []int64{a.ID}, time.Now())
	if err := s.DeleteAnnouncement(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAnnouncement(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting it again: %v, want ErrNotFound", err)
	}
	var pictures, seen int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM announcement_pictures WHERE announcement_id = ?`, a.ID).Scan(&pictures)
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM announcements_seen WHERE announcement_id = ?`, a.ID).Scan(&seen)
	if pictures != 0 || seen != 0 {
		t.Errorf("left behind: %d pictures, %d seen marks", pictures, seen)
	}
	if _, err := s.AnnouncementPicture(ctx, kept.ID, "a.png"); err != nil {
		t.Errorf("another announcement's picture went too: %v", err)
	}
}
