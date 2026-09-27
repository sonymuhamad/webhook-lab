package enum

//go:generate go tool go-enum -f=claimmode.go --marshal --names

// ClaimMode picks how the worker takes due deliveries off the queue. naive
// exists only so lab 01 can measure what it costs.
// ENUM(naive, skiplocked)
type ClaimMode string
