package webhook

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrConflict     = errors.New("conflict")
)

// ValidationError is an input problem the caller can fix; its message is safe
// to return to the client as-is.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// ConflictError means the request clashes with existing data; its message is
// safe to return to the client as-is.
type ConflictError struct {
	Message string
}

func (e ConflictError) Error() string {
	return e.Message
}
