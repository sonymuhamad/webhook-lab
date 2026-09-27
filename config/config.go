package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/sonymuhamad/webhook-lab/enum"
)

const minAdminTokenLength = 32

type Config struct {
	LogLevel  slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	HTTP      HTTP       `envPrefix:"HTTP_"`
	Postgres  Postgres   `envPrefix:"POSTGRES_"`
	Auth      Auth
	Worker    Worker    `envPrefix:"WORKER_"`
	Delivery  Delivery  `envPrefix:"DELIVERY_"`
	Telemetry Telemetry `envPrefix:"OTEL_EXPORTER_OTLP_"`
}

// Telemetry uses the standard OpenTelemetry variable names. An empty endpoint
// turns that signal off. Other OTEL_* variables, such as
// OTEL_METRIC_EXPORT_INTERVAL, are read by the SDK directly.
type Telemetry struct {
	MetricsEndpoint string `env:"METRICS_ENDPOINT"`
	LogsEndpoint    string `env:"LOGS_ENDPOINT"`
}

type HTTP struct {
	Addr            string        `env:"ADDR" envDefault:":8080"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

type Postgres struct {
	URL string `env:"URL,required"`
}

type Auth struct {
	AdminToken string `env:"ADMIN_TOKEN"`
}

type Worker struct {
	Count        int            `env:"COUNT" envDefault:"1"`
	BatchSize    int            `env:"BATCH_SIZE" envDefault:"10"`
	PollInterval time.Duration  `env:"POLL_INTERVAL" envDefault:"500ms"`
	ClaimMode    enum.ClaimMode `env:"CLAIM_MODE" envDefault:"skiplocked"`
	// ClaimLease is how long a claimed delivery stays hidden from other
	// workers. It must outlast a whole batch, which is sent one delivery at a
	// time: BatchSize × DELIVERY_TIMEOUT in the worst case.
	ClaimLease time.Duration `env:"CLAIM_LEASE" envDefault:"2m"`
}

type Delivery struct {
	Timeout     time.Duration `env:"TIMEOUT" envDefault:"10s"`
	MaxAttempts int           `env:"MAX_ATTEMPTS" envDefault:"5"`
	RetryDelay  time.Duration `env:"RETRY_DELAY" envDefault:"30s"`
}

// Load reads configuration from the environment. A .env file in the working
// directory is applied first when present; outside local development it is
// expected to be absent.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}
	return cfg, nil
}

// Validate is called only by the API, so the worker and migrator can run
// without the admin secret. A short or empty token would make the admin
// routes guessable, so the API refuses to start with one.
func (a Auth) Validate() error {
	if len(a.AdminToken) < minAdminTokenLength {
		return fmt.Errorf("ADMIN_TOKEN must be at least %d characters", minAdminTokenLength)
	}
	return nil
}
