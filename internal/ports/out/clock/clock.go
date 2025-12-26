package clock

import "time"

// Clock provides time to the application.
// Using an interface enables deterministic tests via a controllable implementation.
type Clock interface {
	Now() time.Time
}
