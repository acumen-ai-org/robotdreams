package security

import "time"

// Clock abstracts time.Now so TTL/expiry logic can be tested
// deterministically with a fake clock instead of real wall-clock time.
type Clock interface {
	Now() time.Time
}

// RealClock is the default Clock backed by the system wall clock.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }
