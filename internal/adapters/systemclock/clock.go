package systemclock

import "time"

// Clock implements ports.Clock using the local system wall clock.
type Clock struct{}

// New returns a standard-library wall clock adapter.
func New() Clock { return Clock{} }

// Now returns the current local time. Callers should prefer UTC when persisting.
func (Clock) Now() time.Time { return time.Now() }
