package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                string
	HTTPPort              string
	MongoURI              string
	MongoDatabase         string
	MongoCrimesCollection string
	ORSAPIKey             string
	ORSBaseURL            string
	LogLevel              string
	LogFormat             string
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		AppEnv:                getEnv("APP_ENV", "development"),
		HTTPPort:              getEnv("HTTP_PORT", "8080"),
		MongoURI:              getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase:         getEnv("MONGO_DATABASE", "caba_routes"),
		MongoCrimesCollection: getEnv("MONGO_CRIMES_COLLECTION", "crimes"),
		ORSAPIKey:             getEnv("ORS_API_KEY", ""),
		ORSBaseURL:            getEnv("ORS_BASE_URL", "https://api.openrouteservice.org"),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		LogFormat:             getEnv("LOG_FORMAT", "json"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
