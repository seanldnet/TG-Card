package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func CreateAdminActionConfirmation(ctx context.Context, db *sql.DB, driver, adminID, actionKey, requestHash, tokenHash string, expiresAt time.Time) error {
	adminID = strings.TrimSpace(adminID)
	actionKey = strings.TrimSpace(actionKey)
	requestHash = strings.TrimSpace(requestHash)
	tokenHash = strings.TrimSpace(tokenHash)
	if adminID == "" || actionKey == "" || requestHash == "" || tokenHash == "" || !expiresAt.After(time.Now()) {
		return fmt.Errorf("administrator, action, request hash, token hash and future expiry are required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, "DELETE FROM admin_action_confirmations WHERE expires_at <= "+placeholder(driver, 1), time.Now().Unix()); err != nil {
		return err
	}
	query := "INSERT INTO admin_action_confirmations(token_hash,admin_id,action_key,request_hash,expires_at,created_at) VALUES(" +
		placeholder(driver, 1) + "," + placeholder(driver, 2) + "," + placeholder(driver, 3) + "," +
		placeholder(driver, 4) + "," + placeholder(driver, 5) + "," + placeholder(driver, 6) + ")"
	_, err := db.ExecContext(ctx, query, tokenHash, adminID, actionKey, requestHash, expiresAt.Unix(), now)
	return err
}

func ConsumeAdminActionConfirmation(ctx context.Context, db *sql.DB, driver, adminID, actionKey, requestHash, tokenHash string, now time.Time) (bool, error) {
	query := "UPDATE admin_action_confirmations SET consumed_at = " + placeholder(driver, 1) +
		" WHERE token_hash = " + placeholder(driver, 2) +
		" AND admin_id = " + placeholder(driver, 3) +
		" AND action_key = " + placeholder(driver, 4) +
		" AND request_hash = " + placeholder(driver, 5) +
		" AND expires_at > " + placeholder(driver, 6) +
		" AND consumed_at = ''"
	result, err := db.ExecContext(ctx, query, now.UTC().Format(time.RFC3339Nano), tokenHash, adminID, actionKey, requestHash, now.Unix())
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}
