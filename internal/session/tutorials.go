package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/tutorial"
)

// ErrTutorialNotFound is returned for a tutorial a game does not have.
var ErrTutorialNotFound = errors.New("session: no such tutorial")

// ErrTutorialRoom is returned for what a tutorial room does not offer
// (turn notifications: every seat is the learner's).
var ErrTutorialRoom = errors.New("session: not in a tutorial")

// tutorialCache holds each family's tutorials once read: they are embedded
// in the binary and never change while it runs.
type tutorialCache struct {
	mu   sync.Mutex
	byID map[string][]tutorial.Tutorial
}

// Tutorials returns the tutorials of the family gameID belongs to (any of
// its games or variants names it), sorted by kind and order, and the family.
func (m *Manager) Tutorials(gameID string) (string, []tutorial.Tutorial, error) {
	family, fsys, ok := m.catalog.Tutorials(gameID)
	if !ok {
		return "", nil, fmt.Errorf("%w: %q", ErrGameDefNotFound, gameID)
	}
	c := &m.tutorials
	c.mu.Lock()
	defer c.mu.Unlock()
	if list, done := c.byID[family]; done {
		return family, list, nil
	}
	list, err := tutorial.Load(fsys)
	if err != nil {
		return "", nil, err
	}
	if c.byID == nil {
		c.byID = map[string][]tutorial.Tutorial{}
	}
	c.byID[family] = list
	return family, list, nil
}

// TutorialsFor returns the tutorials to offer a player of gameID: the family's, but
// for those that teach another variant of it. A tutorial is still started by its
// family's name; this only decides who is offered it.
func (m *Manager) TutorialsFor(gameID string) (string, []tutorial.Tutorial, error) {
	family, list, err := m.Tutorials(gameID)
	if err != nil {
		return "", nil, err
	}
	out := make([]tutorial.Tutorial, 0, len(list))
	for _, t := range list {
		if !t.VariantOnly || t.Game == gameID {
			out = append(out, t)
		}
	}
	return family, out, nil
}

// Tutorial returns one tutorial of gameID's family.
func (m *Manager) Tutorial(gameID, id string) (string, tutorial.Tutorial, error) {
	family, list, err := m.Tutorials(gameID)
	if err != nil {
		return "", tutorial.Tutorial{}, err
	}
	for _, t := range list {
		if t.ID == id {
			return family, t, nil
		}
	}
	return "", tutorial.Tutorial{}, fmt.Errorf("%w: %q has no tutorial %q", ErrTutorialNotFound, family, id)
}

// StartTutorial opens a room for a tutorial: the game it is played on, from
// its start (seeded, with its setup played), with name in the learner's
// seat and in every other seat too (the page plays the scripted replies for
// them), marked as a tutorial so it stays out of every list of games.
func (m *Manager) StartTutorial(ctx context.Context, gameID, id, name string) (CreateGameResult, error) {
	family, t, err := m.Tutorial(gameID, id)
	if err != nil {
		return CreateGameResult{}, err
	}
	compiled, ok := m.catalog.Get(t.Game)
	if !ok {
		return CreateGameResult{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, t.Game)
	}
	st, impulseSeat, err := tutorial.Start(compiled, t)
	if err != nil {
		return CreateGameResult{}, err
	}
	doc, err := st.MarshalJSON()
	if err != nil {
		return CreateGameResult{}, err
	}
	learner := t.LearnerSeat()
	var others []int
	for _, sd := range compiled.Definition.Seats {
		if sd.Seat != learner {
			others = append(others, sd.Seat)
		}
	}
	return m.CreateGameWithOptions(ctx, CreateOptions{
		GameDefID: t.Game, CreatorName: name, Seat: learner, Also: others,
		StartState: string(doc), StartSeat: impulseSeat, Tutorial: family + "/" + t.ID,
	})
}

// TutorialGuide is Tutorial with each step's guide (tutorial.Guide): the
// selections of the learner's moves and the positions shown.
func (m *Manager) TutorialGuide(gameID, id string) (tutorial.Tutorial, []tutorial.StepGuide, error) {
	_, t, err := m.Tutorial(gameID, id)
	if err != nil {
		return tutorial.Tutorial{}, nil, err
	}
	compiled, ok := m.catalog.Get(t.Game)
	if !ok {
		return tutorial.Tutorial{}, nil, fmt.Errorf("%w: %q", ErrGameDefNotFound, t.Game)
	}
	guide, err := tutorial.Guide(compiled, t)
	if err != nil {
		return tutorial.Tutorial{}, nil, err
	}
	return t, guide, nil
}

// NextTutorial returns the tutorial after t in its series, among those offered
// with the game t is played on, and whether there is one.
func (m *Manager) NextTutorial(t tutorial.Tutorial) (tutorial.Tutorial, bool) {
	_, list, err := m.TutorialsFor(t.Game)
	if err != nil {
		return tutorial.Tutorial{}, false
	}
	return tutorial.NextInSeries(list, t)
}

// TutorialsDone returns the ids of the tutorials of a family an account has finished.
func (m *Manager) TutorialsDone(ctx context.Context, accountID int64, family string) (map[string]bool, error) {
	return m.store.TutorialsDone(ctx, accountID, family)
}

// TutorialDone records that an account has finished the tutorial a room plays:
// it holds the learner's seat, and every move of the script has been played.
// It answers false, recording nothing, for a room not yet at the end.
func (m *Manager) TutorialDone(ctx context.Context, account store.Account, roomCode string) (bool, error) {
	g, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return false, err
	}
	family, id, ok := strings.Cut(g.Tutorial, "/")
	if !ok {
		return false, fmt.Errorf("%w: room %q", ErrTutorialRoom, roomCode)
	}
	_, t, err := m.Tutorial(family, id)
	if err != nil {
		return false, err
	}
	players, err := m.store.ListPlayers(ctx, g.ID)
	if err != nil {
		return false, err
	}
	learner := false
	for _, p := range players {
		if p.Seat == t.LearnerSeat() && p.PrincipalID != "" && p.PrincipalID == account.PrincipalID {
			learner = true
		}
	}
	if !learner {
		return false, fmt.Errorf("%w: the room's learner is someone else", ErrNotYourSeat)
	}
	moves, err := m.store.ListMoves(ctx, g.ID, 0)
	if err != nil {
		return false, err
	}
	played := make([]string, 0, len(moves))
	for _, mv := range moves {
		played = append(played, mv.Notation)
	}
	if !t.Played(played) {
		return false, nil
	}
	return true, m.store.MarkTutorialDone(ctx, account.ID, g.Tutorial, m.now())
}
