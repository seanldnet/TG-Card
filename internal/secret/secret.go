package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

type Manager struct { aead cipher.AEAD }

func New(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { return nil, fmt.Errorf("create secret directory: %w", err) }
	path := filepath.Join(dataDir, "bot-token.key")
	key, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) { return nil, fmt.Errorf("read bot token key: %w", err) }
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil { return nil, fmt.Errorf("generate bot token key: %w", err) }
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if os.IsExist(err) { key, err = os.ReadFile(path) }
			if err != nil { return nil, fmt.Errorf("persist bot token key: %w", err) }
		} else {
			if _, err = file.Write(key); err == nil { err = file.Sync() }
			closeErr := file.Close()
			if err != nil { return nil, fmt.Errorf("write bot token key: %w", err) }
			if closeErr != nil { return nil, fmt.Errorf("close bot token key: %w", closeErr) }
		}
	}
	if len(key) != 32 { return nil, fmt.Errorf("bot token key must contain exactly 32 bytes") }
	block, err := aes.NewCipher(key)
	if err != nil { return nil, fmt.Errorf("initialize AES: %w", err) }
	aead, err := cipher.NewGCM(block)
	if err != nil { return nil, fmt.Errorf("initialize AES-GCM: %w", err) }
	return &Manager{aead: aead}, nil
}

func (m *Manager) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil { return "", fmt.Errorf("generate encryption nonce: %w", err) }
	sealed := m.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (m *Manager) Decrypt(ciphertext string) (string, error) {
	data, err := base64.RawStdEncoding.DecodeString(ciphertext)
	if err != nil { return "", fmt.Errorf("decode encrypted secret: %w", err) }
	size := m.aead.NonceSize()
	if len(data) < size { return "", fmt.Errorf("encrypted secret is truncated") }
	plain, err := m.aead.Open(nil, data[:size], data[size:], nil)
	if err != nil { return "", fmt.Errorf("decrypt secret: %w", err) }
	return string(plain), nil
}

func Fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
