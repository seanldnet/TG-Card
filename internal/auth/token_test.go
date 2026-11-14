package auth

import (
	"testing"
	"time"
)

func TestTokenIssueAndVerify(t *testing.T) {
	manager, err := NewManager("01234567890123456789012345678901", t.TempDir())
	if err != nil { t.Fatal(err) }
	token, err := manager.Issue(Claims{AdminID: "admin-id", Username: "admin", Role: "super"}, time.Minute)
	if err != nil { t.Fatal(err) }
	claims, err := manager.Verify(token)
	if err != nil { t.Fatal(err) }
	if claims.Username != "admin" || claims.Role != "super" { t.Fatalf("unexpected claims: %+v", claims) }
	if _, err := manager.Verify(token+"tampered"); err == nil { t.Fatal("tampered token should be rejected") }
}
