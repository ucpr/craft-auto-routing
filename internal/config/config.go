// Package config resolves credentials and endpoints from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds the credentials and endpoints needed to talk to Craft and
// TypeSafe.
type Config struct {
	CraftBaseURL   string
	CraftAPIToken  string
	TypesafeAPIKey string
	TypesafeModel  string
}

// Load reads configuration from environment variables, applying defaults
// where sensible. It returns an error if required credentials are missing.
//
// CraftBaseURL has no default: it embeds a Craft Connect link id that is
// specific to each user's own space, so it must be supplied via
// CRAFT_BASE_URL rather than baked into the binary.
func Load() (Config, error) {
	cfg := Config{
		CraftBaseURL:   os.Getenv("CRAFT_BASE_URL"),
		CraftAPIToken:  os.Getenv("CRAFT_API_TOKEN"),
		TypesafeAPIKey: os.Getenv("TYPESAFE_API_KEY"),
		TypesafeModel:  envOr("TYPESAFE_MODEL", "jev-latest"),
	}

	if cfg.CraftBaseURL == "" {
		return Config{}, fmt.Errorf("CRAFT_BASE_URL is not set")
	}
	if cfg.CraftAPIToken == "" {
		return Config{}, fmt.Errorf("CRAFT_API_TOKEN is not set")
	}
	if cfg.TypesafeAPIKey == "" {
		return Config{}, fmt.Errorf("TYPESAFE_API_KEY is not set")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
