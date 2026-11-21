package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Claims struct {
	AdminID string `json:"sub"`
	Username string `json:"username"`
	RealName string `json:"realName"`
	Role string `json:"role"`
	TokenVersion int `json:"ver"`
	TokenID string `json:"jti"`
	Expires int64 `json:"exp"`
}

type Manager struct { secret []byte }

func NewManager(secret, dataDir string) (*Manager, error) {
	if secret == "" {
		if err := os.MkdirAll(dataDir, 0o750); err != nil { return nil, fmt.Errorf("create token key directory: %w", err) }
		keyPath := filepath.Join(dataDir, "jwt-secret")
		value, err := os.ReadFile(keyPath)
		if err == nil {
			secret = strings.TrimSpace(string(value))
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read token signing key: %w", err)
		} else {
			raw := make([]byte, 48)
			if _, err := rand.Read(raw); err != nil { return nil, fmt.Errorf("generate token signing key: %w", err) }
			secret = base64.RawURLEncoding.EncodeToString(raw)
			file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if errors.Is(err, os.ErrExist) {
				value, err = os.ReadFile(keyPath)
				if err == nil { secret = strings.TrimSpace(string(value)) }
			} else if err == nil {
				_, err = file.WriteString(secret)
				closeErr := file.Close()
				if err == nil { err = closeErr }
			}
			if err != nil { return nil, fmt.Errorf("persist token signing key: %w", err) }
		}
	}
	if len(secret) < 32 { return nil, fmt.Errorf("JWT_SECRET must contain at least 32 characters") }
	return &Manager{secret: []byte(secret)}, nil
}

func (m *Manager) Issue(claims Claims, lifetime time.Duration) (string, error) {
	if claims.TokenID == "" {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil { return "", fmt.Errorf("generate token id: %w", err) }
		claims.TokenID = base64.RawURLEncoding.EncodeToString(raw)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte("{\"alg\":\"HS256\",\"typ\":\"JWT\"}"))
	claims.Expires = time.Now().Add(lifetime).Unix()
	payload, err := json.Marshal(claims)
	if err != nil { return "", err }
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + m.sign(body), nil
}

func (m *Manager) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 { return Claims{}, errors.New("invalid token") }
	expected := m.sign(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(expected), []byte(parts[2])) { return Claims{}, errors.New("invalid token signature") }
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil { return Claims{}, errors.New("invalid token payload") }
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil { return Claims{}, errors.New("invalid token claims") }
	if claims.AdminID == "" || claims.Username == "" || claims.TokenID == "" || claims.Expires <= time.Now().Unix() {
		return Claims{}, errors.New("expired or incomplete token")
	}
	return claims, nil
}

func (m *Manager) sign(value string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}


func GenerateRefreshToken()(string,error){
	raw:=make([]byte,32)
	if _,err:=rand.Read(raw);err!=nil{return "",fmt.Errorf("generate refresh token: %w",err)}
	return base64.RawURLEncoding.EncodeToString(raw),nil
}
