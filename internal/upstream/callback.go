package upstream

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifyDujiaoCallback checks the documented Dujiao-Next callback headers and
// signature over the exact raw request body. It deliberately rejects stale timestamps.
func VerifyDujiaoCallback(expectedAPIKey, apiSecret, providedAPIKey, timestamp, signature, path string, body []byte, now time.Time) error {
	if expectedAPIKey == "" || apiSecret == "" || providedAPIKey == "" || timestamp == "" || signature == "" {
		return fmt.Errorf("missing Dujiao callback authentication headers")
	}
	if !hmac.Equal([]byte(expectedAPIKey), []byte(providedAPIKey)) {
		return fmt.Errorf("Dujiao callback API key mismatch")
	}
	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil { return fmt.Errorf("invalid Dujiao callback timestamp") }
	delta := now.Unix() - unixSeconds
	if delta < -60 || delta > 60 { return fmt.Errorf("Dujiao callback timestamp outside allowed 60-second window") }
	if !strings.HasPrefix(path, dujiaoAPIPrefix+"/") || strings.Contains(path, "?") {
		return fmt.Errorf("invalid Dujiao callback path")
	}
	bodyHash := md5.Sum(body)
	signingText := "POST\n" + path + "\n" + timestamp + "\n" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signingText))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	provided, err := hex.DecodeString(signature)
	if err != nil { return fmt.Errorf("invalid Dujiao callback signature encoding") }
	expected, _ := hex.DecodeString(expectedSignature)
	if !hmac.Equal(expected, provided) { return fmt.Errorf("Dujiao callback signature verification failed") }
	return nil
}
