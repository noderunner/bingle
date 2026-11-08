//go:build private

package all

// The private overlay (ADR 0127): the games bingle builds apart from its
// public repository, checked out at internal/games/private (which the public
// repository ignores) and compiled in with the build tag "private". Its
// package registers its games as these files do.
import _ "github.com/noderunner/bingle/internal/games/private"
