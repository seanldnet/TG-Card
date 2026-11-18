package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"strings"
)

type UpstreamSettings struct {
	Provider string
	BaseURL string
	MerchantIDCiphertext string
	SecretCiphertext string
	Enabled bool
	CatalogStale bool
	MarkupBPS int64
	MarkupMinor int64
	ExchangeRatePPM int64
	SyncIntervalMinutes int
	ProcurementPollIntervalSeconds int
	ProcurementTimeoutMinutes int
	SupplierLowBalanceThreshold string
	CallbackURL string
	LastTestStatus string
	LastTestAt string
	LastError string
	SyncStatus string
	SyncAt string
	SyncAttemptAt string
	SyncError string
	UpdatedAt string
}

func GetUpstreamSettings(ctx context.Context, db *sql.DB, driver string) (UpstreamSettings, error) {
	var item UpstreamSettings
	var enabled, catalogStale int
	var lastTestAt, syncAt, lastAttemptAt sql.NullString
	err := db.QueryRowContext(ctx, "SELECT provider, base_url, merchant_id_ciphertext, secret_ciphertext, enabled, catalog_stale, markup_bps, markup_minor, exchange_rate_ppm, sync_interval_minutes, procurement_poll_interval_seconds, procurement_timeout_minutes, supplier_low_balance_threshold, callback_url, last_test_status, last_test_at, last_error, sync_status, sync_at, sync_attempt_at, sync_error, updated_at FROM upstream_settings WHERE id = 1").
		Scan(&item.Provider, &item.BaseURL, &item.MerchantIDCiphertext, &item.SecretCiphertext, &enabled, &catalogStale, &item.MarkupBPS, &item.MarkupMinor, &item.ExchangeRatePPM, &item.SyncIntervalMinutes, &item.ProcurementPollIntervalSeconds, &item.ProcurementTimeoutMinutes, &item.SupplierLowBalanceThreshold, &item.CallbackURL, &item.LastTestStatus, &lastTestAt, &item.LastError, &item.SyncStatus, &syncAt, &lastAttemptAt, &item.SyncError, &item.UpdatedAt)
	if err != nil { return UpstreamSettings{}, err }
	item.Enabled = enabled == 1
	if item.ProcurementPollIntervalSeconds < 5 { item.ProcurementPollIntervalSeconds = 30 }
	if item.ProcurementTimeoutMinutes < 1 { item.ProcurementTimeoutMinutes = 30 }
	if item.SupplierLowBalanceThreshold == "" { item.SupplierLowBalanceThreshold = "0" }
	item.CatalogStale = catalogStale == 1
	if lastTestAt.Valid { item.LastTestAt = lastTestAt.String }
	if syncAt.Valid { item.SyncAt = syncAt.String }
	if lastAttemptAt.Valid { item.SyncAttemptAt = lastAttemptAt.String }
	return item, nil
}

func SaveUpstreamSettings(ctx context.Context, db *sql.DB, driver string, item UpstreamSettings) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enabled := 0
	if item.Enabled { enabled = 1 }
	interval := item.SyncIntervalMinutes
	if interval < 5 || interval > 1440 { interval = 15 }
	pollInterval := item.ProcurementPollIntervalSeconds
	if pollInterval < 5 || pollInterval > 3600 { pollInterval = 30 }
	timeoutMinutes := item.ProcurementTimeoutMinutes
	if timeoutMinutes < 1 || timeoutMinutes > 1440 { timeoutMinutes = 30 }
	threshold := strings.TrimSpace(item.SupplierLowBalanceThreshold)
	if threshold == "" { threshold = "0" }
	query := "INSERT INTO upstream_settings(id, provider, base_url, merchant_id_ciphertext, secret_ciphertext, enabled, catalog_stale, markup_bps, markup_minor, exchange_rate_ppm, sync_interval_minutes, last_test_status, last_test_at, last_error, sync_status, sync_at, sync_error, created_at, updated_at) VALUES ("+
		placeholder(driver,1)+", "+placeholder(driver,2)+", "+placeholder(driver,3)+", "+placeholder(driver,4)+", "+placeholder(driver,5)+", "+placeholder(driver,6)+", "+placeholder(driver,7)+", "+placeholder(driver,8)+", "+placeholder(driver,9)+", "+placeholder(driver,10)+", "+placeholder(driver,11)+", 'not_tested', NULL, '', 'not_started', NULL, '', "+placeholder(driver,12)+", "+placeholder(driver,13)+") "+
		"ON CONFLICT(id) DO UPDATE SET provider=excluded.provider, base_url=excluded.base_url, merchant_id_ciphertext=excluded.merchant_id_ciphertext, secret_ciphertext=excluded.secret_ciphertext, enabled=excluded.enabled, catalog_stale=excluded.catalog_stale, markup_bps=excluded.markup_bps, markup_minor=excluded.markup_minor, exchange_rate_ppm=excluded.exchange_rate_ppm, sync_interval_minutes=excluded.sync_interval_minutes, procurement_poll_interval_seconds=excluded.procurement_poll_interval_seconds, procurement_timeout_minutes=excluded.procurement_timeout_minutes, supplier_low_balance_threshold=excluded.supplier_low_balance_threshold, callback_url=excluded.callback_url, last_test_status='not_tested', last_test_at=NULL, last_error='', updated_at=excluded.updated_at"
	if _, err := db.ExecContext(ctx, query, 1, item.Provider, item.BaseURL, item.MerchantIDCiphertext, item.SecretCiphertext, enabled, boolInt(item.CatalogStale), item.MarkupBPS, item.MarkupMinor, item.ExchangeRatePPM, interval, pollInterval, timeoutMinutes, threshold, item.CallbackURL, now, now); err != nil {
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

func SaveUpstreamSyncResult(ctx context.Context, db *sql.DB, driver, status, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE upstream_settings SET sync_status="+placeholder(driver,1)+", sync_error="+placeholder(driver,2)+", sync_attempt_at="+placeholder(driver,3)+", sync_at=CASE WHEN "+placeholder(driver,4)+"='success' THEN "+placeholder(driver,5)+" ELSE sync_at END, updated_at="+placeholder(driver,6)+" WHERE id=1"
	result, err := db.ExecContext(ctx, query, status, message, now, status, now, now)
	if err != nil { return fmt.Errorf("save upstream sync result: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected != 1 { return sql.ErrNoRows }
	return nil
}
