package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/seanldnet/TG-Card/internal/config"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Admin struct {
	ID string
	Username string
	PasswordHash string
	RealName string
	Role string
	Status int
	TokenVersion int
}

type BootstrapResult struct {
	Username string
	Password string
	Created bool
	Generated bool
}

func Open(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	driver, dsn := "sqlite", cfg.DatabaseDSN
	if cfg.DBDriver == "postgres" {
		driver = "pgx"
	} else if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o750); err != nil { return nil, fmt.Errorf("create database directory: %w", err) }
	}
	db, err := sql.Open(driver, dsn)
	if err != nil { return nil, fmt.Errorf("open %s database: %w", cfg.DBDriver, err) }
	if cfg.DBDriver == "sqlite" {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
			if _, err := db.ExecContext(ctx, pragma); err != nil { _ = db.Close(); return nil, fmt.Errorf("configure SQLite: %w", err) }
		}
	} else {
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil { _ = db.Close(); return nil, fmt.Errorf("connect to database: %w", err) }
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB, driver string) error {
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil { return err }
	for _, name := range files {
		base := filepath.Base(name)
		versionText := strings.SplitN(base, "_", 2)[0]
		version, err := strconv.Atoi(versionText)
		if err != nil { return fmt.Errorf("invalid migration filename %q", name) }
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = "+placeholder(driver, 1), version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 { continue }
		content, err := migrationFiles.ReadFile(name)
		if err != nil { return err }
		tx, err := db.BeginTx(ctx, nil)
		if err != nil { return fmt.Errorf("begin migration %d: %w", version, err) }
		for _, statement := range strings.Split(string(content), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" { continue }
			if _, err := tx.ExecContext(ctx, statement); err != nil { _ = tx.Rollback(); return fmt.Errorf("migration %d failed: %w", version, err) }
		}
		insert := "INSERT INTO schema_migrations(version, applied_at) VALUES ("+placeholder(driver, 1)+", "+placeholder(driver, 2)+")"
		if _, err := tx.ExecContext(ctx, insert, version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil { return fmt.Errorf("commit migration %d: %w", version, err) }
	}
	return nil
}

func EnsureAdmin(ctx context.Context, db *sql.DB, driver, username, password string) (BootstrapResult, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM admins").Scan(&count); err != nil { return BootstrapResult{}, fmt.Errorf("count administrators: %w", err) }
	if count > 0 { return BootstrapResult{Username: username}, nil }
	if len(username) < 3 || len(username) > 64 { return BootstrapResult{}, fmt.Errorf("ADMIN_USERNAME must contain 3 to 64 characters") }
	generated := false
	if password == "" {
		var err error
		password, err = randomPassword()
		if err != nil { return BootstrapResult{}, err }
		generated = true
	}
	if len(password) < 12 { return BootstrapResult{}, fmt.Errorf("ADMIN_PASSWORD must contain at least 12 characters") }
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil { return BootstrapResult{}, fmt.Errorf("hash initial administrator password: %w", err) }
	id, err := randomID()
	if err != nil { return BootstrapResult{}, err }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "INSERT INTO admins(id, username, password_hash, real_name, role, status, created_at, updated_at) VALUES ("+
		placeholder(driver,1)+", "+placeholder(driver,2)+", "+placeholder(driver,3)+", "+placeholder(driver,4)+", "+
		placeholder(driver,5)+", "+placeholder(driver,6)+", "+placeholder(driver,7)+", "+placeholder(driver,8)+")"
	if _, err := db.ExecContext(ctx, query, id, username, string(hash), "Administrator", "super", 1, now, now); err != nil {
		return BootstrapResult{}, fmt.Errorf("create initial administrator: %w", err)
	}
	return BootstrapResult{Username: username, Password: password, Created: true, Generated: generated}, nil
}

func FindAdminByUsername(ctx context.Context, db *sql.DB, driver, username string) (Admin, error) {
	query := "SELECT id, username, password_hash, real_name, role, status, token_version FROM admins WHERE username = "+placeholder(driver,1)
	var admin Admin
	err := db.QueryRowContext(ctx, query, username).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.RealName, &admin.Role, &admin.Status, &admin.TokenVersion)
	return admin, err
}

func UpdateLastLogin(ctx context.Context, db *sql.DB, driver, adminID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE admins SET last_login_at = "+placeholder(driver,1)+", updated_at = "+placeholder(driver,2)+" WHERE id = "+placeholder(driver,3)
	_, err := db.ExecContext(ctx, query, now, now, adminID)
	return err
}

func RecordLoginAudit(ctx context.Context, db *sql.DB, driver, adminID, username, ip, userAgent string, success bool) {
	id, err := randomID()
	if err != nil { return }
	details := "failed"
	if success { details = "success" }
	query := "INSERT INTO audit_logs(id, admin_id, action, resource, ip_address, user_agent, details, created_at) VALUES ("+
		placeholder(driver,1)+", "+placeholder(driver,2)+", "+placeholder(driver,3)+", "+placeholder(driver,4)+", "+
		placeholder(driver,5)+", "+placeholder(driver,6)+", "+placeholder(driver,7)+", "+placeholder(driver,8)+")"
	_, _ = db.ExecContext(ctx, query, id, nullableString(adminID), "auth.login", "admin:"+username, ip, userAgent, details, time.Now().UTC().Format(time.RFC3339Nano))
}

func placeholder(driver string, index int) string {
	if driver == "postgres" { return fmt.Sprintf("$%d", index) }
	return "?"
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil { return "", fmt.Errorf("generate identifier: %w", err) }
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func randomPassword() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil { return "", fmt.Errorf("generate initial administrator password: %w", err) }
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func nullableString(value string) any {
	if value == "" { return nil }
	return value
}


func FindAdminByID(ctx context.Context, db *sql.DB, driver, adminID string) (Admin, error) {
	query := "SELECT id, username, password_hash, real_name, role, status, token_version FROM admins WHERE id = " + placeholder(driver, 1)
	var admin Admin
	err := db.QueryRowContext(ctx, query, adminID).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.RealName, &admin.Role, &admin.Status, &admin.TokenVersion)
	return admin, err
}

func UpdateAdminPassword(ctx context.Context, db *sql.DB, driver, adminID, passwordHash string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE admins SET password_hash = " + placeholder(driver, 1) + ", token_version = token_version + 1, updated_at = " + placeholder(driver, 2) + " WHERE id = " + placeholder(driver, 3)
	result, err := db.ExecContext(ctx, query, passwordHash, now, adminID)
	if err != nil { return err }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected != 1 { return fmt.Errorf("administrator not found") }
	return nil
}

func RecordAdminAudit(ctx context.Context, db *sql.DB, driver, adminID, action, resource, ip, userAgent, details string) {
	id, err := randomID()
	if err != nil { return }
	query := "INSERT INTO audit_logs(id, admin_id, action, resource, ip_address, user_agent, details, created_at) VALUES ("+
		placeholder(driver,1)+", "+placeholder(driver,2)+", "+placeholder(driver,3)+", "+placeholder(driver,4)+", "+
		placeholder(driver,5)+", "+placeholder(driver,6)+", "+placeholder(driver,7)+", "+placeholder(driver,8)+")"
	_, _ = db.ExecContext(ctx, query, id, adminID, action, resource, ip, userAgent, details, time.Now().UTC().Format(time.RFC3339Nano))
}

type Bot struct {
	ID string
	Name string
	Username string
	TokenCiphertext string
	TokenFingerprint string
	Status int
	WelcomeText string
	MenuConfig string
	CreatedAt string
	UpdatedAt string
}

func ListBots(ctx context.Context, db *sql.DB) ([]Bot, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name, username, token_ciphertext, token_fingerprint, status, welcome_text, menu_config, created_at, updated_at FROM bots ORDER BY created_at DESC")
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]Bot, 0)
	for rows.Next() {
		var item Bot
		if err := rows.Scan(&item.ID, &item.Name, &item.Username, &item.TokenCiphertext, &item.TokenFingerprint, &item.Status, &item.WelcomeText, &item.MenuConfig, &item.CreatedAt, &item.UpdatedAt); err != nil { return nil, err }
		items = append(items, item)
	}
	return items, rows.Err()
}

func CreateBot(ctx context.Context, db *sql.DB, driver, name, username, encryptedToken, fingerprint, welcome string) (Bot, error) {
	id, err := randomID()
	if err != nil { return Bot{}, err }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := Bot{ID:id, Name:name, Username:username, TokenCiphertext:encryptedToken, TokenFingerprint:fingerprint, Status:1, WelcomeText:welcome, MenuConfig:"[]", CreatedAt:now, UpdatedAt:now}
	_, err = db.ExecContext(ctx, "INSERT INTO bots(id,name,username,token_ciphertext,token_fingerprint,status,welcome_text,menu_config,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)",
		item.ID,item.Name,item.Username,item.TokenCiphertext,item.TokenFingerprint,item.Status,item.WelcomeText,item.MenuConfig,item.CreatedAt,item.UpdatedAt)
	return item, err
}

func UpdateBot(ctx context.Context, db *sql.DB, driver, id, name, welcome string, status int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE bots SET name="+placeholder(driver,1)+", welcome_text="+placeholder(driver,2)+", status="+placeholder(driver,3)+", updated_at="+placeholder(driver,4)+" WHERE id="+placeholder(driver,5)\n\tresult, err := db.ExecContext(ctx, query, name,welcome,status,now,id)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return sql.ErrNoRows }
	return nil
}

func DeleteBot(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM bots WHERE id=?", id)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return sql.ErrNoRows }
	return nil
}
