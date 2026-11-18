package api

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/dsl/schemagen"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/version"
	"github.com/noderunner/bingle/internal/wire"
)

// SchemaID is the $id of docs/api.schema.json.
const SchemaID = "https://github.com/noderunner/bingle/docs/api.schema.json"

// schemaEndpoint is one endpoint of the index whose messages the schema describes.
type schemaEndpoint struct {
	route    string
	request  reflect.Type // nil: no body
	response reflect.Type // nil: not JSON
	media    string       // the response's media type, when it is not JSON
}

// schemaEndpoints are the index's endpoints (indexEndpoints) and GET /api itself.
var schemaEndpoints = []schemaEndpoint{
	{route: "GET /api", response: reflect.TypeFor[apiIndex]()},
	{route: "POST /api/guest", response: reflect.TypeFor[auth.GuestKey]()},
	{route: "GET /api/game-defs", response: reflect.TypeFor[[]wire.GameDef]()},
	{route: "GET /api/game-defs/{id}", response: reflect.TypeFor[session.GameInfo]()},
	{route: "GET /api/game-defs/{id}/guide", media: "text/markdown"},
	{route: "GET /api/bots", response: reflect.TypeFor[[]bots.Info]()},
	{route: "GET /api/games", response: reflect.TypeFor[openGamesResponse]()},
	{route: "POST /api/games", request: reflect.TypeFor[createGameRequest](), response: reflect.TypeFor[createGameResponse]()},
	{route: "POST /api/games/{code}/join", request: reflect.TypeFor[joinGameRequest](), response: reflect.TypeFor[joinGameResponse]()},
	{route: "GET /api/games/{code}", response: reflect.TypeFor[getGameResponse]()},
	{route: "GET /api/games/{code}/state", response: reflect.TypeFor[getGameStateResponse]()},
	{route: "GET /api/games/{code}/observe", response: reflect.TypeFor[observationResponse]()},
	{route: "GET /api/games/{code}/selections", response: reflect.TypeFor[selectionsResponse]()},
	{route: "POST /api/games/{code}/moves", request: reflect.TypeFor[makeMoveRequest](), response: reflect.TypeFor[makeMoveResponse]()},
	{route: "GET /api/games/{code}/moves", response: reflect.TypeFor[[]wire.Move]()},
	{route: "POST /api/games/{code}/resign", request: reflect.TypeFor[playerRequest](), response: reflect.TypeFor[getGameStateResponse]()},
	{route: "GET /api/me/deals", response: reflect.TypeFor[ownDealsResponse]()},
	{route: "POST /api/bot-matches", request: reflect.TypeFor[botMatchRequest](), response: reflect.TypeFor[botMatchResponse]()},
}

// schemaEvents are the WebSocket events of /ws/{code} the schema describes,
// by type. Voice and "rtc" are left out for now.
var schemaEvents = []struct {
	typ  string
	data reflect.Type
}{
	{"joined", reflect.TypeFor[wsJoined]()},
	{"move", reflect.TypeFor[session.MoveEventData]()},
	{"state", reflect.TypeFor[session.StateEventData]()},
	{"chat", reflect.TypeFor[session.ChatLine]()},
	{"connection", reflect.TypeFor[session.ConnectionEventData]()},
	{"kick_vote", reflect.TypeFor[session.KickVoteEventData]()},
	{"seat_replaced", reflect.TypeFor[session.SeatReplacedEventData]()},
	{"slow_turn", reflect.TypeFor[session.SlowTurnEventData]()},
}

// Schema returns docs/api.schema.json: the JSON Schema of the messages of
// version.API, generated from the Go types the handlers send and read, with
// their doc comments read from the repository at root (ADR 0134). Routes are
// not its business: they are in GET /api and docs/api.md. The schema names
// the endpoints and events it covers under "x-endpoints" and "x-events".
func Schema(root string) ([]byte, error) {
	m := schemagen.NewMessages(root, schemagen.MessageOptions{
		Special: map[reflect.Type]map[string]any{
			reflect.TypeFor[time.Time](): {"type": "string", "format": "date-time"},
			reflect.TypeFor[clockSeconds](): {"oneOf": []any{
				map[string]any{"type": "integer"},
				map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			}},
		},
		Required: map[reflect.Type][]string{
			reflect.TypeFor[createGameRequest](): {"game_def_id", "player_name"},
			reflect.TypeFor[joinGameRequest]():   {"player_name"},
			reflect.TypeFor[makeMoveRequest]():   {"player_name"},
			reflect.TypeFor[playerRequest]():     {"player_name"},
			reflect.TypeFor[botMatchRequest]():   {"game_def_id", "bot_a", "bot_b"},
		},
		Names: map[reflect.Type]string{
			reflect.TypeFor[apiIndex]():  "IndexResponse",
			reflect.TypeFor[wsJoined]():  "JoinedEventData",
			reflect.TypeFor[bots.Info](): "BotInfo",
		},
	})

	endpoints := map[string]any{}
	for _, e := range schemaEndpoints {
		out := map[string]any{}
		if e.request != nil {
			out["request"] = m.Request(e.request)
		}
		switch {
		case e.response != nil:
			out["response"] = m.Response(e.response)
		case e.media != "":
			out["media_type"] = e.media
		}
		endpoints[e.route] = out
	}
	errRef := m.Response(reflect.TypeFor[errorResponse]())

	var events []any
	eventTypes := map[string]any{}
	for _, e := range schemaEvents {
		data := m.Response(e.data)
		eventTypes[e.typ] = data
		events = append(events, map[string]any{
			"type":       "object",
			"properties": map[string]any{"type": map[string]any{"const": e.typ}, "data": data},
			"required":   []string{"type", "data"},
		})
	}
	defs, err := m.Defs()
	if err != nil {
		return nil, err
	}
	defs["WebSocketEvent"] = map[string]any{
		"description": "A message the server sends on /ws/{code}. A client skips an event whose type it does not know.",
		"oneOf":       events,
	}

	out := map[string]any{
		"$schema":       "https://json-schema.org/draft/2020-12/schema",
		"$id":           SchemaID,
		"title":         "Bingle API messages",
		"description":   "The requests, responses and WebSocket events of bingle's API, version " + version.API + ". Generated from the Go types of the server: do not edit. Objects let unknown fields through: a client ignores those it does not know.",
		"x-api-version": version.API,
		"x-endpoints":   endpoints,
		"x-error":       errRef,
		"x-events":      eventTypes,
		"$defs":         defs,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
