package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type totpEnrollRequest struct { Password string `json:"password"` }
type totpCodeRequest struct { Code string `json:"code"` }
type totpDisableRequest struct { Password string `json:"password"`; Code string `json:"code"` }


type totpLoginRequirementRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// adminLoginTOTPRequirement confirms the submitted credentials before revealing
// whether a second factor is required. It does not create a session or consume
// a TOTP/recovery code; the regular login endpoint remains authoritative.
func (a *application) adminLoginTOTPRequirement(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	ip := a.clientIP(r)
	allowed,limitErr:=a.allowLoginAttempt(r.Context(),ip)
	if limitErr!=nil { log.Printf("check administrator TOTP login rate limit: %v",limitErr);writeError(w,http.StatusServiceUnavailable,"登录安全限流暂不可用，请稍后重试");return }
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "登录尝试过于频繁，请 15 分钟后再试")
		return
	}
	var input totpLoginRequirementRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无效")
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || input.Password == "" || len(input.Username) > 64 || len(input.Password) > 256 {
		writeError(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}
	admin, err := database.FindAdminByUsername(r.Context(), a.db, a.driver, input.Username)
	if err != nil || admin.Status != 1 || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		if err:=a.recordLoginFailure(r.Context(),ip);err!=nil{log.Printf("record administrator TOTP login failure: %v",err);writeError(w,http.StatusServiceUnavailable,"登录安全限流暂不可用，请稍后重试");return}
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err:=a.clearLoginFailures(r.Context(),ip);err!=nil{log.Printf("clear administrator TOTP login failures: %v",err)}
	writeSuccess(w, http.StatusOK, map[string]bool{"enabled": admin.TOTPEnabled})
}

func (a *application) adminTOTPStatus(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	admin,err:=database.FindAdminByID(r.Context(),a.db,a.driver,claims.AdminID)
	if err!=nil{writeError(w,http.StatusInternalServerError,"读取管理员安全状态失败");return}
	writeSuccess(w,http.StatusOK,map[string]any{"enabled":admin.TOTPEnabled,"enabledAt":admin.TOTPEnabledAt})
}

func (a *application) enrollAdminTOTP(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input totpEnrollRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"请求内容无效");return}
	admin,err:=database.FindAdminByID(r.Context(),a.db,a.driver,claims.AdminID)
	if err!=nil||admin.Status!=1{writeError(w,http.StatusUnauthorized,"管理员账户不可用");return}
	if admin.TOTPEnabled{writeError(w,http.StatusConflict,"TOTP 双重验证已启用");return}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash),[]byte(input.Password))!=nil{writeError(w,http.StatusUnauthorized,"当前密码不正确");return}
	secretValue,err:=auth.GenerateTOTPSecret()
	if err!=nil{writeError(w,http.StatusInternalServerError,"生成动态验证码密钥失败");return}
	ciphertext,err:=a.secrets.Encrypt(secretValue)
	if err!=nil{writeError(w,http.StatusInternalServerError,"加密动态验证码密钥失败");return}
	if err:=database.SetAdminTOTPPendingSecret(r.Context(),a.db,a.driver,admin.ID,ciphertext);err!=nil{writeError(w,http.StatusConflict,"无法开始 TOTP 绑定，请刷新后重试");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,admin.ID,"admin.security.totp.enroll.start","admin:"+admin.ID,a.clientIP(r),r.UserAgent(),"TOTP enrollment started")
	w.Header().Set("Cache-Control","no-store, max-age=0")
	w.Header().Set("Pragma","no-cache")
	writeSuccess(w,http.StatusOK,map[string]string{"secret":secretValue,"provisioningUri":auth.TOTPProvisioningURI(secretValue,"TG-Card",admin.Username)})
}

func (a *application) confirmAdminTOTP(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input totpCodeRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"请求内容无效");return}
	admin,err:=database.FindAdminByID(r.Context(),a.db,a.driver,claims.AdminID)
	if err!=nil||admin.Status!=1{writeError(w,http.StatusUnauthorized,"管理员账户不可用");return}
	if admin.TOTPPendingSecretCiphertext==""{writeError(w,http.StatusConflict,"请先开始 TOTP 绑定");return}
	secretValue,err:=a.secrets.Decrypt(admin.TOTPPendingSecretCiphertext)
	if err!=nil{writeError(w,http.StatusServiceUnavailable,"待绑定动态验证码密钥无法解密");return}
	if !auth.VerifyTOTPCode(secretValue,input.Code,time.Now()){writeError(w,http.StatusBadRequest,"动态验证码不正确或已过期");return}
	codes,_,codeErr:=auth.GenerateTOTPRecoveryCodes(10)
	if codeErr!=nil{writeError(w,http.StatusInternalServerError,"生成恢复码失败");return}
	if err:=database.EnableAdminTOTPWithRecoveryCodes(r.Context(),a.db,a.driver,admin.ID,codes);err!=nil{writeError(w,http.StatusConflict,"启用 TOTP 失败，请重新开始绑定");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,admin.ID,"admin.security.totp.enabled","admin:"+admin.ID,a.clientIP(r),r.UserAgent(),"TOTP enabled; 10 one-time recovery codes issued")
	w.Header().Set("Cache-Control","no-store, max-age=0")
	w.Header().Set("Pragma","no-cache")
	writeSuccess(w,http.StatusOK,map[string]any{"enabled":true,"recoveryCodes":codes})
}

func (a *application) disableAdminTOTP(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input totpDisableRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"请求内容无效");return}
	admin,err:=database.FindAdminByID(r.Context(),a.db,a.driver,claims.AdminID)
	if err!=nil||admin.Status!=1{writeError(w,http.StatusUnauthorized,"管理员账户不可用");return}
	if !admin.TOTPEnabled{writeError(w,http.StatusConflict,"TOTP 双重验证尚未启用");return}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash),[]byte(input.Password))!=nil{writeError(w,http.StatusUnauthorized,"当前密码不正确");return}
	secretValue,err:=a.secrets.Decrypt(admin.TOTPSecretCiphertext)
	if err!=nil{writeError(w,http.StatusServiceUnavailable,"动态验证码密钥无法解密");return}
	if !auth.VerifyTOTPCode(secretValue,strings.TrimSpace(input.Code),time.Now()){writeError(w,http.StatusBadRequest,"动态验证码不正确或已过期");return}
	if err:=database.DisableAdminTOTP(r.Context(),a.db,a.driver,admin.ID);err!=nil{writeError(w,http.StatusInternalServerError,"关闭 TOTP 双重验证失败");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,admin.ID,"admin.security.totp.disabled","admin:"+admin.ID,a.clientIP(r),r.UserAgent(),"TOTP disabled after password and code confirmation")
	writeSuccess(w,http.StatusOK,map[string]bool{"enabled":false})
}
