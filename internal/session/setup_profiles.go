package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// Saved setups (ADR 0094): an account keeps the tables it likes to start, and
// may share them. Anyone, a guest too, may start from a shared one.

const (
	setupNameMax    = 40
	setupsPerFamily = 30
	setupsSharedMax = 100
)

var (
	// ErrSetupAccount is the answer to a guest that tries to save a setup, or
	// to anyone removing what is not theirs.
	ErrSetupAccount = errors.New("session: saving a setup is for accounts: sign in")
	// ErrTooManySetups says an account has as many as it may keep.
	ErrTooManySetups = fmt.Errorf("session: at most %d saved setups per game: delete one first", setupsPerFamily)
)

// SetupProfileRequest is what POST /setup-profiles takes.
type SetupProfileRequest struct {
	Name      string         `json:"name"`
	GameDefID string         `json:"game_def_id"`
	Options   map[string]any `json:"options"`
	Shared    bool           `json:"shared"`
}

// SetupProfileDTO is a saved setup as clients see it.
type SetupProfileDTO struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Owner     string         `json:"owner"`
	Mine      bool           `json:"mine"`
	Shared    bool           `json:"shared"`
	GameDefID string         `json:"game_def_id"`
	Options   map[string]any `json:"options"`
}

func setupProfileDTO(p store.SetupProfile, viewer int64) (SetupProfileDTO, error) {
	out := SetupProfileDTO{ID: p.ID, Name: p.Name, Owner: p.Owner, Mine: p.AccountID == viewer, Shared: p.Shared, GameDefID: p.GameDefID}
	err := json.Unmarshal([]byte(p.Options), &out.Options)
	return out, err
}

// SetupProfiles lists the saved setups of a family of games the caller may use.
func (m *Manager) SetupProfiles(ctx context.Context, family string) ([]SetupProfileDTO, error) {
	var viewer int64
	if acc, ok := signedIn(ctx); ok {
		viewer = acc.ID
	}
	rows, err := m.store.SetupProfiles(ctx, family, viewer, setupsSharedMax)
	if err != nil {
		return nil, err
	}
	out := make([]SetupProfileDTO, 0, len(rows))
	for _, p := range rows {
		dto, err := setupProfileDTO(p, viewer)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// setupName tidies the name of a setup, or refuses it.
func setupName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", fmt.Errorf("%w: a saved setup needs a name", ErrInvalidOptions)
	}
	if len([]rune(name)) > setupNameMax {
		return "", fmt.Errorf("%w: the name of a saved setup is at most %d characters", ErrInvalidOptions, setupNameMax)
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return "", fmt.Errorf("%w: the name of a saved setup has a character that cannot be shown", ErrInvalidOptions)
		}
	}
	return name, nil
}

// SaveSetupProfile stores a setup for the caller's account. The options must make
// a game, so what is saved can always be started.
func (m *Manager) SaveSetupProfile(ctx context.Context, req SetupProfileRequest) (SetupProfileDTO, error) {
	acc, ok := signedIn(ctx)
	if !ok {
		return SetupProfileDTO{}, ErrSetupAccount
	}
	name, err := setupName(req.Name)
	if err != nil {
		return SetupProfileDTO{}, err
	}
	base, ok := m.catalog.Get(req.GameDefID)
	if !ok || strings.Contains(req.GameDefID, "~") {
		return SetupProfileDTO{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, req.GameDefID)
	}
	if _, picker, err := state.SetupCatalogOf(base); err != nil {
		return SetupProfileDTO{}, err
	} else if !picker {
		return SetupProfileDTO{}, fmt.Errorf("%w: %s has no setups to save", ErrInvalidOptions, base.Definition.Name)
	}
	if _, err := m.catalog.Derive(req.GameDefID, req.Options); err != nil {
		return SetupProfileDTO{}, fmt.Errorf("%w: %v", ErrInvalidOptions, err)
	}
	family := base.Definition.Family
	if family == "" {
		family = base.Definition.ID
	}
	n, exists, err := m.store.CountSetupProfiles(ctx, acc.ID, family, name)
	if err != nil {
		return SetupProfileDTO{}, err
	}
	if !exists && n >= setupsPerFamily {
		return SetupProfileDTO{}, ErrTooManySetups
	}
	raw, err := json.Marshal(req.Options)
	if err != nil {
		return SetupProfileDTO{}, err
	}
	id, err := m.store.SaveSetupProfile(ctx, store.SetupProfile{AccountID: acc.ID, Family: family, Name: name, GameDefID: req.GameDefID, Options: string(raw), Shared: req.Shared})
	if err != nil {
		return SetupProfileDTO{}, err
	}
	return SetupProfileDTO{ID: id, Name: name, Owner: acc.Handle, Mine: true, Shared: req.Shared, GameDefID: req.GameDefID, Options: req.Options}, nil
}

// DeleteSetupProfile removes a saved setup: its owner's to do, or an
// administrator's, who signed in by hand, for one shared with everybody.
func (m *Manager) DeleteSetupProfile(ctx context.Context, id int64) error {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return ErrSetupAccount
	}
	row, err := m.store.SetupProfile(ctx, id)
	if err != nil {
		return err
	}
	mine := row.AccountID == p.Account.ID
	moderates := p.Account.Admin && p.Scopes == nil && row.Shared
	if !mine && !moderates {
		// Somebody else's private setup does not exist as far as they can tell.
		return store.ErrNotFound
	}
	return m.store.DeleteSetupProfile(ctx, id)
}
