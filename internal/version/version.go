// Package version holds the version of the bingle engine itself: the server,
// the DSL, the shared rules machinery and the web client. Each game has a
// version of its own (in its JSON definition).
package version

// Engine is the semantic version of bingle. Raise it with each change to the
// engine or the platform (not for a change to one game alone, which raises
// that game's own version). The footer of every page shows it.
const Engine = "0.122.0"

// API is the semantic version of the API's messages: the REST requests and
// responses and the WebSocket events that docs/api.schema.json describes
// (ADR 0134). It moves on its own, not with Engine, so that a client built
// apart from the server can tell whether the server still speaks to it:
//
//   - major: a change that can break a client written against the last
//     version: a field or endpoint removed or renamed, a field's type or
//     meaning changed, or a value a client must handle added to a closed set
//     (a WebSocket event it must act on, a game status);
//   - minor: an addition a client may ignore: an endpoint, an optional field,
//     an event it may skip;
//   - patch: the schema's descriptions alone.
//
// Clients ignore the fields they do not know, which is what keeps an addition
// minor.
const API = "1.7.0"
