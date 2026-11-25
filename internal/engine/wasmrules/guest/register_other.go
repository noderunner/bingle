//go:build !wasip1

package guest

// Register names the rules a module is built around. Built for anything but
// wasip1 there is no module, and it does nothing: rules are linked in with
// wasmrules.Native instead.
func Register(New) {}
