package config

import (
	"fmt"
	"os"
)

// Config holds the process-wide configuration read from the environment.
type Config struct {
	DatabaseURL string
	Port        string
}

// Load reads DATABASE_URL (required, set by docker-compose.yml for the api
// service) and PORT (optional, defaults to "8080") from the environment.
func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return Config{DatabaseURL: databaseURL, Port: port}, nil
}
