// Package config loads application configuration from the environment.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all runtime configuration. Values come from the environment so
// the same binary runs unchanged across local, CI, and production (12-factor).
type Config struct {
	Env         string        // "development" or "production"
	Port        string        // HTTP listen port
	DatabaseURL string        // Postgres DSN
	JWTSecret   string        // HMAC signing key for JWTs
	JWTTTL      time.Duration // token lifetime
}

// minJWTSecretLen is the shortest secret we accept. A short HMAC key materially
// weakens token security, so we fail fast rather than start insecurely.
const minJWTSecretLen = 16

// Load reads and validates configuration. It returns an error listing what is
// missing or invalid rather than panicking, so main can log it cleanly.
func Load() (*Config, error) {
	cfg := &Config{
		Env:         getEnv("ENV", "development"),
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
	}

	ttl, err := getDurationEnv("JWT_TTL", 24*time.Hour)
	if err != nil {
		return nil, err
	}
	cfg.JWTTTL = ttl

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < minJWTSecretLen {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least %d characters", minJWTSecretLen)
	}

	return cfg, nil
}

// IsProduction reports whether the app is running in production mode.
func (c *Config) IsProduction() bool { return c.Env == "production" }

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return d, nil
}
