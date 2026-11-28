package store_test

// Announcements (ADR 0155): who is offered which, and what seeing one does.

import (
	"context"
	"slices"
	"testing"
	"time"
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
	before, err := s.CreateAnnouncement(ctx, "Before", "Old news", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGuest(ctx, "g1", "hash-1"); err != nil {
		t.Fatal(err)
	}
	after1, _ := s.CreateAnnouncement(ctx, "After", "News", now.Add(time.Hour))
	after2, _ := s.CreateAnnouncement(ctx, "Later", "More news", now.Add(2*time.Hour))
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
	a, _ := s.CreateAnnouncement(ctx, "Soon gone", "Never mind", time.Now().Add(time.Hour))
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
	a1, _ := s.CreateAnnouncement(ctx, "One", "1", time.Now().Add(time.Hour))
	a2, _ := s.CreateAnnouncement(ctx, "Two", "2", time.Now().Add(2*time.Hour))
	_ = s.MarkAnnouncementsSeen(ctx, "phone", []int64{a1.ID}, time.Now())
	_ = s.MarkAnnouncementsSeen(ctx, "laptop", []int64{a1.ID, a2.ID}, time.Now())
	if err := s.MergePrincipal(ctx, "laptop", "phone"); err != nil {
		t.Fatal(err)
	}
	if got := waiting(t, s, "phone"); len(got) != 0 {
		t.Errorf("after the merge the phone still has %v waiting", got)
	}
}
