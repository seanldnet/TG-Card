package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port string
	DBDriver string
	DatabaseDSN string
	AppEnv string
	DataDir string
	WebDistDir string
	JWTSecret string
	AdminUsername string
	AdminPassword string
	TronGridAPIKey string
	TronReceiveAddress string
	TronUSDTContract string
}

func Load() (Config, error) {
	dataDir := env("DATA_DIR", "./data")
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	if driver == "" {
		if dsn != "" { driver = "postgres" } else { driver = "sqlite" }
	}
	switch driver {
	case "postgres", "postgresql", "pgx":
		driver = "postgres"
		if dsn == "" { return Config{}, fmt.Errorf("DATABASE_URL is required when DB_DRIVER=postgres") }
	case "sqlite", "sqlite3":
		driver = "sqlite"
		if dsn == "" { dsn = filepath.Join(dataDir, "tg-card.db") }
	default:
		return Config{}, fmt.Errorf("unsupported DB_DRIVER %q: use postgres or sqlite", driver)
	}
	return Config{
		Port: env("PORT", "38647"), DBDriver: driver, DatabaseDSN: dsn,
		AppEnv: env("APP_ENV", "production"), DataDir: dataDir,
		WebDistDir: env("WEB_DIST_DIR", "./web/dist"),
		JWTSecret: strings.TrimSpace(os.Getenv("JWT_SECRET")),
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		TronGridAPIKey: strings.TrimSpace(os.Getenv("TRONGRID_API_KEY")),
		TronReceiveAddress: strings.TrimSpace(os.Getenv("TRON_RECEIVE_ADDRESS")),
		TronUSDTContract: env("TRON_USDT_CONTRACT", "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"),
	}, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" { return value }
	return fallback
}
