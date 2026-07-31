// Package connector defines the abstraction that every monitored-target
// implementation (Unleash, and future connector types) must satisfy.
package connector

import "context"

// Connector reports whether a single monitored target currently satisfies
// its configured pass condition.
type Connector interface {
	// Check reports whether the target currently passes. A non-nil error
	// means the check itself could not be completed (e.g. a network
	// failure); it does not by itself mean the target failed its
	// condition.
	Check(ctx context.Context) (bool, error)
}
