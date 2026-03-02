package robustness

import (
	"sync"
	"time"
)

// Clock exposes a minimal time source for deterministic test flows.
type Clock interface {
	Now() time.Time
}

// StaticClock is a controllable in-memory clock for tests.
type StaticClock struct {
	mu  sync.RWMutex
	now time.Time
}

// NewStaticClock creates a clock seeded with the provided timestamp.
func NewStaticClock(now time.Time) *StaticClock {
	return &StaticClock{now: now}
}

// Now returns the current clock time.
func (c *StaticClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Set sets the current time.
func (c *StaticClock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// Advance moves current time forward by delta.
func (c *StaticClock) Advance(delta time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
}
