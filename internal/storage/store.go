package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store is the small storage contract used for public, non-sensitive media.
// Private support-ticket attachments intentionally do not use this interface.
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Health(ctx context.Context) error
}

func cleanKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	clean := path.Clean(key)
	if key == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", errors.New("invalid object key")
	}
	return clean, nil
}

type Local struct { root string }

func NewLocal(root string) (*Local, error) {
	if strings.TrimSpace(root) == "" { return nil, errors.New("local storage root is required") }
	abs, err := filepath.Abs(root)
	if err != nil { return nil, err }
	if err := os.MkdirAll(abs, 0o750); err != nil { return nil, err }
	return &Local{root: abs}, nil
}

func (s *Local) filename(key string) (string, error) {
	clean, err := cleanKey(key)
	if err != nil { return "", err }
	target := filepath.Join(s.root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(s.root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("object key escapes storage root")
	}
	current := s.root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) { break }
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 { return "", errors.New("symbolic links are not allowed in local storage") }
	}
	return target, nil
}

func (s *Local) Put(ctx context.Context, key string, data []byte, _ string) error {
	if err := ctx.Err(); err != nil { return err }
	target, err := s.filename(key); if err != nil { return err }
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil { return err }
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*"); if err != nil { return err }
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o640); err != nil { _ = tmp.Close(); return err }
	if _, err := tmp.Write(data); err != nil { _ = tmp.Close(); return err }
	if err := tmp.Sync(); err != nil { _ = tmp.Close(); return err }
	if err := tmp.Close(); err != nil { return err }
	return os.Rename(tmpName, target)
}

func (s *Local) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if err := ctx.Err(); err != nil { return nil, "", err }
	target, err := s.filename(key); if err != nil { return nil, "", err }
	f, err := os.Open(target); if err != nil { return nil, "", err }
	contentType := mime.TypeByExtension(filepath.Ext(target))
	return f, contentType, nil
}

func (s *Local) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil { return err }
	target, err := s.filename(key); if err != nil { return err }
	err = os.Remove(target)
	if errors.Is(err, os.ErrNotExist) { return nil }
	return err
}

func (s *Local) List(ctx context.Context, prefix string) ([]string, error) {
	base := s.root
	if strings.TrimSpace(prefix) != "" { var err error; base, err = s.filename(prefix); if err != nil { return nil, err } }
	var keys []string
	err := filepath.WalkDir(base, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil { if errors.Is(walkErr, os.ErrNotExist) { return nil }; return walkErr }
		if entry.IsDir() { return nil }
		if !entry.Type().IsRegular() { return nil }
		rel, err := filepath.Rel(s.root, filename); if err != nil { return err }
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	if errors.Is(err, os.ErrNotExist) { return nil, nil }
	return keys, err
}

func (s *Local) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil { return err }
	info, err := os.Stat(s.root)
	if err != nil { return err }
	if !info.IsDir() { return errors.New("local storage root is not a directory") }
	return nil
}

type S3Config struct {
	Endpoint string
	Region string
	Bucket string
	AccessKey string
	SecretKey string
	Prefix string
}

type S3 struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewS3(cfg S3Config) (*S3, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("S3 endpoint must be an http(s) origin URL without credentials, path, query, or fragment")
	}
	if u.Scheme == "http" {
		host := strings.ToLower(u.Hostname())
		ip := net.ParseIP(host)
		privateEndpoint := host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || !strings.Contains(host, ".")
		if ip != nil { privateEndpoint = ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() }
		if !privateEndpoint { return nil, errors.New("plain HTTP is only allowed for loopback or private development endpoints; use HTTPS for remote S3 services") }
	}
	if strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, errors.New("S3 bucket, access key, and secret key are required")
	}
	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	if prefix != "" {
		if _, err := cleanKey(prefix); err != nil { return nil, fmt.Errorf("invalid S3 prefix: %w", err) }
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: u.Scheme == "https",
		Region: strings.TrimSpace(cfg.Region),
	})
	if err != nil { return nil, err }
	return &S3{client: client, bucket: strings.TrimSpace(cfg.Bucket), prefix: prefix}, nil
}

func (s *S3) objectKey(key string) (string, error) {
	clean, err := cleanKey(key); if err != nil { return "", err }
	if s.prefix == "" { return clean, nil }
	return s.prefix + "/" + clean, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	object, err := s.objectKey(key); if err != nil { return err }
	if contentType == "" { contentType = "application/octet-stream" }
	_, err = s.client.PutObject(ctx, s.bucket, object, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	object, err := s.objectKey(key); if err != nil { return nil, "", err }
	reader, err := s.client.GetObject(ctx, s.bucket, object, minio.GetObjectOptions{})
	if err != nil { return nil, "", err }
	info, err := reader.Stat()
	if err != nil { _ = reader.Close(); return nil, "", err }
	return reader, info.ContentType, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	object, err := s.objectKey(key); if err != nil { return err }
	return s.client.RemoveObject(ctx, s.bucket, object, minio.RemoveObjectOptions{})
}

func (s *S3) List(ctx context.Context, prefix string) ([]string, error) {
	objectPrefix := s.prefix
	if strings.TrimSpace(prefix) != "" { clean, err := cleanKey(prefix); if err != nil { return nil, err }; if objectPrefix != "" { objectPrefix += "/" }; objectPrefix += clean }
	var keys []string
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: objectPrefix, Recursive: true}) {
		if object.Err != nil { return nil, object.Err }
		key := object.Key
		if s.prefix != "" { key = strings.TrimPrefix(key, s.prefix+"/") }
		keys = append(keys, key)
	}
	return keys, nil
}

func (s *S3) Health(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil { return err }
	if !exists { return fmt.Errorf("S3 bucket %q does not exist or is not accessible", s.bucket) }
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil { return err }
	probeKey := "__tg-card-healthcheck/" + hex.EncodeToString(random)
	probeData := []byte("tg-card-storage-healthcheck")
	if err := s.Put(ctx, probeKey, probeData, "text/plain"); err != nil { return fmt.Errorf("S3 write permission check failed: %w", err) }
	reader, _, readErr := s.Open(ctx, probeKey)
	if readErr != nil {
		_ = s.Delete(context.Background(), probeKey)
		return fmt.Errorf("S3 read permission check failed: %w", readErr)
	}
	verified, readErr := io.ReadAll(io.LimitReader(reader, int64(len(probeData)+1)))
	_ = reader.Close()
	deleteErr := s.Delete(ctx, probeKey)
	if readErr != nil { return fmt.Errorf("S3 read permission check failed: %w", readErr) }
	if string(verified) != string(probeData) { return errors.New("S3 read-back verification failed") }
	if deleteErr != nil { return fmt.Errorf("S3 delete permission check failed: %w", deleteErr) }
	return nil
}
