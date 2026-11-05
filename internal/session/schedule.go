package session

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// Games set up for later (ADR 0109). A scheduled game is a room whose lobby stays open: it never starts
// by itself. It has a time, or a list of times for those invited to answer; when all of them have,
// the earliest time they can all make becomes its time.

const (
	// scheduleRange is how far ahead a game can be set.
	scheduleRange = 30 * 24 * time.Hour
	// maxSlots bounds the times offered for a game.
	maxSlots = 20
	// noAgreementWait is how long a creator has when no time suits everyone.
	noAgreementWait = 48 * time.Hour
	// scheduleEvery is how often the schedules are looked at.
	scheduleEvery = time.Minute
	// eventLength is the length of a game on a calendar: nobody can know, and it needs one.
	eventLength = time.Hour
)

var (
	// ErrBadSchedule is returned for a schedule that cannot be set as asked.
	ErrBadSchedule = errors.New("session: that game cannot be scheduled so")
	// ErrNotScheduled is returned for a game with no schedule, or none in that state.
	ErrNotScheduled = errors.New("session: that game is not waiting to be scheduled")
	// ErrNotScheduleCreator is returned to anybody else trying what its creator may do.
	ErrNotScheduleCreator = errors.New("session: only the player who scheduled this game can do that")
	// ErrNotInvited is returned to somebody answering who was not invited.
	ErrNotInvited = errors.New("session: you are not invited to this game")
)

// ScheduleRequest is what a setup asks for: a time or several, and who to invite.
type ScheduleRequest struct {
	At     *time.Time  `json:"at,omitempty"`
	Slots  []time.Time `json:"slots,omitempty"`
	Invite []string    `json:"invite,omitempty"`
}

// checkSlots makes sure times can be offered, and returns them in order.
func (m *Manager) checkSlots(slots []time.Time) ([]time.Time, error) {
	if len(slots) == 0 || len(slots) > maxSlots {
		return nil, fmt.Errorf("%w: offer between 1 and %d times", ErrBadSchedule, maxSlots)
	}
	now := m.now()
	out := make([]time.Time, 0, len(slots))
	seen := map[int64]bool{}
	for _, at := range slots {
		if err := m.checkWhen(at, now); err != nil {
			return nil, err
		}
		if !seen[at.Unix()] {
			seen[at.Unix()] = true
			out = append(out, at.UTC())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, nil
}

func (m *Manager) checkWhen(at, now time.Time) error {
	if !at.After(now) {
		return fmt.Errorf("%w: choose a time that has not passed", ErrBadSchedule)
	}
	if at.After(now.Add(scheduleRange)) {
		return fmt.Errorf("%w: choose a time within 30 days", ErrBadSchedule)
	}
	return nil
}

// checkSchedule makes sure a schedule can be set: it needs an account, a real-time game and a time,
// and no more invitations than there are seats. It returns who is invited.
func (m *Manager) checkSchedule(ctx context.Context, opts *CreateOptions, freeSeats int) ([]store.Account, error) {
	req := opts.Schedule
	me := viewerAccount(ctx)
	if me == 0 {
		return nil, fmt.Errorf("%w: sign in first", ErrBadSchedule)
	}
	if opts.RealTime == nil {
		return nil, fmt.Errorf("%w: only a real-time game can be scheduled", ErrBadSchedule)
	}
	if (req.At == nil) == (len(req.Slots) == 0) {
		return nil, fmt.Errorf("%w: give one time or a list of times", ErrBadSchedule)
	}
	if req.At != nil {
		if err := m.checkWhen(*req.At, m.now()); err != nil {
			return nil, err
		}
	} else {
		slots, err := m.checkSlots(req.Slots)
		if err != nil {
			return nil, err
		}
		req.Slots = slots
		if len(req.Invite) == 0 {
			return nil, fmt.Errorf("%w: invite somebody to answer the times", ErrBadSchedule)
		}
	}
	if len(req.Invite) > freeSeats {
		return nil, fmt.Errorf("%w: that is more players than there are open seats", ErrBadSchedule)
	}
	invited := make([]store.Account, 0, len(req.Invite))
	seen := map[int64]bool{me: true}
	for _, handle := range req.Invite {
		other, err := m.activeAccount(ctx, handle)
		if err != nil {
			return nil, fmt.Errorf("%w: no player is called %q", ErrBadSchedule, strings.TrimSpace(handle))
		}
		if seen[other.ID] {
			continue
		}
		seen[other.ID] = true
		settings, err := m.store.FriendSettings(ctx, other.ID)
		if err != nil {
			return nil, err
		}
		if !settings.AcceptInvites {
			return nil, fmt.Errorf("%w: @%s is not taking invitations to games", ErrBadSchedule, other.Handle)
		}
		invited = append(invited, other)
	}
	if !m.chat.allow("schedule|"+fmt.Sprint(me), m.now()) {
		return nil, ErrChatTooFast
	}
	return invited, nil
}

// mailAccount sends an account a mail about a schedule, if it has a confirmed address. A mail that
// cannot go is no failure of what it tells: the lobby and the friends list say the same.
func (m *Manager) mailAccount(ctx context.Context, a store.Account, build func(base string) mail.Message) {
	if !m.MailEnabled() || !a.EmailVerified {
		return
	}
	_ = m.SendAccountMail(ctx, "schedule", a.Email, "", build)
}

// gameNameOf is what a mail calls a game.
func (m *Manager) gameNameOf(defID string) string {
	if title, _ := m.GameListLabel(defID); title != "" {
		return title
	}
	if compiled, ok := m.catalog.Get(defID); ok {
		return compiled.Definition.Title()
	}
	return "a game"
}

// calendarEvent is a game on the calendar of address.
func (m *Manager) calendarEvent(game store.Game, sched store.Schedule, method, address, base string) mail.Event {
	start, _ := time.Parse(time.RFC3339, sched.StartAt)
	from, _ := m.NotifyFrom()
	return mail.Event{
		UID:       "game-" + game.RoomCode + "@bingle",
		Seq:       sched.IcsSeq,
		Start:     start,
		End:       start.Add(eventLength),
		Summary:   m.gameNameOf(game.GameDefID) + " on Bingle",
		URL:       base + "/game/" + game.RoomCode,
		Method:    method,
		Organizer: from,
		Attendee:  address,
	}
}

// sendCalendar sends an account the game's time, for its calendar.
func (m *Manager) sendCalendar(ctx context.Context, game store.Game, sched store.Schedule, a store.Account) {
	m.mailAccount(ctx, a, func(base string) mail.Message {
		return mail.GameScheduled(a.Email, m.gameNameOf(game.GameDefID), game.RoomCode, base, m.calendarEvent(game, sched, "REQUEST", a.Email, base), m.now())
	})
}

// saveSchedule records the schedule of a new game, tells those invited, and tells its creator's calendar.
func (m *Manager) saveSchedule(ctx context.Context, game store.Game, req *ScheduleRequest, invited []store.Account) error {
	me := viewerAccount(ctx)
	sched := store.Schedule{GameID: game.ID, CreatorAccount: me, Status: store.SchedPolling}
	if req.At != nil {
		sched.Status = store.SchedScheduled
		sched.StartAt = req.At.UTC().Format(time.RFC3339)
	}
	if err := m.store.SaveSchedule(ctx, sched); err != nil {
		return err
	}
	now := m.now().UTC()
	creator, err := m.store.AccountByID(ctx, me)
	if err != nil {
		return err
	}
	// The creator has a row too, for news of the game, and can make all the times they offered.
	if err := m.store.AddGameInvite(ctx, game.ID, me, me, now); err != nil {
		return err
	}
	if _, err := m.store.AnswerGameInvite(ctx, game.ID, me, store.InviteAccepted); err != nil {
		return err
	}
	if err := m.offerSlots(ctx, game.ID, me, req.Slots); err != nil {
		return err
	}
	name := m.gameNameOf(game.GameDefID)
	var at time.Time
	if req.At != nil {
		at = *req.At
	}
	for _, other := range invited {
		if err := m.store.AddGameInvite(ctx, game.ID, other.ID, me, now); err != nil {
			return err
		}
		m.tellFriends(other.PrincipalID)
		a := other
		m.mailAccount(ctx, a, func(base string) mail.Message {
			return mail.GameInvitation(a.Email, creator.Handle, name, game.RoomCode, base, at, req.At == nil)
		})
	}
	if req.At != nil {
		m.sendCalendar(ctx, game, sched, creator)
	}
	return nil
}

// offerSlots offers times, all of which the creator can make to begin with.
func (m *Manager) offerSlots(ctx context.Context, gameID, creator int64, times []time.Time) error {
	if len(times) == 0 {
		return nil
	}
	if err := m.store.ReplaceSlots(ctx, gameID, times); err != nil {
		return err
	}
	slots, err := m.store.SlotsOf(ctx, gameID)
	if err != nil {
		return err
	}
	for _, slot := range slots {
		if err := m.store.SetVote(ctx, slot.ID, creator, true); err != nil {
			return err
		}
	}
	return nil
}

// GameSlotView is a time offered, as a page shows it.
type GameSlotView struct {
	ID    int64  `json:"id"`
	At    string `json:"at"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine,omitempty"`
	Past  bool   `json:"past,omitempty"`
}

// GamePersonView is somebody invited, as a page shows them.
type GamePersonView struct {
	Handle string `json:"handle"`
	Status string `json:"status"`
	// Answered says they have said which times they can make.
	Answered bool `json:"answered,omitempty"`
}

// GameScheduleView is a game's schedule, as a page shows it.
type GameScheduleView struct {
	Status   string           `json:"status"`
	StartAt  string           `json:"start_at,omitempty"`
	DecideBy string           `json:"decide_by,omitempty"`
	Creator  string           `json:"creator"`
	Mine     bool             `json:"mine,omitempty"`
	CanVote  bool             `json:"can_vote,omitempty"`
	People   []GamePersonView `json:"people"`
	Slots    []GameSlotView   `json:"slots,omitempty"`
}

// GameSchedule reads a game's schedule as the one asking sees it; nil for a game with none.
func (m *Manager) GameSchedule(ctx context.Context, room string) (*GameScheduleView, error) {
	game, err := m.store.GetGame(ctx, canonRoom(room))
	if err != nil {
		return nil, err
	}
	sched, err := m.store.ScheduleOf(ctx, game.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	creator, err := m.store.AccountByID(ctx, sched.CreatorAccount)
	if err != nil {
		return nil, err
	}
	me := viewerAccount(ctx)
	out := &GameScheduleView{Status: sched.Status, StartAt: sched.StartAt, DecideBy: sched.DecideBy, Creator: creator.Handle, Mine: me != 0 && me == sched.CreatorAccount, People: []GamePersonView{}}
	invited, err := m.store.InviteesOf(ctx, game.ID)
	if err != nil {
		return nil, err
	}
	slots, err := m.store.SlotsOf(ctx, game.ID)
	if err != nil {
		return nil, err
	}
	answered := func(id int64) bool {
		for _, slot := range slots {
			if slot.Voters[id] {
				return true
			}
		}
		return false
	}
	now := m.now()
	for _, v := range invited {
		if v.Account.ID == sched.CreatorAccount {
			continue
		}
		out.People = append(out.People, GamePersonView{Handle: v.Account.Handle, Status: v.Status, Answered: answered(v.Account.ID)})
		out.CanVote = out.CanVote || (me != 0 && v.Account.ID == me && (v.Status == store.InvitePending || v.Status == store.InviteAccepted))
	}
	out.CanVote = out.CanVote || out.Mine
	for _, slot := range slots {
		out.Slots = append(out.Slots, GameSlotView{ID: slot.ID, At: slot.At.Format(time.RFC3339), Count: len(slot.Voters), Mine: me != 0 && slot.Voters[me], Past: !slot.At.After(now)})
	}
	return out, nil
}

// mayStartScheduled says whether a seated player other than the host may start a game: from its time on.
func (m *Manager) mayStartScheduled(ctx context.Context, gameID int64) bool {
	sched, err := m.store.ScheduleOf(ctx, gameID)
	if err != nil || sched.Status != store.SchedScheduled {
		return false
	}
	at, err := time.Parse(time.RFC3339, sched.StartAt)
	return err == nil && !m.now().Before(at)
}

// markScheduleStarted notes that a scheduled game has begun.
func (m *Manager) markScheduleStarted(ctx context.Context, gameID int64) {
	sched, err := m.store.ScheduleOf(ctx, gameID)
	if err != nil {
		return
	}
	sched.Status = store.SchedStarted
	_ = m.store.SaveSchedule(ctx, sched)
}

// loadSchedule reads what an action on a room's schedule needs.
func (m *Manager) loadSchedule(ctx context.Context, room string) (store.Game, store.Schedule, error) {
	game, err := m.store.GetGame(ctx, room)
	if err != nil {
		return store.Game{}, store.Schedule{}, err
	}
	sched, err := m.store.ScheduleOf(ctx, game.ID)
	if errors.Is(err, store.ErrNotFound) {
		return store.Game{}, store.Schedule{}, ErrNotScheduled
	}
	return game, sched, err
}

// VoteSlot says whether the account me can make a time. Saying so accepts an invitation still open.
func (m *Manager) VoteSlot(ctx context.Context, me store.Account, room string, slotID int64, on bool) error {
	room = canonRoom(room)
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, sched, err := m.loadSchedule(ctx, room)
	if err != nil {
		return err
	}
	if sched.Status != store.SchedPolling && sched.Status != store.SchedNoAgreement {
		return ErrNotScheduled
	}
	invited, err := m.store.InviteesOf(ctx, game.ID)
	if err != nil {
		return err
	}
	ok := false
	for _, v := range invited {
		if v.Account.ID == me.ID && (v.Status == store.InvitePending || v.Status == store.InviteAccepted) {
			ok = true
			if v.Status == store.InvitePending {
				if _, err := m.store.AnswerGameInvite(ctx, game.ID, me.ID, store.InviteAccepted); err != nil {
					return err
				}
				m.tellFriends(me.PrincipalID)
			}
		}
	}
	if !ok {
		return ErrNotInvited
	}
	slots, err := m.store.SlotsOf(ctx, game.ID)
	if err != nil {
		return err
	}
	found := false
	for _, slot := range slots {
		if slot.ID == slotID {
			found = true
			if !slot.At.After(m.now()) {
				return fmt.Errorf("%w: that time has passed", ErrBadSchedule)
			}
		}
	}
	if !found {
		return fmt.Errorf("%w: no such time", ErrBadSchedule)
	}
	if err := m.store.SetVote(ctx, slotID, me.ID, on); err != nil {
		return err
	}
	if err := m.resolveSchedule(ctx, game, sched); err != nil {
		return err
	}
	m.publishState(ctx, room, "join")
	return nil
}

// resolveSchedule looks at a game's answers. It waits until everybody counted has answered; then the
// earliest time still to come that they can all make is the game's time, and if there is none, its
// creator is told.
func (m *Manager) resolveSchedule(ctx context.Context, game store.Game, sched store.Schedule) error {
	if sched.Status != store.SchedPolling && sched.Status != store.SchedNoAgreement {
		return nil
	}
	invited, err := m.store.InviteesOf(ctx, game.ID)
	if err != nil {
		return err
	}
	slots, err := m.store.SlotsOf(ctx, game.ID)
	if err != nil {
		return err
	}
	var counted []store.Invitee
	for _, v := range invited {
		if v.Status == store.InvitePending || v.Status == store.InviteAccepted {
			counted = append(counted, v)
		}
	}
	for _, v := range counted {
		answered := false
		for _, slot := range slots {
			answered = answered || slot.Voters[v.Account.ID]
		}
		if !answered {
			return nil
		}
	}
	now := m.now()
	for _, slot := range slots {
		if !slot.At.After(now) {
			continue
		}
		all := true
		for _, v := range counted {
			all = all && slot.Voters[v.Account.ID]
		}
		if all {
			return m.fixSchedule(ctx, game, sched, slot.At, counted)
		}
	}
	if sched.Status == store.SchedPolling {
		return m.noAgreement(ctx, game, sched)
	}
	return nil
}

// fixSchedule makes a time the game's time, and tells everybody's calendar.
func (m *Manager) fixSchedule(ctx context.Context, game store.Game, sched store.Schedule, at time.Time, counted []store.Invitee) error {
	sched.Status = store.SchedScheduled
	sched.StartAt = at.UTC().Format(time.RFC3339)
	sched.DecideBy = ""
	sched.IcsSeq++
	if err := m.store.SaveSchedule(ctx, sched); err != nil {
		return err
	}
	if err := m.store.ReplaceSlots(ctx, game.ID, nil); err != nil {
		return err
	}
	for _, v := range counted {
		// Somebody who answers has accepted.
		if _, err := m.store.AnswerGameInvite(ctx, game.ID, v.Account.ID, store.InviteAccepted); err != nil {
			return err
		}
		if err := m.store.SetInviteNotice(ctx, game.ID, v.Account.ID, "a time has been set. Open the game to see it."); err != nil {
			return err
		}
		m.tellFriends(v.Account.PrincipalID)
		m.sendCalendar(ctx, game, sched, v.Account)
	}
	return nil
}

// noAgreement gives a game's creator their time to do something about it.
func (m *Manager) noAgreement(ctx context.Context, game store.Game, sched store.Schedule) error {
	by := m.now().UTC().Add(noAgreementWait)
	sched.Status = store.SchedNoAgreement
	sched.DecideBy = by.Format(time.RFC3339)
	if err := m.store.SaveSchedule(ctx, sched); err != nil {
		return err
	}
	creator, err := m.store.AccountByID(ctx, sched.CreatorAccount)
	if err != nil {
		return err
	}
	if err := m.store.SetInviteNotice(ctx, game.ID, creator.ID, "no time suits everyone. Offer new times or cancel it within 48 hours."); err != nil {
		return err
	}
	m.tellFriends(creator.PrincipalID)
	m.mailAccount(ctx, creator, func(base string) mail.Message {
		return mail.NoCommonTime(creator.Email, m.gameNameOf(game.GameDefID), game.RoomCode, base, by)
	})
	return nil
}

// DecideSchedule is the creator ceasing to wait: whoever has not answered is left out.
func (m *Manager) DecideSchedule(ctx context.Context, me store.Account, room string) error {
	room = canonRoom(room)
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, sched, err := m.creatorsSchedule(ctx, me, room)
	if err != nil {
		return err
	}
	if sched.Status != store.SchedPolling && sched.Status != store.SchedNoAgreement {
		return ErrNotScheduled
	}
	invited, err := m.store.InviteesOf(ctx, game.ID)
	if err != nil {
		return err
	}
	slots, err := m.store.SlotsOf(ctx, game.ID)
	if err != nil {
		return err
	}
	for _, v := range invited {
		if v.Status != store.InvitePending && v.Status != store.InviteAccepted {
			continue
		}
		answered := false
		for _, slot := range slots {
			answered = answered || slot.Voters[v.Account.ID]
		}
		if !answered {
			if _, err := m.store.AnswerGameInvite(ctx, game.ID, v.Account.ID, store.InviteDropped); err != nil {
				return err
			}
			m.tellFriends(v.Account.PrincipalID)
		}
	}
	if err := m.resolveSchedule(ctx, game, sched); err != nil {
		return err
	}
	m.publishState(ctx, room, "join")
	return nil
}

func (m *Manager) creatorsSchedule(ctx context.Context, me store.Account, room string) (store.Game, store.Schedule, error) {
	game, sched, err := m.loadSchedule(ctx, room)
	if err != nil {
		return game, sched, err
	}
	if sched.CreatorAccount != me.ID {
		return game, sched, ErrNotScheduleCreator
	}
	return game, sched, nil
}

// OfferNewTimes is the creator offering other times, and everybody starts again.
func (m *Manager) OfferNewTimes(ctx context.Context, me store.Account, room string, times []time.Time) error {
	room = canonRoom(room)
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, sched, err := m.creatorsSchedule(ctx, me, room)
	if err != nil {
		return err
	}
	if sched.Status != store.SchedPolling && sched.Status != store.SchedNoAgreement {
		return ErrNotScheduled
	}
	checked, err := m.checkSlots(times)
	if err != nil {
		return err
	}
	if err := m.offerSlots(ctx, game.ID, me.ID, checked); err != nil {
		return err
	}
	sched.Status = store.SchedPolling
	sched.DecideBy = ""
	if err := m.store.SaveSchedule(ctx, sched); err != nil {
		return err
	}
	if err := m.store.SetInviteNotice(ctx, game.ID, me.ID, ""); err != nil {
		return err
	}
	invited, err := m.store.InviteesOf(ctx, game.ID)
	if err != nil {
		return err
	}
	for _, v := range invited {
		if v.Account.ID == me.ID || (v.Status != store.InvitePending && v.Status != store.InviteAccepted) {
			continue
		}
		if err := m.store.SetInviteNotice(ctx, game.ID, v.Account.ID, "new times have been offered. Open the game to answer."); err != nil {
			return err
		}
		m.tellFriends(v.Account.PrincipalID)
		a := v.Account
		m.mailAccount(ctx, a, func(base string) mail.Message {
			return mail.NewTimesOffered(a.Email, m.gameNameOf(game.GameDefID), game.RoomCode, base)
		})
	}
	m.tellFriends(me.PrincipalID)
	m.publishState(ctx, room, "join")
	return nil
}

// CancelSchedule is the creator closing a game that has not started.
func (m *Manager) CancelSchedule(ctx context.Context, me store.Account, room string) error {
	room = canonRoom(room)
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, sched, err := m.creatorsSchedule(ctx, me, room)
	if err != nil {
		return err
	}
	return m.closeSchedule(ctx, game, sched, "the player who scheduled it cancelled it.")
}

// closeSchedule closes a game that will not be played, and tells everybody invited.
func (m *Manager) closeSchedule(ctx context.Context, game store.Game, sched store.Schedule, reason string) error {
	if sched.Status == store.SchedStarted || sched.Status == store.SchedClosed || game.Status != "waiting" {
		return ErrNotScheduled
	}
	hadTime := sched.Status == store.SchedScheduled
	sched.Status = store.SchedClosed
	sched.IcsSeq++
	if err := m.store.SaveSchedule(ctx, sched); err != nil {
		return err
	}
	if err := m.store.FinishGame(ctx, game.ID, nil, "unscheduled"); err != nil {
		return err
	}
	invited, err := m.store.InviteesOf(ctx, game.ID)
	if err != nil {
		return err
	}
	for _, v := range invited {
		if v.Status == store.InviteDeclined || v.Status == store.InviteDropped {
			continue
		}
		if err := m.store.SetInviteNotice(ctx, game.ID, v.Account.ID, "closed: "+reason); err != nil {
			return err
		}
		m.tellFriends(v.Account.PrincipalID)
		a := v.Account
		// Only those who were sent the time have an entry on their calendars to take back.
		withEntry := hadTime && (v.Status == store.InviteAccepted)
		m.mailAccount(ctx, a, func(base string) mail.Message {
			var event *mail.Event
			if withEntry {
				e := m.calendarEvent(game, sched, "CANCEL", a.Email, base)
				event = &e
			}
			return mail.GameCancelled(a.Email, m.gameNameOf(game.GameDefID), game.RoomCode, reason, base, event, m.now())
		})
	}
	m.publishState(ctx, game.RoomCode, "join")
	return nil
}

// tellAcceptedTime sends somebody who has just accepted the game's time, if it has one.
func (m *Manager) tellAcceptedTime(ctx context.Context, game store.Game, a store.Account) error {
	sched, err := m.store.ScheduleOf(ctx, game.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if sched.Status == store.SchedScheduled {
		m.sendCalendar(ctx, game, sched, a)
		return nil
	}
	return m.resolveSchedule(ctx, game, sched)
}

// GameCalendar is a game's time as a file to download, with no people in it.
func (m *Manager) GameCalendar(ctx context.Context, room, base string) (string, error) {
	game, sched, err := m.loadSchedule(ctx, canonRoom(room))
	if err != nil {
		return "", err
	}
	if sched.Status != store.SchedScheduled {
		return "", ErrNotScheduled
	}
	if base == "" {
		base = m.publicBase()
	}
	return m.calendarEvent(game, sched, "PUBLISH", "", base).ICS(m.now()), nil
}

// publicBase is the server's public URL, where one is known.
func (m *Manager) publicBase() string {
	if m.notify != nil {
		return m.notify.opts.BaseURL
	}
	return ""
}

// RunSchedules moves on the games nobody has answered: one whose last time has passed has no
// agreement, and one whose creator has let the 48 hours go by is closed.
func (m *Manager) RunSchedules(ctx context.Context) {
	t := time.NewTicker(scheduleEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.CheckSchedules(ctx)
		}
	}
}

// CheckSchedules looks at each game waiting for a decision.
func (m *Manager) CheckSchedules(ctx context.Context) {
	rooms, err := m.store.OpenSchedules(ctx)
	if err != nil {
		m.logger().Error("schedule: could not read the games waiting for a decision", "err", err)
		return
	}
	for _, room := range rooms {
		if err := m.checkSchedule1(ctx, room); err != nil && !errors.Is(err, ErrNotScheduled) {
			m.logger().Error("schedule: could not move a game on", "err", err)
		}
	}
}

func (m *Manager) checkSchedule1(ctx context.Context, room string) error {
	unlock := m.lockRoom(ctx, room)
	defer unlock()
	game, sched, err := m.loadSchedule(ctx, room)
	if err != nil {
		return err
	}
	now := m.now()
	switch sched.Status {
	case store.SchedNoAgreement:
		by, err := time.Parse(time.RFC3339, sched.DecideBy)
		if err != nil || now.Before(by) {
			return err
		}
		return m.closeSchedule(ctx, game, sched, "no time suited everyone, and no new times were offered.")
	case store.SchedPolling:
		slots, err := m.store.SlotsOf(ctx, game.ID)
		if err != nil {
			return err
		}
		for _, slot := range slots {
			if slot.At.After(now) {
				return nil
			}
		}
		if err := m.noAgreement(ctx, game, sched); err != nil {
			return err
		}
		m.publishState(ctx, room, "join")
	}
	return nil
}
