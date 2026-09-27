package webhook

//go:generate go tool mockgen -source=transactor.go -destination=mock/transactor.go -package=mock

import "context"

type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
