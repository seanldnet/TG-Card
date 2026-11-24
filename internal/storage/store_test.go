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

func TestLocalRejectsTraversal(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil { t.Fatal(err) }
	for _, key := range []string{"../outside", "/absolute", "avatars/../../outside"} {
		if err := store.Put(context.Background(), key, []byte("no"), ""); err == nil { t.Fatalf("expected %q to be rejected", key) }
	}
}

func TestCleanKey(t *testing.T) {
	for _, key := range []string{"", ".", "..", "../x", "/x"} {
		if _, err := cleanKey(key); err == nil { t.Fatalf("expected invalid key %q", key) }
	}
	if got, err := cleanKey("avatars\\test.png"); err != nil || got != "avatars/test.png" { t.Fatalf("cleanKey = %q, %v", got, err) }
}
