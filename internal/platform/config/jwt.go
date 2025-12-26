package config

import (
	"fmt"
	"os"
	"time"
)

// JWTConfig configures JWT verification against a JWKS endpoint.
//
// These values are deployment-provided (see docs/plan-service-implementation.md).
type JWTConfig struct {
	Issuer   string
	Audience string
	JWKSURL  string

	ClockSkew              time.Duration
	JWKSRefreshInterval    time.Duration
	JWKSMinRefreshInterval time.Duration

	HTTPTimeout time.Duration
}

func LoadJWTConfigFromEnv() (JWTConfig, error) {
	issuer := os.Getenv("JWT_ISSUER")
	audience := os.Getenv("JWT_AUDIENCE")
	jwksURL := os.Getenv("JWT_JWKS_URL")
	if issuer == "" || audience == "" || jwksURL == "" {
		return JWTConfig{}, fmt.Errorf("missing required env vars: JWT_ISSUER, JWT_AUDIENCE, JWT_JWKS_URL")
	}

	// Reasonable defaults that make local/dev/test behavior predictable.
	cfg := JWTConfig{
		Issuer:    issuer,
		Audience:  audience,
		JWKSURL:   jwksURL,
		ClockSkew: 30 * time.Second,
		// Refresh periodically to pick up key rotation even if an old key is still cached.
		JWKSRefreshInterval: 5 * time.Minute,
		// Bound refresh frequency when a token presents an unknown kid (avoid thundering herd).
		JWKSMinRefreshInterval: 10 * time.Second,
		HTTPTimeout:            5 * time.Second,
	}

	if v := os.Getenv("JWT_CLOCK_SKEW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return JWTConfig{}, fmt.Errorf("JWT_CLOCK_SKEW must be a duration (e.g. 30s): %w", err)
		}
		cfg.ClockSkew = d
	}
	if v := os.Getenv("JWT_JWKS_REFRESH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return JWTConfig{}, fmt.Errorf("JWT_JWKS_REFRESH_INTERVAL must be a duration (e.g. 5m): %w", err)
		}
		cfg.JWKSRefreshInterval = d
	}
	if v := os.Getenv("JWT_JWKS_MIN_REFRESH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return JWTConfig{}, fmt.Errorf("JWT_JWKS_MIN_REFRESH_INTERVAL must be a duration (e.g. 10s): %w", err)
		}
		cfg.JWKSMinRefreshInterval = d
	}

	return cfg, nil
}
