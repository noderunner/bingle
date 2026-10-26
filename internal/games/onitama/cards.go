package onitama

// Card is one Move card: the squares a piece may move to, as (right, forward)
// offsets from the point of view of the player holding it. Stamp is the
// colour of the stamp that decides who plays first when the card starts at
// the side of the mat: "red" or "blue".
type Card struct {
	ID    string
	Name  string
	Moves [][2]int
	Stamp string
}

// cards are the sixteen Move cards of the base game.
var cards = []Card{
	{"tiger", "Tiger", [][2]int{{0, 2}, {0, -1}}, "blue"},
	{"dragon", "Dragon", [][2]int{{-2, 1}, {2, 1}, {-1, -1}, {1, -1}}, "red"},
	{"frog", "Frog", [][2]int{{-2, 0}, {-1, 1}, {1, -1}}, "red"},
	{"rabbit", "Rabbit", [][2]int{{2, 0}, {1, 1}, {-1, -1}}, "blue"},
	{"crab", "Crab", [][2]int{{0, 1}, {-2, 0}, {2, 0}}, "blue"},
	{"elephant", "Elephant", [][2]int{{-1, 1}, {1, 1}, {-1, 0}, {1, 0}}, "red"},
	{"goose", "Goose", [][2]int{{-1, 1}, {-1, 0}, {1, 0}, {1, -1}}, "blue"},
	{"rooster", "Rooster", [][2]int{{1, 1}, {1, 0}, {-1, 0}, {-1, -1}}, "red"},
	{"monkey", "Monkey", [][2]int{{-1, 1}, {1, 1}, {-1, -1}, {1, -1}}, "blue"},
	{"mantis", "Mantis", [][2]int{{-1, 1}, {1, 1}, {0, -1}}, "red"},
	{"horse", "Horse", [][2]int{{0, 1}, {-1, 0}, {0, -1}}, "red"},
	{"ox", "Ox", [][2]int{{0, 1}, {1, 0}, {0, -1}}, "blue"},
	{"crane", "Crane", [][2]int{{0, 1}, {-1, -1}, {1, -1}}, "blue"},
	{"boar", "Boar", [][2]int{{0, 1}, {-1, 0}, {1, 0}}, "red"},
	{"eel", "Eel", [][2]int{{-1, 1}, {-1, -1}, {1, 0}}, "blue"},
	{"cobra", "Cobra", [][2]int{{-1, 0}, {1, 1}, {1, -1}}, "red"},
}

func cardByID(id string) (Card, bool) {
	for _, c := range cards {
		if c.ID == id {
			return c, true
		}
	}
	return Card{}, false
}
