package handler

import (
	"ClaranAIM/pkg/response"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// SettingsHandler 暴露用户级系统设置接口，例如可复用的大模型配置和提示词模板。
type SettingsHandler struct {
	svc settingsclient.Service
}

// 下面这组变量保存当前包需要复用的运行时状态或配置入口，调用方应通过公开函数间接使用。
var gatewaySettingsService settingsclient.Service

// InitSettingsService 将 settings-service 的客户端门面注册到 api-gateway。
func InitSettingsService(svc settingsclient.Service) {
	gatewaySettingsService = svc
}

// NewSettingsHandler 创建系统设置 HTTP handler。
func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{svc: gatewaySettingsService}
}

// ensureService 是当前包内部使用的方法，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func (h *SettingsHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "settings-service未初始化")
		return false
	}
	return true
}

// ListLLMProfiles 返回当前用户保存的可复用大模型配置。
func (h *SettingsHandler) ListLLMProfiles(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	profiles, err := h.svc.ListLLMProfiles(ctx, userID, c.DefaultQuery("usage_type", ""))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "profiles": profiles})
}

// SaveLLMProfile 创建或更新当前用户的大模型配置。
func (h *SettingsHandler) SaveLLMProfile(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req llmProfileReq
	if err := bindSettingsJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	profile, err := h.svc.SaveLLMProfile(ctx, userID, settingsclient.SaveLLMProfileInput{
		ID:           parseSettingsNumber(req.ID),
		Name:         req.Name,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		ModelName:    req.ModelName,
		UsageType:    defaultSettingString(req.UsageType, settingsclient.ProviderTranslate),
		IsDefault:    req.IsDefault,
		Enabled:      req.Enabled,
		APIKeyAction: defaultSettingString(req.APIKeyAction, settingsclient.APIKeyActionKeep),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "profile": profile})
}

// DeleteLLMProfile 删除当前用户名下的一条大模型配置。
func (h *SettingsHandler) DeleteLLMProfile(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的配置ID")
		return
	}
	if err := h.svc.DeleteLLMProfile(ctx, userID, id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true})
}

// ListPrompts 返回当前用户保存的提示词模板。
func (h *SettingsHandler) ListPrompts(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	prompts, err := h.svc.ListPrompts(ctx, userID)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "prompts": prompts})
}

// SavePrompt 创建或更新当前用户的一条提示词模板。
func (h *SettingsHandler) SavePrompt(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req promptReq
	if err := bindSettingsJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	prompt, err := h.svc.SavePrompt(ctx, userID, settingsclient.SavePromptInput{
		ID:        parseSettingsNumber(req.ID),
		Type:      defaultSettingString(req.Type, settingsclient.ProviderTranslate),
		Name:      req.Name,
		Content:   req.Content,
		IsDefault: req.IsDefault,
		Enabled:   req.Enabled,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "prompt": prompt})
}

// llmProfileReq 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type llmProfileReq struct {
	ID           json.Number `json:"id"`
	Name         string      `json:"name"`
	ProviderType string      `json:"provider_type"`
	BaseURL      string      `json:"base_url"`
	APIKey       string      `json:"api_key"`
	ModelName    string      `json:"model_name"`
	UsageType    string      `json:"usage_type"`
	IsDefault    bool        `json:"is_default"`
	Enabled      *bool       `json:"enabled"`
	APIKeyAction string      `json:"api_key_action"`
}

// promptReq 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type promptReq struct {
	ID        json.Number `json:"id"`
	Type      string      `json:"type"`
	Name      string      `json:"name"`
	Content   string      `json:"content"`
	IsDefault bool        `json:"is_default"`
	Enabled   *bool       `json:"enabled"`
}

// bindSettingsJSON 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func bindSettingsJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// parseSettingsNumber 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func parseSettingsNumber(value json.Number) int64 {
	if strings.TrimSpace(value.String()) == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(value.String(), 10, 64)
	return parsed
}

// defaultSettingString 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func defaultSettingString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
