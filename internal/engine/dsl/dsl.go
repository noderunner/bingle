// Package dsl defines the JSON game-definition DSL and parses it into Go
// types. It deliberately knows nothing about which primitive/board-generator
// names are actually implemented — that cross-check happens later, in
// internal/engine/catalog, so this package stays free of a dependency on
// (and import cycle with) the primitives/catalog packages.
//
// The DSL grew in two layers. The original (v1) layer describes a board
// graph, single-occupant pieces with a movement primitive, a setup and a
// round-robin turn order. The v2 layer — all of it optional, so v1
// documents parse unchanged — adds what stacking/placement games like Tak
// need: per-seat reserves ("hand"), declarative actions (place / move-stack),
// piece stacking attributes, board regions, an opening phase, declarative
// win rules, a notation style and rendering hints.
package dsl

import (
	"encoding/json"
	"fmt"
)

// GameDefinition is the top-level shape of a game-definition JSON document
// (see docs/history/plan.md section 6.1 for the annotated example this mirrors).
type GameDefinition struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Family groups sibling variants of one game in pickers; ExpandVariants
	// sets it on every expanded document (to the base document's ID).
	Family string `json:"family,omitempty"`
	// Description is a short paragraph about the game, shown when the player
	// hovers over the game in the picker.
	Description string `json:"description,omitempty"`
	// Version is the semantic version (major.minor.patch) of this game's
	// implementation: the definition and the Go code behind it. It is raised
	// whenever either changes, and the picker marks every game below 1.0.0 as
	// a beta. Variants share their base document's version.
	Version string `json:"version,omitempty"`
	// Designers are the people who designed the game; Publisher is the most
	// recent publisher, when there is one. Both are shown in the picker.
	Designers []string `json:"designers,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	// Rules is human-readable rules text, one paragraph per element,
	// shown in the UI and served to AI clients by the get_rules tool.
	Rules []string `json:"rules,omitempty"`

	Board Board     `json:"board"`
	Seats []SeatDef `json:"seats"`
	// ExtraColors are piece colours besides the seats' own that a position
	// may hold (Fist's neutral verdigris pawn, Blooms' second colour of each
	// player); no reserve may.
	ExtraColors []string            `json:"extra_colors,omitempty"`
	Pieces      map[string]PieceDef `json:"pieces"`

	// Reserves is each seat's starting hand: reserve pool -> count. A
	// pool is named by the "pool" attribute of piece types (default: the
	// piece type's own name), so several piece types can draw on one
	// supply (Tak's flat and standing stones). Every seat (color) gets its
	// own copy of these counts.
	Reserves map[string]int `json:"reserves,omitempty"`

	Setup []SetupEntry `json:"setup,omitempty"`
	// Preview is a sample of a game in play, drawn in the list of games and
	// on the setup page in place of an empty start (Gomoku, Hex, ...). Like
	// setup, entries on one square stack from the bottom up. A square the
	// board does not have (a smaller variant of the game) is left out.
	Preview   []SetupEntry `json:"preview,omitempty"`
	TurnOrder TurnOrderDef `json:"turn_order"`

	// Actions declares the non-movement-primitive things a player may do on
	// their turn (placing a piece from the reserve, moving a stack, ...).
	// Pieces whose movement names a primitive additionally get an implicit
	// "step" action.
	Actions []ActionDef `json:"actions,omitempty"`

	WinCondition WinConditionDef `json:"win_condition"`

	// Notation selects the move-notation style: "" / "generic", or "ptn"
	// (Tak's Portable Tak Notation; requires an algebraic grid).
	Notation string `json:"notation,omitempty"`

	Callbacks CallbackRefs `json:"callbacks,omitempty"`

	// Engine says who implements the rules: "" (the declarative actions and
	// win rules of this document) or "ruleset" (a Go ruleset registered with
	// the module takes over move generation, validation and scoring; the
	// document still describes the board, seats, pieces and display).
	Engine string `json:"engine,omitempty"`
	// Params are free-form numbers and strings a ruleset reads (victory
	// points needed, caps, ...). Variants change them with a merge patch.
	Params map[string]any `json:"params,omitempty"`
	// Display is data for clients to draw what the position document carries
	// beyond pieces: terrain tints and per-unit badges of a ruleset game.
	Display *DisplayDef `json:"display,omitempty"`
	// Cards defines the suits, decks and zones of a card game (see CardsDef).
	Cards *CardsDef `json:"cards,omitempty"`
	// Guide is the game's long-form rules document: tabs of Markdown text
	// (with tables and images) shown in the rules window.
	Guide *GuideDef `json:"guide,omitempty"`

	// Variants lists sibling games derived from this document by a JSON
	// merge patch (e.g. a different board size). See ExpandVariants.
	Variants []VariantDef `json:"variants,omitempty"`

	// PieRule says whether the game is played with the pie rule — the second
	// player may take the first player's side instead of replying to the
	// opening move — and whether the table may choose: "standard" (played
	// with it unless the table turns it off), "optional" (played without it
	// unless the table asks for it), "on" or "off" (settled: a game derived
	// from one of the two above), or absent (the game has no pie rule).
	// The catalog brings the setup option with it, and the swap action for a
	// game on the generic engine; a ruleset game offers the swap in its own
	// moves. See catalog.applyPieRule and ADR 0056.
	PieRule string `json:"pie_rule,omitempty"`

	// Options lists the setup choices the game offers its creator (which
	// cults sit at the table, an expansion to include). A ruleset that
	// implements catalog.Deriver turns the chosen options into a game
	// definition of its own — a derived game, named base~suffix — so every
	// combination is an ordinary game for the rest of the platform.
	Options []OptionDef `json:"options,omitempty"`
}

// OptionDef is one setup choice of a game, described for clients.
type OptionDef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Kind is "bool" (a check box), "seat-choice" (each seat picks one of
	// Choices, no two the same) or "book-swap" (a cult picks one of Choices
	// to replace its Slot).
	Kind string `json:"kind"`
	// Text explains the option.
	Text string `json:"text,omitempty"`
	// Choices are the values of a seat-choice option.
	Choices []OptionChoice `json:"choices,omitempty"`
	// Slots (kind "book-swap") are the things a choice may replace, each in
	// the cult named by its ID: the page offers a Choice for every Slot whose
	// cult is at the table.
	Slots []OptionChoice `json:"slots,omitempty"`
	// Default is the value when nothing is chosen: a bool, or for a
	// seat-choice the choice ids per seat.
	Default any `json:"default,omitempty"`
}

// OptionChoice is one value of a seat-choice option.
type OptionChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Color is the seat color the choice brings with it.
	Color string `json:"color,omitempty"`
	Text  string `json:"text,omitempty"`
}

// GuideDef lists the tabs of a game's rules guide. A tab's File names a
// Markdown document in the module's assets.
type GuideDef struct {
	Tabs []GuideTab `json:"tabs"`
}

// GuideTab is one tab of a rules guide.
type GuideTab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	File  string `json:"file"`
}

// DisplayDef is the legend of a ruleset game's overlays: what each tint and
// badge key in the position's view means and how to draw it.
type DisplayDef struct {
	// Tints colours board cells by key (terrain).
	Tints map[string]OverlayDef `json:"tints,omitempty"`
	// Badges are small marks drawn on a cell (a unit's condition).
	Badges map[string]OverlayDef `json:"badges,omitempty"`
	// Courts and other named choices a ruleset offers are described in
	// text: key -> one-line description.
	Terms map[string]string `json:"terms,omitempty"`
	// NoHands hides the generic reserve ("hand") panels: the ruleset's own
	// view shows what a seat holds.
	NoHands bool `json:"no_hands,omitempty"`
}

// OverlayDef is one entry of a DisplayDef legend.
type OverlayDef struct {
	Label string `json:"label"`
	// Color is a CSS color (a hex string).
	Color string `json:"color,omitempty"`
	// Glyph is a short text mark (badges).
	Glyph string `json:"glyph,omitempty"`
	// Text explains the effect.
	Text string `json:"text,omitempty"`
}

// VariantDef describes one variant of a base definition.
type VariantDef struct {
	// ID is the variant's complete game-definition ID.
	ID string `json:"id"`
	// Name overrides the base name when non-empty.
	Name string `json:"name,omitempty"`
	// Patch is an RFC 7386 JSON merge patch applied to the base document.
	Patch json.RawMessage `json:"patch,omitempty"`
}

// Board describes the game's board graph as exactly one of: a built-in
// Generator (e.g. a grid), or an explicit hand-authored graph given as
// Positions and Edges (the escape hatch for irregular/hex boards). Which
// form is actually used is validated by the future board/catalog packages,
// not here.
type Board struct {
	Generator *BoardGenerator `json:"generator,omitempty"`
	Positions []Position      `json:"positions,omitempty"`
	Edges     []Edge          `json:"edges,omitempty"`

	// Regions names sets of positions (e.g. a player's home row) that win
	// rules and placement restrictions can refer to. Grid boards always
	// have "left", "right", "top" and "bottom" built in.
	Regions map[string][]string `json:"regions,omitempty"`

	// Style is a rendering hint for clients: "squares" (cells are filled
	// squares, the default) or "points" (pieces sit on line intersections,
	// as in Go) "none" (no board at all, as in a card game), or, on a hex board, "free" (no board is drawn: only the
	// tiles on the table and the cells a tile may go to, fitted to the
	// picture, as in Hive). Hex boards are drawn as hexagons.
	Style string `json:"style,omitempty"`

	// Borders tints board edges for clients: region name -> seat color
	// (Hex: top/bottom belong to one player, left/right to the other).
	Borders map[string]string `json:"borders,omitempty"`

	// Canvas is the coordinate space of an area map's polygons.
	Canvas *Canvas `json:"canvas,omitempty"`
}

// Canvas is the width and height of an area map's coordinate space.
type Canvas struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// BoardGenerator configures a built-in board generator: a "grid" of
// Width x Height positions with a given Adjacency mode ("orthogonal4" /
// "orthogonal8"), a "hex" parallelogram of hexagonal cells, or a "hexhex"
// hexagon with Size cells per side.
type BoardGenerator struct {
	Type   string `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int    `json:"size,omitempty"`
	// Size2, for a "hexhex" board, is the length of its four long sides,
	// Size being its two short ones: a hexagon with no middle cell, which
	// Meridians is played on. Absent (or equal to Size) = every side alike.
	Size2     int    `json:"size2,omitempty"`
	Adjacency string `json:"adjacency"`
	// Labels is "xy" (positions named "x,y", the default) or "algebraic"
	// (positions named like chess squares: "a1" is bottom-left).
	Labels string `json:"labels,omitempty"`
}

// Position identifies a single hand-authored board position in an explicit
// (non-generated) board graph.
type Position struct {
	ID string `json:"id"`
	// X and Y optionally give the position's layout coordinates (any unit;
	// y grows downwards) for rendering.
	X *float64 `json:"x,omitempty"`
	Y *float64 `json:"y,omitempty"`

	// The remaining fields describe an *area* of an area map (board style
	// "areas"): a region outlined with straight edges, like a country or an
	// ocean on a world map.

	// Name is the area's display name.
	Name string `json:"name,omitempty"`
	// Polygons outline the area: each polygon is a list of [x, y] corners in
	// the board's canvas. More than one polygon draws one area in several
	// pieces (an ocean that wraps round the edge of the map).
	Polygons [][][2]float64 `json:"polygons,omitempty"`
	// Anchor is where the area's name and its tokens are centred.
	Anchor *[2]float64 `json:"anchor,omitempty"`
	// Tags classify the area for rules and drawing ("ocean", "land", ...).
	Tags []string `json:"tags,omitempty"`
}

// Edge connects two hand-authored positions (by ID) in an explicit board
// graph.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PieceDef describes one named piece type available to the game, and how it
// moves.
type PieceDef struct {
	Movement MovementDef `json:"movement"`

	// Label is the human-readable name ("Standing stone").
	Label string `json:"label,omitempty"`
	// Code is a short ASCII code used in notation and text renderings
	// (e.g. "S" for a Tak wall).
	Code string `json:"code,omitempty"`
	// Rules is a short explanation of how the piece moves, shown when the
	// pointer rests on the piece's button.
	Rules string `json:"rules,omitempty"`
	// Shape is a rendering hint: disc, square, flat, wall, cap, ring,
	// diamond, triangle or cross.
	Shape string `json:"shape,omitempty"`
	// Tags classify the piece for win rules and stack rules (e.g.
	// "road", "flat").
	Tags []string `json:"tags,omitempty"`
	// Pool names the reserve pool this piece type is drawn from when
	// placed; empty means a pool named after the piece type itself.
	Pool string `json:"pool,omitempty"`
	// Stack describes how this piece interacts with stacking.
	Stack StackDef `json:"stack,omitempty"`
	// Promote lists promotions: a piece of this type that ends a move in the
	// given region becomes another type (draughts men becoming kings).
	Promote []PromoteDef `json:"promote,omitempty"`
	// Color is a fixed colour for a piece type that is its own colour
	// (Blooms' four stones), used where the type is drawn on a button.
	Color string `json:"color,omitempty"`
}

// PromoteDef is one promotion rule.
type PromoteDef struct {
	// Seat restricts the rule to one seat (0 = every seat).
	Seat int `json:"seat,omitempty"`
	// Region is the board region where promotion happens.
	Region string `json:"region"`
	// To is the piece type promoted to.
	To string `json:"to"`
}

// StackDef describes how a piece behaves when other pieces try to move onto
// its square.
type StackDef struct {
	// Coverable, when explicitly false, forbids moving or dropping pieces
	// onto a square whose top piece is this one. Absent means true.
	Coverable *bool `json:"coverable,omitempty"`
	// FlattenableBy lists piece types that may flatten this piece (move
	// onto it anyway, turning it into FlattenTo) despite it being
	// uncoverable.
	FlattenableBy []string `json:"flattenable_by,omitempty"`
	// FlattenTo is the piece type this piece becomes when flattened.
	FlattenTo string `json:"flatten_to,omitempty"`
}

// MovementDef names a movement primitive (resolved against the primitive
// registry by the future catalog package) and its occupancy mode.
type MovementDef struct {
	Primitive string `json:"primitive"`
	Occupancy string `json:"occupancy"`
	// Directions optionally restricts the directions the piece may step or
	// jump in. Entries are compass names ("N", "NE", ...) or, relative to
	// the owning seat's Forward: "forward", "backward", "left", "right",
	// "forward-left", "forward-right", "backward-left", "backward-right".
	// Empty means every direction the board offers.
	Directions []string `json:"directions,omitempty"`
}

// SeatDef declares one seat a player can occupy, and the color of piece
// that seat plays.
type SeatDef struct {
	Seat  int    `json:"seat"`
	Color string `json:"color"`
	// Label is an optional display name for the seat ("White").
	Label string `json:"label,omitempty"`
	// Forward is the seat's forward direction on a grid ("N", "E", "S" or
	// "W"; N is towards the top of the screen). Movement directions such as
	// "forward-left" are relative to it.
	Forward string `json:"forward,omitempty"`
}

// SetupEntry places one piece of a given color on a given board position at
// game start. Several entries naming the same position build a stack,
// bottom first.
type SetupEntry struct {
	Position string `json:"position"`
	Piece    string `json:"piece"`
	Color    string `json:"color"`
}

// TurnOrderDef describes how turns rotate among seats, e.g. a "round-robin"
// Type over the given ordered Seats.
type TurnOrderDef struct {
	Type  string `json:"type"`
	Seats []int  `json:"seats"`
	// Opening, if set, restricts the first few plies of the game.
	Opening *OpeningDef `json:"opening,omitempty"`
	// PassWhenStuck makes the turn skip a seat that has no legal move (the
	// next seat that can move plays instead), as in Reversi. When nobody can
	// move, the turn stays where it would have gone and an end rule with the
	// "no-legal-moves" trigger finishes the game.
	PassWhenStuck bool `json:"pass_when_stuck,omitempty"`
}

// OpeningDef restricts the game's first Plies plies: only the listed
// actions/pieces are allowed, and the piece placed may belong to the
// opponent (Tak's "each player places an opposing flatstone first").
type OpeningDef struct {
	Plies   int      `json:"plies"`
	Actions []string `json:"actions,omitempty"`
	Pieces  []string `json:"pieces,omitempty"`
	// Owner is "mover" (default) or "opponent": whose color — and whose
	// reserve — a piece placed during the opening comes from.
	Owner string `json:"owner,omitempty"`
}

// ActionDef declares one kind of action a player may take on their turn.
type ActionDef struct {
	// ID names the action; moves carry it as their "action" field.
	ID string `json:"id"`
	// Type is "place", "move-stack", "jump", "slide", "leap", "swap", "step" or "pass". Pieces whose movement
	// names a primitive get an implicit "step" action; declaring a "step"
	// action explicitly instead gives per-direction control (Breakthrough's
	// straight moves cannot capture while its diagonal moves can).
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`

	// --- step (explicit) ---

	// Directions restricts the directions of an explicit step action (see
	// MovementDef.Directions for the vocabulary); empty = every direction.
	Directions []string `json:"directions,omitempty"`
	// Occupancy is the explicit step's destination rule: "empty-only",
	// "capture-only" or "capture-or-empty".
	Occupancy string `json:"occupancy,omitempty"`

	// --- place ---

	// Pieces lists the piece types that may be placed (first = default).
	Pieces []string `json:"pieces,omitempty"`
	// Source is where placed pieces come from: "reserve" (default; the
	// seat's hand is decremented) or "supply" (unlimited).
	Source string `json:"source,omitempty"`
	// Region restricts placement to a named board region.
	Region string `json:"region,omitempty"`
	// Gravity, if set to a direction name ("S"), only allows placement on
	// squares that are supported in that direction by another piece or the
	// board edge (Connect Four).
	Gravity string `json:"gravity,omitempty"`
	// Flips makes a placement flip lines of opposing pieces bracketed by the
	// new piece and another piece of the placer's color (Reversi/Othello).
	Flips *FlipsDef `json:"flips,omitempty"`
	// Captures makes a placement capture opposing groups left without
	// liberties (Go).
	Captures *CapturesDef `json:"captures,omitempty"`
	// Ko forbids a placement that would recreate the position as it was
	// before the opponent's last move (the simple ko rule).
	Ko bool `json:"ko,omitempty"`
	// Custodian makes a placement (or a slide) capture runs of opposing
	// pieces that are flanked by the moved piece and another piece of the
	// mover's colour (Pente's captured pairs; Hnefatafl's sandwiches).
	Custodian *CustodianDef `json:"custodian,omitempty"`

	// --- move-stack ---

	// CarryLimit is the maximum number of pieces that may be picked up
	// from a stack: an integer, or "board-size".
	CarryLimit *Quantity `json:"carry_limit,omitempty"`
	// Flatten configures how a flattening piece may be used.
	Flatten *FlattenDef `json:"flatten,omitempty"`

	// --- jump ---

	// Mandatory makes jumping compulsory: while any jump is available for
	// the mover, only jumps are legal (draughts).
	Mandatory bool `json:"mandatory,omitempty"`
	// Chain lets a piece keep jumping in the same move; a chain must then
	// be played to its end.
	Chain bool `json:"chain,omitempty"`

	// --- swap ---

	// Plies lists the plies (0 = before anything was played) at which a
	// swap action may be taken; default [1], the second player's first
	// move (the pie rule).
	Plies []int `json:"plies,omitempty"`

	// --- slide ---

	// ForbidRegion names a board region the action's pieces may not land
	// on (Hnefatafl's corners and throne, for everyone but the king).
	ForbidRegion string `json:"forbid_region,omitempty"`

	// MaxDistance limits how many squares a slide may cover (0 = until
	// blocked). Slides use Pieces, Directions and Occupancy like explicit
	// step actions; the piece may not pass over other pieces. For a leap
	// it is how far (in board steps) the piece may land.
	MaxDistance int `json:"max_distance,omitempty"`
	// Then is a compulsory follow-up to the slide (Amazons' arrow).
	Then *ThenDef `json:"then,omitempty"`

	// --- leap ---

	// CloneDistance is the furthest a leap may go while the piece stays
	// behind (a clone: Ataxx's step of one); beyond it, up to MaxDistance,
	// the piece jumps and its origin is vacated.
	CloneDistance int `json:"clone_distance,omitempty"`
	// Infect makes every opposing piece adjacent to the landing square
	// change to the mover's colour.
	Infect bool `json:"infect,omitempty"`
}

// ThenDef is the second half of a compound move: after the slide, the same
// player puts a Piece from the unlimited supply on any empty square the
// slid piece could now slide to (the square it left counts as empty).
type ThenDef struct {
	// Type is "shoot".
	Type  string `json:"type"`
	Piece string `json:"piece"`
	// Directions and MaxDistance default to the slide's own.
	Directions  []string `json:"directions,omitempty"`
	MaxDistance int      `json:"max_distance,omitempty"`
}

// CapturesDef configures liberty captures for a place action: after a stone
// is placed, every opposing group of orthogonally connected stones with no
// empty neighbouring square is removed; a placement that leaves its own
// group without liberties (suicide) is illegal unless it captured something
// or AllowSuicide is set.
type CapturesDef struct {
	AllowSuicide bool `json:"allow_suicide,omitempty"`
}

// CustodianDef configures custodian captures for a place action: after the
// piece lands, along each direction a run of exactly Length opposing pieces
// closed off by one of the placer's own pieces is removed.
type CustodianDef struct {
	// Lengths lists the capturable run lengths (default [2]).
	Lengths []int `json:"lengths,omitempty"`
	// Anvils names board regions whose empty squares count as the closing
	// piece (Hnefatafl's corners and empty throne).
	Anvils []string `json:"anvils,omitempty"`
	// CountIn names a reserve pool that is credited with the number of
	// pieces captured (it must be declared in reserves, usually at 0), so a
	// "pool" win rule can count captures.
	CountIn string `json:"count_in,omitempty"`
}

// FlipsDef configures bracketing flips for a place action.
type FlipsDef struct {
	// Require makes a placement legal only if it flips at least one piece.
	Require bool `json:"require,omitempty"`
}

// FlattenDef constrains when a flattener may flatten a flattenable piece.
type FlattenDef struct {
	// FinalSquareOnly requires the flattening to happen on the last square
	// of the move.
	FinalSquareOnly bool `json:"final_square_only,omitempty"`
	// AloneOnly requires the flattener to be the only piece dropped there.
	AloneOnly bool `json:"alone_only,omitempty"`
}

// Quantity is an integer that may be given either as a JSON number or as a
// named, board-derived keyword ("board-size", "board-width",
// "board-height").
type Quantity struct {
	N       int
	Keyword string
}

// UnmarshalJSON accepts a number or a string keyword.
func (q *Quantity) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		q.N, q.Keyword = n, ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("dsl: quantity must be an integer or a keyword string, got %s", string(data))
	}
	q.N, q.Keyword = 0, s
	return nil
}

// MarshalJSON mirrors UnmarshalJSON.
func (q Quantity) MarshalJSON() ([]byte, error) {
	if q.Keyword != "" {
		return json.Marshal(q.Keyword)
	}
	return json.Marshal(q.N)
}

// WinConditionDef names how the game decides a winner: "none" (no win
// condition), "callback" (deferred to GameCallbacks.CheckWinner) or "rules"
// (evaluate Rules in order after every move; the first that fires decides).
type WinConditionDef struct {
	Type  string       `json:"type"`
	Rules []WinRuleDef `json:"rules,omitempty"`
}

// WinRuleDef is one declarative win/end rule.
type WinRuleDef struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"` // "connect", "line", "ring", "reach", "pool", "absent" or "end"
	// Reason is reported as the game's end reason when the rule fires.
	Reason string `json:"reason,omitempty"`
	// Result says what the rule means for the seat that satisfies it:
	// "win" (the default) or "lose" — e.g. Yavalath, where making a line of
	// three loses. A lost game is won by the opponent (a draw when there
	// are more than two seats).
	Result string `json:"result,omitempty"`

	// Tag/Piece select which pieces count (matched against the top piece
	// of each square). Either may be empty to mean "any piece".
	Tag   string `json:"tag,omitempty"`
	Piece string `json:"piece,omitempty"`

	// Regions lists pairs of board region names (connect rules): a seat
	// wins by owning a connected group of matching squares touching both
	// regions of any pair.
	Regions [][]string `json:"regions,omitempty"`

	// Of and Touch extend connect rules to "a group touching at least
	// Touch of these regions" (Havannah's fork: three of the six sides; a
	// bridge: two of the six corners). Use instead of Regions.
	Of    []string `json:"of,omitempty"`
	Touch int      `json:"touch,omitempty"`

	// Region names the target region of a reach rule: a seat wins when one
	// of its matching pieces stands in it.
	Region string `json:"region,omitempty"`

	// Seat restricts a connect/line rule to one seat (0 = every seat). In
	// Hex each player connects a different pair of edges.
	Seat int `json:"seat,omitempty"`

	// Length is the required run length (line rules).
	Length int `json:"length,omitempty"`

	// Piece (above) with type "absent": the rule fires when no piece of
	// that type is left on the board, and then Seat wins (Hnefatafl's
	// captured king).

	// Pool and AtLeast configure "pool" rules: a seat wins (or loses, with
	// result "lose") when its reserve pool holds at least AtLeast — e.g.
	// Pente's ten captured stones.
	Pool    string `json:"pool,omitempty"`
	AtLeast int    `json:"at_least,omitempty"`

	// When lists end triggers (end rules): "board-full", "reserve-empty"
	// (some seat has nothing left in hand), "no-legal-moves" (the seat to
	// move has no legal action), "ply-limit" (Plies plies have been
	// played) or "passes" (Count consecutive pass actions).
	When []string `json:"when,omitempty"`
	// Plies is the ply count for the "ply-limit" trigger.
	Plies int `json:"plies,omitempty"`
	// Count is the number of consecutive passes for the "passes" trigger.
	Count int `json:"count,omitempty"`
	// Decide says who wins when an end rule fires.
	Decide *DecideDef `json:"decide,omitempty"`
}

// DecideDef resolves an end rule's winner.
type DecideDef struct {
	// Type is "draw", "most" (highest count of matching top pieces wins,
	// tie = draw), "score" (area scoring: stones plus empty regions that
	// border only that color, plus Komi), "mover-wins" or "mover-loses".
	Type  string `json:"type"`
	Tag   string `json:"tag,omitempty"`
	Piece string `json:"piece,omitempty"`
	// Komi maps a seat number (as a string) to bonus points for "score".
	Komi map[string]float64 `json:"komi,omitempty"`
}

// CallbackRefs names optional Go-callback hooks a game definition wants
// wired in by the future catalog package. It is empty for games (like the
// demo) that use the no-op default; fields will be added here as games that
// need named hooks are built.
type CallbackRefs struct {
}
