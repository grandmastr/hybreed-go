// Package config loads and validates runtime configuration from the environment.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Env      string `env:"ENV" envDefault:"development"`
	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8080"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	DatabaseURL string `env:"DATABASE_URL,required"`
	AutoMigrate bool   `env:"AUTO_MIGRATE" envDefault:"true"`

	RedisURL string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`

	JWTSecret       string        `env:"JWT_SECRET,required"`
	AccessTokenTTL  time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"720h"`
	OTPTTL          time.Duration `env:"OTP_TTL" envDefault:"10m"`
	OTPMaxAttempts  int           `env:"OTP_MAX_ATTEMPTS" envDefault:"5"`

	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
}

// IsProduction reports whether the app runs in the production environment.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads configuration from the process environment. For local development
// it first best-effort loads a `.env` file (ignored in production deployments,
// which inject real environment variables).
func Load() (Config, error) {
	_ = godotenv.Load()

	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}
