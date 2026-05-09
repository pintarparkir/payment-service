package configs

import (
	"log"

	"github.com/caarlos0/env"
	"github.com/joho/godotenv"
)

// loadEnvFile loads .env file when running outside container.
// In container/k8s, env is injected directly; missing .env is non-fatal.
func loadEnvFile(path string) {
	if path == "" {
		path = "configs/.env"
	}
	if err := godotenv.Load(path); err != nil {
		log.Printf("configs: skip loading %s: %v", path, err)
	}
}

// parseEnv reads tagged fields from environment into Config.
func parseEnv() Config {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("configs: parse env failed: %v", err)
	}
	return cfg
}
