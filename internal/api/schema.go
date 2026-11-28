package api

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/clientbuilds"
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
	{route: "GET /api/game-defs/{id}/clients/{client}", response: reflect.TypeFor[session.ClientIndex]()},
	{route: "GET /api/game-defs/{id}/clients/{client}/files/{path}", media: "application/octet-stream"},
	// The bingle-3d client itself (ADR 0151), on a server that offers it.
	{route: "GET /api/client", response: reflect.TypeFor[clientBuildsResponse]()},
	{route: "GET /api/client/files/{file}", media: "application/octet-stream"},
	{route: "POST /api/client/setup", media: "application/toml"},
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
	{route: "GET /api/me/games", response: reflect.TypeFor[ownGamesResponse]()},
	{route: "GET /api/me/deals", response: reflect.TypeFor[ownDealsResponse]()},
	{route: "POST /api/bot-matches", request: reflect.TypeFor[botMatchRequest](), response: reflect.TypeFor[botMatchResponse]()},

	// Voice chat (docs/voice.md): the network helpers of every call, and a
	// game's call. Its messages arrive on /ws/{code}?only=voice.
	{route: "GET /api/rtc/config", response: reflect.TypeFor[rtcConfigResponse]()},
	{route: "GET /api/games/{code}/voice", response: reflect.TypeFor[voicePeersResponse]()},
	{route: "POST /api/games/{code}/voice", request: reflect.TypeFor[voiceRequest](), response: reflect.TypeFor[voicePeersResponse]()},
	{route: "POST /api/games/{code}/voice/signal", request: reflect.TypeFor[signalRequest](), response: reflect.TypeFor[okResponse]()},

	// Signing in, for a client that is not a browser (ADR 0138): the answers
	// are those of a sign-in that asks for a token.
	{route: "POST /auth/login", request: reflect.TypeFor[auth.LoginRequest](), response: reflect.TypeFor[auth.SessionTokenResponse]()},
	{route: "POST /auth/second", request: reflect.TypeFor[auth.SecondRequest](), response: reflect.TypeFor[auth.SessionTokenResponse]()},
	{route: "GET /auth/me", response: reflect.TypeFor[auth.Me]()},
	{route: "POST /auth/tour-seen", response: reflect.TypeFor[auth.TourSeenResponse]()},
	{route: "POST /auth/announcements-seen", request: reflect.TypeFor[auth.AnnouncementsSeenRequest](), response: reflect.TypeFor[auth.AnnouncementsSeenResponse]()},
	{route: "GET /api/announcements", response: reflect.TypeFor[auth.AnnouncementsResponse]()},
	{route: "POST /auth/language", request: reflect.TypeFor[auth.LanguageRequest](), response: reflect.TypeFor[auth.LanguageResponse]()},
	{route: "POST /auth/translate", request: reflect.TypeFor[auth.TranslateRequest](), response: reflect.TypeFor[auth.TranslateResponse]()},
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
	{route: "POST /api/friends/block", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/friends/unblock", request: reflect.TypeFor[friendRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "GET /api/friends/blocks", response: reflect.TypeFor[blocksResponse]()},
	{route: "GET /api/friends/settings", response: reflect.TypeFor[store.FriendSettings]()},
	{route: "PUT /api/friends/settings", request: reflect.TypeFor[friendSettingsRequest](), response: reflect.TypeFor[store.FriendSettings]()},
	{route: "POST /api/dm", request: reflect.TypeFor[dmRequest](), response: reflect.TypeFor[session.DirectMessage]()},
	{route: "POST /api/dm/history", request: reflect.TypeFor[dmReadRequest](), response: reflect.TypeFor[dmHistoryResponse]()},
	{route: "POST /api/dm/threads", response: reflect.TypeFor[dmThreadsResponse]()},
	{route: "POST /api/dm/call", request: reflect.TypeFor[dmCallRequest](), response: reflect.TypeFor[okResponse]()},
	{route: "POST /api/dm/signal", request: reflect.TypeFor[dmSignalRequest](), response: reflect.TypeFor[okResponse]()},
}

// schemaEvents are the WebSocket events of /ws/{code} the schema describes,
// by type. A socket opened with only=voice is a voice call's: it sends
// "joined", "voice" and "rtc" and nothing else, and only it sends "rtc".
var schemaEvents = []struct {
	typ  string
	data reflect.Type
}{
	{"joined", reflect.TypeFor[wsJoined]()},
	{"move", reflect.TypeFor[session.MoveEventData]()},
	{"state", reflect.TypeFor[session.StateEventData]()},
	{"chat", reflect.TypeFor[session.ChatLine]()},
	{"chat_translation", reflect.TypeFor[session.ChatTranslationEventData]()},
	{"connection", reflect.TypeFor[session.ConnectionEventData]()},
	{"kick_vote", reflect.TypeFor[session.KickVoteEventData]()},
	{"seat_replaced", reflect.TypeFor[session.SeatReplacedEventData]()},
	{"slow_turn", reflect.TypeFor[session.SlowTurnEventData]()},
	{"voice", reflect.TypeFor[session.VoiceEventData]()},
	{"rtc", reflect.TypeFor[session.RTCSignal]()},
}

// signalMessages are the definitions whose data is a WebRTC message, which
// the server passes on unread: the schema says what a browser puts there.
var signalMessages = []string{"VoiceSignalRequest", "RTCSignal", "DirectSignalRequest", "DirectSignal"}

// signalData is the schema of a WebRTC message's data, for each kind.
var signalData = map[string]any{
	"description": "What a browser's RTCPeerConnection exchanges, by the message's kind. The server does not read it: it checks only that it is JSON, not null and at most 16 KB. A client that is not a browser sends and expects what a browser does (docs/voice.md).",
	"oneOf": []any{
		map[string]any{
			"title":       "SessionDescription",
			"description": "kind offer or answer: RTCSessionDescription's type and sdp.",
			"type":        "object",
			"properties": map[string]any{
				"type": map[string]any{"type": "string", "enum": []any{"offer", "answer"}},
				"sdp":  map[string]any{"type": "string"},
			},
			"required": []any{"type", "sdp"},
		},
		map[string]any{
			"title":       "IceCandidate",
			"description": "kind ice: RTCIceCandidate.toJSON(), one candidate per message, with no message for the end of the candidates.",
			"type":        "object",
			"properties": map[string]any{
				"candidate":        map[string]any{"type": "string"},
				"sdpMid":           map[string]any{"type": []any{"string", "null"}},
				"sdpMLineIndex":    map[string]any{"type": []any{"integer", "null"}},
				"usernameFragment": map[string]any{"type": []any{"string", "null"}},
			},
			"required": []any{"candidate"},
		},
		map[string]any{
			"title":                "Retry",
			"description":          "kind retry: {}, the side that does not offer asking the other to offer again.",
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	},
}

// schemaDirectEvents are the WebSocket events of /ws-dm, a signed-in
// account's own socket: its messages, its calls, its badges and its friends.
var schemaDirectEvents = []struct {
	typ  string
	data reflect.Type
}{
	{"ready", reflect.TypeFor[dmReady]()},
	{"dm", reflect.TypeFor[session.DirectMessage]()},
	{"dm-translation", reflect.TypeFor[session.ChatTranslationEventData]()},
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
			reflect.TypeFor[dmRequest]():         {"to", "text"},
			reflect.TypeFor[dmReadRequest]():     {"with"},
			reflect.TypeFor[dmSignalRequest]():   {"to", "kind", "data"},
			reflect.TypeFor[voiceRequest]():      {"player_name"},
			reflect.TypeFor[signalRequest]():     {"player_name", "to", "kind", "data"},
		},
		Names: map[reflect.Type]string{
			reflect.TypeFor[apiIndex]():           "IndexResponse",
			reflect.TypeFor[wsJoined]():           "JoinedEventData",
			reflect.TypeFor[bots.Info]():          "BotInfo",
			reflect.TypeFor[dmReady]():            "DirectReadyEventData",
			reflect.TypeFor[okResponse]():         "OKResponse",
			reflect.TypeFor[friendRequest]():      "FriendRequest",
			reflect.TypeFor[blocksResponse]():     "BlocksResponse",
			reflect.TypeFor[dmRequest]():          "DirectMessageRequest",
			reflect.TypeFor[dmReadRequest]():      "DirectHistoryRequest",
			reflect.TypeFor[dmCallRequest]():      "DirectCallRequest",
			reflect.TypeFor[dmSignalRequest]():    "DirectSignalRequest",
			reflect.TypeFor[dmHistoryResponse]():  "DirectHistoryResponse",
			reflect.TypeFor[dmThreadsResponse]():  "DirectThreadsResponse",
			reflect.TypeFor[auth.Me]():            "MeResponse",
			reflect.TypeFor[signalRequest]():      "VoiceSignalRequest",
			reflect.TypeFor[rtcConfigResponse]():  "RTCConfigResponse",
			reflect.TypeFor[iceServer]():          "ICEServer",
			reflect.TypeFor[clientbuilds.Build](): "ClientBuild",
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
	defs["SignalData"] = signalData
	for _, name := range signalMessages {
		props := defs[name].(map[string]any)["properties"].(map[string]any)
		props["data"] = map[string]any{"$ref": "#/$defs/SignalData"}
	}
	defs["WebSocketEvent"] = map[string]any{
		"description": "A message the server sends on /ws/{code}. A client skips an event whose type it does not know. A voice call's socket, /ws/{code}?only=voice, sends joined, voice and rtc only, and is the only one to send rtc: a client opens it and waits for joined before it posts to /voice, so as to miss no offer.",
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
