// Package version holds the version of the bingle engine itself: the server,
// the DSL, the shared rules machinery and the web client. Each game has a
// version of its own (in its JSON definition).
package version

// Engine is the semantic version of bingle. Raise it with each change to the
// engine or the platform (not for a change to one game alone, which raises
// that game's own version). The footer of every page shows it.
const Engine = "0.24.0"
