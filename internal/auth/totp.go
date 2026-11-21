package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func GenerateTOTPSecret() (string,error) {
	raw:=make([]byte,20)
	if _,err:=rand.Read(raw);err!=nil{return "",fmt.Errorf("generate TOTP secret: %w",err)}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw),nil
}

func TOTPCode(secret string,at time.Time)(string,error){
	key,err:=base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err!=nil||len(key)<16{return "",fmt.Errorf("invalid TOTP secret")}
	counter:=uint64(at.Unix()/30)
	var message [8]byte
	binary.BigEndian.PutUint64(message[:],counter)
	mac:=hmac.New(sha1.New,key)
	_,_=mac.Write(message[:])
	sum:=mac.Sum(nil)
	offset:=sum[len(sum)-1]&0x0f
	value:=binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff
	return fmt.Sprintf("%06d",value%1000000),nil
}

func VerifyTOTPCode(secret,code string,at time.Time)bool {
	code=strings.TrimSpace(code)
	if len(code)!=6{return false}
	for _,r:=range code{if r<'0'||r>'9'{return false}}
	for drift:=-1;drift<=1;drift++{
		expected,err:=TOTPCode(secret,at.Add(time.Duration(drift)*30*time.Second))
		if err==nil&&hmac.Equal([]byte(expected),[]byte(code)){return true}
	}
	return false
}

func TOTPProvisioningURI(secret,issuer,account string)string{
	issuer=strings.TrimSpace(issuer);account=strings.TrimSpace(account)
	label:=url.PathEscape(issuer+":"+account)
	query:=url.Values{}
	query.Set("secret",secret)
	query.Set("issuer",issuer)
	query.Set("algorithm","SHA1")
	query.Set("digits","6")
	query.Set("period","30")
	return "otpauth://totp/"+label+"?"+query.Encode()
}
