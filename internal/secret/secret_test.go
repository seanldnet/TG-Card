package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptAndPersistKey(t *testing.T) {
	dir := t.TempDir()
	manager, err := New(dir)
	if err != nil { t.Fatal(err) }
	ciphertext, err := manager.Encrypt("123456:telegram-secret-token")
	if err != nil { t.Fatal(err) }
	plain, err := manager.Decrypt(ciphertext)
	if err != nil { t.Fatal(err) }
	if plain != "123456:telegram-secret-token" { t.Fatalf("unexpected plaintext: %q", plain) }
	restarted, err := New(dir)
	if err != nil { t.Fatal(err) }
	plain, err = restarted.Decrypt(ciphertext)
	if err != nil { t.Fatal(err) }
	if plain != "123456:telegram-secret-token" { t.Fatal("persisted key did not decrypt ciphertext") }
	keyInfo, err := os.Stat(filepath.Join(dir, "bot-token.key"))
	if err != nil { t.Fatal(err) }
	if keyInfo.Mode().Perm() != 0o600 { t.Fatalf("key permissions = %o, want 600", keyInfo.Mode().Perm()) }
	if _, err := restarted.Decrypt(ciphertext[:len(ciphertext)-2] + "xx"); err == nil {
		t.Fatal("tampered ciphertext should not decrypt")
	}
}
