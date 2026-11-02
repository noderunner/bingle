package rating_test

import (
	"math"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/rating"
)

func near(t *testing.T, what string, got, want, within float64) {
	t.Helper()
	if math.Abs(got-want) > within {
		t.Errorf("%s = %v, want %v (within %v)", what, got, want, within)
	}
}

// The worked example from Glickman's "Example of the Glicko-2 system".
func TestGlickmansExample(t *testing.T) {
	p := rating.Player{Rating: 1500, Deviation: 200, Volatility: 0.06}
	got := rating.Update(p, []rating.Result{
		{Opponent: rating.Player{Rating: 1400, Deviation: 30, Volatility: 0.06}, Score: 1},
		{Opponent: rating.Player{Rating: 1550, Deviation: 100, Volatility: 0.06}, Score: 0},
		{Opponent: rating.Player{Rating: 1700, Deviation: 300, Volatility: 0.06}, Score: 0},
	})
	near(t, "rating", got.Rating, 1464.06, 0.01)
	near(t, "deviation", got.Deviation, 151.52, 0.01)
	near(t, "volatility", got.Volatility, 0.05999, 0.00001)
}

// Between two new players a win moves them apart by the same amount, and a draw
// moves neither.
func TestTwoNewPlayers(t *testing.T) {
	a, b := rating.New(), rating.New()
	win := rating.Update(a, []rating.Result{{Opponent: b, Score: 1}})
	loss := rating.Update(b, []rating.Result{{Opponent: a, Score: 0}})
	if win.Rating <= a.Rating || loss.Rating >= b.Rating {
		t.Errorf("the winner has %v and the loser %v", win.Rating, loss.Rating)
	}
	near(t, "symmetry", win.Rating-a.Rating, b.Rating-loss.Rating, 1e-6)
	if win.Deviation >= a.Deviation {
		t.Errorf("a game did not make the rating more certain: %v", win.Deviation)
	}
	draw := rating.Update(a, []rating.Result{{Opponent: b, Score: 0.5}})
	near(t, "a draw between equals", draw.Rating, a.Rating, 1e-6)
}

// Beating somebody much stronger moves a rating further than beating somebody
// much weaker, and a settled rating moves less than a new one.
func TestWhatMovesARating(t *testing.T) {
	p := rating.Player{Rating: 1500, Deviation: 100, Volatility: 0.06}
	strong := rating.Player{Rating: 1900, Deviation: 60, Volatility: 0.06}
	weak := rating.Player{Rating: 1100, Deviation: 60, Volatility: 0.06}
	up := rating.Update(p, []rating.Result{{Opponent: strong, Score: 1}}).Rating - p.Rating
	little := rating.Update(p, []rating.Result{{Opponent: weak, Score: 1}}).Rating - p.Rating
	if up <= little || little <= 0 {
		t.Errorf("beating the strong %+v, beating the weak %+v", up, little)
	}
	fresh := rating.Player{Rating: 1500, Deviation: 350, Volatility: 0.06}
	if moved := rating.Update(fresh, []rating.Result{{Opponent: strong, Score: 1}}).Rating - fresh.Rating; moved <= up {
		t.Errorf("a new player moved %v and a settled one %v", moved, up)
	}
}

// Idleness makes a rating less certain, up to a new player's deviation, and does
// not change the rating.
func TestAgeing(t *testing.T) {
	p := rating.Player{Rating: 1700, Deviation: 60, Volatility: 0.06}
	if got := p.Aged(0); got != p {
		t.Errorf("no time changed the player: %+v", got)
	}
	month := p.Aged(30 * 24 * time.Hour)
	if month.Rating != p.Rating || month.Deviation <= p.Deviation {
		t.Errorf("after a month: %+v", month)
	}
	if ages := p.Aged(100 * 365 * 24 * time.Hour); ages.Deviation != rating.InitialDeviation {
		t.Errorf("after a century the deviation is %v", ages.Deviation)
	}
}

func TestWhatIsShown(t *testing.T) {
	if !rating.New().Provisional() {
		t.Error("a new player is not provisional")
	}
	if (rating.Player{Deviation: 100}).Provisional() {
		t.Error("a settled player is provisional")
	}
	if rating.Listed(rating.MinGames-1, rating.MinOpponents) || rating.Listed(rating.MinGames, rating.MinOpponents-1) || !rating.Listed(rating.MinGames, rating.MinOpponents) {
		t.Error("the bar for a leaderboard is wrong")
	}
}
