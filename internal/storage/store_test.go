package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPutOpenDelete(t *testing.T) {
	store, err := NewLocal(filepath.Join(t.TempDir(), "uploads"))
	if err != nil { t.Fatal(err) }
	ctx := context.Background()
	if err := store.Put(ctx, "avatars/test.png", []byte("image-data"), "image/png"); err != nil { t.Fatal(err) }
	reader, contentType, err := store.Open(ctx, "avatars/test.png")
	if err != nil { t.Fatal(err) }
	data, err := io.ReadAll(reader); _ = reader.Close()
	if err != nil { t.Fatal(err) }
	if string(data) != "image-data" { t.Fatalf("unexpected content %q", data) }
	if contentType != "image/png" { t.Fatalf("unexpected content type %q", contentType) }
	if err := store.Delete(ctx, "avatars/test.png"); err != nil { t.Fatal(err) }
	if _, err := os.Stat(filepath.Join(store.root, "avatars", "test.png")); !os.IsNotExist(err) { t.Fatalf("expected file to be removed, got %v", err) }
}

func TestLocalListReturnsRelativeKeys(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil { t.Fatal(err) }
	if err := store.Put(context.Background(), "upstream/image.jpg", []byte("x"), "image/jpeg"); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(store.root, "upstream", ".upload-inflight"), []byte("partial"), 0o600); err != nil { t.Fatal(err) }
	keys, err := store.List(context.Background(), "upstream")
	if err != nil { t.Fatal(err) }
	if len(keys) != 1 || keys[0] != "upstream/image.jpg" { t.Fatalf("unexpected keys: %#v", keys) }
}

func TestLocalRejectsTraversal(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil { t.Fatal(err) }
	for _, key := range []string{"../outside", "/absolute", "avatars/../../outside"} {
		if err := store.Put(context.Background(), key, []byte("no"), ""); err == nil { t.Fatalf("expected %q to be rejected", key) }
	}
}

func TestLocalRejectsSymlinkedDirectories(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil { t.Skipf("symlink unavailable: %v", err) }
	store, err := NewLocal(root)
	if err != nil { t.Fatal(err) }
	if err := store.Put(context.Background(), "linked/escape.txt", []byte("no"), "text/plain"); err == nil { t.Fatal("expected symlinked path to be rejected") }
}

func TestNewS3AcceptsOriginEndpoint(t *testing.T) {
	store, err := NewS3(S3Config{Endpoint:"https://s3.example.com", Bucket:"media", AccessKey:"access", SecretKey:"secret", Prefix:"tg-card/media"})
	if err != nil { t.Fatal(err) }
	if store.bucket != "media" || store.prefix != "tg-card/media" { t.Fatalf("unexpected S3 config: %#v", store) }
}

func TestS3RequiresHTTPSForPublicEndpoints(t *testing.T) {
	if _, err := NewS3(S3Config{Endpoint:"http://s3.example.com", Bucket:"media", AccessKey:"access", SecretKey:"secret"}); err == nil { t.Fatal("expected plaintext public endpoint to be rejected") }
	if _, err := NewS3(S3Config{Endpoint:"http://127.0.0.1:9000", Bucket:"media", AccessKey:"access", SecretKey:"secret"}); err != nil { t.Fatalf("expected private development endpoint to be allowed: %v", err) }
}

func TestNewS3RejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []S3Config{{Endpoint:"file:///tmp",Bucket:"bucket",AccessKey:"a",SecretKey:"s"},{Endpoint:"https://example.com/path",Bucket:"bucket",AccessKey:"a",SecretKey:"s"},{Endpoint:"https://example.com",Bucket:"bucket"}} {
		if _, err := NewS3(cfg); err == nil { t.Fatal("expected invalid S3 configuration to be rejected") }
	}
}

func TestCleanKey(t *testing.T) {
	for _, key := range []string{"", ".", "..", "../x", "/x"} {
		if _, err := cleanKey(key); err == nil { t.Fatalf("expected invalid key %q", key) }
	}
	if got, err := cleanKey("avatars\\test.png"); err != nil || got != "avatars/test.png" { t.Fatalf("cleanKey = %q, %v", got, err) }
}
