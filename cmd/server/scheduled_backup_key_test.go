package main

import (
	"testing"

	"github.com/seanldnet/TG-Card/internal/secret"
)

func TestResolveScheduledBackupPassphrase(t *testing.T) {
	manager, err := secret.New(t.TempDir())
	if err != nil { t.Fatal(err) }
	stored := "abcdefghijklmnopqrstuvwxyz012345"
	ciphertext, err := manager.Encrypt(stored)
	if err != nil { t.Fatal(err) }

	t.Run("saved website key takes precedence", func(t *testing.T) {
		got, err := resolveScheduledBackupPassphrase(siteSettings{BackupEncryptionPassphraseCiphertext: ciphertext}, manager.Decrypt, "environment-key-that-is-at-least-32-characters")
		if err != nil { t.Fatal(err) }
		if got != stored { t.Fatalf("got saved key %q, want configured key", got) }
	})
	t.Run("environment key remains fallback", func(t *testing.T) {
		want := "environment-key-that-is-at-least-32-characters"
		got, err := resolveScheduledBackupPassphrase(siteSettings{}, manager.Decrypt, want)
		if err != nil { t.Fatal(err) }
		if got != want { t.Fatalf("got %q, want fallback key", got) }
	})
	t.Run("missing key fails closed", func(t *testing.T) {
		if _, err := resolveScheduledBackupPassphrase(siteSettings{}, manager.Decrypt, ""); err == nil { t.Fatal("expected missing key error") }
	})
	t.Run("corrupt saved key does not fall back to environment", func(t *testing.T) {
		_, err := resolveScheduledBackupPassphrase(siteSettings{BackupEncryptionPassphraseCiphertext: "not-valid-ciphertext"}, manager.Decrypt, "environment-key-that-is-at-least-32-characters")
		if err == nil { t.Fatal("expected decrypt error") }
	})
}
