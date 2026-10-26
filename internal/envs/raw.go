package envs

import "encoding/json"

// rawJSON wraps an already-encoded JSON document so it is embedded as-is
// rather than re-encoded as a string.
func rawJSON(b []byte) json.RawMessage { return json.RawMessage(b) }
