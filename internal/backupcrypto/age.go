package backupcrypto

import (
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// ValidatePassphrase requires an externally managed secret that is not stored
// inside the application data volume included in backups.
func ValidatePassphrase(passphrase string) error {
	if len(strings.TrimSpace(passphrase)) < 32 {
		return fmt.Errorf("BACKUP_ENCRYPTION_PASSPHRASE must contain at least 32 non-whitespace characters")
	}
	return nil
}

func Encrypt(dst io.Writer, src io.Reader, passphrase string) error {
	if err := ValidatePassphrase(passphrase); err != nil { return err }
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil { return fmt.Errorf("create backup encryption recipient: %w", err) }
	w, err := age.Encrypt(dst, recipient)
	if err != nil { return fmt.Errorf("start backup encryption: %w", err) }
	if _, err := io.Copy(w, src); err != nil { _ = w.Close(); return fmt.Errorf("encrypt backup data: %w", err) }
	if err := w.Close(); err != nil { return fmt.Errorf("finalize encrypted backup: %w", err) }
	return nil
}

func Decrypt(dst io.Writer, src io.Reader, passphrase string) error {
	if err := ValidatePassphrase(passphrase); err != nil { return err }
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil { return fmt.Errorf("create backup decryption identity: %w", err) }
	r, err := age.Decrypt(src, identity)
	if err != nil { return fmt.Errorf("decrypt backup: %w", err) }
	if _, err := io.Copy(dst, r); err != nil { return fmt.Errorf("read decrypted backup: %w", err) }
	return nil
}

func EncryptFile(inputPath, outputPath, passphrase string) error {
	return transformFile(inputPath, outputPath, func(dst io.Writer, src io.Reader) error { return Encrypt(dst, src, passphrase) })
}

func DecryptFile(inputPath, outputPath, passphrase string) error {
	return transformFile(inputPath, outputPath, func(dst io.Writer, src io.Reader) error { return Decrypt(dst, src, passphrase) })
}

func transformFile(inputPath, outputPath string, transform func(io.Writer, io.Reader) error) error {
	src, err := os.Open(inputPath)
	if err != nil { return err }
	defer src.Close()
	dst, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil { return err }
	ok := false
	defer func() { _ = dst.Close(); if !ok { _ = os.Remove(outputPath) } }()
	if err := transform(dst, src); err != nil { return err }
	if err := dst.Sync(); err != nil { return err }
	if err := dst.Close(); err != nil { return err }
	ok = true
	return nil
}
