package tron

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func validateBase58Address(address string) error {
	if len(address) < 26 || len(address) > 35 {
		return fmt.Errorf("expected a Base58Check address")
	}
	value := new(big.Int)
	base := big.NewInt(58)
	for _, char := range address {
		index := strings.IndexRune(base58Alphabet, char)
		if index < 0 { return fmt.Errorf("contains a non-Base58 character") }
		value.Mul(value, base)
		value.Add(value, big.NewInt(int64(index)))
	}
	decoded := value.Bytes()
	leadingZeros := 0
	for leadingZeros < len(address) && address[leadingZeros] == '1' { leadingZeros++ }
	raw := make([]byte, leadingZeros+len(decoded))
	copy(raw[leadingZeros:], decoded)
	if len(raw) != 25 { return fmt.Errorf("invalid decoded address length") }
	if raw[0] != 0x41 { return fmt.Errorf("address is not a TRON mainnet address") }
	first := sha256.Sum256(raw[:21])
	second := sha256.Sum256(first[:])
	for i := 0; i < 4; i++ {
		if raw[21+i] != second[i] { return fmt.Errorf("Base58Check checksum mismatch") }
	}
	return nil
}
