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


func TestTokenIssueGeneratesUniqueTokenIDs(t *testing.T) {
 manager,err:=NewManager("01234567890123456789012345678901",t.TempDir())
 if err!=nil{t.Fatal(err)}
 claims:=Claims{AdminID:"admin-id",Username:"admin",Role:"super"}
 first,err:=manager.Issue(claims,time.Minute)
 if err!=nil{t.Fatal(err)}
 second,err:=manager.Issue(claims,time.Minute)
 if err!=nil{t.Fatal(err)}
 if first==second{t.Fatal("two sessions issued in the same second must have different bearer tokens")}
 firstClaims,err:=manager.Verify(first)
 if err!=nil{t.Fatal(err)}
 secondClaims,err:=manager.Verify(second)
 if err!=nil{t.Fatal(err)}
 if firstClaims.TokenID==""||secondClaims.TokenID==""||firstClaims.TokenID==secondClaims.TokenID{t.Fatalf("each token must have a distinct JTI: first=%q second=%q",firstClaims.TokenID,secondClaims.TokenID)}
}
