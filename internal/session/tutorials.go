package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	st, toMove, err := tutorial.Start(compiled, t)
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
		StartState: string(doc), StartSeat: toMove, Tutorial: family + "/" + t.ID,
	})
}
