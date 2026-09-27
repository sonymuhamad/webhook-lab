//go:build wireinject

package di

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/httpapi"
	"github.com/sonymuhamad/webhook-lab/postgres"
	"github.com/sonymuhamad/webhook-lab/sender"
	"github.com/sonymuhamad/webhook-lab/usecase"
	"github.com/sonymuhamad/webhook-lab/worker"
)

var postgresSet = wire.NewSet(
	postgres.NewPool,
	postgres.NewTransactor,
	wire.Bind(new(webhook.Transactor), new(*postgres.Transactor)),
	postgres.NewTenantRepository,
	wire.Bind(new(webhook.TenantRepository), new(*postgres.TenantRepository)),
	postgres.NewEndpointRepository,
	wire.Bind(new(webhook.EndpointRepository), new(*postgres.EndpointRepository)),
	postgres.NewMessageRepository,
	wire.Bind(new(webhook.MessageRepository), new(*postgres.MessageRepository)),
	postgres.NewDeliveryRepository,
	wire.Bind(new(webhook.DeliveryRepository), new(*postgres.DeliveryRepository)),
)

func InitAPI(ctx context.Context, cfg config.Config) (*http.Server, func(), error) {
	wire.Build(
		wire.FieldsOf(new(config.Config), "HTTP", "Postgres", "Auth"),
		postgresSet,
		wire.Bind(new(httpapi.Pinger), new(*pgxpool.Pool)),
		usecase.NewTenant,
		wire.Bind(new(webhook.TenantUsecase), new(*usecase.Tenant)),
		usecase.NewEndpoint,
		wire.Bind(new(webhook.EndpointUsecase), new(*usecase.Endpoint)),
		usecase.NewMessage,
		wire.Bind(new(webhook.MessageUsecase), new(*usecase.Message)),
		httpapi.NewRouter,
		httpapi.NewServer,
	)
	return nil, nil, nil
}

func InitWorker(ctx context.Context, cfg config.Config) (*worker.Worker, func(), error) {
	wire.Build(
		wire.FieldsOf(new(config.Config), "Postgres", "Worker", "Delivery"),
		postgresSet,
		provideClaimer,
		sender.NewHTTP,
		wire.Bind(new(webhook.Sender), new(*sender.HTTP)),
		usecase.NewDelivery,
		wire.Bind(new(webhook.DeliveryUsecase), new(*usecase.Delivery)),
		worker.New,
	)
	return nil, nil, nil
}
