package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
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

func (s *S3) Health(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil { return err }
	if !exists { return fmt.Errorf("S3 bucket %q does not exist or is not accessible", s.bucket) }
	return nil
}
