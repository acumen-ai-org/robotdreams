package security

import "time"

// Clock abstracts time.Now so TTL and expiry logic can run against a fake clock.
type Clock interface {
	Now() time.Time
}

// RealClock is the default Clock backed by the system wall clock.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }
