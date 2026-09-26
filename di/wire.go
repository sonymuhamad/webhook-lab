//go:build wireinject

package di

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/httpapi"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

func InitAPI(ctx context.Context, cfg config.Config) (*http.Server, func(), error) {
	wire.Build(
		wire.FieldsOf(new(config.Config), "HTTP", "Postgres"),
		postgres.NewPool,
		wire.Bind(new(httpapi.Pinger), new(*pgxpool.Pool)),
		httpapi.NewRouter,
		httpapi.NewServer,
	)
	return nil, nil, nil
}
