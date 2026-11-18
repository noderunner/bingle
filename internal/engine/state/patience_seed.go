package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// A game of patience is dealt from a seed, and the same seed deals the same
// cards in the same order every time, so a deal can be played again (ADR
// 0136). The seed is a number a player can copy and type: a new game draws
// one below 2^32, and any number up to 2^64 - 1 may be given.

// DealtBySeed reports whether game's deal comes from a seed a player may
// see and choose: a game of patience.
func DealtBySeed(game *catalog.CompiledGame) bool {
	return game != nil && game.Patience != nil
}

// NewDealSeed draws a seed for a new deal: short enough to copy by hand.
func NewDealSeed() uint64 {
	return randomSeed()%(1<<32-1) + 1
}

// ParseDealSeed reads a seed a player gave, a whole number of at most 20
// digits; spaces around it are ignored.
func ParseDealSeed(s string) (uint64, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("a seed is a whole number from 0 to %d", uint64(1<<64-1))
	}
	return n, nil
}

// DealtFrom is the starting position of a game dealt by seed.
func DealtFrom(game *catalog.CompiledGame, seed uint64) *BoardState {
	return declaredStart(game, seed)
}
