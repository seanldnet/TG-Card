package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/database"
	"golang.org/x/crypto/bcrypt"
)

const adminActionConfirmationHeader = "X-Admin-Action-Confirmation"

type adminActionConfirmationRequest struct {
	Password         string          `json:"password"`
	TOTPCode         string          `json:"totpCode"`
	TOTPRecoveryCode string          `json:"totpRecoveryCode"`
	ActionKey        string          `json:"actionKey"`
	Method           string          `json:"method"`
	Path             string          `json:"path"`
	Payload          json.RawMessage `json:"payload"`
}

var protectedAdminActionKeys = map[string]bool{
	"wallet.balance.adjust":          true,
	"upstream.procurement.retry":     true,
	"upstream.procurement.refund":    true,
	"upstream.procurement.resolve":   true,
	"payment.transaction.reconcile":  true,
	"support.ticket.refund":          true,
	"order.delivery.retry":           true,
	"order.delivery.resolve-step":    true,
	"order.cleanup":                  true,
	"settings.payment.update":        true,
	"settings.recharge-bonus.update": true,
	"settings.site.update":           true,
	"settings.upstream.update":       true,
	"settings.upstream.disable":      true,
}

func canonicalAdminActionPayload(raw []byte) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("action payload must be valid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", fmt.Errorf("action payload contains trailing JSON data")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func adminActionRequestHash(method, path string, rawPayload []byte) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete {
		return "", fmt.Errorf("unsupported sensitive action method")
	}
	if !strings.HasPrefix(path, "/api/") || strings.ContainsAny(path, "?#") || strings.ContainsAny(path, "\\r\\n") || strings.Contains(path, "\\\\") || strings.Contains(path, "/../") {
		return "", fmt.Errorf("invalid sensitive action path")
	}
	payload, err := canonicalAdminActionPayload(rawPayload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(method + "\n" + path + "\n" + payload))
	return hex.EncodeToString(sum[:]), nil
}

func generateAdminActionConfirmationToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate administrator action confirmation: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (a *application) confirmAdminAction(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	var input adminActionConfirmationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "二次确认请求内容无效")
		return
	}
	input.ActionKey = strings.TrimSpace(input.ActionKey)
	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	input.Path = strings.TrimSpace(input.Path)
	if !protectedAdminActionKeys[input.ActionKey] {
		writeError(w, http.StatusBadRequest, "该操作不支持二次确认")
		return
	}
	requestHash, err := adminActionRequestHash(input.Method, input.Path, input.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "二次确认目标无效")
		return
	}
	admin, err := database.FindAdminByID(r.Context(), a.db, a.driver, claims.AdminID)
	if err != nil || admin.Status != 1 {
		writeError(w, http.StatusUnauthorized, "管理员账户不可用")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "当前管理员密码不正确")
		return
	}
	usedRecoveryCode := false
	if admin.TOTPEnabled {
		validCode := false
		if strings.TrimSpace(input.TOTPCode) != "" {
			secretValue, decryptErr := a.secrets.Decrypt(admin.TOTPSecretCiphertext)
			if decryptErr != nil {
				writeError(w, http.StatusServiceUnavailable, "管理员动态验证码密钥无法解密")
				return
			}
			validCode = auth.VerifyTOTPCode(secretValue, input.TOTPCode, time.Now())
		}
		recoveryCandidate := strings.TrimSpace(input.TOTPRecoveryCode)
		if recoveryCandidate == "" && !validCode {
			recoveryCandidate = strings.TrimSpace(input.TOTPCode)
		}
		if !validCode && recoveryCandidate != "" {
			consumed, consumeErr := database.ConsumeAdminTOTPRecoveryCode(r.Context(), a.db, a.driver, admin.ID, recoveryCandidate)
			if consumeErr != nil {
				writeError(w, http.StatusInternalServerError, "校验一次性恢复码失败")
				return
			}
			validCode = consumed
			usedRecoveryCode = consumed
		}
		if !validCode {
			writeError(w, http.StatusUnauthorized, "请提供有效的动态验证码或一次性恢复码")
			return
		}
	}
	token, err := generateAdminActionConfirmationToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成二次确认凭证失败")
		return
	}
	expiresAt := time.Now().Add(2 * time.Minute)
	if err := database.CreateAdminActionConfirmation(r.Context(), a.db, a.driver, admin.ID, input.ActionKey, requestHash, adminTokenHash(token), expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "保存二次确认凭证失败")
		return
	}
	details := "action=" + input.ActionKey + "; target=" + input.Method + " " + input.Path
	if usedRecoveryCode {
		details += "; one-time recovery code consumed"
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, admin.ID, "admin.action.confirm", "admin-action:"+input.ActionKey, a.clientIP(r), r.UserAgent(), details)
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	writeSuccess(w, http.StatusOK, map[string]any{"token": token, "expiresInSeconds": 120, "actionKey": input.ActionKey})
}

func (a *application) requireAdminActionConfirmation(actionKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		admin, err := database.FindAdminByID(r.Context(), a.db, a.driver, claims.AdminID)
		if err != nil || admin.Status != 1 {
			writeError(w, http.StatusUnauthorized, "管理员账户不可用")
			return
		}
		writeRequired := func(message string) {
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			writeJSON(w, http.StatusPreconditionRequired, map[string]any{
				"code": -2,
				"data": map[string]any{"actionKey": actionKey, "totpRequired": admin.TOTPEnabled},
				"error": message,
				"message": message,
			})
		}
		token := strings.TrimSpace(r.Header.Get(adminActionConfirmationHeader))
		if token == "" {
			writeRequired("此操作需要管理员二次确认")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "操作请求内容过大")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		requestHash, err := adminActionRequestHash(r.Method, r.URL.EscapedPath(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "操作请求内容无效")
			return
		}
		consumed, err := database.ConsumeAdminActionConfirmation(r.Context(), a.db, a.driver, admin.ID, actionKey, requestHash, adminTokenHash(token), time.Now())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "校验二次确认凭证失败")
			return
		}
		if !consumed {
			writeRequired("二次确认凭证无效、已过期或已使用，请重新确认")
			return
		}
		next(w, r)
	}
}
