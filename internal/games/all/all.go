// Package all registers every game family this repository ships, one file
// each, so that importing it gives a binary all of them. Each file has a
// build tag that leaves its family out: build with -tags no_cthulhuwars (or
// no_tak, no_classics, ...) for a binary without it. This is the one place a
// game is named to be included (game plugins plan, step 2).
package all
