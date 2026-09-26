package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	LogLevel slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	HTTP     HTTP       `envPrefix:"HTTP_"`
	Postgres Postgres   `envPrefix:"POSTGRES_"`
}

type HTTP struct {
	Addr            string        `env:"ADDR" envDefault:":8080"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

type Postgres struct {
	URL string `env:"URL,required"`
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
