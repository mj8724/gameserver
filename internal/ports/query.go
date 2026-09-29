package ports

import "context"

// GameQuerier answers the live game query surface (M3.4). It is strictly
// additive: the query result never influences ready/readiness semantics, and
// an unreachable game yields unavailable fields rather than an error.
type GameQuerier interface {
	Query(ctx context.Context, host string, port int) (GameQueryInfo, error)
}

// GameQueryInfo is the parsed A2S info projected into the status response.
type GameQueryInfo struct {
	Name    string
	Map     string
	Players int
	Max     int
}
