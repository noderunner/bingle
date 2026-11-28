package app_test

// Major announcements (ADR 0155): only administrators write them; each guest
// or account that was there before one was published is offered it until it
// closes it; the mark goes where the guest goes; a withdrawn one is gone.

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
)

// announcementServer is a server with its administrator and ana, and the
// administrator's browser, signed in.
func announcementServer(t *testing.T) (*app.App, string, *browser) {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0" // the seeded administrator needs no second factor here
	a, base := serve(t, cfg)
	admin := newBrowser(t, base)
	admin.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	admin.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)
	given, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	ana := newBrowser(t, base)
	ana.login("ana", given)
	if code := ana.do("POST", "/auth/password", map[string]any{"old": given, "new": anaPassword}, nil); code != 200 {
		t.Fatalf("ana's password: %d", code)
	}
	return a, base, admin
}

// announce publishes an announcement as the administrator, a few seconds
// ahead so that every guest and account made so far is older than it.
func announce(t *testing.T, a *app.App, admin *browser, title string) int64 {
	t.Helper()
	a.Resolver().SetClock(func() time.Time { return time.Now().Add(3 * time.Second) })
	defer a.Resolver().SetClock(time.Now)
	var out auth.Announcement
	if code := admin.do("POST", "/auth/admin/announcements", map[string]any{"title": title, "body": "**" + title + "** is here. [Read more](/client)"}, &out); code != http.StatusCreated || out.ID == 0 {
		t.Fatalf("announce %q: %d %+v", title, code, out)
	}
	return out.ID
}

func waitingFor(b *browser) []int64 {
	b.t.Helper()
	var me struct {
		Announcements []int64 `json:"announcements"`
	}
	if code := b.do("GET", "/auth/me", nil, &me); code != 200 {
		b.t.Fatalf("me: %d", code)
	}
	return me.Announcements
}

func listed(b *browser) []auth.Announcement {
	b.t.Helper()
	var out auth.AnnouncementsResponse
	if code := b.do("GET", "/api/announcements", nil, &out); code != 200 {
		b.t.Fatalf("GET /api/announcements: %d", code)
	}
	return out.Announcements
}

// Only an administrator, signed in and not by a token, writes one.
func TestAnnouncements_OnlyAdministratorsWriteThem(t *testing.T) {
	_, base, admin := announcementServer(t)
	body := map[string]any{"title": "News", "body": "Something new"}
	guest := newBrowser(t, base)
	makeGuest(guest)
	if code := guest.do("POST", "/auth/admin/announcements", body, nil); code != http.StatusUnauthorized {
		t.Errorf("a guest announces: %d, want 401", code)
	}
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	if code := ana.do("POST", "/auth/admin/announcements", body, nil); code != http.StatusForbidden {
		t.Errorf("an account announces: %d, want 403", code)
	}
	if code := ana.do("GET", "/auth/admin/announcements", nil, nil); code != http.StatusForbidden {
		t.Errorf("an account reads the admin's list: %d, want 403", code)
	}
	var made struct{ Token string }
	if code := admin.do("POST", "/auth/tokens", map[string]any{"name": "a script", "scopes": []string{"play"}}, &made); code != http.StatusCreated {
		t.Fatalf("token: %d", code)
	}
	if code := doJSONAs(t, made.Token, "POST", base+"/auth/admin/announcements", body, nil); code != http.StatusForbidden {
		t.Errorf("an administrator's token announces: %d, want 403", code)
	}
	if len(listed(guest)) != 0 {
		t.Fatal("something was announced")
	}
	if code := admin.do("POST", "/auth/admin/announcements", map[string]any{"title": " ", "body": "x"}, nil); code != http.StatusBadRequest {
		t.Errorf("an announcement without a title: %d, want 400", code)
	}
	if code := admin.do("POST", "/auth/admin/announcements", body, nil); code != http.StatusCreated {
		t.Errorf("the administrator announces: %d, want 201", code)
	}
	if got := listed(guest); len(got) != 1 || got[0].Title != "News" || got[0].Body != "Something new" {
		t.Errorf("listed: %+v", got)
	}
}

// Somebody who was there is offered an announcement until they close the
// card, which marks every one it showed; then never again.
func TestAnnouncements_OfferedOnceToThoseWhoWereThere(t *testing.T) {
	a, base, admin := announcementServer(t)
	guest := newBrowser(t, base)
	makeGuest(guest)
	if got := waitingFor(guest); len(got) != 0 {
		t.Fatalf("waiting before anything was announced: %v", got)
	}
	one := announce(t, a, admin, "One")
	two := announce(t, a, admin, "Two")
	if got := waitingFor(guest); len(got) != 2 || got[0] != one || got[1] != two {
		t.Fatalf("waiting: %v, want [%d %d]", got, one, two)
	}
	if got := waitingFor(guest); len(got) != 2 {
		t.Fatalf("looking again changed what is waiting: %v", got)
	}
	var out auth.AnnouncementsSeenResponse
	if code := guest.do("POST", "/auth/announcements-seen", map[string]any{"ids": []int64{one, two}}, &out); code != 200 || out.Announcements == nil || len(out.Announcements) != 0 {
		t.Fatalf("POST /auth/announcements-seen: %d %+v", code, out)
	}
	if got := waitingFor(guest); len(got) != 0 {
		t.Errorf("after closing the card: %v still waiting", got)
	}
	// They are still there to look back at.
	if got := listed(guest); len(got) != 2 {
		t.Errorf("listed: %+v", got)
	}
}

// A newcomer is not shown what was announced before it existed.
func TestAnnouncements_NotForThoseWhoCameAfter(t *testing.T) {
	_, base, admin := announcementServer(t)
	if code := admin.do("POST", "/auth/admin/announcements", map[string]any{"title": "Old news", "body": "Before your time"}, nil); code != http.StatusCreated {
		t.Fatalf("announce: %d", code)
	}
	newcomer := newBrowser(t, base)
	if got := waitingFor(newcomer); len(got) != 0 {
		t.Errorf("a newcomer: %v waiting", got)
	}
	makeGuest(newcomer)
	if got := waitingFor(newcomer); len(got) != 0 {
		t.Errorf("a guest made after the announcement: %v waiting", got)
	}
	if got := listed(newcomer); len(got) != 1 {
		t.Errorf("the help window's list leaves it out: %+v", got)
	}
}

// Signing in brings the guest's seen marks to the account.
func TestAnnouncements_SeenMarksFollowTheGuestIntoTheAccount(t *testing.T) {
	a, base, admin := announcementServer(t)
	b := newBrowser(t, base)
	makeGuest(b)
	id := announce(t, a, admin, "News")
	other := newBrowser(t, base)
	other.login("ana", anaPassword)
	if got := waitingFor(other); len(got) != 1 {
		t.Fatalf("ana, there before it: %v waiting", got)
	}
	if code := b.do("POST", "/auth/announcements-seen", map[string]any{"ids": []int64{id}}, nil); code != 200 {
		t.Fatalf("seen: %d", code)
	}
	if code := b.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if got := waitingFor(b); len(got) != 0 {
		t.Errorf("the guest had closed it, the account it signed in to still has %v waiting", got)
	}
}

// A withdrawn announcement is offered no more, nor listed for looking back,
// but the admin page still lists it.
func TestAnnouncements_AWithdrawnOneIsGone(t *testing.T) {
	a, base, admin := announcementServer(t)
	guest := newBrowser(t, base)
	makeGuest(guest)
	id := announce(t, a, admin, "Oops")
	if got := waitingFor(guest); len(got) != 1 {
		t.Fatalf("waiting: %v", got)
	}
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	if code := ana.do("POST", "/auth/admin/announcements/"+strconv.FormatInt(id, 10)+"/withdraw", nil, nil); code != http.StatusForbidden {
		t.Errorf("an account withdraws one: %d, want 403", code)
	}
	if code := admin.do("POST", "/auth/admin/announcements/"+strconv.FormatInt(id, 10)+"/withdraw", nil, nil); code != 200 {
		t.Fatalf("withdraw: %d", code)
	}
	if code := admin.do("POST", "/auth/admin/announcements/999/withdraw", nil, nil); code != http.StatusNotFound {
		t.Errorf("withdrawing none: %d, want 404", code)
	}
	if got := waitingFor(guest); len(got) != 0 {
		t.Errorf("a withdrawn one is waiting: %v", got)
	}
	if got := listed(guest); len(got) != 0 {
		t.Errorf("a withdrawn one is listed: %+v", got)
	}
	var all auth.AnnouncementsResponse
	if code := admin.do("GET", "/auth/admin/announcements", nil, &all); code != 200 || len(all.Announcements) != 1 || !all.Announcements[0].Withdrawn {
		t.Errorf("the admin's list: %d %+v", code, all)
	}
}
