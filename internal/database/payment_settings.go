package database

import (
	"context"
	"database/sql"
	"time"
)

type PaymentSettings struct {
	TronAPIKeyCiphertext string
	TronReceiveAddress string
	TronUSDTContract string
	TronEnabled bool
}

func GetPaymentSettings(ctx context.Context, db *sql.DB, driver string) (PaymentSettings, error) {
	var s PaymentSettings
	var enabled int
	err := db.QueryRowContext(ctx, "SELECT tron_api_key_ciphertext, tron_receive_address, tron_usdt_contract, tron_enabled FROM payment_settings WHERE id=1").Scan(
		&s.TronAPIKeyCiphertext, &s.TronReceiveAddress, &s.TronUSDTContract, &enabled,
	)
	if err != nil { return s, err }
	s.TronEnabled = enabled == 1
	return s, nil
}

func SavePaymentSettings(ctx context.Context, db *sql.DB, driver string, s PaymentSettings) error {
	enabled := 0
	if s.TronEnabled { enabled = 1 }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "INSERT INTO payment_settings(id, tron_api_key_ciphertext, tron_receive_address, tron_usdt_contract, tron_enabled, updated_at) VALUES("+
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+") "+
		"ON CONFLICT(id) DO UPDATE SET tron_api_key_ciphertext=excluded.tron_api_key_ciphertext, tron_receive_address=excluded.tron_receive_address, tron_usdt_contract=excluded.tron_usdt_contract, tron_enabled=excluded.tron_enabled, updated_at=excluded.updated_at"
	_, err := db.ExecContext(ctx, query, 1, s.TronAPIKeyCiphertext, s.TronReceiveAddress, s.TronUSDTContract, enabled, now)
	return err
}
