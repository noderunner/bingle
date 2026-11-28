package batch

import "time"

// SetWall makes every batch's wall-clock limit d, so that a test need not wait
// the minute that is the least a setting can be.
func (m *Manager) SetWall(d time.Duration) { m.wall = func(Ceilings) time.Duration { return d } }
