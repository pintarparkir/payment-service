package configs

// Config holds payment-service configuration.
// All fields are loaded from environment variables with safe defaults.
type Config struct {
	AppName  string `env:"APP_NAME" envDefault:"payment-service"`
	AppEnv   string `env:"APP_ENV" envDefault:"local"`
	AppPort  string `env:"APP_PORT" envDefault:"8083"`  // REST: mini app + Midtrans webhook
	GrpcPort string `env:"GRPC_PORT" envDefault:"9092"` // gRPC for s2s callers

	DbHost     string `env:"DB_HOST" envDefault:"localhost"`
	DbPort     string `env:"DB_PORT" envDefault:"5432"`
	DbUsername string `env:"DB_USERNAME" envDefault:"postgres"`
	DbPassword string `env:"DB_PASSWORD" envDefault:"postgres"`
	DbName     string `env:"DB_NAME" envDefault:"payment_service"`
	DbMaxOpen  int    `env:"DB_MAX_OPEN" envDefault:"25"`
	DbMaxIdle  int    `env:"DB_MAX_IDLE" envDefault:"10"`

	RedisHost      string `env:"REDIS_HOST" envDefault:"localhost"`
	RedisPort      string `env:"REDIS_PORT" envDefault:"6379"`
	RedisPassword  string `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB        int    `env:"REDIS_DB" envDefault:"0"`
	RedisAppConfig string `env:"REDIS_APP_CONFIG" envDefault:"payment"`

	RabbitURL      string `env:"RABBIT_URL" envDefault:"amqp://guest:guest@localhost:5672/"`
	RabbitExchange string `env:"RABBIT_EXCHANGE" envDefault:"parkirpintar.events"`

	OTLPEndpoint string `env:"OTLP_ENDPOINT" envDefault:"localhost:4317"`

	// RS256 public key PEM from super-app. Empty = skip signature check (dev).
	SuperAppJWTPubKey string `env:"SUPER_APP_JWT_PUBLIC_KEY_PEM" envDefault:""`

	// ── Midtrans QRIS sandbox ──
	MidtransServerKey     string `env:"MIDTRANS_SERVER_KEY" envDefault:"SB-Mid-server-XXXXXXXX"`
	MidtransBaseURL       string `env:"MIDTRANS_BASE_URL" envDefault:"https://api.sandbox.midtrans.com/v2"`
	MidtransWebhookSecret string `env:"MIDTRANS_WEBHOOK_SECRET" envDefault:"change-me-in-prod"`

	// Stub mode (default true in local) bypasses real Midtrans calls and returns
	// synthetic QRIS payloads. Flip to false to call the sandbox.
	MidtransStubMode bool `env:"MIDTRANS_STUB_MODE" envDefault:"true"`

	// gRPC client to billing-service for invoice amount lookup at intent time.
	BillingGrpcAddr string `env:"BILLING_GRPC_ADDR" envDefault:"localhost:9091"`
}

// ConfigLoader controls the source of config (env file path, etc.).
type ConfigLoader struct {
	Env     string
	EnvFile string
}
