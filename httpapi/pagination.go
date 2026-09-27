package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	webhook "github.com/sonymuhamad/webhook-lab"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

type paginationResponse struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
}

// parsePagination reads ?limit= and ?offset=. A missing value takes its
// default; an out-of-range value is rejected rather than clamped, so the
// client learns its request was wrong.
func parsePagination(r *http.Request) (webhook.Pagination, error) {
	limit, err := queryInt(r, "limit", defaultPageLimit)
	if err != nil {
		return webhook.Pagination{}, err
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		return webhook.Pagination{}, err
	}

	if limit < 1 || limit > maxPageLimit {
		return webhook.Pagination{}, webhook.ValidationError{
			Message: fmt.Sprintf("limit must be between 1 and %d", maxPageLimit),
		}
	}
	if offset < 0 {
		return webhook.Pagination{}, webhook.ValidationError{Message: "offset must not be negative"}
	}
	return webhook.Pagination{Limit: limit, Offset: offset}, nil
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, webhook.ValidationError{Message: name + " must be an integer"}
	}
	return n, nil
}
