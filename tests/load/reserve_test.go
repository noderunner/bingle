package load

import (
	"os"
	"strconv"

	"github.com/noderunner/bingle/internal/cpu"
)

// BINGLE_LIVE_CPUS keeps that many CPUs for live play in the load test, as it
// does in the server. This file is apart from the test so that the test alone
// still builds on a checkout from before the budget (ADR 0162).
func init() {
	if n, err := strconv.Atoi(os.Getenv("BINGLE_LIVE_CPUS")); err == nil && n > 0 {
		cpu.Shared().SetReserve(n)
	}
}
