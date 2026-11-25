//go:build wasip1 && !bingle_nobot

package guest

import "unsafe"

// The bot call is optional in ABI v0. A module built with the tag
// bingle_nobot leaves it out, as a module built before the call was added
// does, which is how the host's own search over a module is tested.

//go:wasmexport bingle_bot
func exportBot(ptr unsafe.Pointer, size uint32) uint64 { return call(CallBot, ptr, size) }
