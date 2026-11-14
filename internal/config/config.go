package config

import (
	"os"
	"strings"
)

type Config struct {
	Port string
	DBDriver string
	DatabaseDSN string
	AppEnv string
}

func Load() Config {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	if driver == "" {
		if dsn != "" { driver = "postgres" } else { driver = "sqlite" }
	}
	if dsn == "" { dsn = "./data/tg-card.db" }
	return Config{Port: env("PORT", "38647"), DBDriver: driver, DatabaseDSN: dsn, AppEnv: env("APP_ENV", "production")}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" { return value }
	return fallback
}
