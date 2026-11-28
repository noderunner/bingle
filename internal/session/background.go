package session

import (
	"context"
	"time"
)

// The manager starts some work of its own, away from the request that asked
// for it: a finished game's count and rated result, a tournament moving on,
// bots playing on, a bot match, a turn notification, a reader's chat
// catching up. It is started with detach, and Close stops it and waits for
// it, so that none of it is still using the store when the store closes.

// closeWait bounds how long Close waits for that work. Everything it does
// with the store ends with its context; what can outlast that is a bot
// thinking about a move, which ends within its own limit (botMoveTimeout).
const closeWait = 10 * time.Second

// detach runs work in a goroutine of its own, with a context that ends when
// the manager is closed, and Close waits for it. Once Close has begun it
// starts nothing and reports false.
func (m *Manager) detach(work func(ctx context.Context)) bool {
	m.bgMu.Lock()
	defer m.bgMu.Unlock()
	if m.stopped {
		return false
	}
	m.bg.Add(1)
	go func() {
		defer m.bg.Done()
		work(m.stopCtx)
	}()
	return true
}

// Close stops the work the manager started on its own and waits for it, up
// to closeWait. The manager's store calls it as it closes (see
// store.Store.OnClose), so a caller need not; calling it again does no harm.
// The manager is not to be used after it.
func (m *Manager) Close() {
	m.bgMu.Lock()
	m.stopped = true
	m.bgMu.Unlock()
	m.stop()
	done := make(chan struct{})
	go func() {
		m.bg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	default:
	}
	m.logger().Debug("stopping: waiting for background work")
	select {
	case <-done:
	case <-time.After(closeWait):
		m.logger().Warn("stopping: background work still running", "waited", closeWait)
	}
}
