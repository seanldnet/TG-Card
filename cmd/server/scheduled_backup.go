package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

const scheduledBackupLastRunKey = "scheduled_backup_last_run_date"

func (a *application) runScheduledBackupScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkScheduledBackup(ctx)
		}
	}
}

func (a *application) checkScheduledBackup(ctx context.Context) {
	raw, err := database.GetSiteSettings(ctx, a.db)
	if err != nil { log.Printf("scheduled backup: load settings: %v", err); return }
	var settings siteSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil { log.Printf("scheduled backup: decode settings: %v", err); return }
	if settings.BackupTime == "" { settings.BackupTime = "03:00" }
	if !settings.BackupEnabled { return }
	now := time.Now()
	scheduled, err := time.Parse("15:04", settings.BackupTime)
	if err != nil { log.Printf("scheduled backup: invalid configured time %q", settings.BackupTime); return }
	if now.Hour()*60+now.Minute() < scheduled.Hour()*60+scheduled.Minute() { return }
	date := now.Format("2006-01-02")
	last, err := database.GetSetting(ctx, a.db, a.driver, scheduledBackupLastRunKey)
	if err != nil || last == date { return }
	// Persist the attempt before running so restarts or multiple scheduler ticks cannot
	// create repeated multi-gigabyte archives or send duplicate documents.
	if err := database.SaveSettings(ctx, a.db, a.driver, map[string]string{scheduledBackupLastRunKey: date}); err != nil {
		log.Printf("scheduled backup: persist run date: %v", err); return
	}
	if err := a.createAndDeliverBackup(ctx, settings, date); err != nil {
		log.Printf("scheduled backup failed for %s: %v", date, err)
		_ = a.sendOwnerText(ctx, settings.TelegramOwnerID, "TG-Card 每日备份失败（"+date+"）：\n"+err.Error())
		return
	}
	log.Printf("scheduled backup delivered to Owner for %s", date)
}

func (a *application) createAndDeliverBackup(ctx context.Context, settings siteSettings, date string) error {
	if a.driver != "postgres" { return fmt.Errorf("scheduled backup currently requires PostgreSQL; configured driver is %q", a.driver) }
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" { return fmt.Errorf("DATABASE_URL is not configured") }
	u, err := url.Parse(dsn); if err != nil { return fmt.Errorf("parse database URL: %w", err) }
	tmp, err := os.MkdirTemp("", "tg-card-backup-*"); if err != nil { return err }
	defer os.RemoveAll(tmp)
	dumpPath := filepath.Join(tmp, "database.dump")
	cmd := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "--format=custom", "--file", dumpPath)
	cmd.Env = append(os.Environ(), "PGHOST="+u.Hostname(), "PGDATABASE="+strings.TrimPrefix(u.Path, "/"), "PGUSER="+u.User.Username(), "PGPASSWORD="+passwordFromURL(u), "PGPORT="+portFromURL(u), "PGSSLMODE="+sslModeFromURL(u))
	if output, err := cmd.CombinedOutput(); err != nil { return fmt.Errorf("pg_dump failed: %w: %s", err, strings.TrimSpace(string(output))) }
	archivePath := filepath.Join(tmp, "tg-card-backup-"+date+".tar.gz")
	if err := writeBackupArchive(archivePath, dumpPath, a.dataDir); err != nil { return err }
	file, err := os.Open(archivePath); if err != nil { return err }
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil { return err }
	checksum := hex.EncodeToString(hash.Sum(nil))
	if _, err := file.Seek(0, io.SeekStart); err != nil { return err }
	caption := fmt.Sprintf("TG-Card 每日备份\n日期：%s\n内容：PostgreSQL 数据库 + 应用持久化文件\nSHA-256：%s", date, checksum)
	return a.sendOwnerDocument(ctx, settings.TelegramOwnerID, archivePath, caption)
}

func passwordFromURL(u *url.URL) string { if u.User == nil { return "" }; p, _ := u.User.Password(); return p }
func portFromURL(u *url.URL) string { if p := u.Port(); p != "" { return p }; return "5432" }
func sslModeFromURL(u *url.URL) string { if s := u.Query().Get("sslmode"); s != "" { return s }; return "prefer" }

func writeBackupArchive(destination, dumpPath, dataDir string) error {
	out, err := os.Create(destination); if err != nil { return err }
	defer out.Close()
	gz := gzip.NewWriter(out); defer gz.Close()
	tw := tar.NewWriter(gz); defer tw.Close()
	addFile := func(path, name string) error {
		info, err := os.Stat(path); if err != nil { return err }
		header, err := tar.FileInfoHeader(info, ""); if err != nil { return err }
		header.Name = name
		if err := tw.WriteHeader(header); err != nil { return err }
		f, err := os.Open(path); if err != nil { return err }; defer f.Close()
		_, err = io.Copy(tw, f); return err
	}
	if err := addFile(dumpPath, "database.dump"); err != nil { return err }
	if dataDir != "" {
		err := filepath.Walk(dataDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil { return walkErr }
			if path == dataDir { return nil }
			rel, err := filepath.Rel(dataDir, path); if err != nil { return err }
			name := filepath.ToSlash(filepath.Join("data", rel))
			header, err := tar.FileInfoHeader(info, ""); if err != nil { return err }; header.Name = name
			if err := tw.WriteHeader(header); err != nil { return err }
			if info.IsDir() { return nil }
			f, err := os.Open(path); if err != nil { return err }; defer f.Close()
			_, err = io.Copy(tw, f); return err
		})
		if err != nil { return err }
	}
	return nil
}

func (a *application) sendOwnerText(ctx context.Context, ownerID, message string) error {
	if strings.TrimSpace(ownerID) == "" { return fmt.Errorf("Telegram Owner ID is not configured") }
	bots, err := database.ListBots(ctx, a.db); if err != nil { return err }
	for _, bot := range bots {
		if bot.Status != 1 { continue }
		token, err := a.secrets.Decrypt(bot.TokenCiphertext); if err != nil { continue }
		payload, _ := json.Marshal(map[string]string{"chat_id": ownerID, "text": message})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(payload)); if err != nil { continue }
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 20*time.Second}).Do(req); if err != nil { continue }
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); resp.Body.Close()
		var result struct { OK bool `json:"ok"` }
		_ = json.Unmarshal(body, &result)
		if resp.StatusCode == http.StatusOK && result.OK { return nil }
	}
	return fmt.Errorf("no active Telegram bot could deliver a message to Owner")
}

func (a *application) sendOwnerDocument(ctx context.Context, ownerID, path, caption string) error {
	if strings.TrimSpace(ownerID) == "" { return fmt.Errorf("Telegram Owner ID is not configured") }
	bots, err := database.ListBots(ctx, a.db); if err != nil { return err }
	for _, bot := range bots {
		if bot.Status != 1 { continue }
		token, err := a.secrets.Decrypt(bot.TokenCiphertext); if err != nil { continue }
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("chat_id", ownerID)
		_ = writer.WriteField("caption", caption)
		part, err := writer.CreateFormFile("document", filepath.Base(path)); if err != nil { return err }
		file, err := os.Open(path); if err != nil { return err }
		_, copyErr := io.Copy(part, file); file.Close(); if copyErr != nil { return copyErr }
		if err := writer.Close(); err != nil { return err }
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendDocument", &body); if err != nil { continue }
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := (&http.Client{Timeout: 2*time.Minute}).Do(req); if err != nil { continue }
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); resp.Body.Close()
		var result struct { OK bool `json:"ok"`; Description string `json:"description"` }
		_ = json.Unmarshal(responseBody, &result)
		if resp.StatusCode == http.StatusOK && result.OK { return nil }
		log.Printf("Telegram sendDocument rejected: status=%d description=%s", resp.StatusCode, result.Description)
	}
	return fmt.Errorf("no active Telegram bot could deliver the backup document to Owner")
}
