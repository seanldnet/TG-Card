package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/storage"
)

const (
	storageProviderKey = "media_storage_provider"
	storageEndpointKey = "media_storage_endpoint"
	storageRegionKey = "media_storage_region"
	storageBucketKey = "media_storage_bucket"
	storagePrefixKey = "media_storage_prefix"
	storageAccessKeyCiphertextKey = "media_storage_access_key_ciphertext"
	storageSecretKeyCiphertextKey = "media_storage_secret_key_ciphertext"
)

type mediaStorageSettingsRequest struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Region string `json:"region"`
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	ClearCredentials bool `json:"clearCredentials"`
}

type mediaStorageSettingsView struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Region string `json:"region"`
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
	AccessKeyConfigured bool `json:"accessKeyConfigured"`
	SecretKeyConfigured bool `json:"secretKeyConfigured"`
}

func (a *application) loadMediaStorageSettings(ctx context.Context) (mediaStorageSettingsRequest, string, string, error) {
	values := make(map[string]string)
	for _, key := range []string{storageProviderKey, storageEndpointKey, storageRegionKey, storageBucketKey, storagePrefixKey, storageAccessKeyCiphertextKey, storageSecretKeyCiphertextKey} {
		value, err := database.GetSetting(ctx, a.db, a.driver, key)
		if err != nil { return mediaStorageSettingsRequest{}, "", "", err }
		values[key] = value
	}
	provider := values[storageProviderKey]
	if provider == "" { provider = "local" }
	cfg := mediaStorageSettingsRequest{Provider: provider, Endpoint: values[storageEndpointKey], Region: values[storageRegionKey], Bucket: values[storageBucketKey], Prefix: values[storagePrefixKey]}
	accessKey, secretKey := "", ""
	var err error
	if values[storageAccessKeyCiphertextKey] != "" {
		accessKey, err = a.secrets.Decrypt(values[storageAccessKeyCiphertextKey]); if err != nil { return cfg, "", "", err }
	}
	if values[storageSecretKeyCiphertextKey] != "" {
		secretKey, err = a.secrets.Decrypt(values[storageSecretKeyCiphertextKey]); if err != nil { return cfg, "", "", err }
	}
	return cfg, accessKey, secretKey, nil
}

func (a *application) buildMediaStore(ctx context.Context, cfg mediaStorageSettingsRequest, accessKey, secretKey string) (storage.Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "local":
		return storage.NewLocal(filepath.Join(a.dataDir, "uploads"))
	case "s3":
		store, err := storage.NewS3(storage.S3Config{Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, AccessKey: accessKey, SecretKey: secretKey, Prefix: cfg.Prefix})
		if err != nil { return nil, err }
		return store, nil
	default:
		return nil, errors.New("unsupported media storage provider")
	}
}

func (a *application) putMedia(ctx context.Context, key string, data []byte, contentType string) error {
	a.storageWriteMu.RLock()
	defer a.storageWriteMu.RUnlock()
	store := a.currentMediaStore()
	if store == nil { return errors.New("media storage is not initialized") }
	return store.Put(ctx, key, data, contentType)
}

func (a *application) currentMediaStore() storage.Store {
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
	if a.mediaStore != nil { return a.mediaStore }
	return a.localMediaStore
}

func (a *application) reloadMediaStore(ctx context.Context) error {
	cfg, accessKey, secretKey, err := a.loadMediaStorageSettings(ctx)
	if err != nil { return err }
	store, err := a.buildMediaStore(ctx, cfg, accessKey, secretKey)
	if err != nil { return err }
	a.storageMu.Lock()
	a.mediaStore = store
	a.storageMu.Unlock()
	return nil
}

func (a *application) getMediaStorageSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	cfg, accessKey, secretKey, err := a.loadMediaStorageSettings(r.Context())
	if err != nil { log.Printf("load media storage settings: %v", err); writeError(w, http.StatusInternalServerError, "无法读取对象存储配置"); return }
	writeSuccess(w, http.StatusOK, mediaStorageSettingsView{Provider: cfg.Provider, Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, Prefix: cfg.Prefix, AccessKeyConfigured: accessKey != "", SecretKeyConfigured: secretKey != ""})
}

func (a *application) updateMediaStorageSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r); if !ok { return }
	a.storageWriteMu.Lock()
	defer a.storageWriteMu.Unlock()
	var input mediaStorageSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil { writeError(w, http.StatusBadRequest, "对象存储配置格式无效"); return }
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.Endpoint, input.Region, input.Bucket, input.Prefix = strings.TrimSpace(input.Endpoint), strings.TrimSpace(input.Region), strings.TrimSpace(input.Bucket), strings.Trim(strings.TrimSpace(input.Prefix), "/")
	if input.Provider != "local" && input.Provider != "s3" { writeError(w, http.StatusBadRequest, "存储类型仅支持 local 或 s3"); return }
	old, oldAccessKey, oldSecretKey, err := a.loadMediaStorageSettings(r.Context())
	if err != nil { writeError(w, http.StatusInternalServerError, "无法读取现有对象存储配置"); return }
	if old.Provider == "s3" && input.Provider == "local" {
		writeError(w, http.StatusConflict, "请先执行“迁移对象存储到本地并切换”，不能直接切换以免历史图片失效")
		return
	}
	if old.Provider == "s3" && input.Provider == "s3" && (old.Endpoint != input.Endpoint || old.Bucket != input.Bucket || old.Prefix != input.Prefix) {
		writeError(w, http.StatusConflict, "更换 S3 Endpoint、Bucket 或路径前缀前，请先迁移回本地，再配置新的对象存储")
		return
	}
	accessKey, secretKey := input.AccessKey, input.SecretKey
	if input.ClearCredentials { accessKey, secretKey = "", "" } else {
		if accessKey == "" { accessKey = oldAccessKey }
		if secretKey == "" { secretKey = oldSecretKey }
	}
	if input.Provider == "s3" && (input.Endpoint == "" || input.Bucket == "" || accessKey == "" || secretKey == "") {
		writeError(w, http.StatusBadRequest, "启用 S3 前必须填写 Endpoint、Bucket、Access Key 和 Secret Key"); return
	}
		store, err := a.buildMediaStore(r.Context(), input, accessKey, secretKey)
	if err != nil { log.Printf("validate media storage configuration: %v", err); writeError(w, http.StatusBadRequest, "对象存储配置无效，请检查地址和参数"); return }
	healthCtx, healthCancel := context.WithTimeout(r.Context(), 8*time.Second)
	healthErr := store.Health(healthCtx)
	healthCancel()
	if healthErr != nil { log.Printf("validate media storage connection: %v", healthErr); writeError(w, http.StatusBadGateway, "对象存储连接或 Bucket 检查失败，请检查地址、权限及凭据"); return }
	accessCipher, secretCipher := "", ""
	if accessKey != "" { accessCipher, err = a.secrets.Encrypt(accessKey); if err != nil { writeError(w, http.StatusInternalServerError, "无法加密 Access Key"); return } }
	if secretKey != "" { secretCipher, err = a.secrets.Encrypt(secretKey); if err != nil { writeError(w, http.StatusInternalServerError, "无法加密 Secret Key"); return } }
	if err := database.SaveSettings(r.Context(), a.db, a.driver, map[string]string{
		storageProviderKey: input.Provider, storageEndpointKey: input.Endpoint, storageRegionKey: input.Region,
		storageBucketKey: input.Bucket, storagePrefixKey: input.Prefix,
		storageAccessKeyCiphertextKey: accessCipher, storageSecretKeyCiphertextKey: secretCipher,
	}); err != nil { log.Printf("save media storage settings: %v", err); writeError(w, http.StatusInternalServerError, "保存对象存储配置失败"); return }
	if err := a.reloadMediaStore(r.Context()); err != nil { log.Printf("activate media storage: %v", err); writeError(w, http.StatusInternalServerError, "配置已保存但存储后端启用失败，请检查服务日志"); return }
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "media.storage.update", "media:storage", a.clientIP(r), r.UserAgent(), "provider="+input.Provider)
	writeSuccess(w, http.StatusOK, mediaStorageSettingsView{Provider: input.Provider, Endpoint: input.Endpoint, Region: input.Region, Bucket: input.Bucket, Prefix: input.Prefix, AccessKeyConfigured: accessKey != "", SecretKeyConfigured: secretKey != ""})
}

func (a *application) testMediaStorageSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	var input mediaStorageSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil { writeError(w, http.StatusBadRequest, "对象存储配置格式无效"); return }
	_, oldAccess, oldSecret, err := a.loadMediaStorageSettings(r.Context())
	if err != nil { writeError(w, http.StatusInternalServerError, "无法读取现有对象存储配置"); return }
	if input.AccessKey == "" { input.AccessKey = oldAccess }
	if input.SecretKey == "" { input.SecretKey = oldSecret }
	store, err := a.buildMediaStore(r.Context(), input, input.AccessKey, input.SecretKey)
	if err != nil { log.Printf("test media storage: %v", err); writeError(w, http.StatusBadGateway, "对象存储连接失败，请检查 Endpoint、Bucket、区域及凭据"); return }
	healthCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := store.Health(healthCtx); err != nil { writeError(w, http.StatusBadGateway, "对象存储健康检查失败"); return }
	writeSuccess(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *application) uploadedMedia(w http.ResponseWriter, r *http.Request) {
	a.storageWriteMu.RLock()
	defer a.storageWriteMu.RUnlock()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/uploads/")
	if key == "" || strings.Contains(key, "\\") || strings.Contains(key, "..") || !(strings.HasPrefix(key, "avatars/") || strings.HasPrefix(key, "site/") || strings.HasPrefix(key, "upstream/")) { http.NotFound(w, r); return }
	a.storageMu.RLock(); active, local := a.mediaStore, a.localMediaStore; a.storageMu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second); defer cancel()
	var reader io.ReadCloser
	var contentType string
	var err error
	if active != nil { reader, contentType, err = active.Open(ctx, key) }
	if err != nil && local != nil && active != local { reader, contentType, err = local.Open(ctx, key) }
	if err != nil { http.NotFound(w, r); return }
	defer reader.Close()
	if contentType == "" { contentType = mime.TypeByExtension(filepath.Ext(key)) }
	if contentType == "" { contentType = "application/octet-stream" }
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead { return }
	if _, err := io.Copy(w, reader); err != nil { log.Printf("stream uploaded media %q: %v", key, err) }
}

func (a *application) migrateLocalMediaToObjectStorage(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	a.storageWriteMu.Lock()
	defer a.storageWriteMu.Unlock()
	a.storageMu.RLock()
	active, local := a.mediaStore, a.localMediaStore
	a.storageMu.RUnlock()
	if active == nil || local == nil || active == local {
		writeError(w, http.StatusConflict, "当前未启用对象存储，无需迁移")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	keys, err := local.List(ctx, "")
	if err != nil { log.Printf("list local media for migration: %v", err); writeError(w, http.StatusInternalServerError, "读取本地媒体文件失败"); return }
	var copied, skipped int
	var totalBytes int64
	for _, key := range keys {
		if strings.HasSuffix(key, "/") || !(strings.HasPrefix(key, "avatars/") || strings.HasPrefix(key, "site/") || strings.HasPrefix(key, "upstream/")) { skipped++; continue }
		reader, contentType, err := local.Open(ctx, key)
		if err != nil { writeError(w, http.StatusBadGateway, "读取本地文件失败，迁移已中止"); return }
		data, readErr := io.ReadAll(io.LimitReader(reader, 20<<20+1))
		_ = reader.Close()
		if readErr != nil || len(data) == 0 || len(data) > 20<<20 {
			writeError(w, http.StatusBadGateway, "本地文件读取失败或超过 20 MiB，迁移已中止")
			return
		}
		if err := active.Put(ctx, key, data, contentType); err != nil {
			log.Printf("upload local media during migration: key=%q error=%v", key, err)
			writeError(w, http.StatusBadGateway, "对象存储写入失败，迁移已中止；本地原文件保留")
			return
		}
		check, _, err := active.Open(ctx, key)
		if err != nil { writeError(w, http.StatusBadGateway, "对象存储回读校验失败；本地原文件保留"); return }
		verified, verifyErr := io.ReadAll(io.LimitReader(check, 20<<20+1))
		_ = check.Close()
		if verifyErr != nil || len(verified) != len(data) {
			writeError(w, http.StatusBadGateway, "对象存储回读校验失败；本地原文件保留")
			return
		}
		sourceHash, targetHash := sha256.Sum256(data), sha256.Sum256(verified)
		if sourceHash != targetHash {
			writeError(w, http.StatusBadGateway, "对象存储内容校验不一致；本地原文件保留")
			return
		}
		copied++
		totalBytes += int64(len(data))
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "media.storage.migrate", "media:storage", a.clientIP(r), r.UserAgent(), "copied="+strconv.Itoa(copied)+" bytes="+strconv.FormatInt(totalBytes, 10))
	writeSuccess(w, http.StatusOK, map[string]any{"copied": copied, "skipped": skipped, "bytes": totalBytes, "localFilesRetained": true, "verified": true})
}

func (a *application) migrateObjectMediaToLocal(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	a.storageWriteMu.Lock()
	defer a.storageWriteMu.Unlock()
	a.storageMu.RLock()
	active, local := a.mediaStore, a.localMediaStore
	a.storageMu.RUnlock()
	if active == nil || local == nil || active == local {
		writeError(w, http.StatusConflict, "当前未启用对象存储，无需迁移")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	keys, err := active.List(ctx, "")
	if err != nil { log.Printf("list object media for local migration: %v", err); writeError(w, http.StatusBadGateway, "读取对象存储文件失败"); return }
	var copied, skipped int
	var totalBytes int64
	for _, key := range keys {
		if !(strings.HasPrefix(key, "avatars/") || strings.HasPrefix(key, "site/") || strings.HasPrefix(key, "upstream/")) { skipped++; continue }
		reader, contentType, err := active.Open(ctx, key)
		if err != nil { writeError(w, http.StatusBadGateway, "读取对象文件失败，迁移已中止；对象存储原文件保留"); return }
		data, readErr := io.ReadAll(io.LimitReader(reader, 20<<20+1))
		_ = reader.Close()
		if readErr != nil || len(data) == 0 || len(data) > 20<<20 {
			writeError(w, http.StatusBadGateway, "对象文件读取失败或超过 20 MiB，迁移已中止；对象存储原文件保留")
			return
		}
		if err := local.Put(ctx, key, data, contentType); err != nil {
			log.Printf("write local media during reverse migration: key=%q error=%v", key, err)
			writeError(w, http.StatusInternalServerError, "本地文件写入失败，迁移已中止；对象存储原文件保留")
			return
		}
		check, _, err := local.Open(ctx, key)
		if err != nil { writeError(w, http.StatusInternalServerError, "本地回读校验失败；对象存储原文件保留"); return }
		verified, verifyErr := io.ReadAll(io.LimitReader(check, 20<<20+1))
		_ = check.Close()
		if verifyErr != nil || len(verified) != len(data) || sha256.Sum256(data) != sha256.Sum256(verified) {
			writeError(w, http.StatusInternalServerError, "本地文件校验不一致；对象存储原文件保留")
			return
		}
		copied++
		totalBytes += int64(len(data))
	}
	if err := database.SaveSettings(ctx, a.db, a.driver, map[string]string{storageProviderKey: "local"}); err != nil {
		log.Printf("switch media backend to local after migration: %v", err)
		writeError(w, http.StatusInternalServerError, "文件已复制并校验，但切换存储后端失败；请检查配置")
		return
	}
	if err := a.reloadMediaStore(ctx); err != nil {
		log.Printf("reload local media backend after migration: %v", err)
		writeError(w, http.StatusInternalServerError, "文件已复制并校验，但重新加载本地后端失败")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "media.storage.migrate_local", "media:storage", a.clientIP(r), r.UserAgent(), "copied="+strconv.Itoa(copied)+" bytes="+strconv.FormatInt(totalBytes, 10))
	writeSuccess(w, http.StatusOK, map[string]any{"copied": copied, "skipped": skipped, "bytes": totalBytes, "objectFilesRetained": true, "verified": true, "provider": "local"})
}
