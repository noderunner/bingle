package envs

import (
	"context"
	"fmt"
	"sync"
)

// MaxBatch bounds how many steps one StepMany call accepts.
const MaxBatch = 128

// StepRequest is one step of a batch: the move as notation, or its fixed
// action ID.
type StepRequest struct {
	EnvID    string `json:"env_id"`
	Move     string `json:"move,omitempty"`
	ActionID *int   `json:"action_id,omitempty"`
}

// StepOutcome is the result of one batched step: an observation, or the
// error that rejected it (the other steps are unaffected).
type StepOutcome struct {
	EnvID       string       `json:"env_id"`
	Observation *Observation `json:"observation,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// StepAction plays the move with the given fixed action ID.
func (m *Manager) StepAction(ctx context.Context, id string, action int) (Observation, error) {
	notation, err := m.actionNotation(id, action)
	if err != nil {
		return Observation{}, err
	}
	return m.Step(ctx, id, notation)
}

func (m *Manager) actionNotation(id string, action int) (string, error) {
	e, err := m.get(id)
	if err != nil {
		return "", err
	}
	space := stateActionSpace(e)
	if !space.Fixed {
		return "", fmt.Errorf("envs: game %q has no fixed action space; pass the move notation", e.game.Definition.ID)
	}
	if action < 0 || action >= len(space.Moves) {
		return "", fmt.Errorf("envs: action_id %d is out of range (0..%d)", action, len(space.Moves)-1)
	}
	return space.Moves[action], nil
}

// StepMany plays several environments' moves in one call — the vectorised
// step training loops want to amortise per-call overhead. Steps on different
// environments run concurrently; steps on the same environment run in the
// order given. Results are returned in request order.
func (m *Manager) StepMany(ctx context.Context, reqs []StepRequest) []StepOutcome {
	out := make([]StepOutcome, len(reqs))
	groups := map[string][]int{}
	var order []string
	for i, r := range reqs {
		if _, seen := groups[r.EnvID]; !seen {
			order = append(order, r.EnvID)
		}
		groups[r.EnvID] = append(groups[r.EnvID], i)
	}

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, envID := range order {
		idxs := groups[envID]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			for _, i := range idxs {
				r := reqs[i]
				var obs Observation
				var err error
				switch {
				case r.ActionID != nil:
					obs, err = m.StepAction(ctx, r.EnvID, *r.ActionID)
				case r.Move != "":
					obs, err = m.Step(ctx, r.EnvID, r.Move)
				default:
					err = fmt.Errorf("envs: a step needs a move or an action_id")
				}
				out[i] = StepOutcome{EnvID: r.EnvID}
				if err != nil {
					out[i].Error = err.Error()
					continue
				}
				o := obs
				out[i].Observation = &o
			}
		}()
	}
	wg.Wait()
	return out
}
