package auth

import (
	"testing"
	"strings"
	"time"
)

func TestTOTPCodeAndWindowVerification(t *testing.T) {
	secret,err:=GenerateTOTPSecret()
	if err!=nil{t.Fatal(err)}
	now:=time.Unix(1_800_000_000,0).UTC()
	code,err:=TOTPCode(secret,now)
	if err!=nil{t.Fatal(err)}
	if !VerifyTOTPCode(secret,code,now){t.Fatal("generated TOTP code should verify")}
	if !VerifyTOTPCode(secret,code,now.Add(30*time.Second)){t.Fatal("previous time-step code should verify within the allowed drift window")}
	if VerifyTOTPCode(secret,"00000",now){t.Fatal("short code must be rejected")}
	if VerifyTOTPCode(secret,"12a456",now){t.Fatal("non-numeric code must be rejected")}
	if VerifyTOTPCode("invalid","123456",now){t.Fatal("invalid secret must be rejected")}
}

func TestTOTPProvisioningURIContainsIssuerAndParameters(t *testing.T) {
	uri:=TOTPProvisioningURI("JBSWY3DPEHPK3PXP","TG-Card","admin@example.com")
	if uri==""||!strings.HasPrefix(uri,"otpauth://totp/"){t.Fatalf("unexpected provisioning URI: %s",uri)}
	for _,part:=range []string{"secret=JBSWY3DPEHPK3PXP","issuer=TG-Card","algorithm=SHA1","digits=6","period=30"}{
		if !containsString(uri,part){t.Fatalf("provisioning URI missing %q: %s",part,uri)}
	}
}

func containsString(value,part string)bool{
	for i:=0;i+len(part)<=len(value);i++{if value[i:i+len(part)]==part{return true}}
	return false
}

func TestTOTPCodeMatchesRFC6238SHA1Vector(t *testing.T) {
	code,err:=TOTPCode("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",time.Unix(59,0).UTC())
	if err!=nil{t.Fatal(err)}
	if code!="287082"{t.Fatalf("RFC 6238 6-digit SHA1 code=%s want 287082",code)}
}


func TestGenerateTOTPRecoveryCodesAreUniqueAndHighEntropy(t *testing.T) {
	codes,err:=GenerateTOTPRecoveryCodes(10);if err!=nil{t.Fatal(err)}
	if len(codes)!=10{t.Fatalf("generated %d codes, want 10",len(codes))}
	seen:=make(map[string]bool,len(codes))
	for _,code:=range codes{
		if len(code)<24{t.Fatalf("recovery code too short: %q",code)}
		if seen[code]{t.Fatalf("duplicate recovery code generated: %q",code)}
		seen[code]=true
	}
	if _,err:=GenerateTOTPRecoveryCodes(4);err==nil{t.Fatal("must reject too few recovery codes")}
}
