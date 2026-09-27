package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// registerPoolMetrics reports pool usage on every metric export. Acquire
// waits are the signal to watch under load: they count requests that found
// no idle connection and had to wait for one.
func registerPoolMetrics(pool *pgxpool.Pool) (metric.Registration, error) {
	meter := otel.Meter("github.com/sonymuhamad/webhook-lab/postgres")

	connections, err := meter.Int64ObservableGauge("db.pool.connections",
		metric.WithDescription("Pool connections by state."))
	if err != nil {
		return nil, fmt.Errorf("create connections gauge: %w", err)
	}
	maxConnections, err := meter.Int64ObservableGauge("db.pool.connections.max",
		metric.WithDescription("Maximum size of the pool."))
	if err != nil {
		return nil, fmt.Errorf("create max connections gauge: %w", err)
	}
	acquireWaits, err := meter.Int64ObservableCounter("db.pool.acquire.waits",
		metric.WithDescription("Acquires that waited because no idle connection was available."))
	if err != nil {
		return nil, fmt.Errorf("create acquire waits counter: %w", err)
	}

	acquired := metric.WithAttributes(attribute.String("state", "acquired"))
	idle := metric.WithAttributes(attribute.String("state", "idle"))
	return meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stat := pool.Stat()
		o.ObserveInt64(connections, int64(stat.AcquiredConns()), acquired)
		o.ObserveInt64(connections, int64(stat.IdleConns()), idle)
		o.ObserveInt64(maxConnections, int64(stat.MaxConns()))
		o.ObserveInt64(acquireWaits, stat.EmptyAcquireCount())
		return nil
	}, connections, maxConnections, acquireWaits)
}
