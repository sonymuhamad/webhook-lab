package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/sonymuhamad/webhook-lab/config"
)

// Setup installs the global meter provider and returns a slog handler that
// exports logs, or nil when log export is off. The returned shutdown flushes
// buffered telemetry and must run before the process exits.
//
// Each process gets a random service.instance.id, which Prometheus turns
// into the instance label, so several workers show up as separate series.
func Setup(ctx context.Context, cfg config.Telemetry, serviceName string) (slog.Handler, func(context.Context) error, error) {
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceInstanceID(uuid.NewString()),
	))
	if err != nil {
		return nil, nil, fmt.Errorf("build resource: %w", err)
	}

	var shutdowns []func(context.Context) error
	shutdown := func(ctx context.Context) error {
		var errs []error
		for _, fn := range shutdowns {
			errs = append(errs, fn(ctx))
		}
		return errors.Join(errs...)
	}

	if cfg.MetricsEndpoint != "" {
		exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(cfg.MetricsEndpoint))
		if err != nil {
			return nil, nil, fmt.Errorf("create metric exporter: %w", err)
		}
		provider := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		)
		otel.SetMeterProvider(provider)
		shutdowns = append(shutdowns, provider.Shutdown)
	}

	var logHandler slog.Handler
	if cfg.LogsEndpoint != "" {
		exporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(cfg.LogsEndpoint))
		if err != nil {
			return nil, nil, errors.Join(fmt.Errorf("create log exporter: %w", err), shutdown(ctx))
		}
		provider := sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		)
		logHandler = otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(provider))
		shutdowns = append(shutdowns, provider.Shutdown)
	}

	return logHandler, shutdown, nil
}

// NewLogger writes JSON logs to stdout and, when export is not nil, sends the
// same records to it. Both destinations drop records below level.
func NewLogger(level slog.Level, export slog.Handler) *slog.Logger {
	stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	if export == nil {
		return slog.New(stdout)
	}
	return slog.New(slog.NewMultiHandler(stdout, levelHandler{level: level, Handler: export}))
}

// levelHandler adds a minimum level to a handler that has no option for one.
type levelHandler struct {
	level slog.Level
	slog.Handler
}

func (h levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level && h.Handler.Enabled(ctx, level)
}

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{level: h.level, Handler: h.Handler.WithAttrs(attrs)}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{level: h.level, Handler: h.Handler.WithGroup(name)}
}
