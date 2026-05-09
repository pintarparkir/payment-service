package configs

// NewConfig is the public entry point: returns a fully-resolved Config.
//
// Resolution order: built-in defaults (env tag) ← .env file ← OS env vars.
// In production, secrets come from injected env vars (k8s secret, Vault sidecar).
func NewConfig(loader ConfigLoader) Config {
	if loader.Env != "production" {
		loadEnvFile(loader.EnvFile)
	}
	return parseEnv()
}
