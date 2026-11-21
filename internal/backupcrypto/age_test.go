package backupcrypto

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgeEncryptDecryptRoundTrip(t *testing.T) {
	passphrase := "test-only-passphrase-with-at-least-32-characters"
	plain := bytes.Repeat([]byte("TG-Card backup fixture\x00"), 4096)
	var encrypted bytes.Buffer
	if err := Encrypt(&encrypted, bytes.NewReader(plain), passphrase); err != nil { t.Fatal(err) }
	if bytes.Contains(encrypted.Bytes(), plain) { t.Fatal("ciphertext contains plaintext fixture") }
	var decrypted bytes.Buffer
	if err := Decrypt(&decrypted, bytes.NewReader(encrypted.Bytes()), passphrase); err != nil { t.Fatal(err) }
	if !bytes.Equal(decrypted.Bytes(), plain) { t.Fatal("decrypted content differs from original") }
}

func TestAgeRejectsWrongPassphraseAndTampering(t *testing.T) {
	passphrase := "test-only-passphrase-with-at-least-32-characters"
	var encrypted bytes.Buffer
	if err := Encrypt(&encrypted, strings.NewReader("sensitive backup data"), passphrase); err != nil { t.Fatal(err) }
	if err := Decrypt(&bytes.Buffer{}, bytes.NewReader(encrypted.Bytes()), "wrong-passphrase-that-is-long-enough"); err == nil {
		t.Fatal("wrong passphrase must fail")
	}
	tampered := append([]byte(nil), encrypted.Bytes()...)
	tampered[len(tampered)-1] ^= 0x01
	if err := Decrypt(&bytes.Buffer{}, bytes.NewReader(tampered), passphrase); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
}

func TestAgeRequiresExternalStrongPassphrase(t *testing.T) {
	for _, passphrase := range []string{"", "short", strings.Repeat(" ", 64), strings.Repeat("x", 31)} {
		if err := ValidatePassphrase(passphrase); err == nil { t.Fatalf("expected rejection for %q", passphrase) }
	}
	if err := ValidatePassphrase(strings.Repeat("x", 32)); err != nil {
		t.Fatalf("32-character passphrase should be accepted: %v", err)
	}
}
