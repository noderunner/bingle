package app_test

import (
	"context"
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

type scheduleView struct {
	Status   string `json:"status"`
	StartAt  string `json:"start_at"`
	DecideBy string `json:"decide_by"`
	Mine     bool   `json:"mine"`
	CanVote  bool   `json:"can_vote"`
	People   []struct {
		Handle   string `json:"handle"`
		Status   string `json:"status"`
		Answered bool   `json:"answered"`
	} `json:"people"`
	Slots []struct {
		ID    int64  `json:"id"`
		At    string `json:"at"`
		Count int    `json:"count"`
		Mine  bool   `json:"mine"`
	} `json:"slots"`
}

func scheduleOf(t *testing.T, b *browser, room string) scheduleView {
	t.Helper()
	var g struct {
		Status     string        `json:"status"`
		EndReason  string        `json:"end_reason"`
		Schedule   *scheduleView `json:"schedule"`
		InviteOnly bool          `json:"invite_only"`
	}
	if code := b.do("GET", "/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("the game: %d", code)
	}
	if g.Schedule == nil {
		t.Fatal("the game has no schedule")
	}
	return *g.Schedule
}

func inHours(n int) string {
	return time.Now().UTC().Add(time.Duration(n) * time.Hour).Format(time.RFC3339)
}

// schedule makes a real-time game for later, and returns its room.
func schedule(t *testing.T, b *browser, sched map[string]any) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "real_time": map[string]any{}, "schedule": sched}
	if code := b.do("POST", "/api/games", body, &created); code != 200 {
		t.Fatalf("scheduling: %d", code)
	}
	return created.RoomCode
}

func vote(t *testing.T, b *browser, room string, slot int64, on bool) {
	t.Helper()
	if code := b.do("POST", "/api/games/"+room+"/schedule/vote", map[string]any{"slot": slot, "on": on}, nil); code != 200 {
		t.Fatalf("voting: %d", code)
	}
}

// What cannot be scheduled.
func TestSchedule_TheRules(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	member(t, a, base, "bob")
	opted := member(t, a, base, "ori")
	opted.do("PUT", "/api/friends/settings", map[string]any{"accept_invites": false}, nil)
	guest := newBrowser(t, base)
	make := func(b *browser, realTime bool, sched map[string]any) int {
		body := map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "schedule": sched}
		if realTime {
			body["real_time"] = map[string]any{}
		}
		return b.do("POST", "/api/games", body, nil)
	}
	for _, c := range []struct {
		name string
		b    *browser
		rt   bool
		s    map[string]any
	}{
		{"a guest", guest, true, map[string]any{"at": inHours(5)}},
		{"not real time", ana, false, map[string]any{"at": inHours(5)}},
		{"no time", ana, true, map[string]any{}},
		{"both", ana, true, map[string]any{"at": inHours(5), "slots": []string{inHours(6)}, "invite": []string{"bob"}}},
		{"the past", ana, true, map[string]any{"at": inHours(-1)}},
		{"too far ahead", ana, true, map[string]any{"at": inHours(31 * 24)}},
		{"times with nobody to answer", ana, true, map[string]any{"slots": []string{inHours(6)}}},
		{"nobody by that handle", ana, true, map[string]any{"at": inHours(5), "invite": []string{"nobody-at-all"}}},
		{"somebody who opted out", ana, true, map[string]any{"at": inHours(5), "invite": []string{"ori"}}},
		{"more than the seats", ana, true, map[string]any{"at": inHours(5), "invite": []string{"bob", "ori"}}},
	} {
		if code := make(c.b, c.rt, c.s); code == 200 {
			t.Errorf("%s: it was scheduled", c.name)
		}
	}
	// Too many times, all else being well.
	many := make2Slots(21)
	if code := make(ana, true, map[string]any{"slots": many, "invite": []string{"bob"}}); code == 200 {
		t.Error("21 times were accepted")
	}
}

func make2Slots(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = inHours(2 + i)
	}
	return out
}

// A scheduled time: the room is there, nothing starts by itself, invitations and calendars go out.
func TestSchedule_AGameAtATime(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	ana, bob := newBrowser(t, e.URL), newBrowser(t, e.URL)
	if code := ana.login("ana", signUp(t, e, ana, "ana", "ana@example.com")); code != 200 {
		t.Fatal("Ana signs in")
	}
	if code := bob.login("bob", signUp(t, e, bob, "bob", "bob@example.com")); code != 200 {
		t.Fatal("Bob signs in")
	}
	before := len(e.SMTP.Mails())
	at := inHours(30)
	room := schedule(t, ana, map[string]any{"at": at, "invite": []string{"bob"}})
	sc := scheduleOf(t, ana, room)
	if sc.Status != "scheduled" || sc.StartAt != at || !sc.Mine {
		t.Errorf("the schedule: %+v", sc)
	}

	// Two mails: the creator's calendar, and Bob's invitation, which has none.
	mails := e.waitMails(t, before+2)[before:]
	var toAna, toBob string
	for _, m := range mails {
		switch strings.Join(m.To, ",") {
		case "ana@example.com":
			toAna = m.Data
		case "bob@example.com":
			toBob = m.Data
		}
	}
	if !strings.Contains(toAna, "text/calendar") {
		t.Errorf("Ana's mail has no calendar: %s", toAna)
	}
	if strings.Contains(toBob, "text/calendar") || !strings.Contains(toBob, "@ana") {
		t.Errorf("Bob's invitation: %s", toBob)
	}
	cal := decodeICS(t, toAna)
	for _, want := range []string{"BEGIN:VCALENDAR", "METHOD:REQUEST", "ATTENDEE;RSVP=FALSE:mailto:ana@example.com", "UID:game-" + room + "@bingle", "DTSTART:" + mustICSTime(t, at)} {
		if !strings.Contains(strings.ReplaceAll(cal, "\r\n ", ""), want) {
			t.Errorf("the calendar lacks %q:\n%s", want, cal)
		}
	}
	if strings.Contains(cal, "bob@example.com") {
		t.Error("Ana's calendar names Bob's address")
	}
	// Bob accepts: his calendar comes, and it names only him.
	if code := friendPost(bob, "invite-answer", map[string]any{"room": room, "accept": true}); code != 200 {
		t.Fatalf("Bob accepts: %d", code)
	}
	got := e.waitMails(t, before+3)[before+2]
	if strings.Join(got.To, ",") != "bob@example.com" {
		t.Fatalf("the mail went to %v", got.To)
	}
	if ics := decodeICS(t, got.Data); !strings.Contains(ics, "mailto:bob@example.com") || strings.Contains(ics, "ana@example.com") {
		t.Errorf("Bob's calendar: %s", ics)
	}

	// Seats, and chat, before it starts; and nobody but the host can start it yet.
	if code := bob.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("Bob sits: %d", code)
	}
	if code := bob.do("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Bob", "text": "see you then"}, nil); code != 200 {
		t.Errorf("Bob chats: %d", code)
	}
	var g struct {
		Status string `json:"status"`
	}
	bob.do("GET", "/api/games/"+room, nil, &g)
	if g.Status != "waiting" {
		t.Errorf("a full table started by itself: %q", g.Status)
	}
	if code := bob.do("POST", "/api/games/"+room+"/start", map[string]any{"player_name": "Bob"}, nil); code == 200 {
		t.Error("Bob started it before its time")
	}
	// The calendar file, for accounts, with no people in it.
	resp, err := bob.c.Get(e.URL + "/api/games/" + room + "/schedule.ics")
	if err != nil {
		t.Fatal(err)
	}
	rawBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	raw := string(rawBytes)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/calendar") || !strings.Contains(raw, "METHOD:PUBLISH") || strings.Contains(raw, "mailto:") || !strings.Contains(raw, "/game/"+room) {
		t.Errorf("the file: %d %s", resp.StatusCode, raw)
	}
	if code := newBrowser(t, e.URL).do("GET", "/api/games/"+room+"/schedule.ics", nil, nil); code == 200 {
		t.Error("a guest read the file")
	}
	// Its creator cancels: the calendars are taken back.
	count := len(e.SMTP.Mails())
	if code := bob.do("POST", "/api/games/"+room+"/schedule/cancel", map[string]any{}, nil); code != 403 {
		t.Errorf("Bob cancels: %d, want 403", code)
	}
	if code := ana.do("POST", "/api/games/"+room+"/schedule/cancel", map[string]any{}, nil); code != 200 {
		t.Fatalf("Ana cancels: %d", code)
	}
	for _, m := range e.waitMails(t, count+2)[count:] {
		if !strings.Contains(decodeICS(t, m.Data), "METHOD:CANCEL") {
			t.Errorf("no cancellation to %v", m.To)
		}
	}
	if f := friendsOf(t, bob); len(f.Invites) != 1 || !strings.Contains(f.Invites[0].Notice, "closed") {
		t.Errorf("Bob's news: %+v", f.Invites)
	}
}

// decodeICS finds the calendar file in a mail.
func decodeICS(t *testing.T, data string) string {
	t.Helper()
	data = strings.ReplaceAll(data, "\r\n", "\n")
	_, rest, ok := strings.Cut(data, "Content-Transfer-Encoding: base64\n\n")
	if !ok {
		t.Fatalf("no calendar in the mail: %s", data)
	}
	body, _, _ := strings.Cut(rest, "\n--")
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\n", ""))
	if err != nil {
		t.Fatalf("the calendar: %v", err)
	}
	return string(raw)
}

func mustICSTime(t *testing.T, iso string) string {
	t.Helper()
	at, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatal(err)
	}
	return at.UTC().Format("20060102T150405Z")
}

// The earliest time they can all make, once all have answered.
func TestSchedule_FindingATime(t *testing.T) {
	a, base := membersServer(t)
	ana, bob, cyd := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "cyd")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	times := []string{inHours(5), inHours(10), inHours(20)}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Ana", "real_time": map[string]any{}, "schedule": map[string]any{"slots": times, "invite": []string{"bob", "cyd"}}}, &created); code != 200 {
		t.Fatalf("scheduling: %d", code)
	}
	room := created.RoomCode
	sc := scheduleOf(t, ana, room)
	if sc.Status != "polling" || len(sc.Slots) != 3 || !sc.Slots[0].Mine || sc.Slots[0].Count != 1 {
		t.Fatalf("to begin with: %+v", sc)
	}
	// A stranger cannot answer.
	dan := member(t, a, base, "dan")
	if code := dan.do("POST", "/api/games/"+room+"/schedule/vote", map[string]any{"slot": sc.Slots[0].ID, "on": true}, nil); code != 403 {
		t.Errorf("a stranger answers: %d, want 403", code)
	}
	// Answering accepts. Bob can make the first two, and changes his mind about the first.
	vote(t, bob, room, sc.Slots[0].ID, true)
	vote(t, bob, room, sc.Slots[1].ID, true)
	vote(t, bob, room, sc.Slots[0].ID, false)
	if s := scheduleOf(t, ana, room); s.Status != "polling" || s.People[0].Status != "accepted" || !s.People[0].Answered {
		t.Errorf("after Bob: %+v", s)
	}
	// Cyd says the last two. Only the second suits all three, and it is the game's time.
	vote(t, cyd, room, sc.Slots[1].ID, true)
	s := scheduleOf(t, ana, room)
	if s.Status != "scheduled" || s.StartAt != times[1] || len(s.Slots) != 0 {
		t.Errorf("the time: %+v, want %s", s, times[1])
	}
	if f := friendsOf(t, bob); len(f.Invites) != 1 || !strings.Contains(f.Invites[0].Notice, "time has been set") {
		t.Errorf("Bob's news: %+v", f.Invites)
	}
}

// With no time in common the creator can offer new ones; a silent player can be left out.
func TestSchedule_NoCommonTimeAndWhoIsWaitedFor(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	times := []string{inHours(5), inHours(10)}
	room := schedule(t, ana, map[string]any{"slots": times, "invite": []string{"bob"}})
	sc := scheduleOf(t, ana, room)
	vote(t, ana, room, sc.Slots[1].ID, false)

	// Ana can only make the first and Bob only the second.
	vote(t, bob, room, sc.Slots[1].ID, true)
	s := scheduleOf(t, ana, room)
	if s.Status != "no_agreement" || s.DecideBy == "" {
		t.Fatalf("no common time: %+v", s)
	}
	if f := friendsOf(t, ana); len(f.Invites) != 1 || !strings.Contains(f.Invites[0].Notice, "48 hours") {
		t.Errorf("Ana's news: %+v", f.Invites)
	}
	// Only its creator can offer new times.
	fresh := []string{inHours(40), inHours(41)}
	if code := bob.do("POST", "/api/games/"+room+"/schedule/slots", map[string]any{"slots": fresh}, nil); code != 403 {
		t.Errorf("Bob offers times: %d, want 403", code)
	}
	if code := ana.do("POST", "/api/games/"+room+"/schedule/slots", map[string]any{"slots": fresh}, nil); code != 200 {
		t.Fatalf("Ana offers times: %d", code)
	}
	s = scheduleOf(t, bob, room)
	if s.Status != "polling" || s.DecideBy != "" || len(s.Slots) != 2 || s.Slots[0].Mine {
		t.Fatalf("after new times: %+v", s)
	}
	if f := friendsOf(t, bob); len(f.Invites) != 1 || !strings.Contains(f.Invites[0].Notice, "new times") {
		t.Errorf("Bob's news: %+v", f.Invites)
	}
	// Bob can make the second, and that is the earliest that suits both.
	vote(t, bob, room, s.Slots[1].ID, true)
	got := scheduleOf(t, ana, room)
	if got.Status != "scheduled" || got.StartAt != fresh[1] || len(got.Slots) != 0 {
		t.Fatalf("the time: %+v, want %s", got, fresh[1])
	}

	// Somebody who never answers holds it up until the creator stops waiting.
	silent := schedule(t, ana, map[string]any{"slots": []string{inHours(7)}, "invite": []string{"bob"}})
	if s := scheduleOf(t, ana, silent); s.Status != "polling" {
		t.Fatalf("with one silent: %+v", s)
	}
	if code := bob.do("POST", "/api/games/"+silent+"/schedule/decide", map[string]any{}, nil); code != 403 {
		t.Errorf("Bob decides: %d, want 403", code)
	}
	if code := ana.do("POST", "/api/games/"+silent+"/schedule/decide", map[string]any{}, nil); code != 200 {
		t.Fatalf("Ana stops waiting: %d", code)
	}
	if s := scheduleOf(t, ana, silent); s.Status != "scheduled" || s.People[0].Status != "dropped" {
		t.Errorf("after stopping waiting: %+v", s)
	}

	// A decline stops counting: it resolves without them.
	declined := schedule(t, ana, map[string]any{"slots": []string{inHours(8)}, "invite": []string{"bob"}})
	if code := friendPost(bob, "invite-answer", map[string]any{"room": declined, "accept": false}); code != 200 {
		t.Fatalf("Bob declines: %d", code)
	}
	if s := scheduleOf(t, ana, declined); s.Status != "scheduled" {
		t.Errorf("after a decline: %+v", s)
	}
}

// Time passing: the last time goes by with nobody agreed, then the 48 hours, and it closes. From a
// scheduled time on, any seated player can start it.
func TestSchedule_AsTimePasses(t *testing.T) {
	a, base := membersServer(t)
	ana, bob := member(t, a, base, "ana"), member(t, a, base, "bob")
	now := time.Now()
	a.Manager().SetClock(func() time.Time { return now })
	ctx := context.Background()
	advance := func(d time.Duration) { now = now.Add(d); a.Manager().CheckSchedules(ctx) }

	room := schedule(t, ana, map[string]any{"slots": []string{inHours(5)}, "invite": []string{"bob"}})
	advance(4 * time.Hour)
	if s := scheduleOf(t, ana, room); s.Status != "polling" {
		t.Fatalf("before its last time: %+v", s)
	}
	advance(2 * time.Hour)
	if s := scheduleOf(t, ana, room); s.Status != "no_agreement" {
		t.Fatalf("after its last time: %+v", s)
	}
	advance(47 * time.Hour)
	if s := scheduleOf(t, ana, room); s.Status != "no_agreement" {
		t.Fatalf("within the 48 hours: %+v", s)
	}
	advance(2 * time.Hour)
	if s := scheduleOf(t, ana, room); s.Status != "closed" {
		t.Fatalf("after the 48 hours: %+v", s)
	}
	var g struct {
		Status    string `json:"status"`
		EndReason string `json:"end_reason"`
	}
	ana.do("GET", "/api/games/"+room, nil, &g)
	if g.Status != "finished" || g.EndReason != "unscheduled" {
		t.Errorf("the game: %+v", g)
	}
	for _, who := range []*browser{ana, bob} {
		if f := friendsOf(t, who); len(f.Invites) != 1 || !strings.Contains(f.Invites[0].Notice, "closed") {
			t.Errorf("the news: %+v", f.Invites)
		}
	}

	// From a fixed time on, any seated player can start, once the seats are full.
	at := now.UTC().Add(3 * time.Hour).Format(time.RFC3339)
	var made struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana", "real_time": map[string]any{}, "schedule": map[string]any{"at": at, "invite": []string{"bob"}}}, &made); code != 200 {
		t.Fatalf("scheduling: %d", code)
	}
	if code := bob.do("POST", "/api/games/"+made.RoomCode+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("Bob sits: %d", code)
	}
	if code := bob.do("POST", "/api/games/"+made.RoomCode+"/start", map[string]any{"player_name": "Bob"}, nil); code == 200 {
		t.Fatal("Bob started it before its time")
	}
	now = now.Add(4 * time.Hour)
	if code := bob.do("POST", "/api/games/"+made.RoomCode+"/start", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("Bob starts it on time: %d", code)
	}
	if s := scheduleOf(t, ana, made.RoomCode); s.Status != "started" {
		t.Errorf("after the start: %+v", s)
	}
}

// Through the page: the setup offers it for a real-time game, in the viewer's time zone, and the lobby
// has the times to answer.
func TestWebSchedule_TheSetupAndTheLobby(t *testing.T) {
	a, base := membersServer(t)
	anaB, bobB := member(t, a, base, "ana"), member(t, a, base, "bob")
	befriend(t, anaB, bobB, "bob", "ana")
	ana, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ana, base, anaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ana, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/?game=tic-tac-toe")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "the setup", `document.getElementById("real-time") !== null && document.getElementById("create-submit") !== null`, 20*time.Second)
	waitJS(t, ana, "nothing until real time", `document.getElementById("schedule-field").hidden`, 10*time.Second)
	evalString(t, ana, `(function(){var b=document.getElementById("real-time");b.checked=true;b.dispatchEvent(new Event("change",{bubbles:true}));return "ok"})()`)
	waitJS(t, ana, "the choice", `!document.getElementById("schedule-field").hidden`, 10*time.Second)
	waitJS(t, ana, "the friend", `document.querySelector('.schedule-friend[value="bob"]') !== null`, 10*time.Second)
	if !strings.Contains(evalString(t, ana, `(document.querySelector('input[name="schedule-mode"][value="windows"]').click(), document.getElementById("schedule-tz").textContent)`), "time zone") {
		t.Error("no word on the time zone")
	}
	if !evalBool(t, ana, `document.getElementById("invite-only").checked`) {
		t.Error("Invite only should start ticked for a scheduled game")
	}
	if got := evalString(t, ana, `document.getElementById("create-submit").textContent`); got != "Schedule Game" {
		t.Errorf("the button says %q", got)
	}
	evalString(t, ana, `(function(){
		function local(h){var d=new Date(Date.now()+h*3600000);var p=n=>String(n).padStart(2,"0");return d.getFullYear()+"-"+p(d.getMonth()+1)+"-"+p(d.getDate())+"T"+p(d.getHours())+":"+p(d.getMinutes());}
		document.getElementById("schedule-add-slot").click();
		var i=document.querySelectorAll(".schedule-slot");i[0].value=local(5);i[1].value=local(9);
		document.querySelector('.schedule-friend[value="bob"]').click();
		document.getElementById("create-submit").click();return "ok"})()`)
	waitJS(t, ana, "the times in the lobby", `document.querySelectorAll('#schedule-slot-list input[data-slot]').length === 2`, 20*time.Second)
	if !evalBool(t, ana, `document.querySelectorAll('#schedule-slot-list input[data-slot]:checked').length === 2`) {
		t.Error("the creator's times should start ticked")
	}
	clickSel(t, ana, "#schedule-slot-list input[data-slot]")
	waitJS(t, ana, "an answer taken", `document.querySelectorAll('#schedule-slot-list input[data-slot]:checked').length === 1`, 10*time.Second)
	waitJS(t, ana, "the waiting button", `document.getElementById("schedule-decide") !== null`, 5*time.Second)

	// Stop waiting for Bob: the lobby turns into one with a time.
	clickSel(t, ana, "#schedule-decide")
	waitJS(t, ana, "the time", `document.getElementById("schedule-when") !== null && document.getElementById("schedule-ics") !== null && document.getElementById("schedule-slot-list") === null`, 10*time.Second)
}
