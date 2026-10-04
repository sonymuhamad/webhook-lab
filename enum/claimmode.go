package enum

//go:generate go tool go-enum -f=claimmode.go --marshal --names

// ClaimMode picks how the worker takes due deliveries off the queue. naive
// exists only so lab 01 can measure what it costs. skiplocked takes the
// oldest due deliveries of all tenants; fair takes them from each tenant in
// turn, so one tenant's backlog does not delay the others (lab 03).
// ENUM(naive, skiplocked, fair)
type ClaimMode string
