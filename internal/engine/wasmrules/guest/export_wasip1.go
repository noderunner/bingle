//go:build wasip1

package guest

import "unsafe"

// The module's side of ABI v0 (ADR 0147): the host writes a JSON request
// into memory it asked bingle_alloc for, calls an export with where it is,
// and gets back where the JSON reply is, as ptr<<32 | len. It copies the
// reply and gives it to bingle_free.

// registered is the Handler the exports answer from.
var registered = NewHandler(nil)

// Register names the rules a module is built around. A module's main package
// calls it from an init function: a module built with -buildmode=c-shared
// runs those, and never main.
func Register(n New) { registered = NewHandler(n) }

// held keeps every buffer the host has been given and has not freed, so the
// collector leaves it where it is. Go's collector does not move memory, so
// its address stays good.
var held = map[unsafe.Pointer][]byte{}

func hold(buf []byte) unsafe.Pointer {
	if len(buf) == 0 {
		return nil
	}
	p := unsafe.Pointer(unsafe.SliceData(buf))
	held[p] = buf
	return p
}

//go:wasmexport bingle_abi
func exportABI() int32 { return ABI }

//go:wasmexport bingle_alloc
func exportAlloc(size uint32) unsafe.Pointer { return hold(make([]byte, size)) }

//go:wasmexport bingle_free
func exportFree(ptr unsafe.Pointer, size uint32) { delete(held, ptr) }

// call answers one call. The request is read before anything is returned,
// and nothing of it is kept: the host may write its next request over it.
func call(name string, ptr unsafe.Pointer, size uint32) uint64 {
	var req []byte
	if size > 0 {
		req = unsafe.Slice((*byte)(ptr), size)
	}
	reply := registered.Call(name, req)
	return uint64(uintptr(hold(reply)))<<32 | uint64(len(reply))
}

//go:wasmexport bingle_init
func exportInit(ptr unsafe.Pointer, size uint32) uint64 { return call(CallInit, ptr, size) }

//go:wasmexport bingle_start
func exportStart(ptr unsafe.Pointer, size uint32) uint64 { return call(CallStart, ptr, size) }

//go:wasmexport bingle_impulse_seat
func exportImpulseSeat(ptr unsafe.Pointer, size uint32) uint64 {
	return call(CallImpulseSeat, ptr, size)
}

//go:wasmexport bingle_legal
func exportLegal(ptr unsafe.Pointer, size uint32) uint64 { return call(CallLegal, ptr, size) }

//go:wasmexport bingle_apply
func exportApply(ptr unsafe.Pointer, size uint32) uint64 { return call(CallApply, ptr, size) }

//go:wasmexport bingle_format
func exportFormat(ptr unsafe.Pointer, size uint32) uint64 { return call(CallFormat, ptr, size) }

//go:wasmexport bingle_parse
func exportParse(ptr unsafe.Pointer, size uint32) uint64 { return call(CallParse, ptr, size) }

//go:wasmexport bingle_gestures
func exportGestures(ptr unsafe.Pointer, size uint32) uint64 { return call(CallGestures, ptr, size) }

//go:wasmexport bingle_validate
func exportValidate(ptr unsafe.Pointer, size uint32) uint64 { return call(CallValidate, ptr, size) }

//go:wasmexport bingle_outcome
func exportOutcome(ptr unsafe.Pointer, size uint32) uint64 { return call(CallOutcome, ptr, size) }
