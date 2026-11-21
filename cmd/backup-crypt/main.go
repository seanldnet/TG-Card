package main

import (
	"fmt"
	"os"

	"github.com/seanldnet/TG-Card/internal/backupcrypto"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: backup-crypt <encrypt|decrypt> <input> <output>")
		os.Exit(2)
	}
	passphrase := os.Getenv("BACKUP_ENCRYPTION_PASSPHRASE")
	var err error
	switch os.Args[1] {
	case "encrypt":
		err = backupcrypto.EncryptFile(os.Args[2], os.Args[3], passphrase)
	case "decrypt":
		err = backupcrypto.DecryptFile(os.Args[2], os.Args[3], passphrase)
	default:
		fmt.Fprintln(os.Stderr, "operation must be encrypt or decrypt")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
