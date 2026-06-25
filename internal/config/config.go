package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv      string
	HTTPPort    string
	DatabaseURL string
	ORSAPIKey   string
	ORSBaseURL  string
	LogLevel    string
	LogFormat   string

	// Auth
	JWTSecret         string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	RefreshCookieName string
	CookieSecure      bool
	CookieSameSite    string

	// Redis + resilience. The two feature flags both require RedisEnabled;
	// app.New validates that and fails fast otherwise. All three default to
	// false so a bare `go run ./cmd/api` needs no Redis (the baseline mode).
	RedisEnabled      bool
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RateLimitEnabled  bool
	RouteCacheEnabled bool

	// MetricsEnabled exposes GET /api/v1/debug/stats (pgxpool + cache + runtime).
	// It leaks internals, so it defaults off and the handler is loopback-only.
	MetricsEnabled bool
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPPort:    getEnv("HTTP_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable"),
		ORSAPIKey:   getEnv("ORS_API_KEY", ""),
		ORSBaseURL:  getEnv("ORS_BASE_URL", "https://api.openrouteservice.org"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		LogFormat:   getEnv("LOG_FORMAT", "json"),

		JWTSecret:         getEnv("JWT_SECRET", ""),
		AccessTokenTTL:    time.Duration(getEnvInt("ACCESS_TOKEN_TTL_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL:   time.Duration(getEnvInt("REFRESH_TOKEN_TTL_DAYS", 7)) * 24 * time.Hour,
		RefreshCookieName: getEnv("REFRESH_COOKIE_NAME", "refresh_token"),
		CookieSecure:      getEnvBool("COOKIE_SECURE", false),
		CookieSameSite:    getEnv("COOKIE_SAMESITE", "lax"),

		RedisEnabled:      getEnvBool("REDIS_ENABLED", false),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           getEnvInt("REDIS_DB", 0),
		RateLimitEnabled:  getEnvBool("RATE_LIMIT_ENABLED", false),
		RouteCacheEnabled: getEnvBool("ROUTE_CACHE_ENABLED", false),

		MetricsEnabled: getEnvBool("METRICS_ENABLED", false),
	}
}

// IsDevelopment reports whether the app runs in the development environment,
// where insecure defaults (e.g. an empty JWT secret) are tolerated.
func (c Config) IsDevelopment() bool {
	return c.AppEnv == "development"
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}
