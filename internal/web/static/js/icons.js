// Unit icons of Cthulhu Wars: one small silhouette per kind of unit, drawn
// after the miniatures of the game, in a 100 × 100 box centred on the origin.
//
// An icon is a list of closed body paths (filled in the cult's colour and
// outlined), stroke-only detail paths, and dots (eyes and the like). Cultists
// share one hooded figure, tinted by the cult they belong to.

/** Path data of the unit icons, by unit id. */
export const UNIT_ICONS = {
	// A hooded, robed worshipper.
	cultist: {
		body: [
			"M 0 -46 C 13 -46 19 -34 17 -22 C 15 -14 12 -10 14 -2 C 18 10 30 24 33 46 L -33 46 C -30 24 -18 10 -14 -2 C -12 -10 -15 -14 -17 -22 C -19 -34 -13 -46 0 -46 Z",
		],
		detail: ["M -8 -26 C -8 -35 8 -35 8 -26 C 8 -17 4 -13 0 -13 C -4 -13 -8 -17 -8 -26 Z", "M -9 8 C -3 12 3 12 9 8", "M -19 30 L -23 46", "M 19 30 L 23 46"],
		dots: [
			[-3.6, -25, 1.9],
			[3.6, -25, 1.9],
		],
	},

	// Great Cthulhu: a squat fish-man.
	"deep-one": {
		body: [
			"M -42 16 C -48 -2 -36 -14 -22 -16 C -26 -28 -16 -36 -8 -30 C -4 -26 4 -26 8 -30 C 16 -36 26 -28 22 -16 C 36 -14 48 -2 42 16 C 40 26 30 30 20 26 L 15 36 L 9 27 L -9 27 L -15 36 L -20 26 C -30 30 -40 26 -42 16 Z",
		],
		detail: ["M -24 4 C -8 14 8 14 24 4", "M -38 2 L -48 -6", "M 38 2 L 48 -6", "M -12 -8 L -16 -2", "M 12 -8 L 16 -2"],
		dots: [
			[-14, -21, 4.4],
			[14, -21, 4.4],
		],
	},

	// An amorphous heap of eyes and mouths.
	shoggoth: {
		body: [
			"M -30 28 C -48 26 -48 6 -38 -2 C -46 -18 -30 -32 -16 -26 C -12 -42 10 -44 16 -28 C 32 -34 46 -16 38 -2 C 50 8 46 28 28 28 C 22 38 -4 40 -10 30 C -16 36 -26 36 -30 28 Z",
		],
		detail: ["M -16 16 C -8 24 8 24 16 16", "M -12 18 L -10 22", "M -4 20 L -3 25", "M 4 20 L 3 25", "M 12 18 L 10 22"],
		dots: [
			[-20, -8, 4.6],
			[2, -17, 3.6],
			[18, -5, 4.6],
			[-5, 3, 2.8],
			[28, 12, 2.8],
			[-30, 12, 2.8],
		],
	},

	// A winged, many-limbed crawler with a tentacled head.
	starspawn: {
		body: [
			"M -2 -6 C 0 -22 10 -38 32 -46 C 29 -36 31 -29 36 -23 C 31 -23 29 -19 29 -14 C 23 -19 20 -15 20 -9 C 14 -13 10 -9 8 -2 Z",
			"M -44 10 C -46 -4 -36 -14 -24 -14 C -20 -22 -8 -24 -2 -16 C 6 -12 18 -6 28 -2 C 42 4 42 22 24 26 L -20 26 C -34 26 -42 22 -44 10 Z",
		],
		detail: ["M -38 8 C -42 16 -46 20 -44 28", "M -30 10 C -32 20 -34 26 -30 34", "M -22 12 C -22 20 -20 28 -16 34", "M -4 26 L -6 38", "M 10 26 L 14 38", "M 22 26 L 30 36", "M 2 -12 C 8 -22 16 -30 26 -38"],
		dots: [[-28, -4, 3]],
	},

	// Great Cthulhu himself: tentacled head, bat wings, upright body.
	cthulhu: {
		body: [
			"M -7 -6 C -22 -26 -38 -38 -48 -44 C -47 -28 -43 -14 -35 -2 C -41 1 -45 7 -45 14 C -35 9 -27 9 -20 13 C -18 5 -12 0 -7 2 Z",
			"M 7 -6 C 22 -26 38 -38 48 -44 C 47 -28 43 -14 35 -2 C 41 1 45 7 45 14 C 35 9 27 9 20 13 C 18 5 12 0 7 2 Z",
			"M -10 -34 C -10 -44 10 -44 10 -34 L 10 -22 L 14 -6 L 12 22 L 17 46 L 4 46 L 0 30 L -4 46 L -17 46 L -12 22 L -14 -6 L -10 -22 Z",
		],
		detail: ["M -7 -26 C -10 -20 -9 -14 -10 -8", "M -2 -26 C -3 -19 -2 -13 -3 -7", "M 3 -26 C 4 -19 3 -13 4 -7", "M 8 -26 C 11 -20 10 -14 11 -8"],
		dots: [
			[-4.2, -36, 1.8],
			[4.2, -36, 1.8],
		],
	},

	// A faceless, horned flyer.
	nightgaunt: {
		body: [
			"M -5 -4 C -20 -20 -36 -28 -48 -26 C -42 -16 -40 -8 -36 0 C -30 -4 -24 -2 -21 4 C -16 0 -10 2 -6 10 Z",
			"M 5 -4 C 20 -20 36 -28 48 -26 C 42 -16 40 -8 36 0 C 30 -4 24 -2 21 4 C 16 0 10 2 6 10 Z",
			"M -7 -30 L -14 -44 L -4 -36 C -2 -38 2 -38 4 -36 L 14 -44 L 7 -30 C 10 -24 8 -16 6 -12 L 8 26 L 4 46 L -4 46 L -8 26 L -6 -12 C -8 -16 -10 -24 -7 -30 Z",
		],
		detail: ["M -6 4 C -2 8 2 8 6 4", "M -2 20 L -4 36", "M 2 20 L 4 36"],
		dots: [],
	},

	// A great toothed mouth on a floating sphere, a stalk on top.
	"flying-polyp": {
		body: [
			"M 0 -30 C 26 -30 42 -14 42 6 C 42 28 24 40 0 40 C -24 40 -42 28 -42 6 C -42 -14 -26 -30 0 -30 Z",
			"M -3 -30 C -6 -40 4 -48 12 -42 C 6 -42 6 -36 5 -30 Z",
		],
		detail: [
			"M -24 6 C -14 -10 14 -10 24 6 C 14 24 -14 24 -24 6 Z",
			"M -17 4 L -13 11 L -9 4 L -5 12 L -1 4 L 3 12 L 7 4 L 11 11 L 15 4",
			"M -34 -8 C -30 -16 -24 -20 -18 -22",
		],
		dots: [],
	},

	// A winged serpent that never stops.
	"hunting-horror": {
		body: [
			"M -6 -2 C -20 -18 -36 -24 -48 -20 C -42 -12 -40 -6 -36 2 C -30 -2 -24 0 -20 7 C -15 3 -10 5 -6 10 Z",
			"M 6 -2 C 20 -18 36 -24 48 -20 C 42 -12 40 -6 36 2 C 30 -2 24 0 20 7 C 15 3 10 5 6 10 Z",
			"M -6 -14 C -6 -24 6 -24 6 -14 L 8 8 C 16 18 14 32 4 38 C -6 44 -18 38 -14 30 C -10 24 -2 28 0 32 C 4 28 -4 18 -8 8 Z",
		],
		detail: ["M -2 -12 L 0 -8 L 2 -12", "M -30 -10 C -26 -2 -22 2 -16 6", "M 30 -10 C 26 -2 22 2 16 6"],
		dots: [[0, -17, 1.6]],
	},

	// The howling messenger: a slender figure with grasping claws and a tail.
	nyarlathotep: {
		body: [
			"M 0 -46 C 7 -46 9 -39 7 -31 L 5 -23 C 13 -27 22 -35 28 -31 C 30 -26 24 -21 18 -15 C 24 -17 32 -15 31 -9 C 27 -5 19 -7 13 -11 L 9 -3 C 12 6 8 14 2 20 C -4 26 -2 32 6 34 C 14 36 18 42 14 48 C 8 40 -4 40 -10 32 C -14 24 -10 16 -6 8 C -10 0 -9 -3 -9 -3 L -13 -11 C -19 -7 -27 -5 -31 -9 C -32 -15 -24 -17 -18 -15 C -24 -21 -30 -26 -28 -31 C -22 -35 -13 -27 -5 -23 L -7 -31 C -9 -39 -7 -46 0 -46 Z",
		],
		detail: ["M 0 -38 L 0 -10", "M -28 -30 L -34 -38", "M 28 -30 L 34 -38", "M -2 14 L 4 12", "M 0 24 L 6 22"],
		dots: [[0, -31, 2.2]],
	},

	// A hunched, long-armed eater of the dead.
	ghoul: {
		body: [
			"M -34 -6 C -34 -22 -20 -32 -8 -28 C -2 -38 12 -40 18 -30 C 28 -28 32 -16 28 -6 L 37 14 L 30 19 L 22 7 L 21 25 L 25 46 L 10 46 L 6 30 L -2 46 L -16 46 L -14 25 L -22 13 L -28 19 L -36 14 Z",
		],
		detail: ["M 6 -20 L 12 -14", "M -10 -8 C -4 -2 4 -2 10 -8", "M 22 7 L 30 19"],
		dots: [[12, -23, 2.2]],
	},

	// A many-legged crustacean of Yuggoth.
	fungi: {
		body: [
			"M 0 -30 L 9 -42 L 12 -26 L 28 -36 L 22 -18 L 42 -18 L 28 -4 L 44 8 L 24 10 L 32 28 L 14 20 L 12 40 L 0 26 L -12 40 L -14 20 L -32 28 L -24 10 L -44 8 L -28 -4 L -42 -18 L -22 -18 L -28 -36 L -12 -26 L -9 -42 Z",
		],
		detail: ["M -16 -2 C -8 -12 8 -12 16 -2 C 8 10 -8 10 -16 -2 Z"],
		dots: [
			[-6, -3, 3],
			[6, -3, 3],
		],
	},

	// A walking tree with a canopy of tentacles on three hooved legs.
	"dark-young": {
		body: [
			"M -8 22 C -28 20 -44 8 -46 -12 C -36 -6 -28 -8 -24 -16 C -32 -28 -24 -42 -13 -40 C -9 -48 9 -48 13 -40 C 24 -42 32 -28 24 -16 C 28 -8 36 -6 46 -12 C 44 8 28 20 8 22 L 18 46 L 6 46 L 0 32 L -6 46 L -18 46 Z",
		],
		detail: ["M -20 -20 C -14 -14 -10 -20 -6 -28", "M 20 -20 C 14 -14 10 -20 6 -28", "M 0 -34 C -4 -26 4 -22 0 -14", "M -30 -4 C -22 -2 -16 -6 -12 -12", "M 30 -4 C 22 -2 16 -6 12 -12"],
		dots: [
			[-5, -2, 2.2],
			[5, -2, 2.2],
		],
	},

	// The Black Goat of the Woods: a vast shape with tentacles and hooves.
	"shub-niggurath": {
		body: [
			"M -44 20 C -48 0 -34 -14 -18 -16 C -22 -28 -12 -34 -6 -28 L -3 -44 L 3 -28 C 9 -34 20 -28 16 -16 C 34 -14 48 0 44 20 C 44 28 40 34 34 34 L 30 46 L 22 34 L -22 34 L -30 46 L -34 34 C -40 34 -44 28 -44 20 Z",
		],
		detail: ["M -26 -22 C -34 -30 -40 -26 -42 -34", "M 26 -22 C 34 -30 40 -26 42 -34", "M -16 4 C -6 14 6 14 16 4", "M -28 10 L -32 20", "M 28 10 L 32 20", "M -10 -4 L -10 2", "M 10 -4 L 10 2"],
		dots: [
			[-14, -8, 3.2],
			[14, -8, 3.2],
			[0, 0, 2.4],
		],
	},

	// A shambling dead thing with its arms flung up.
	undead: {
		body: [
			"M 0 -42 C 9 -42 13 -34 11 -26 L 13 -19 C 21 -30 29 -40 35 -37 C 39 -33 28 -21 21 -11 L 19 -1 L 23 14 L 19 46 L 6 46 L 4 24 L -4 24 L -6 46 L -19 46 L -23 14 L -19 -1 L -21 -11 C -28 -21 -39 -33 -35 -37 C -29 -40 -21 -30 -13 -19 L -11 -26 C -13 -34 -9 -42 0 -42 Z",
		],
		detail: ["M -5 -30 L -2 -26 L 2 -30", "M -8 2 L -4 8 L 0 2 L 4 8 L 8 2", "M -4 24 L 0 36 L 4 24"],
		dots: [
			[-4, -33, 1.8],
			[4, -33, 1.8],
		],
	},

	// A crow-headed flyer of the dark between the stars.
	byakhee: {
		body: [
			"M -10 -4 C -26 -24 -40 -34 -50 -32 C -45 -20 -43 -10 -39 -2 C -33 -6 -27 -4 -25 2 C -20 -2 -14 0 -10 6 Z",
			"M 3 -26 C 3 -36 15 -38 18 -30 L 30 -26 L 18 -20 C 18 -12 14 -4 10 4 L 16 12 C 24 6 36 4 44 10 C 38 14 32 18 26 24 C 24 32 16 40 8 40 L 10 46 L 0 42 L -8 46 L -6 38 C -14 32 -14 20 -10 12 C -10 4 -4 -6 3 -12 Z",
		],
		detail: ["M 22 -26 L 30 -26", "M 12 22 C 20 18 28 18 36 16"],
		dots: [[11, -28, 1.9]],
	},

	// The Pallid Mask: a tall tattered figure in a crown.
	"king-in-yellow": {
		body: [
			"M 0 -47 L 5 -38 L 10 -45 L 11 -32 C 14 -27 12 -19 8 -15 L 16 -9 L 40 -23 L 33 -4 L 18 6 L 25 46 L 14 39 L 8 47 L 0 39 L -8 47 L -14 39 L -25 46 L -18 6 L -33 -4 L -40 -23 L -16 -9 L -8 -15 C -12 -19 -14 -27 -11 -32 L -10 -45 L -5 -38 Z",
		],
		detail: ["M -6 -28 L 6 -28", "M 0 -26 L 0 -18", "M -12 8 L -14 30", "M 12 8 L 14 30", "M -5 -12 L 5 -12"],
		dots: [
			[-4, -31, 1.7],
			[4, -31, 1.7],
		],
	},

	// --- Windwalker -------------------------------------------------------------------

	// A gaunt, antlered figure bent over its hunger.
	wendigo: {
		body: [
			"M -6 -30 C -10 -38 -4 -44 0 -44 C 4 -44 10 -38 6 -30 C 12 -26 15 -16 13 -6 L 20 8 L 15 11 L 9 2 L 11 18 L 17 46 L 8 46 L 2 26 L -2 26 L -8 46 L -17 46 L -11 18 L -9 2 L -15 11 L -20 8 L -13 -6 C -15 -16 -12 -26 -6 -30 Z",
		],
		detail: [
			"M -4 -42 C -10 -44 -15 -40 -16 -34",
			"M -10 -43 L -14 -49",
			"M 4 -42 C 10 -44 15 -40 16 -34",
			"M 10 -43 L 14 -49",
			"M -6 -14 L 6 -14",
			"M -5 -8 L 5 -8",
			"M -3 -24 C -1 -22 1 -22 3 -24",
		],
		dots: [
			[-2.6, -34, 1.7],
			[2.6, -34, 1.7],
		],
	},

	// A shaggy, horned beast of the ice.
	"gnoph-keh": {
		body: [
			"M -36 30 C -46 10 -42 -10 -28 -20 L -36 -36 C -30 -33 -25 -30 -20 -27 C -14 -35 14 -35 20 -27 C 25 -30 30 -33 36 -36 L 28 -20 C 42 -10 46 10 36 30 L 28 46 L 12 46 L 4 38 L 0 46 L -4 38 L -12 46 L -28 46 Z",
		],
		detail: [
			"M -26 -4 L -32 2",
			"M -22 8 L -29 14",
			"M 26 -4 L 32 2",
			"M 22 8 L 29 14",
			"M -12 8 C -5 16 5 16 12 8",
			"M -7 8 L -6 13",
			"M 0 9 L 0 14",
			"M 7 8 L 6 13",
			"M -12 -20 L -8 -26",
			"M 12 -20 L 8 -26",
		],
		dots: [
			[-11, -9, 3.2],
			[11, -9, 3.2],
		],
	},

	// Harbinger of the Great Old Ones: squat, tusked, crab-clawed, three-eyed.
	"rhan-tegoth": {
		body: [
			"M -24 -12 C -24 -30 24 -30 24 -12 C 34 -6 36 14 28 30 C 22 40 10 44 0 44 C -10 44 -22 40 -28 30 C -36 14 -34 -6 -24 -12 Z",
			"M -26 -4 C -38 -2 -48 -12 -48 -26 C -42 -22 -38 -18 -34 -20 C -38 -28 -34 -36 -26 -38 C -30 -30 -26 -22 -22 -18 Z",
			"M 26 -4 C 38 -2 48 -12 48 -26 C 42 -22 38 -18 34 -20 C 38 -28 34 -36 26 -38 C 30 -30 26 -22 22 -18 Z",
		],
		detail: ["M -9 10 C -14 20 -12 30 -7 38", "M 9 10 C 14 20 12 30 7 38", "M -10 8 C -4 14 4 14 10 8"],
		dots: [
			[-9, -14, 3.4],
			[0, -19, 3.4],
			[9, -14, 3.4],
		],
	},

	// God of the Wind: a towering shape in a ragged storm of cloak.
	ithaqua: {
		body: [
			"M 0 -46 C 9 -46 13 -37 11 -29 L 24 -17 C 38 -11 45 0 47 13 C 38 6 31 6 25 10 C 30 19 31 31 27 44 C 21 36 16 34 10 36 L 6 46 L 0 35 L -6 46 L -10 36 C -16 34 -21 36 -27 44 C -31 31 -30 19 -25 10 C -31 6 -38 6 -47 13 C -45 0 -38 -11 -24 -17 L -11 -29 C -13 -37 -9 -46 0 -46 Z",
		],
		detail: [
			"M -38 -4 C -32 -8 -26 -5 -22 1",
			"M 38 -4 C 32 -8 26 -5 22 1",
			"M -14 6 C -8 12 8 12 14 6",
			"M -20 22 C -12 28 12 28 20 22",
			"M 0 -22 L 0 -8",
		],
		dots: [
			[-4.4, -31, 2.2],
			[4.4, -31, 2.2],
		],
	},

	// --- Sleeper ----------------------------------------------------------------------

	// A robed occultist in a pointed hat, a familiar at its feet.
	wizard: {
		body: [
			"M 0 -48 L 9 -30 L 20 -26 L 20 -22 L -20 -22 L -20 -26 L -9 -30 Z",
			"M -9 -22 C -11 -14 -9 -8 -12 -2 C -18 10 -28 24 -30 44 L 30 44 C 28 24 18 10 12 -2 C 9 -8 11 -14 9 -22 Z",
		],
		detail: ["M -6 -12 C -3 -9 3 -9 6 -12", "M -14 22 L -18 44", "M 14 22 L 18 44", "M 24 -4 L 24 44", "M 0 -38 L 0 -34"],
		dots: [
			[-3.4, -17, 1.7],
			[3.4, -17, 1.7],
			[34, 36, 3.2],
		],
	},

	// An ancient serpent with a man's body: a hooded snake's head, a coiled tail.
	"serpent-man": {
		body: [
			"M 0 -46 C 9 -46 13 -39 11 -31 C 17 -27 18 -19 13 -15 C 19 -9 21 3 16 13 C 22 17 26 25 25 33 C 23 41 15 43 8 40 C 14 36 16 31 12 28 C 6 26 -2 29 -8 35 C -16 43 -30 43 -36 36 C -40 30 -34 24 -26 26 C -18 28 -12 24 -9 17 C -14 9 -14 -7 -12 -15 C -17 -19 -16 -27 -10 -31 C -12 -39 -9 -46 0 -46 Z",
		],
		detail: ["M -6 -24 C -2 -20 2 -20 6 -24", "M 0 -20 L 0 -16", "M -8 -8 L 8 -8", "M -9 4 L 9 4", "M -6 16 L 8 16", "M -28 31 C -22 33 -16 31 -12 27"],
		dots: [
			[-4.2, -35, 2],
			[4.2, -35, 2],
		],
	},

	// A boiling mass of black slime, tendrils reaching.
	"formless-spawn": {
		body: [
			"M -38 30 C -49 12 -44 -12 -27 -21 C -23 -36 -3 -42 11 -33 C 29 -39 45 -22 39 -4 C 49 8 45 28 29 33 C 15 41 -10 41 -21 35 C -29 37 -35 35 -38 30 Z",
		],
		detail: ["M 33 -22 C 41 -30 42 -40 36 -46", "M -30 -16 C -40 -24 -42 -34 -36 -42", "M -4 -34 C -4 -42 0 -46 6 -46", "M -14 20 C -6 26 6 26 14 20"],
		dots: [
			[-12, -6, 4],
			[8, -12, 3],
			[16, 4, 3.4],
		],
	},

	// The Sleeper: a swollen, drowsy toad-bat with a vast mouth.
	tsathoggua: {
		body: [
			"M -34 32 C -48 20 -46 -4 -34 -14 C -38 -26 -30 -36 -18 -34 L -24 -49 L -10 -41 C -4 -43 4 -43 10 -41 L 24 -49 L 18 -34 C 30 -36 38 -26 34 -14 C 46 -4 48 20 34 32 L 40 46 L 22 43 L 16 36 C 6 41 -6 41 -16 36 L -22 43 L -40 46 Z",
		],
		detail: ["M -22 -6 C -8 6 8 6 22 -6", "M -16 18 C -6 27 6 27 16 18", "M -22 -20 L -12 -22", "M 22 -20 L 12 -22", "M -26 0 L -32 4", "M 26 0 L 32 4"],
		dots: [
			[-12, -17, 1.6],
			[12, -17, 1.6],
		],
	},

	// --- High Priests -----------------------------------------------------------------

	// A mitred figure in a long robe, a staff in his hand.
	"high-priest": {
		body: [
			"M 0 -46 L 13 -26 C 16 -17 12 -11 14 -2 C 18 10 30 24 34 46 L -34 46 C -30 24 -18 10 -14 -2 C -12 -11 -16 -17 -13 -26 Z",
		],
		detail: ["M -8 -22 C -8 -30 8 -30 8 -22 C 8 -15 4 -11 0 -11 C -4 -11 -8 -15 -8 -22 Z", "M -14 6 L 14 6", "M 0 -46 L 0 -38", "M -4 -42 L 4 -42", "M 28 -20 L 28 46", "M 24 -24 L 32 -24"],
		dots: [
			[-3.2, -21, 1.7],
			[3.2, -21, 1.7],
		],
	},

	// Hastur the Unspeakable: a vast head hung with tentacles.
	hastur: {
		body: [
			"M 0 -44 C 22 -44 36 -30 36 -12 C 36 0 30 8 24 12 C 30 20 32 30 28 42 C 22 36 20 30 16 26 C 14 36 12 44 6 48 C 4 40 2 32 0 26 C -2 32 -4 40 -6 48 C -12 44 -14 36 -16 26 C -20 30 -22 36 -28 42 C -32 30 -30 20 -24 12 C -30 8 -36 0 -36 -12 C -36 -30 -22 -44 0 -44 Z",
		],
		detail: ["M -22 -4 C -14 -10 -6 -8 -4 -2", "M 22 -4 C 14 -10 6 -8 4 -2", "M -10 6 C -4 14 4 14 10 6", "M 36 -12 C 44 -14 46 -6 42 0"],
		dots: [
			[-14, -10, 3.6],
			[14, -10, 3.6],
		],
	},

	// Dimensional Shambler: a folded shard of space with a gap in it.
	"dimensional-shambler": {
		body: [
			"M -6 -46 L 22 -26 L 14 -6 L 40 6 L 18 40 L 0 24 L -22 44 L -30 14 L -44 -4 L -20 -14 Z",
		],
		detail: ["M -6 -46 L -2 -10 L 14 -6", "M -2 -10 L -20 -14", "M -2 -10 L 0 24", "M -30 14 L -2 -10"],
		dots: [
			[10, -16, 3],
			[-12, 6, 3],
		],
	},

	// Star Vampire: a bloated bat-winged body with a long proboscis.
	"star-vampire": {
		body: [
			"M 0 -30 C 8 -30 12 -24 12 -16 C 26 -34 44 -34 48 -22 C 40 -20 38 -12 40 -4 C 32 -8 26 -4 24 4 C 20 -2 14 -2 12 4 C 14 18 8 28 0 28 C -8 28 -14 18 -12 4 C -14 -2 -20 -2 -24 4 C -26 -4 -32 -8 -40 -4 C -38 -12 -40 -20 -48 -22 C -44 -34 -26 -34 -12 -16 C -12 -24 -8 -30 0 -30 Z",
		],
		detail: ["M 0 -6 L 0 44", "M -4 38 L 0 46 L 4 38", "M -12 4 L -20 -10", "M 12 4 L 20 -10"],
		dots: [
			[-5, -18, 2.6],
			[5, -18, 2.6],
		],
	},

	// Elder Thing: a barrel body between a star-shaped head and a fan of tentacles.
	"elder-thing": {
		body: [
			"M -16 -14 C -16 -26 16 -26 16 -14 L 16 22 C 16 32 -16 32 -16 22 Z",
			"M 0 -48 L 5 -34 L 18 -38 L 10 -27 L 22 -22 L 8 -20 L 0 -16 L -8 -20 L -22 -22 L -10 -27 L -18 -38 L -5 -34 Z",
		],
		detail: ["M -16 -4 L -36 -10", "M 16 -4 L 36 -10", "M -16 8 L -38 16", "M 16 8 L 38 16", "M -8 32 L -12 46", "M 0 32 L 0 46", "M 8 32 L 12 46"],
		dots: [
			[0, -32, 2.6],
		],
	},

	// Servitor of the Outer Gods: a wavering robed shape, face a blank hood.
	servitor: {
		body: [
			"M 0 -44 C 16 -44 20 -26 14 -12 C 22 -2 28 18 34 36 C 24 32 22 44 12 38 C 6 46 -6 46 -12 38 C -22 44 -24 32 -34 36 C -28 18 -22 -2 -14 -12 C -20 -26 -16 -44 0 -44 Z",
		],
		detail: ["M -10 -26 C -4 -20 4 -20 10 -26", "M 0 -4 L 0 30", "M -12 12 C -6 8 6 8 12 12"],
		dots: [
			[-5, -32, 2],
			[5, -32, 2],
		],
	},

	// Moonbeast: a pale, slippery dome that can swell and shrink, with a tentacled mouth.
	moonbeast: {
		body: [
			"M -42 22 C -46 -2 -30 -28 -4 -30 C 18 -32 38 -18 42 6 C 45 18 38 28 26 30 L -24 30 C -34 30 -41 27 -42 22 Z",
		],
		detail: ["M -24 30 C -28 40 -22 46 -16 40", "M -8 30 C -10 42 -2 47 3 39", "M 10 30 C 10 42 20 46 22 36", "M 26 30 L 34 42", "M -22 -4 C -10 -14 8 -14 20 -4"],
		dots: [
			[-14, -8, 3.2],
			[2, -12, 3.6],
			[18, -8, 3.2],
		],
	},

	// Gnorri: an upright amphibian with a fin crest, gills and webbed hands.
	gnorri: {
		body: [
			"M 0 -46 L 8 -32 L 18 -38 L 16 -22 C 28 -16 30 -2 24 10 C 30 20 30 36 22 46 L 8 46 L 0 34 L -8 46 L -22 46 C -30 36 -30 20 -24 10 C -30 -2 -28 -16 -16 -22 L -18 -38 L -8 -32 Z",
		],
		detail: ["M -12 -8 C -4 -4 4 -4 12 -8", "M -22 4 L -30 0", "M 22 4 L 30 0", "M -22 10 L -32 8", "M 22 10 L 32 8", "M 0 -32 L 0 -20"],
		dots: [
			[-8, -18, 3.2],
			[8, -18, 3.2],
		],
	},

	// Shantak: a scaled steed with a bird's head and wings spread to fly.
	shantak: {
		body: [
			"M -46 -22 C -34 -36 -18 -32 -8 -20 L -2 -34 L 6 -46 L 12 -32 L 28 -26 L 18 -18 C 22 -6 20 6 12 12 L 14 32 L 8 46 L 2 46 L 2 24 L -10 24 L -12 46 L -20 46 L -20 20 C -34 14 -44 0 -46 -22 Z",
		],
		detail: ["M -40 -14 C -30 -24 -20 -22 -14 -14", "M -36 -4 C -28 -12 -20 -10 -14 -4", "M 12 -32 L 22 -36"],
		dots: [[10, -26, 2.4]],
	},

	// Wamp: a pale, segmented cave-dweller with a round mouth.
	wamp: {
		body: [
			"M -44 10 C -44 -8 -26 -12 -14 -2 C -6 -12 8 -14 16 -4 C 26 -14 42 -8 44 6 C 46 22 30 30 20 22 C 12 30 -2 30 -10 22 C -20 30 -40 26 -44 10 Z",
		],
		detail: ["M -14 -2 L -14 24", "M 16 -4 L 16 24", "M 30 4 C 34 0 40 2 40 8"],
		dots: [
			[34, -2, 2.6],
			[-28, 6, 2.2],
			[0, 8, 2.2],
		],
	},

	// Voonith: a round shell on six jointed legs ending in claws.
	voonith: {
		body: [
			"M 0 -36 C 22 -36 30 -20 28 -6 C 26 8 14 14 0 14 C -14 14 -26 8 -28 -6 C -30 -20 -22 -36 0 -36 Z",
		],
		detail: ["M -26 0 L -40 18 L -34 36", "M 26 0 L 40 18 L 34 36", "M -14 12 L -22 40 L -12 48", "M 14 12 L 22 40 L 12 48", "M -40 18 L -48 10", "M -40 18 L -48 24", "M 40 18 L 48 10", "M 40 18 L 48 24", "M -12 -14 C -4 -8 4 -8 12 -14"],
		dots: [
			[-9, -22, 3],
			[9, -22, 3],
		],
	},

	// Hound of Tindalos: a jagged skeleton of a dog, all angles.
	"hound-of-tindalos": {
		body: [
			"M -46 -10 L -34 -24 L -22 -22 L -10 -30 L 4 -22 L 20 -20 L 34 -30 L 38 -18 L 28 -10 L 36 4 L 44 30 L 36 32 L 28 14 L 14 12 L 8 34 L -2 34 L -2 14 L -16 16 L -24 34 L -34 34 L -30 8 L -42 6 Z",
		],
		detail: ["M -6 -14 L -2 8", "M 4 -14 L 8 6", "M 14 -12 L 18 4", "M -46 -10 L -38 -4", "M 34 -30 L 44 -40"],
		dots: [[-30, -16, 2.6]],
	},

	// Abhoth: a living pool of spawning slime, eyes and mouths among the spawn.
	abhoth: {
		body: [
			"M -40 24 C -50 4 -36 -14 -22 -16 C -26 -32 -6 -40 6 -30 C 22 -38 40 -22 32 -6 C 46 4 46 24 30 32 C 20 42 -4 44 -14 36 C -26 42 -38 36 -40 24 Z",
		],
		detail: ["M -14 36 C -18 44 -8 48 -6 42", "M 18 38 C 22 46 32 44 30 36", "M -22 10 C -12 16 -2 14 6 8", "M 10 -2 C 18 4 26 2 28 -6"],
		dots: [
			[-18, -2, 5],
			[8, -14, 4],
			[22, 8, 5],
			[-4, 18, 4],
			[-28, 14, 3],
			[14, 26, 3.4],
		],
	},

	// Chaugnar Faugn: a squat, elephant-headed idol on a throne, tusks at the trunk.
	"chaugnar-faugn": {
		body: [
			"M 0 -46 C 14 -46 22 -38 22 -26 C 36 -30 46 -18 40 -6 C 36 2 28 2 24 -2 C 24 8 20 18 14 22 L 14 46 L -14 46 L -14 22 C -20 18 -24 8 -24 -2 C -28 2 -36 2 -40 -6 C -46 -18 -36 -30 -22 -26 C -22 -38 -14 -46 0 -46 Z",
		],
		detail: ["M 0 -16 C -2 -2 -2 10 6 22 C 10 27 15 23 12 19", "M -10 -6 L -15 6", "M 10 -6 L 15 6", "M -14 30 L 14 30"],
		dots: [
			[-9, -24, 2.8],
			[9, -24, 2.8],
		],
	},

	// Cthugha: a living flame, a smaller fire in its heart.
	cthugha: {
		body: [
			"M 0 -48 C 8 -32 24 -26 26 -8 C 36 -18 34 -34 30 -42 C 46 -26 48 0 40 18 C 32 40 14 48 0 48 C -14 48 -32 40 -40 18 C -48 0 -46 -26 -30 -42 C -34 -34 -36 -18 -26 -8 C -24 -26 -8 -32 0 -48 Z",
		],
		detail: ["M 0 -14 C 8 -2 14 6 10 18 C 8 28 -8 28 -10 18 C -14 6 -8 -2 0 -14 Z", "M 0 4 C 3 10 3 16 0 20"],
		dots: [],
	},

	// Mother Hydra: three serpent heads on one broad, scaled body.
	"mother-hydra": {
		body: [
			"M -42 46 C -38 22 -28 10 -24 -10 C -28 -20 -32 -32 -24 -38 C -16 -44 -10 -34 -14 -24 C -10 -16 -6 -6 0 4 C 6 -6 10 -16 14 -24 C 10 -34 16 -44 24 -38 C 32 -32 28 -20 24 -10 C 28 10 38 22 42 46 Z",
			"M -6 6 C -9 -10 -7 -30 0 -40 C 7 -30 9 -10 6 6 Z",
		],
		detail: ["M -20 20 C -10 26 10 26 20 20", "M -26 32 C -12 40 12 40 26 32", "M -18 -34 L -26 -36", "M 18 -34 L 26 -36"],
		dots: [
			[-17, -32, 2.2],
			[17, -32, 2.2],
			[0, -32, 2.2],
		],
	},

	// Yig: a rattlesnake coiled under a flared hood.
	yig: {
		body: [
			"M 0 -46 C 12 -46 20 -38 20 -26 C 20 -18 14 -14 10 -10 C 22 -6 30 6 24 18 C 34 24 36 38 24 44 C 10 50 -24 50 -34 40 C -42 30 -34 20 -22 20 C -30 12 -24 0 -12 -4 C -16 -10 -20 -18 -20 -26 C -20 -38 -12 -46 0 -46 Z",
		],
		detail: ["M -14 -16 C -8 -10 8 -10 14 -16", "M -20 20 C -10 26 10 28 24 18", "M -26 32 C -12 40 12 42 28 36", "M 0 -26 L 0 -34 M -3 -34 L 0 -40 L 3 -34"],
		dots: [
			[-6, -26, 2.4],
			[6, -26, 2.4],
		],
	},

	// Filth: a small drip of living slime.
	filth: {
		body: [
			"M -30 24 C -40 10 -30 -6 -16 -8 C -18 -22 -2 -28 6 -18 C 16 -26 32 -14 26 -2 C 38 6 34 22 22 26 C 12 32 -20 32 -30 24 Z",
		],
		detail: ["M -14 28 C -16 38 -8 40 -8 32", "M 12 28 C 12 38 20 40 20 30"],
		dots: [
			[-10, 4, 3.4],
			[6, -4, 3],
			[16, 12, 3.4],
		],
	},

	// Mutant: a hunched, half-changed worshipper with one swollen arm.
	mutant: {
		body: [
			"M -4 -44 C 10 -44 16 -32 12 -20 C 10 -12 14 -6 22 -2 C 34 4 40 18 34 30 C 30 40 24 44 18 46 L -22 46 C -26 34 -20 24 -14 14 C -18 8 -16 -4 -12 -10 C -16 -20 -14 -44 -4 -44 Z",
		],
		detail: ["M -8 -26 C -4 -22 4 -22 8 -26", "M 22 2 C 30 10 30 22 24 32", "M -12 30 L -16 46"],
		dots: [
			[-5, -33, 2.2],
			[5, -33, 2.2],
			[28, 20, 2],
		],
	},

	// Abomination: a taller hybrid with a clawed arm and a crest.
	abomination: {
		body: [
			"M 0 -48 L 8 -38 L 16 -40 L 14 -28 C 22 -22 22 -10 16 -2 C 28 6 40 22 36 46 L -36 46 C -40 22 -28 6 -16 -2 C -22 -10 -22 -22 -14 -28 L -16 -40 L -8 -38 Z",
		],
		detail: ["M -10 -22 C -4 -16 4 -16 10 -22", "M 36 18 L 46 10 M 36 24 L 48 22 M 34 30 L 46 34", "M -14 22 L -20 46", "M 14 22 L 20 46"],
		dots: [
			[-6, -30, 2.4],
			[6, -30, 2.4],
		],
	},

	// Spawn of Yog-Sothoth: a swollen body with a cluster of glowing spheres.
	"spawn-of-yog-sothoth": {
		body: [
			"M -20 40 C -42 34 -44 6 -28 -6 C -34 -22 -18 -38 0 -34 C 18 -38 34 -22 28 -6 C 44 6 42 34 20 40 C 10 46 -10 46 -20 40 Z",
		],
		detail: ["M -22 -6 C -12 0 12 0 22 -6", "M -26 18 C -12 26 12 26 26 18"],
		dots: [
			[-14, -16, 5],
			[2, -22, 5.5],
			[16, -12, 4.5],
			[-6, 4, 4],
			[10, 12, 4.5],
			[-16, 16, 3.6],
		],
	},

	// Yog-Sothoth: a cluster of luminous spheres, the Gate itself.
	"yog-sothoth": {
		body: [
			"M 0 -46 C 14 -46 20 -34 18 -24 C 30 -30 44 -22 42 -8 C 40 2 32 6 28 8 C 38 14 40 30 28 36 C 20 40 12 38 8 34 C 8 44 -8 44 -8 34 C -12 38 -20 40 -28 36 C -40 30 -38 14 -28 8 C -32 6 -40 2 -42 -8 C -44 -22 -30 -30 -18 -24 C -20 -34 -14 -46 0 -46 Z",
		],
		detail: ["M -12 -4 C -6 4 6 4 12 -4", "M 0 -30 L 0 -14", "M -26 -6 L -16 -4", "M 26 -6 L 16 -4"],
		dots: [
			[0, -34, 5],
			[-24, -16, 4.4],
			[24, -16, 4.4],
			[-18, 22, 4.6],
			[18, 22, 4.6],
			[0, 8, 6],
		],
	},

	// Proto-Shoggoth: a small shapeless heap that wears a face.
	"proto-shoggoth": {
		body: [
			"M -30 30 C -44 24 -40 6 -30 0 C -38 -14 -22 -28 -10 -22 C -6 -38 14 -38 18 -22 C 32 -26 42 -8 34 2 C 44 10 38 30 24 30 C 18 38 -4 40 -10 32 C -16 36 -26 36 -30 30 Z",
		],
		detail: ["M -12 10 C -4 16 8 16 14 10", "M -16 -2 L -20 4", "M 16 -2 L 20 4"],
		dots: [
			[-9, -6, 3.6],
			[9, -6, 3.6],
			[0, -14, 2.4],
		],
	},

	// Ubbo-Sathla: a vast sprawl of cells, a great eye in the middle of the mass.
	"ubbo-sathla": {
		body: [
			"M -46 22 C -50 4 -36 -8 -24 -10 C -28 -26 -10 -40 4 -34 C 18 -42 36 -28 30 -12 C 44 -8 50 10 42 24 C 40 38 24 42 12 38 C 2 46 -16 46 -22 38 C -34 42 -44 36 -46 22 Z",
		],
		detail: ["M -20 -16 C -14 -22 -6 -22 -2 -18", "M 14 -20 C 22 -22 28 -16 26 -10", "M -32 14 C -22 22 -10 22 -4 16", "M 8 18 C 18 24 30 22 36 14"],
		dots: [
			[0, 0, 11],
			[-26, 4, 4.4],
			[24, 4, 4.4],
			[-10, -26, 3.4],
			[16, -28, 3.4],
		],
	},
};

/** The units' classes, smallest to largest, and how big each is drawn. */
export const CLASS_SCALE = { cultist: 0.78, monster: 0.9, terror: 0.95, goo: 1 };

/** The class of a piece definition: its first tag that names one. */
export function classOfPiece(def) {
	if (!def) return "monster";
	for (const t of def.tags || []) if (t in CLASS_SCALE) return t;
	return def.shape in CLASS_SCALE ? def.shape : "monster";
}

/** True for pieces drawn from this table. */
export function hasUnitIcon(shape) {
	return Object.prototype.hasOwnProperty.call(UNIT_ICONS, shape);
}

/**
 * Small pictograms of the numbers on a cult's card, in a 24 × 24 box: they
 * stand for the words Power, Doom, Elder Signs, Gates and Spellbooks. Each is a
 * list of filled paths (`fill`) and stroke-only paths (`line`); both take the
 * colour of the surrounding text.
 */
export const STAT_ICONS = {
	// A lightning bolt.
	power: { fill: ["M13.5 1.5 4 13.5h6.6L9.5 22.5 20 9.8h-6.8z"] },
	// A skull.
	doom: {
		fill: ["M12 2C7 2 3.5 5.6 3.5 10.2c0 2.7 1.3 4.8 3.2 6.2V20c0 .6.4 1 1 1h1.8v-2.2h1.2V21h2.6v-2.2h1.2V21h1.8c.6 0 1-.4 1-1v-3.6c1.9-1.4 3.2-3.5 3.2-6.2C20.5 5.6 17 2 12 2z"],
		cut: ["M9.2 8.6a1.9 1.9 0 1 0 0 3.8 1.9 1.9 0 0 0 0-3.8z", "M14.8 8.6a1.9 1.9 0 1 0 0 3.8 1.9 1.9 0 0 0 0-3.8z", "M12 12.8 10.9 15h2.2z"],
	},
	// An Elder Sign: a five-pointed star.
	signs: { fill: ["M12 1.8l2.7 6.3 6.8.6-5.2 4.5 1.6 6.7L12 16.3 6.1 19.9l1.6-6.7L2.5 8.7l6.8-.6z"] },
	// A Gate: a ring with a four-pointed star in it.
	gates: {
		line: ["M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z"],
		fill: ["M12 6.6l1.4 3.9 3.9 1.5-3.9 1.5L12 17.4l-1.4-3.9-3.9-1.5 3.9-1.5z"],
	},
	// A Desecration token: the diamond of the map chip with its cross.
	desecrations: {
		fill: ["M12 1.5l10.5 10.5L12 22.5 1.5 12z"],
		cut: ["M10.8 6h2.4v12h-2.4z", "M6 10.8h12v2.4H6z"],
	},
	// An open book.
	books: {
		fill: ["M3 5.5c3-1.3 6-1.1 9 .8v13.2c-3-1.9-6-2.1-9-.8z", "M21 5.5c-3-1.3-6-1.1-9 .8v13.2c3-1.9 6-2.1 9-.8z"],
	},
};

/**
 * A pictogram as an inline SVG element. `kind` is a key of STAT_ICONS; the
 * label is what a screen reader says (the icon stands for that word).
 */
export function statIcon(kind, label, px = 16) {
	const def = STAT_ICONS[kind];
	if (!def) return null;
	const NS = "http://www.w3.org/2000/svg";
	const svg = document.createElementNS(NS, "svg");
	svg.setAttribute("viewBox", "0 0 24 24");
	svg.setAttribute("width", String(px));
	svg.setAttribute("height", String(px));
	svg.setAttribute("class", `stat-icon stat-${kind}`);
	svg.setAttribute("role", "img");
	svg.setAttribute("aria-label", label || kind);
	const add = (d, cls) => {
		const p = document.createElementNS(NS, "path");
		p.setAttribute("d", d);
		p.setAttribute("class", cls);
		svg.append(p);
	};
	for (const d of def.fill || []) add(d, "stat-fill");
	for (const d of def.line || []) add(d, "stat-line");
	for (const d of def.cut || []) add(d, "stat-cut");
	return svg;
}
