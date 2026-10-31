package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// Email notifications: a seat can ask to be mailed when it is its turn.
// See docs/history/email-notifications-plan.md for the threats this is built against;
// in short: the address is write-only (no endpoint returns it), it must be
// confirmed by a link mailed to it, only fixed templates are ever sent, and
// everything is rate limited and stops on a login failure.

// Mailer sends one message. *mail.Sender is the real one.
type Mailer interface {
	Send(ctx context.Context, msg mail.Message) error
}

// NotifyOptions switch the feature on.
type NotifyOptions struct {
	Mailer  Mailer
	From    string // the address the mail comes from, shown to players so they can whitelist it
	BaseURL string // the public https URL of this server, for the links in the mail
	// HourlyLimit and DailyLimit cap all mail the server sends.
	HourlyLimit int
	DailyLimit  int
	// AwayUnit is the length of the "N minutes" of the away setting (a minute;
	// tests shorten it).
	AwayUnit time.Duration
}

// Errors of the notification feature.
var (
	ErrNotifyOff     = errors.New("session: email notifications are not set up on this server")
	ErrBadToken      = errors.New("session: that secret is not valid for this seat or address")
	ErrNotifyLimited = errors.New("session: too many emails lately, try again later")
	ErrSuppressed    = errors.New("session: that address asked not to be emailed")
	ErrNotVerified   = errors.New("session: confirm the email address first")
	ErrBadAddress    = errors.New("session: that is not an email address")
)

// Limits: how much one address, one game or one client can cause.
const (
	confirmEvery     = 10 * time.Minute // per address
	confirmPerDay    = 5                // per address
	confirmPerIPHour = 5
	turnEveryPerGame = 15 * time.Minute // per address and game
	turnPerDayAddr   = 20
	turnPerDayGame   = 100
	noticeEvery      = time.Hour
	confirmLifetime  = 24 * time.Hour
	breakerCooldown  = time.Hour
	maxAwayMinutes   = 24 * 60
)

type turnKey struct{ seat, seq int }

type notifyJob struct {
	msg mail.Message
}

type notifier struct {
	m    *Manager
	opts NotifyOptions

	jobs chan notifyJob

	mu          sync.Mutex // guards everything below and serialises limit checks
	lastTurn    map[string]turnKey
	conns       map[string]map[int]int
	timers      map[string]*time.Timer
	pausedUntil time.Time
	authFailed  bool
}

// EnableNotify turns email notifications on. Call it before the server starts,
// and run RunNotifier to deliver the mail.
func (m *Manager) EnableNotify(o NotifyOptions) {
	if o.Mailer == nil || o.From == "" || o.BaseURL == "" {
		return
	}
	if o.HourlyLimit <= 0 {
		o.HourlyLimit = 20
	}
	if o.DailyLimit <= 0 {
		o.DailyLimit = 150
	}
	if o.AwayUnit <= 0 {
		o.AwayUnit = time.Minute
	}
	o.BaseURL = strings.TrimRight(o.BaseURL, "/")
	n := &notifier{m: m, opts: o, jobs: make(chan notifyJob, 64),
		lastTurn: map[string]turnKey{}, conns: map[string]map[int]int{}, timers: map[string]*time.Timer{}}
	m.notify = n
	m.hub.setTap(n.tap)
}

// NotifyFrom is the address the mail is sent from, and whether the feature is on.
func (m *Manager) NotifyFrom() (string, bool) {
	if m.notify == nil {
		return "", false
	}
	return m.notify.opts.From, true
}

// RunNotifier delivers queued mail until ctx ends. It is the only place mail
// leaves the server, one message at a time, so a slow mail server never delays a
// move.
func (m *Manager) RunNotifier(ctx context.Context) {
	n := m.notify
	if n == nil {
		return
	}
	tidy := time.NewTicker(time.Hour)
	defer tidy.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-n.jobs:
			n.deliver(ctx, j)
		case <-tidy.C:
			// Addresses nobody confirmed within a week, and old records of mail sent.
			if err := m.store.PurgeContacts(ctx, m.now().Add(-7*24*time.Hour)); err != nil {
				m.logger().Error("notify: tidying up", "err", err)
			}
		}
	}
}

func (n *notifier) deliver(ctx context.Context, j notifyJob) {
	n.mu.Lock()
	blocked := n.authFailed || n.m.now().Before(n.pausedUntil)
	n.mu.Unlock()
	if blocked {
		return
	}
	err := n.opts.Mailer.Send(ctx, j.msg)
	if err == nil {
		return
	}
	// Never log the address, the message or the server's conversation.
	switch mail.Classify(err) {
	case mail.BadCredentials:
		n.mu.Lock()
		n.authFailed = true
		n.mu.Unlock()
		n.m.logger().Error("notify: the mail server refused the login: email notifications are stopped until bingle is restarted", "err", err)
	case mail.RateLimited:
		n.mu.Lock()
		n.pausedUntil = n.m.now().Add(breakerCooldown)
		n.mu.Unlock()
		n.m.logger().Warn("notify: the mail server asks us to slow down", "pause", breakerCooldown, "err", err)
	default:
		n.m.logger().Warn("notify: a message was not delivered", "err", err)
	}
}

// enqueue adds a job, dropping the oldest when the queue is full.
func (n *notifier) enqueue(j notifyJob) {
	for {
		select {
		case n.jobs <- j:
			return
		default:
			select {
			case <-n.jobs:
				n.m.logger().Warn("notify: the mail queue is full, dropped the oldest message")
			default:
			}
		}
	}
}

// ---- tokens and hashes ---------------------------------------------------------

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system's random source is gone: nothing safe can be done
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func sameHash(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func addrHash(addr string) string { return hashOf(strings.ToLower(addr)) }

func nameKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// ---- limits ----------------------------------------------------------------------

// reserve checks the limits for a mail of the kind and, if it may go, records
// it (so a burst cannot get past the limits before the worker sends).
func (n *notifier) reserve(ctx context.Context, kind, addr string, gameID int64, ip string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.m.store
	now := n.m.now()
	count := func(f store.SentFilter) int {
		c, err := st.CountSent(ctx, f)
		if err != nil {
			return 1 << 30 // fail closed
		}
		return c
	}
	if count(store.SentFilter{Since: now.Add(-time.Hour)}) >= n.opts.HourlyLimit || count(store.SentFilter{Since: now.Add(-24 * time.Hour)}) >= n.opts.DailyLimit {
		return ErrNotifyLimited
	}
	switch kind {
	case "confirm":
		if count(store.SentFilter{Kinds: []string{"confirm"}, AddrHash: addr, Since: now.Add(-confirmEvery)}) > 0 ||
			count(store.SentFilter{Kinds: []string{"confirm"}, AddrHash: addr, Since: now.Add(-24 * time.Hour)}) >= confirmPerDay ||
			(ip != "" && count(store.SentFilter{Kinds: []string{"confirm"}, IP: ip, Since: now.Add(-time.Hour)}) >= confirmPerIPHour) {
			return ErrNotifyLimited
		}
	case "turn":
		if count(store.SentFilter{Kinds: []string{"turn"}, AddrHash: addr, GameID: gameID, Since: now.Add(-turnEveryPerGame)}) > 0 ||
			count(store.SentFilter{Kinds: []string{"turn"}, AddrHash: addr, Since: now.Add(-24 * time.Hour)}) >= turnPerDayAddr ||
			count(store.SentFilter{Kinds: []string{"turn"}, GameID: gameID, Since: now.Add(-24 * time.Hour)}) >= turnPerDayGame {
			return ErrNotifyLimited
		}
	case "notice":
		if count(store.SentFilter{Kinds: []string{"notice"}, AddrHash: addr, Since: now.Add(-noticeEvery)}) > 0 {
			return ErrNotifyLimited
		}
	}
	return st.RecordSent(ctx, kind, addr, gameID, ip, now)
}

// ---- whose seat -----------------------------------------------------------------------

func (m *Manager) humanByName(ctx context.Context, gameID int64, name string) ([]store.Player, store.Player, error) {
	players, err := m.store.ListPlayers(ctx, gameID)
	if err != nil {
		return nil, store.Player{}, err
	}
	for _, p := range players {
		if p.Bot == "" && strings.EqualFold(p.Name, strings.TrimSpace(name)) {
			return players, p, nil
		}
	}
	return players, store.Player{}, ErrNotSeated
}

// ErrNoPrincipal is the answer to an anonymous request for something only a
// seat's owner may do.
var ErrNoPrincipal = errors.New("session: only the seat's owner may do that: send its guest key (POST /api/guest makes one for a script)")

// ownSeat is the seat held by name, if it is the request's principal's. A
// seat's notifications are its owner's business only, so unlike most actions
// an anonymous request cannot change them even for a seat nobody owns.
func (m *Manager) ownSeat(ctx context.Context, gameID int64, name string) (store.Player, error) {
	_, p, err := m.humanByName(ctx, gameID, name)
	if err != nil {
		return store.Player{}, err
	}
	if auth.IDFrom(ctx) == "" {
		return store.Player{}, ErrNoPrincipal
	}
	if err := m.mayActFor(ctx, &p); err != nil {
		return store.Player{}, err
	}
	return p, nil
}

// ---- contacts -------------------------------------------------------------------------

// ContactInfo is what a browser is told about its address: never the address.
type ContactInfo struct {
	ID     int64  `json:"contact_id"`
	Secret string `json:"contact_secret,omitempty"` // only when the contact is created
	Status string `json:"status"`                   // "pending", "verified" or "stopped"
	Masked string `json:"masked"`                   // "m••••@example.com"
	// ConfirmationSent is false when a mail was due but a limit held it back.
	ConfirmationSent bool   `json:"confirmation_sent"`
	Note             string `json:"note,omitempty"`
}

func (m *Manager) infoOf(ctx context.Context, c store.Contact) ContactInfo {
	status := "pending"
	if c.Verified() {
		status = "verified"
	}
	if stopped, _ := m.store.IsSuppressed(ctx, c.AddrHash); stopped {
		status = "stopped"
	}
	return ContactInfo{ID: c.ID, Status: status, Masked: mail.Mask(c.Address)}
}

func (m *Manager) ownedContact(ctx context.Context, id int64, secret string) (store.Contact, error) {
	c, err := m.store.GetContact(ctx, id)
	if err != nil || secret == "" || !sameHash(c.SecretHash, hashOf(secret)) {
		return store.Contact{}, ErrBadToken
	}
	return c, nil
}

func (m *Manager) mustNotify() (*notifier, error) {
	if m.notify == nil {
		return nil, ErrNotifyOff
	}
	return m.notify, nil
}

// SaveContact sets the address a browser wants to be notified at: a new contact
// (id 0) or a change to the contact it owns (id and secret). The address is
// not used until the link mailed to it is followed. ip is the client's address,
// for the limit on confirmation mails per client.
func (m *Manager) SaveContact(ctx context.Context, ip, address string, id int64, secret string) (ContactInfo, error) {
	n, err := m.mustNotify()
	if err != nil {
		return ContactInfo{}, err
	}
	addr, err := mail.ValidAddress(address)
	if err != nil {
		return ContactInfo{}, ErrBadAddress
	}
	ah := addrHash(addr)
	if stopped, _ := m.store.IsSuppressed(ctx, ah); stopped {
		return ContactInfo{}, ErrSuppressed
	}
	confirm := newToken()
	expires := m.now().Add(confirmLifetime)

	var c store.Contact
	created := false
	var oldAddr, oldStop string
	if id != 0 {
		c, err = m.ownedContact(ctx, id, secret)
		if err != nil {
			return ContactInfo{}, err
		}
		if c.AddrHash == ah && c.Verified() {
			return m.infoOf(ctx, c), nil // nothing changes
		}
		if c.Verified() {
			oldAddr, oldStop = c.Address, c.StopToken
		}
		if err := m.store.ResetContact(ctx, c.ID, addr, ah, hashOf(confirm), expires); err != nil {
			return ContactInfo{}, err
		}
		c.Address, c.AddrHash = addr, ah
		c.VerifiedAt = ""
	} else {
		secret = newToken()
		c = store.Contact{Address: addr, AddrHash: ah, SecretHash: hashOf(secret), StopToken: newToken(), ConfirmHash: hashOf(confirm)}
		c.ID, err = m.store.CreateContact(ctx, c, expires)
		if err != nil {
			return ContactInfo{}, err
		}
		created = true
	}
	info := m.infoOf(ctx, c)
	if created {
		info.Secret = secret
	}
	if oldAddr != "" {
		if oa, _ := mail.ValidAddress(oldAddr); oa != "" && n.reserve(ctx, "notice", addrHash(oa), 0, ip) == nil {
			n.enqueue(notifyJob{msg: mail.AddressChanged(oa, n.opts.BaseURL, oldStop)})
		}
	}
	if err := n.reserve(ctx, "confirm", ah, 0, ip); err != nil {
		info.Note = "A confirmation was already sent to this address lately (or this server is busy). Look in the inbox and the spam folder, or try again in a little while."
		return info, nil
	}
	n.enqueue(notifyJob{msg: mail.Confirmation(addr, n.opts.BaseURL, confirm, c.StopToken)})
	info.ConfirmationSent = true
	return info, nil
}

// ContactStatus describes a contact the caller owns.
func (m *Manager) ContactStatus(ctx context.Context, id int64, secret string) (ContactInfo, error) {
	if _, err := m.mustNotify(); err != nil {
		return ContactInfo{}, err
	}
	c, err := m.ownedContact(ctx, id, secret)
	if err != nil {
		return ContactInfo{}, err
	}
	return m.infoOf(ctx, c), nil
}

// ResendConfirmation mails a new confirmation link for an unverified contact.
func (m *Manager) ResendConfirmation(ctx context.Context, ip string, id int64, secret string) (ContactInfo, error) {
	n, err := m.mustNotify()
	if err != nil {
		return ContactInfo{}, err
	}
	c, err := m.ownedContact(ctx, id, secret)
	if err != nil {
		return ContactInfo{}, err
	}
	info := m.infoOf(ctx, c)
	if c.Verified() || info.Status == "stopped" {
		return info, nil
	}
	confirm := newToken()
	if err := m.store.SetConfirmToken(ctx, c.ID, hashOf(confirm), m.now().Add(confirmLifetime)); err != nil {
		return ContactInfo{}, err
	}
	if err := n.reserve(ctx, "confirm", c.AddrHash, 0, ip); err != nil {
		info.Note = "A confirmation was sent to this address lately. Look in the inbox and the spam folder, or try again in a little while."
		return info, nil
	}
	n.enqueue(notifyJob{msg: mail.Confirmation(c.Address, n.opts.BaseURL, confirm, c.StopToken)})
	info.ConfirmationSent = true
	return info, nil
}

// ForgetContact deletes a contact and every seat setting that used it.
func (m *Manager) ForgetContact(ctx context.Context, id int64, secret string) error {
	if _, err := m.mustNotify(); err != nil {
		return err
	}
	c, err := m.ownedContact(ctx, id, secret)
	if err != nil {
		return err
	}
	return m.store.DeleteContact(ctx, c.ID)
}

// ConfirmContact verifies an address by the token from its confirmation mail.
func (m *Manager) ConfirmContact(ctx context.Context, token string) (ContactInfo, error) {
	if _, err := m.mustNotify(); err != nil {
		return ContactInfo{}, err
	}
	c, err := m.store.ConfirmContact(ctx, hashOf(token), m.now())
	if err != nil {
		return ContactInfo{}, ErrBadToken
	}
	return m.infoOf(ctx, c), nil
}

// StopMail is the unsubscribe link: the address is never mailed again.
func (m *Manager) StopMail(ctx context.Context, token string) (ContactInfo, error) {
	if _, err := m.mustNotify(); err != nil {
		return ContactInfo{}, err
	}
	c, err := m.store.ContactByStopToken(ctx, token)
	if err != nil {
		return ContactInfo{}, ErrBadToken
	}
	if err := m.store.Suppress(ctx, c.AddrHash); err != nil {
		return ContactInfo{}, err
	}
	return m.infoOf(ctx, c), nil
}

// ---- a seat's setting --------------------------------------------------------------------

// GameNotify is a seat's notification setting, as the seat's owner may see it.
type GameNotify struct {
	Configured  bool         `json:"configured"` // the person has made a choice for this game
	Enabled     bool         `json:"enabled"`
	AwayMinutes int          `json:"away_minutes"` // 0: every time; N: only when the turn waited N minutes
	Contact     *ContactInfo `json:"contact,omitempty"`
}

func (m *Manager) gameNotify(ctx context.Context, gameID int64, key string) GameNotify {
	p, err := m.store.GetNotifyPref(ctx, gameID, key)
	if err != nil {
		return GameNotify{}
	}
	out := GameNotify{Configured: true, Enabled: p.Enabled, AwayMinutes: p.AwayMinutes}
	if p.ContactID != 0 {
		if c, err := m.store.GetContact(ctx, p.ContactID); err == nil {
			info := m.infoOf(ctx, c)
			out.Contact = &info
			if info.Status != "verified" {
				out.Enabled = false
			}
		}
	}
	return out
}

// GameNotify returns the setting of the seat held by name, to its owner.
func (m *Manager) GameNotify(ctx context.Context, roomCode, name string) (GameNotify, error) {
	if _, err := m.mustNotify(); err != nil {
		return GameNotify{}, err
	}
	game, err := m.store.GetGame(ctx, canonRoom(roomCode))
	if err != nil {
		return GameNotify{}, err
	}
	p, err := m.ownSeat(ctx, game.ID, name)
	if err != nil {
		return GameNotify{}, err
	}
	return m.gameNotify(ctx, game.ID, nameKey(p.Name)), nil
}

// SetGameNotify changes the setting of the seat held by name, for its owner.
// Switching it on needs a verified contact the caller owns.
func (m *Manager) SetGameNotify(ctx context.Context, roomCode, name string, contactID int64, contactSecret string, enabled bool, awayMinutes int) (GameNotify, error) {
	if _, err := m.mustNotify(); err != nil {
		return GameNotify{}, err
	}
	if awayMinutes < 0 || awayMinutes > maxAwayMinutes {
		return GameNotify{}, fmt.Errorf("session: away_minutes must be between 0 and %d", maxAwayMinutes)
	}
	game, err := m.store.GetGame(ctx, canonRoom(roomCode))
	if err != nil {
		return GameNotify{}, err
	}
	p, err := m.ownSeat(ctx, game.ID, name)
	if err != nil {
		return GameNotify{}, err
	}
	key := nameKey(p.Name)
	pref := store.NotifyPref{GameID: game.ID, NameKey: key, Enabled: false, AwayMinutes: awayMinutes}
	if old, err := m.store.GetNotifyPref(ctx, game.ID, key); err == nil {
		pref.ContactID = old.ContactID
	}
	if contactID != 0 {
		c, err := m.ownedContact(ctx, contactID, contactSecret)
		if err != nil {
			return GameNotify{}, err
		}
		pref.ContactID = c.ID
	}
	if enabled {
		if pref.ContactID == 0 {
			return GameNotify{}, ErrNotVerified
		}
		c, err := m.store.GetContact(ctx, pref.ContactID)
		if err != nil || !c.Verified() {
			return GameNotify{}, ErrNotVerified
		}
		if stopped, _ := m.store.IsSuppressed(ctx, c.AddrHash); stopped {
			return GameNotify{}, ErrSuppressed
		}
		pref.Enabled = true
	}
	if err := m.store.SetNotifyPref(ctx, pref); err != nil {
		return GameNotify{}, err
	}
	return m.gameNotify(ctx, game.ID, key), nil
}

// ---- who is connected ------------------------------------------------------------------

// TrackConnection notes that a player's page is connected for the seat (so no
// "it is your turn" mail is sent while they are watching). Call the returned
// function when the connection ends.
func (m *Manager) TrackConnection(roomCode string, seat int) func() {
	n := m.notify
	if n == nil || seat <= 0 {
		return func() {}
	}
	roomCode = canonRoom(roomCode)
	n.mu.Lock()
	if n.conns[roomCode] == nil {
		n.conns[roomCode] = map[int]int{}
	}
	n.conns[roomCode][seat]++
	n.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			n.mu.Lock()
			defer n.mu.Unlock()
			if n.conns[roomCode][seat]--; n.conns[roomCode][seat] <= 0 {
				delete(n.conns[roomCode], seat)
				if len(n.conns[roomCode]) == 0 {
					delete(n.conns, roomCode)
				}
			}
		})
	}
}

func (n *notifier) connected(room string, seat int) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.conns[room][seat] > 0
}

// ---- the turn passes -------------------------------------------------------------------------

// tap looks at every event of every room and notices a turn passing to a seat.
func (n *notifier) tap(room string, ev Event) {
	var seat, seq int
	switch d := ev.Data.(type) {
	case moveEventData:
		if d.TurnSeat == nil || d.Status != "active" {
			return
		}
		seat, seq = *d.TurnSeat, d.Seq
	case stateEventData:
		if d.TurnSeat == nil || d.Status != "active" || d.Lobby {
			return
		}
		seat, seq = *d.TurnSeat, d.MoveSeq
	default:
		return
	}
	k := turnKey{seat, seq}
	n.mu.Lock()
	if n.lastTurn[room] == k {
		n.mu.Unlock()
		return
	}
	n.lastTurn[room] = k
	n.mu.Unlock()
	go n.turnPassed(room, seat, seq)
}

func (n *notifier) turnPassed(room string, seat, seq int) {
	ctx := context.Background()
	game, player, ok := n.seatPlayer(ctx, room, seat)
	if !ok {
		return
	}
	pref, err := n.m.store.GetNotifyPref(ctx, game.ID, nameKey(player.Name))
	if err != nil || !pref.Enabled || pref.ContactID == 0 {
		return
	}
	key := room + "|" + fmt.Sprint(seat)
	n.mu.Lock()
	if t := n.timers[key]; t != nil {
		t.Stop()
		delete(n.timers, key)
	}
	n.mu.Unlock()
	if pref.AwayMinutes > 0 {
		wait := time.Duration(pref.AwayMinutes) * n.opts.AwayUnit
		n.mu.Lock()
		n.timers[key] = time.AfterFunc(wait, func() { n.send(room, seat, seq, true) })
		n.mu.Unlock()
		return
	}
	n.send(room, seat, seq, false)
}

func (n *notifier) seatPlayer(ctx context.Context, room string, seat int) (store.Game, store.Player, bool) {
	game, err := n.m.store.GetGame(ctx, room)
	if err != nil {
		return store.Game{}, store.Player{}, false
	}
	players, err := n.m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return store.Game{}, store.Player{}, false
	}
	for _, p := range players {
		if p.Seat == seat && p.Bot == "" {
			return game, p, true
		}
	}
	return store.Game{}, store.Player{}, false
}

// send mails the seat's person that it is their turn, if it still is and the
// seat's setting, the address and the limits all allow it. ignoreConnected is
// set for the "only if I was away" mode, whose wait is the point.
func (n *notifier) send(room string, seat, seq int, ignoreConnected bool) {
	ctx := context.Background()
	game, player, ok := n.seatPlayer(ctx, room, seat)
	if !ok || game.Status != "active" || game.TurnSeat == nil || *game.TurnSeat != seat || game.MoveSeq != seq {
		return // the turn moved on
	}
	if !ignoreConnected && n.connected(room, seat) {
		return
	}
	pref, err := n.m.store.GetNotifyPref(ctx, game.ID, nameKey(player.Name))
	if err != nil || !pref.Enabled || pref.ContactID == 0 {
		return
	}
	c, err := n.m.store.GetContact(ctx, pref.ContactID)
	if err != nil || !c.Verified() {
		return
	}
	if stopped, _ := n.m.store.IsSuppressed(ctx, c.AddrHash); stopped {
		return
	}
	to, err := mail.ValidAddress(c.Address)
	if err != nil {
		return
	}
	if err := n.reserve(ctx, "turn", c.AddrHash, game.ID, ""); err != nil {
		return
	}
	n.enqueue(notifyJob{msg: mail.Turn(to, n.gameName(game.GameDefID), game.RoomCode, n.opts.BaseURL, c.StopToken)})
}

func (n *notifier) gameName(defID string) string {
	base, _, _ := strings.Cut(defID, "~")
	if g, ok := n.m.catalog.Get(base); ok {
		return g.Definition.Name
	}
	return "your game"
}
