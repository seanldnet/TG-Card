package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type UpstreamSettings struct {
	Provider string
	BaseURL string
	MerchantIDCiphertext string
	SecretCiphertext string
	Enabled bool
	LastTestStatus string
	LastTestAt string
	LastError string
	SyncStatus string
	SyncAt string
	SyncError string
	UpdatedAt string
}

func GetUpstreamSettings(ctx context.Context, db *sql.DB, driver string) (UpstreamSettings, error) {
	var item UpstreamSettings
	var enabled int
	var lastTestAt, syncAt sql.NullString
	err := db.QueryRowContext(ctx, "SELECT provider, base_url, merchant_id_ciphertext, secret_ciphertext, enabled, last_test_status, last_test_at, last_error, sync_status, sync_at, sync_error, updated_at FROM upstream_settings WHERE id = 1").
		Scan(&item.Provider, &item.BaseURL, &item.MerchantIDCiphertext, &item.SecretCiphertext, &enabled, &item.LastTestStatus, &lastTestAt, &item.LastError, &item.SyncStatus, &syncAt, &item.SyncError, &item.UpdatedAt)
	if err != nil { return UpstreamSettings{}, err }
	item.Enabled = enabled == 1
	if lastTestAt.Valid { item.LastTestAt = lastTestAt.String }
	if syncAt.Valid { item.SyncAt = syncAt.String }
	return item, nil
}

func SaveUpstreamSettings(ctx context.Context, db *sql.DB, driver string, item UpstreamSettings) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enabled := 0
	if item.Enabled { enabled = 1 }
	query := "INSERT INTO upstream_settings(id, provider, base_url, merchant_id_ciphertext, secret_ciphertext, enabled, last_test_status, last_test_at, last_error, sync_status, sync_at, sync_error, created_at, updated_at) VALUES ("+
		placeholder(driver,1)+", "+placeholder(driver,2)+", "+placeholder(driver,3)+", "+placeholder(driver,4)+", "+placeholder(driver,5)+", "+placeholder(driver,6)+", 'not_tested', NULL, '', 'not_started', NULL, '', "+placeholder(driver,7)+", "+placeholder(driver,8)+") "+
		"ON CONFLICT(id) DO UPDATE SET provider=excluded.provider, base_url=excluded.base_url, merchant_id_ciphertext=excluded.merchant_id_ciphertext, secret_ciphertext=excluded.secret_ciphertext, enabled=excluded.enabled, last_test_status='not_tested', last_test_at=NULL, last_error='', updated_at=excluded.updated_at"
	if _, err := db.ExecContext(ctx, query, 1, item.Provider, item.BaseURL, item.MerchantIDCiphertext, item.SecretCiphertext, enabled, now, now); err != nil {
		return fmt.Errorf("save upstream settings: %w", err)
	}
	return nil
}

func SaveUpstreamTestResult(ctx context.Context, db *sql.DB, driver, status, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE upstream_settings SET last_test_status="+placeholder(driver,1)+", last_test_at="+placeholder(driver,2)+", last_error="+placeholder(driver,3)+", updated_at="+placeholder(driver,4)+" WHERE id=1"
	result, err := db.ExecContext(ctx, query, status, now, message, now)
	if err != nil { return fmt.Errorf("save upstream test result: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected != 1 { return sql.ErrNoRows }
	return nil
}
