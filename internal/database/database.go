package database

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
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
	AvatarURL string
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
	query := "SELECT id, username, password_hash, real_name, avatar_url, role, status, token_version FROM admins WHERE username = "+placeholder(driver,1)
	var admin Admin
	err := db.QueryRowContext(ctx, query, username).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.RealName, &admin.AvatarURL, &admin.Role, &admin.Status, &admin.TokenVersion)
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
	query := "SELECT id, username, password_hash, real_name, avatar_url, role, status, token_version FROM admins WHERE id = " + placeholder(driver, 1)
	var admin Admin
	err := db.QueryRowContext(ctx, query, adminID).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.RealName, &admin.AvatarURL, &admin.Role, &admin.Status, &admin.TokenVersion)
	return admin, err
}


func UpdateAdminProfile(ctx context.Context, db *sql.DB, driver, adminID, realName, avatarURL string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE admins SET real_name = " + placeholder(driver, 1) + ", avatar_url = " + placeholder(driver, 2) + ", updated_at = " + placeholder(driver, 3) + " WHERE id = " + placeholder(driver, 4)
	result, err := db.ExecContext(ctx, query, realName, avatarURL, now, adminID)
	if err != nil { return err }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected != 1 { return fmt.Errorf("administrator not found") }
	return nil
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
	query := "INSERT INTO bots(id,name,username,token_ciphertext,token_fingerprint,status,welcome_text,menu_config,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+")"
	_, err = db.ExecContext(ctx, query, item.ID,item.Name,item.Username,item.TokenCiphertext,item.TokenFingerprint,item.Status,item.WelcomeText,item.MenuConfig,item.CreatedAt,item.UpdatedAt)
	return item, err
}

func UpdateBot(ctx context.Context, db *sql.DB, driver, id, name, welcome string, status int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE bots SET name="+placeholder(driver,1)+", welcome_text="+placeholder(driver,2)+", status="+placeholder(driver,3)+", updated_at="+placeholder(driver,4)+" WHERE id="+placeholder(driver,5)
	result, err := db.ExecContext(ctx, query, name,welcome,status,now,id)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return sql.ErrNoRows }
	return nil
}

func UpdateBotMenuConfig(ctx context.Context, db *sql.DB, driver, id, menuConfig string) error {
 now:=time.Now().UTC().Format(time.RFC3339Nano);res,err:=db.ExecContext(ctx,"UPDATE bots SET menu_config="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3),menuConfig,now,id);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}
func DeleteBot(ctx context.Context, db *sql.DB, driver, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM bots WHERE id="+placeholder(driver,1), id)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return sql.ErrNoRows }
	return nil
}

type Category struct {
	ID string `json:"id"`
	Name string `json:"name"`
	SortOrder int `json:"sortOrder"`
	Status int `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Product struct {
	ID string `json:"id"`
	CategoryID string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	Name string `json:"name"`
	Description string `json:"description"`
	PriceMinor int64 `json:"priceMinor"`
	Currency string `json:"currency"`
	DeliveryType string `json:"deliveryType"`
	Status int `json:"status"`
	StockCount int `json:"stockCount"`
	BulkDiscountJSON string `json:"bulkDiscountJson"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type StockSecret struct {
	Ciphertext string
	Fingerprint string
}

func ListCategories(ctx context.Context, db *sql.DB) ([]Category, error) {
	rows, err := db.QueryContext(ctx, "SELECT id,name,sort_order,status,created_at,updated_at FROM product_categories ORDER BY sort_order,name")
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]Category, 0)
	for rows.Next() {
		var item Category
		if err := rows.Scan(&item.ID,&item.Name,&item.SortOrder,&item.Status,&item.CreatedAt,&item.UpdatedAt); err != nil { return nil, err }
		items = append(items,item)
	}
	return items, rows.Err()
}

func CreateCategory(ctx context.Context, db *sql.DB, driver, name string, sortOrder int) (Category, error) {
	id, err := randomID()
	if err != nil { return Category{}, err }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := Category{ID:id,Name:name,SortOrder:sortOrder,Status:1,CreatedAt:now,UpdatedAt:now}
	query := "INSERT INTO product_categories(id,name,sort_order,status,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")"
	_, err = db.ExecContext(ctx,query,item.ID,item.Name,item.SortOrder,item.Status,item.CreatedAt,item.UpdatedAt)
	return item,err
}

func DeleteCategory(ctx context.Context, db *sql.DB, driver, id string) error {
	result,err:=db.ExecContext(ctx,"DELETE FROM product_categories WHERE id="+placeholder(driver,1),id)
	if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

func ListProducts(ctx context.Context, db *sql.DB) ([]Product, error) {
	rows,err:=db.QueryContext(ctx,`SELECT p.id,COALESCE(p.category_id,''),COALESCE(c.name,''),p.name,p.description,p.price_minor,p.currency,p.delivery_type,p.status,COALESCE(p.bulk_discount_json,'[]'),
	(SELECT COUNT(1) FROM product_keys k WHERE k.product_id=p.id AND k.status=0),p.created_at,p.updated_at
	FROM products p LEFT JOIN product_categories c ON c.id=p.category_id ORDER BY p.created_at DESC`)
	if err!=nil{return nil,err};defer rows.Close()
	items:=make([]Product,0)
	for rows.Next(){var p Product;if err:=rows.Scan(&p.ID,&p.CategoryID,&p.CategoryName,&p.Name,&p.Description,&p.PriceMinor,&p.Currency,&p.DeliveryType,&p.Status,&p.BulkDiscountJSON,&p.StockCount,&p.CreatedAt,&p.UpdatedAt);err!=nil{return nil,err};items=append(items,p)}
	return items,rows.Err()
}

func CreateProduct(ctx context.Context, db *sql.DB, driver string, p Product) (Product,error) {
	id,err:=randomID();if err!=nil{return Product{},err};now:=time.Now().UTC().Format(time.RFC3339Nano)
	p.ID=id;p.CreatedAt=now;p.UpdatedAt=now;if p.Currency==""{p.Currency="USDT"};if p.DeliveryType==""{p.DeliveryType="card"};if p.Status==0{p.Status=1}
	var category any;if p.CategoryID!=""{category=p.CategoryID}
	query:="INSERT INTO products(id,category_id,name,description,price_minor,currency,delivery_type,status,bulk_discount_json,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+")"
	if p.BulkDiscountJSON=="" { p.BulkDiscountJSON="[]" }
	_,err=db.ExecContext(ctx,query,p.ID,category,p.Name,p.Description,p.PriceMinor,p.Currency,p.DeliveryType,p.Status,p.BulkDiscountJSON,p.CreatedAt,p.UpdatedAt)
	return p,err
}

func UpdateProduct(ctx context.Context, db *sql.DB, driver string, p Product) error {
	now:=time.Now().UTC().Format(time.RFC3339Nano);var category any;if p.CategoryID!=""{category=p.CategoryID}
	query:="UPDATE products SET category_id="+placeholder(driver,1)+",name="+placeholder(driver,2)+",description="+placeholder(driver,3)+",price_minor="+placeholder(driver,4)+",currency="+placeholder(driver,5)+",delivery_type="+placeholder(driver,6)+",status="+placeholder(driver,7)+",bulk_discount_json="+placeholder(driver,8)+",updated_at="+placeholder(driver,9)+" WHERE id="+placeholder(driver,10)
	if p.BulkDiscountJSON=="" { p.BulkDiscountJSON="[]" }
	result,err:=db.ExecContext(ctx,query,category,p.Name,p.Description,p.PriceMinor,p.Currency,p.DeliveryType,p.Status,p.BulkDiscountJSON,now,p.ID);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

func DeleteProduct(ctx context.Context, db *sql.DB, driver, id string) error {
	result,err:=db.ExecContext(ctx,"DELETE FROM products WHERE id="+placeholder(driver,1),id);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

func ImportProductKeys(ctx context.Context, db *sql.DB, driver, productID string, secrets []StockSecret) (int,error) {
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return 0,err};defer tx.Rollback()
	var exists int
	if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM products WHERE id="+placeholder(driver,1),productID).Scan(&exists);err!=nil{return 0,err}
	if exists==0{return 0,sql.ErrNoRows}
	inserted:=0
	for _,item:=range secrets {
		id,err:=randomID();if err!=nil{return inserted,err}
		query:="INSERT INTO product_keys(id,product_id,secret_ciphertext,secret_fingerprint,status,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",0,"+placeholder(driver,5)+") ON CONFLICT(product_id,secret_fingerprint) DO NOTHING"
		result,err:=tx.ExecContext(ctx,query,id,productID,item.Ciphertext,item.Fingerprint,time.Now().UTC().Format(time.RFC3339Nano))
		if err!=nil { return inserted,err }
		n,_:=result.RowsAffected();inserted+=int(n)
	}
	if err:=tx.Commit();err!=nil{return inserted,err};return inserted,nil
}


// TelegramUser stores the buyer identity and the platform's internal USDT balance.
type TelegramUser struct {
	ID string `json:"id"`
	TelegramUserID string `json:"telegramUserId"`
	Username string `json:"username"`
	FirstName string `json:"firstName"`
	LastName string `json:"lastName"`
	LastSeenBotID string `json:"lastSeenBotId"`
	BalanceAtomic int64 `json:"balanceAtomic"`
	OrderCount int64 `json:"orderCount"`
	TotalSpentAtomic int64 `json:"totalSpentAtomic"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func UpsertTelegramUser(ctx context.Context, db *sql.DB, driver, telegramUserID, username, firstName, lastName, botID string) error {
	if telegramUserID == "" { return fmt.Errorf("Telegram user ID is required") }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := randomID()
	if err != nil { return err }
	query := "INSERT INTO telegram_users(id,telegram_user_id,username,first_name,last_name,last_seen_bot_id,balance_atomic,created_at,updated_at) VALUES("+
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+",0,"+placeholder(driver,7)+","+placeholder(driver,8)+") "+
		"ON CONFLICT(telegram_user_id) DO UPDATE SET username=excluded.username,first_name=excluded.first_name,last_name=excluded.last_name,last_seen_bot_id=excluded.last_seen_bot_id,updated_at=excluded.updated_at"
	_, err = db.ExecContext(ctx,query,id,telegramUserID,username,firstName,lastName,botID,now,now)
	return err
}

func ListTelegramUsers(ctx context.Context, db *sql.DB) ([]TelegramUser,error) {
	query := `SELECT u.id,u.telegram_user_id,u.username,u.first_name,u.last_name,u.last_seen_bot_id,u.balance_atomic,u.created_at,u.updated_at,
	(SELECT COUNT(1) FROM orders o WHERE o.telegram_user_id=u.telegram_user_id),
	(SELECT COALESCE(SUM(o.payment_amount_atomic),0) FROM orders o WHERE o.telegram_user_id=u.telegram_user_id AND o.status IN ('paid','delivering','delivery_failed','delivered'))
	FROM telegram_users u ORDER BY u.updated_at DESC LIMIT 1000`
	rows,err:=db.QueryContext(ctx,query);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]TelegramUser,0)
	for rows.Next(){var u TelegramUser;if err:=rows.Scan(&u.ID,&u.TelegramUserID,&u.Username,&u.FirstName,&u.LastName,&u.LastSeenBotID,&u.BalanceAtomic,&u.CreatedAt,&u.UpdatedAt,&u.OrderCount,&u.TotalSpentAtomic);err!=nil{return nil,err};items=append(items,u)}
	return items,rows.Err()
}

func AdjustTelegramUserBalance(ctx context.Context, db *sql.DB, driver, userID, adminID string, amountAtomic int64, direction, note string) (int64,error) {
	if amountAtomic<=0 || (direction!="credit" && direction!="debit") { return 0,fmt.Errorf("invalid balance adjustment") }
	if len(note)>500 { note=note[:500] }
	change:=amountAtomic;if direction=="debit"{change=-amountAtomic}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return 0,err};defer tx.Rollback()
	update:="UPDATE telegram_users SET balance_atomic=balance_atomic+"+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND balance_atomic+"+placeholder(driver,4)+">=0"
	result,err:=tx.ExecContext(ctx,update,change,time.Now().UTC().Format(time.RFC3339Nano),userID,change);if err!=nil{return 0,err}
	n,err:=result.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("用户不存在或余额不足")}
	var balance int64;var telegramUserID string
	if err:=tx.QueryRowContext(ctx,"SELECT balance_atomic,telegram_user_id FROM telegram_users WHERE id="+placeholder(driver,1),userID).Scan(&balance,&telegramUserID);err!=nil{return 0,err}
	id,err:=randomID();if err!=nil{return 0,err}
	insert:="INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+")"
	_,err=tx.ExecContext(ctx,insert,id,telegramUserID,adminID,direction,amountAtomic,balance,note,time.Now().UTC().Format(time.RFC3339Nano));if err!=nil{return 0,err}
	if err:=tx.Commit();err!=nil{return 0,err};return balance,nil
}


func GetAdminEmail(ctx context.Context,db *sql.DB,driver,adminID string)(string,bool,error){
	var email string;var verified int
	err:=db.QueryRowContext(ctx,"SELECT email,email_verified FROM admins WHERE id="+placeholder(driver,1),adminID).Scan(&email,&verified)
	return email,verified==1,err
}

func SaveAdminEmailCode(ctx context.Context,db *sql.DB,driver,adminID,email,codeHash string,expiresAt time.Time)error{
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	var previous string
	lookup:="SELECT created_at FROM admin_email_verification_codes WHERE admin_id="+placeholder(driver,1)+" ORDER BY created_at DESC LIMIT 1"
	if err:=tx.QueryRowContext(ctx,lookup,adminID).Scan(&previous);err==nil{
		if sent,parseErr:=time.Parse(time.RFC3339Nano,previous);parseErr==nil&&time.Since(sent)<time.Minute{return fmt.Errorf("验证码发送过于频繁，请 60 秒后再试")}
	}else if err!=sql.ErrNoRows{return err}
	if _,err:=tx.ExecContext(ctx,"DELETE FROM admin_email_verification_codes WHERE admin_id="+placeholder(driver,1),adminID);err!=nil{return err}
	id,err:=randomID();if err!=nil{return err}
	query:="INSERT INTO admin_email_verification_codes(id,admin_id,email,code_hash,expires_at,created_at,attempts) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+",0)"
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	if _,err:=tx.ExecContext(ctx,query,id,adminID,email,codeHash,expiresAt.UTC().Format(time.RFC3339Nano),now);err!=nil{return err}
	return tx.Commit()
}

func VerifyAdminEmailCode(ctx context.Context,db *sql.DB,driver,adminID,email,codeHash string)error{
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	query:="SELECT id,code_hash,expires_at,attempts FROM admin_email_verification_codes WHERE admin_id="+placeholder(driver,1)+" AND email="+placeholder(driver,2)
	var codeID,savedHash,expires string;var attempts int
	if err:=tx.QueryRowContext(ctx,query,adminID,email).Scan(&codeID,&savedHash,&expires,&attempts);err!=nil{return fmt.Errorf("验证码错误或已过期")}
	if attempts>=5{return fmt.Errorf("验证码错误次数过多，请重新获取验证码")}
	expiresAt,parseErr:=time.Parse(time.RFC3339Nano,expires)
	if parseErr!=nil||!time.Now().UTC().Before(expiresAt){_,_=tx.ExecContext(ctx,"DELETE FROM admin_email_verification_codes WHERE id="+placeholder(driver,1),codeID);_ = tx.Commit();return fmt.Errorf("验证码已过期，请重新获取")}
	if subtle.ConstantTimeCompare([]byte(savedHash),[]byte(codeHash))!=1{
		updateAttempts:="UPDATE admin_email_verification_codes SET attempts=attempts+1 WHERE id="+placeholder(driver,1)
		if _,err:=tx.ExecContext(ctx,updateAttempts,codeID);err!=nil{return err}
		if err:=tx.Commit();err!=nil{return err}
		return fmt.Errorf("验证码错误")
	}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	update:="UPDATE admins SET email="+placeholder(driver,1)+",email_verified=1,updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)
	if _,err:=tx.ExecContext(ctx,update,email,now,adminID);err!=nil{return err}
	if _,err:=tx.ExecContext(ctx,"DELETE FROM admin_email_verification_codes WHERE admin_id="+placeholder(driver,1),adminID);err!=nil{return err}
	return tx.Commit()
}
