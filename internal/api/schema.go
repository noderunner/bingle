package api

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/dsl/schemagen"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
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

// schemaEndpoints are the index's endpoints (indexEndpoints), GET /api itself
// and GET /api/me/deals.
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
	{route: "POST /api/games/{code}/bots", request: reflect.TypeFor[addBotRequest](), response: reflect.TypeFor[session.AddBotResult]()},
	{route: "POST /api/games/{code}/start", request: reflect.TypeFor[playerRequest](), response: reflect.TypeFor[gameStatusResponse]()},
	{route: "POST /api/games/{code}/seat", request: reflect.TypeFor[seatRequest](), response: reflect.TypeFor[gameStatusResponse]()},
	{route: "POST /api/games/{code}/leave", request: reflect.TypeFor[playerRequest](), response: reflect.TypeFor[gameStatusResponse]()},
	{route: "POST /api/games/{code}/remove", request: reflect.TypeFor[seatRequest](), response: reflect.TypeFor[gameStatusResponse]()},
	{route: "GET /api/me/deals", response: reflect.TypeFor[ownDealsResponse]()},
	{route: "POST /api/bot-matches", request: reflect.TypeFor[botMatchRequest](), response: reflect.TypeFor[botMatchResponse]()},

	// Signing in, for a client that is not a browser (ADR 0138): the answers
	// are those of a sign-in that asks for a token.
	{route: "POST /auth/login", request: reflect.TypeFor[auth.LoginRequest](), response: reflect.TypeFor[auth.SessionTokenResponse]()},
	{route: "POST /auth/second", request: reflect.TypeFor[auth.SecondRequest](), response: reflect.TypeFor[auth.SessionTokenResponse]()},
	{route: "GET /auth/me", response: reflect.TypeFor[auth.Me]()},
	{route: "POST /auth/logout", response: reflect.TypeFor[auth.LogoutResponse]()},

	// Friends (ADR 0105), who is here and direct messages, for a signed-in
	// account. Writing and calling need the account to be here.
	{route: "POST /api/here", request: reflect.TypeFor[hereRequest](), response: reflect.TypeFor[hereResponse]()},
	{route: "GET /api/here", response: reflect.TypeFor[hereResponse]()},
	{route: "GET /api/friends", response: reflect.TypeFor[session.FriendsList]()},
	{route: "POST /api/friends/request", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/friends/answer", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/friends/remove", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/friends/invite", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/friends/invite-answer", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/friends/dismiss", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "GET /api/friends/settings", response: reflect.TypeFor[store.FriendSettings]()},
	{route: "PUT /api/friends/settings", request: reflect.TypeFor[store.FriendSettings](), response: reflect.TypeFor[store.FriendSettings]()},
	{route: "POST /api/dm", request: reflect.TypeFor[dmRequest](), response: reflect.TypeFor[session.DirectMessage]()},
	{route: "POST /api/dm/history", request: reflect.TypeFor[dmReadRequest](), response: reflect.TypeFor[dmHistoryResponse]()},
	{route: "POST /api/dm/threads", response: reflect.TypeFor[dmThreadsResponse]()},
	{route: "POST /api/dm/call", request: reflect.TypeFor[dmCallRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/dm/signal", request: reflect.TypeFor[dmSignalRequest](), response: reflect.TypeFor[okResponse]()},
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

// schemaDirectEvents are the WebSocket events of /ws-dm, a signed-in
// account's own socket: its messages, its calls, its badges and its friends.
var schemaDirectEvents = []struct {
	typ  string
	data reflect.Type
}{
	{"ready", reflect.TypeFor[dmReady]()},
	{"dm", reflect.TypeFor[session.DirectMessage]()},
	{"dm-call", reflect.TypeFor[session.DirectCall]()},
	{"dm-rtc", reflect.TypeFor[session.DirectSignal]()},
	{"badge", reflect.TypeFor[session.BadgeEventData]()},
	{"friends", reflect.TypeFor[session.FriendsEventData]()},
}

// eventSchemas are the data of each event type, by type, and the schema of
// each event as a whole, for a WebSocketEvent's oneOf.
func eventSchemas(m *schemagen.Messages, list []struct {
	typ  string
	data reflect.Type
}) (map[string]any, []any) {
	byType := map[string]any{}
	var events []any
	for _, e := range list {
		data := m.Response(e.data)
		byType[e.typ] = data
		events = append(events, map[string]any{
			"type":       "object",
			"properties": map[string]any{"type": map[string]any{"const": e.typ}, "data": data},
			"required":   []string{"type", "data"},
		})
	}
	return byType, events
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
			reflect.TypeFor[seatRequest]():       {"player_name", "seat"},
			reflect.TypeFor[addBotRequest]():     {"player_name", "bot"},
			reflect.TypeFor[botMatchRequest]():   {"game_def_id", "bot_a", "bot_b"},
			reflect.TypeFor[auth.LoginRequest](): {"handle", "password"},
			reflect.TypeFor[dmRequest]():         {"to", "text"},
			reflect.TypeFor[dmReadRequest]():     {"with"},
			reflect.TypeFor[dmSignalRequest]():   {"to", "kind", "data"},
		},
		Names: map[reflect.Type]string{
			reflect.TypeFor[apiIndex]():          "IndexResponse",
			reflect.TypeFor[wsJoined]():          "JoinedEventData",
			reflect.TypeFor[bots.Info]():         "BotInfo",
			reflect.TypeFor[dmReady]():           "DirectReadyEventData",
			reflect.TypeFor[okResponse]():        "OKResponse",
			reflect.TypeFor[friendRequest]():     "FriendRequest",
			reflect.TypeFor[dmRequest]():         "DirectMessageRequest",
			reflect.TypeFor[dmReadRequest]():     "DirectHistoryRequest",
			reflect.TypeFor[dmCallRequest]():     "DirectCallRequest",
			reflect.TypeFor[dmSignalRequest]():   "DirectSignalRequest",
			reflect.TypeFor[dmHistoryResponse](): "DirectHistoryResponse",
			reflect.TypeFor[dmThreadsResponse](): "DirectThreadsResponse",
			reflect.TypeFor[auth.Me]():           "MeResponse",
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

	eventTypes, events := eventSchemas(m, schemaEvents)
	directTypes, directEvents := eventSchemas(m, schemaDirectEvents)
	defs, err := m.Defs()
	if err != nil {
		return nil, err
	}
	defs["WebSocketEvent"] = map[string]any{
		"description": "A message the server sends on /ws/{code}. A client skips an event whose type it does not know.",
		"oneOf":       events,
	}
	defs["DirectWebSocketEvent"] = map[string]any{
		"description": "A message the server sends on /ws-dm, a signed-in account's own socket. A client skips an event whose type it does not know.",
		"oneOf":       directEvents,
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
		"x-dm-events":   directTypes,
		"$defs":         defs,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
