package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/llm"
)

type llmSettingsRequest struct {
	Provider string `json:"provider"`
	BaseURL string `json:"baseUrl"`
	Model string `json:"model"`
	APIKey string `json:"apiKey"`
	TimeoutSeconds int `json:"timeoutSeconds"`
	Enabled bool `json:"enabled"`
}

func (a *application) getLLMSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	view, err := llm.LoadConfigView(r.Context(), a.db, a.driver)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取 AI 模型配置失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	writeSuccess(w, http.StatusOK, view)
}

func (a *application) updateLLMSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	var input llmSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "AI 模型配置格式无效")
		return
	}
	input.Provider = strings.TrimSpace(input.Provider)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Model = strings.TrimSpace(input.Model)
	input.APIKey = strings.TrimSpace(input.APIKey)
	if len(input.Provider) > 40 || len(input.BaseURL) > 500 || len(input.Model) > 200 || len(input.APIKey) > 4096 {
		writeError(w, http.StatusBadRequest, "AI 模型配置字段长度超出限制")
		return
	}
	err := llm.SaveStoredConfig(r.Context(), a.db, a.driver, llm.Config{
		Provider: input.Provider, BaseURL: input.BaseURL, Model: input.Model, APIKey: input.APIKey,
	}, input.TimeoutSeconds, input.Enabled, a.secrets.Encrypt)
	if err != nil {
		if err == llm.ErrNotConfigured {
			writeError(w, http.StatusBadRequest, "请先配置 API Key")
			return
		}
		writeError(w, http.StatusBadRequest, "AI 模型配置无效或保存失败")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "settings.ai.llm.update", "ai-llm-config", a.clientIP(r), r.UserAgent(), "provider="+input.Provider+"; model="+input.Model+"; enabled="+map[bool]string{true:"true",false:"false"}[input.Enabled])
	view, err := llm.LoadConfigView(r.Context(), a.db, a.driver)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "配置已保存，但读取配置状态失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	writeSuccess(w, http.StatusOK, view)
}
