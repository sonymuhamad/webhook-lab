package enum

//go:generate go tool go-enum -f=deliverystatus.go --marshal --sql --names

// DeliveryStatus mirrors the CHECK constraint on deliveries.status; a new
// value needs a migration as well as a change here.
// ENUM(pending, succeeded, failed)
type DeliveryStatus string
