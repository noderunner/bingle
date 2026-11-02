// Package rating is the arithmetic of player ratings: Glicko-2, as Mark
// Glickman describes it in "Example of the Glicko-2 system". It is pure, knows
// nothing of games or accounts, and has no dependency.
//
// Glicko-2 carries a deviation with each rating, how uncertain the rating is, so
// that somebody who plays four games a year moves differently from somebody who
// plays weekly, and is shown as provisional. Bingle treats each game as a rating
// period of its own, so a rating changes the moment a game ends, and grows the
// deviation by the time since the player's last game.
package rating

import (
	"math"
	"time"
)

const (
	// InitialRating, InitialDeviation and InitialVolatility are a new player's.
	InitialRating     = 1500.0
	InitialDeviation  = 350.0
	InitialVolatility = 0.06

	// ProvisionalDeviation is the deviation above which a rating is shown as
	// provisional.
	ProvisionalDeviation = 110.0
	// MinGames and MinOpponents are what a player needs for others to see their
	// rating: enough games, against enough different people.
	MinGames     = 5
	MinOpponents = 3
	// MaxGamesPerDay is how many games between the same two accounts, in one
	// game family, change their ratings in any 24 hours: two accounts cannot feed
	// each other wins all day.
	MaxGamesPerDay = 5

	// Period is how long a rating period is for growing a deviation: an idle
	// player's rating grows less certain by one period's worth a week.
	Period = 7 * 24 * time.Hour

	scale = 173.7178 // Glicko-2's scale, against the familiar 1500 one
	tau   = 0.5      // how far volatility may move; Glickman suggests 0.3 to 1.2
	// convergence is the precision of the volatility's iteration.
	convergence = 1e-6
)

// Player is a rating, its deviation and its volatility.
type Player struct {
	Rating     float64
	Deviation  float64
	Volatility float64
}

// New is a player nobody has seen yet.
func New() Player {
	return Player{Rating: InitialRating, Deviation: InitialDeviation, Volatility: InitialVolatility}
}

// Provisional says the rating is not settled enough to be read as a number.
func (p Player) Provisional() bool { return p.Deviation > ProvisionalDeviation }

// Listed says whether a player with that record may appear on a leaderboard.
func Listed(games, opponents int) bool { return games >= MinGames && opponents >= MinOpponents }

// Aged is the player after a time without a game: the rating is unchanged and
// the deviation has grown, never past a new player's.
func (p Player) Aged(idle time.Duration) Player {
	if idle <= 0 {
		return p
	}
	periods := float64(idle) / float64(Period)
	phi := p.Deviation / scale
	p.Deviation = math.Min(math.Sqrt(phi*phi+periods*p.Volatility*p.Volatility)*scale, InitialDeviation)
	return p
}

// Result is one game of a rating period: the opponent as they stood before it,
// and the score (1 a win, 0.5 a draw, 0 a loss).
type Result struct {
	Opponent Player
	Score    float64
}

// Update is the player after a rating period with those results.
func Update(p Player, results []Result) Player {
	mu := (p.Rating - InitialRating) / scale
	phi := p.Deviation / scale
	if len(results) == 0 {
		return Player{Rating: p.Rating, Deviation: math.Sqrt(phi*phi+p.Volatility*p.Volatility) * scale, Volatility: p.Volatility}
	}

	var vInverse, sum float64
	for _, r := range results {
		muJ := (r.Opponent.Rating - InitialRating) / scale
		phiJ := r.Opponent.Deviation / scale
		g := 1 / math.Sqrt(1+3*phiJ*phiJ/(math.Pi*math.Pi))
		e := 1 / (1 + math.Exp(-g*(mu-muJ)))
		vInverse += g * g * e * (1 - e)
		sum += g * (r.Score - e)
	}
	v := 1 / vInverse
	delta := v * sum

	sigma := newVolatility(phi, v, delta, p.Volatility)
	phiStar := math.Sqrt(phi*phi + sigma*sigma)
	phiNew := 1 / math.Sqrt(1/(phiStar*phiStar)+1/v)
	muNew := mu + phiNew*phiNew*sum
	return Player{
		Rating:     muNew*scale + InitialRating,
		Deviation:  math.Min(phiNew*scale, InitialDeviation),
		Volatility: sigma,
	}
}

// newVolatility is step 5 of the algorithm: the Illinois method on Glickman's f.
func newVolatility(phi, v, delta, sigma float64) float64 {
	a := math.Log(sigma * sigma)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		d := phi*phi + v + ex
		return ex*(delta*delta-phi*phi-v-ex)/(2*d*d) - (x-a)/(tau*tau)
	}
	lo := a
	var hi float64
	if delta*delta > phi*phi+v {
		hi = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*tau) < 0 {
			k++
		}
		hi = a - k*tau
	}
	fLo, fHi := f(lo), f(hi)
	for math.Abs(hi-lo) > convergence {
		c := lo + (lo-hi)*fLo/(fHi-fLo)
		fC := f(c)
		if fC*fHi <= 0 {
			lo, fLo = hi, fHi
		} else {
			fLo /= 2
		}
		hi, fHi = c, fC
	}
	return math.Exp(lo / 2)
}
