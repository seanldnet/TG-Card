package llm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const configSettingKey = "ai_support_llm_config"
const supportPromptSettingKey = "ai_support_operator_prompt"

type StoredConfig struct {
	Provider string `json:"provider"`
	BaseURL string `json:"baseUrl"`
	Model string `json:"model"`
	APIKeyCiphertext string `json:"apiKeyCiphertext"`
	TimeoutSeconds int `json:"timeoutSeconds"`
	TokenLimitParameter string `json:"tokenLimitParameter,omitempty"`
	Enabled bool `json:"enabled"`
}

type ConfigView struct {
	SupportPrompt string `json:"supportPrompt"`
	Provider string `json:"provider"`
	BaseURL string `json:"baseUrl"`
	Model string `json:"model"`
	TimeoutSeconds int `json:"timeoutSeconds"`
	TokenLimitParameter string `json:"tokenLimitParameter"`
	Enabled bool `json:"enabled"`
	APIKeyConfigured bool `json:"apiKeyConfigured"`
}

type SecretEncrypt func(string) (string, error)
type SecretDecrypt func(string) (string, error)

func settingPlaceholder(driver string, index int) string {
	if driver == "postgres" { return fmt.Sprintf("$%d", index) }
	return "?"
}

// SaveStoredConfig persists non-secret provider metadata and an encrypted API key.
// A blank key keeps the existing ciphertext; it never overwrites a configured key
// with an empty value.
func SaveStoredConfig(ctx context.Context, db *sql.DB, driver string, input Config, timeoutSeconds int, enabled bool, encrypt SecretEncrypt) error {
	if db == nil || encrypt == nil { return errors.New("LLM config storage is unavailable") }
	input.Provider = strings.TrimSpace(input.Provider)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.Model = strings.TrimSpace(input.Model)
	input.TokenLimitParameter = strings.ToLower(strings.TrimSpace(input.TokenLimitParameter))
	if input.TokenLimitParameter == "" { input.TokenLimitParameter = "auto" }
	if input.TokenLimitParameter != "auto" && input.TokenLimitParameter != "max_completion_tokens" && input.TokenLimitParameter != "max_tokens" { return ErrInvalidConfig }
	input.APIKey = strings.TrimSpace(input.APIKey)
	if input.Provider == "" || input.BaseURL == "" || input.Model == "" {
		return ErrInvalidConfig
	}
	if timeoutSeconds <= 0 || timeoutSeconds > 90 { timeoutSeconds = 30 }
	var ciphertext string
	if input.APIKey != "" {
		var err error
		ciphertext, err = encrypt(input.APIKey)
		if err != nil { return fmt.Errorf("encrypt LLM API key: %w", err) }
	} else {
		var existing string
		err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = "+settingPlaceholder(driver, 1), configSettingKey).Scan(&existing)
		if err != nil { return fmt.Errorf("API key is required for initial configuration: %w", err) }
		var current StoredConfig
		if err := json.Unmarshal([]byte(existing), &current); err != nil { return fmt.Errorf("decode saved LLM config: %w", err) }
		ciphertext = current.APIKeyCiphertext
		if ciphertext == "" { return ErrNotConfigured }
	}
	if _, err := NewOpenAICompatible(Config{Provider:input.Provider, BaseURL:input.BaseURL, APIKey:"validation-only", Model:input.Model, Timeout:time.Duration(timeoutSeconds)*time.Second, TokenLimitParameter:input.TokenLimitParameter}); err != nil {
		return err
	}
	value, err := json.Marshal(StoredConfig{Provider:input.Provider, BaseURL:input.BaseURL, Model:input.Model, APIKeyCiphertext:ciphertext, TimeoutSeconds:timeoutSeconds, TokenLimitParameter:input.TokenLimitParameter, Enabled:enabled})
	if err != nil { return err }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "INSERT INTO settings(key,value,updated_at) VALUES ("+settingPlaceholder(driver,1)+","+settingPlaceholder(driver,2)+","+settingPlaceholder(driver,3)+") ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at"
	_, err = db.ExecContext(ctx, query, configSettingKey, string(value), now)
	return err
}

func LoadStoredConfig(ctx context.Context, db *sql.DB, driver string, decrypt SecretDecrypt) (Config, bool, error) {
	if db == nil || decrypt == nil { return Config{}, false, errors.New("LLM config storage is unavailable") }
	var raw string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = "+settingPlaceholder(driver, 1), configSettingKey).Scan(&raw)
	if err == sql.ErrNoRows { return Config{}, false, nil }
	if err != nil { return Config{}, false, err }
	var stored StoredConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil { return Config{}, false, fmt.Errorf("decode saved LLM config: %w", err) }
	if stored.APIKeyCiphertext == "" { return Config{}, false, ErrNotConfigured }
	key, err := decrypt(stored.APIKeyCiphertext)
	if err != nil { return Config{}, false, fmt.Errorf("decrypt saved LLM API key: %w", err) }
	return Config{Provider:stored.Provider, BaseURL:stored.BaseURL, Model:stored.Model, APIKey:key, Timeout:time.Duration(stored.TimeoutSeconds)*time.Second, TokenLimitParameter:stored.TokenLimitParameter}, stored.Enabled, nil
}

func LoadConfigView(ctx context.Context, db *sql.DB, driver string) (ConfigView, error) {
	if db == nil { return ConfigView{}, errors.New("LLM config storage is unavailable") }
	var raw string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = "+settingPlaceholder(driver, 1), configSettingKey).Scan(&raw)
	if err == sql.ErrNoRows { return ConfigView{}, nil }
	if err != nil { return ConfigView{}, err }
	var stored StoredConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil { return ConfigView{}, fmt.Errorf("decode saved LLM config: %w", err) }
	return ConfigView{Provider:stored.Provider, BaseURL:stored.BaseURL, Model:stored.Model, TimeoutSeconds:stored.TimeoutSeconds, TokenLimitParameter:stored.TokenLimitParameter, Enabled:stored.Enabled, APIKeyConfigured:stored.APIKeyCiphertext != ""}, nil
}


// SaveSupportPrompt stores the operator-authored AI support guidance separately from provider credentials.
func SaveSupportPrompt(ctx context.Context, db *sql.DB, driver, prompt string) error {
 if db == nil { return errors.New("LLM config storage is unavailable") }
 prompt = strings.TrimSpace(prompt)
 if len([]rune(prompt)) > 3000 { return fmt.Errorf("AI support prompt exceeds 3000 characters") }
 now := time.Now().UTC().Format(time.RFC3339Nano)
 query := "INSERT INTO settings(key,value,updated_at) VALUES ("+settingPlaceholder(driver,1)+","+settingPlaceholder(driver,2)+","+settingPlaceholder(driver,3)+") ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at"
 _, err := db.ExecContext(ctx, query, supportPromptSettingKey, prompt, now)
 return err
}

func LoadSupportPrompt(ctx context.Context, db *sql.DB, driver string) (string, error) {
 if db == nil { return "", errors.New("LLM config storage is unavailable") }
 var prompt string
 err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = "+settingPlaceholder(driver,1), supportPromptSettingKey).Scan(&prompt)
 if err == sql.ErrNoRows { return "", nil }
 if err != nil { return "", err }
 return limitSupportText(prompt, 3000), nil
}
