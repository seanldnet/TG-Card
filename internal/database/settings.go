package database

import (
	"context"
	"database/sql"
	"time"
)

// GetSetting returns a persisted setting, or an empty value when it has not been set.
func GetSetting(ctx context.Context, db *sql.DB, driver, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = "+placeholder(driver, 1), key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SaveSettings upserts a group of settings in one transaction.
func SaveSettings(ctx context.Context, db *sql.DB, driver string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "INSERT INTO settings(key, value, updated_at) VALUES (" +
		placeholder(driver, 1) + "," + placeholder(driver, 2) + "," + placeholder(driver, 3) +
		") ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at"
	for key, value := range values {
		if _, err := tx.ExecContext(ctx, query, key, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
