package ports

import "time"

// RealClock is the production Clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

// FixedClock is for tests.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }
