// Package service 实现系统级和用户级可复用设置。
package service

import (
	"ClaranAIM/internal/settings-service/dao"
	"ClaranAIM/internal/settings-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"errors"
	"strings"
)

// 下面这组常量定义当前包使用的固定取值，集中声明可以避免业务代码中散落魔法字符串或魔法数字。
const (
	APIKeyActionKeep  = settingsclient.APIKeyActionKeep
	APIKeyActionSet   = settingsclient.APIKeyActionSet
	APIKeyActionClear = settingsclient.APIKeyActionClear
)

// DefaultLLMConfig 保存平台默认 OpenAI 兼容模型供应商配置。
type DefaultLLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// SettingsService 管理用户/系统 LLM profile 和 prompt 模板。
type SettingsService interface {
	SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*settingsclient.LLMProfile, error)
	ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]settingsclient.LLMProfile, error)
	DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error
	SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*settingsclient.PromptTemplate, error)
	ListPrompts(ctx context.Context, ownerID int64) ([]settingsclient.PromptTemplate, error)
	ResolveTranslationConfig(ctx context.Context, ownerID int64) (ResolvedLLMConfig, error)
	ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (ResolvedLLMConfig, error)
}

// SaveLLMProfileInput 描述一份可复用模型供应商配置。
type SaveLLMProfileInput = settingsclient.SaveLLMProfileInput

// SavePromptInput 描述一次 prompt 模板保存请求。
type SavePromptInput = settingsclient.SavePromptInput

// ResolvedLLMConfig 是为某个任务解析出的模型供应商和 prompt 配置。
type ResolvedLLMConfig = settingsclient.ResolvedLLMConfig

// settingsServiceImpl 是 SettingsService 的默认实现，负责密钥治理、默认配置回退和 DTO 脱敏。
type settingsServiceImpl struct {
	repo       dao.SettingsRepository
	defaultLLM DefaultLLMConfig
}

// NewSettingsService 创建设置业务服务。
func NewSettingsService(repo dao.SettingsRepository, defaultLLM DefaultLLMConfig) SettingsService {
	return &settingsServiceImpl{repo: repo, defaultLLM: defaultLLM}
}

// SaveLLMProfile 保存一份可复用 LLM 供应商配置。
// API Key 保存后不再返回给浏览器，响应中只暴露 has_api_key 供前端展示是否已配置。
func (s *settingsServiceImpl) SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*settingsclient.LLMProfile, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("配置名称不能为空")
	}
	existing, err := s.loadWritableProfile(ctx, ownerID, input)
	if err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	} else if existing != nil {
		enabled = existing.Enabled
	}
	profile := &model.LLMProfile{}
	if existing != nil {
		*profile = *existing
	}
	profile.Scope = model.ScopeUser
	profile.OwnerID = ownerID
	profile.Name = strings.TrimSpace(input.Name)
	profile.ProviderType = defaultString(input.ProviderType, "openai_compatible")
	if strings.TrimSpace(input.BaseURL) != "" {
		profile.BaseURL = strings.TrimSpace(input.BaseURL)
	}
	if strings.TrimSpace(input.ModelName) != "" {
		profile.ModelName = strings.TrimSpace(input.ModelName)
	}
	profile.UsageType = defaultString(input.UsageType, model.ProviderTranslate)
	profile.IsDefault = input.IsDefault
	profile.Enabled = enabled
	switch defaultString(input.APIKeyAction, APIKeyActionKeep) {
	case APIKeyActionSet:
		profile.APIKey = input.APIKey
	case APIKeyActionClear:
		profile.APIKey = ""
	case APIKeyActionKeep:
	default:
		return nil, errors.New("无效的API Key操作")
	}
	if err := s.repo.SaveLLMProfile(ctx, profile); err != nil {
		return nil, err
	}
	return sanitizeLLMProfile(profile), nil
}

// loadWritableProfile 加载当前用户可修改的 LLM profile；按 ID 或名称去重更新。
func (s *settingsServiceImpl) loadWritableProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*model.LLMProfile, error) {
	if input.ID > 0 {
		profile, err := s.repo.GetLLMProfile(ctx, input.ID)
		if err != nil || profile == nil {
			return profile, err
		}
		if profile.Scope != model.ScopeUser || profile.OwnerID != ownerID {
			return nil, errors.New("只能修改自己的LLM配置")
		}
		return profile, nil
	}
	return s.repo.GetLLMProfileByName(ctx, model.ScopeUser, ownerID, strings.TrimSpace(input.Name))
}

// ListLLMProfiles 返回当前用户的 LLM profile 列表，并对密钥脱敏。
func (s *settingsServiceImpl) ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]settingsclient.LLMProfile, error) {
	enabled := true
	profiles, err := s.repo.ListLLMProfiles(ctx, dao.LLMProfileFilter{Scope: model.ScopeUser, OwnerID: ownerID, UsageType: usageType, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	out := make([]settingsclient.LLMProfile, 0, len(profiles))
	for i := range profiles {
		out = append(out, *sanitizeLLMProfile(&profiles[i]))
	}
	return out, nil
}

// DeleteLLMProfile 删除当前用户拥有的模型供应商配置。
func (s *settingsServiceImpl) DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error {
	profile, err := s.repo.GetLLMProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return nil
	}
	if profile.Scope != model.ScopeUser || profile.OwnerID != ownerID {
		return errors.New("只能删除自己的LLM配置")
	}
	return s.repo.DeleteLLMProfile(ctx, profileID)
}

// SavePrompt 保存当前用户拥有的 prompt 模板。
func (s *settingsServiceImpl) SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*settingsclient.PromptTemplate, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	promptType := defaultString(input.Type, model.ProviderTranslate)
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, errors.New("Prompt内容不能为空")
	}
	existing, err := s.repo.GetPromptByType(ctx, model.ScopeUser, ownerID, promptType)
	if err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	} else if existing != nil {
		enabled = existing.Enabled
	}
	prompt := &model.PromptTemplate{}
	if existing != nil {
		*prompt = *existing
	}
	prompt.Scope = model.ScopeUser
	prompt.OwnerID = ownerID
	prompt.Type = promptType
	prompt.Name = defaultString(input.Name, promptType)
	prompt.Content = content
	prompt.IsDefault = input.IsDefault
	prompt.Enabled = enabled
	if err := s.repo.SavePrompt(ctx, prompt); err != nil {
		return nil, err
	}
	return promptToDTO(prompt), nil
}

// ListPrompts 返回当前用户的 prompt 模板列表。
func (s *settingsServiceImpl) ListPrompts(ctx context.Context, ownerID int64) ([]settingsclient.PromptTemplate, error) {
	enabled := true
	prompts, err := s.repo.ListPrompts(ctx, dao.PromptFilter{Scope: model.ScopeUser, OwnerID: ownerID, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	out := make([]settingsclient.PromptTemplate, 0, len(prompts))
	for i := range prompts {
		out = append(out, *promptToDTO(&prompts[i]))
	}
	return out, nil
}

// ResolveTranslationConfig 解析用户翻译功能要使用的 LLM 和 prompt。
// 用户没有配置时回退到平台默认 LLM；如果默认配置也不完整，则返回明确错误。
func (s *settingsServiceImpl) ResolveTranslationConfig(ctx context.Context, ownerID int64) (ResolvedLLMConfig, error) {
	enabled := true
	profiles, err := s.repo.ListLLMProfiles(ctx, dao.LLMProfileFilter{Scope: model.ScopeUser, OwnerID: ownerID, UsageType: model.ProviderTranslate, Enabled: &enabled})
	if err != nil {
		return ResolvedLLMConfig{}, err
	}
	var selected *model.LLMProfile
	for i := range profiles {
		if profiles[i].IsDefault {
			selected = &profiles[i]
			break
		}
	}
	if selected == nil && len(profiles) > 0 {
		selected = &profiles[0]
	}
	resolved := ResolvedLLMConfig{
		APIKey:         s.defaultLLM.APIKey,
		BaseURL:        s.defaultLLM.BaseURL,
		ModelName:      s.defaultLLM.Model,
		ProviderType:   "openai_compatible",
		PromptTemplate: model.DefaultTranslatePrompt,
	}
	if selected != nil {
		resolved.ProfileID = selected.ID
		resolved.APIKey = selected.APIKey
		resolved.BaseURL = selected.BaseURL
		resolved.ModelName = selected.ModelName
		resolved.ProviderType = selected.ProviderType
	}
	prompt, err := s.repo.GetPromptByType(ctx, model.ScopeUser, ownerID, model.ProviderTranslate)
	if err != nil {
		return ResolvedLLMConfig{}, err
	}
	if prompt != nil && prompt.Enabled && strings.TrimSpace(prompt.Content) != "" {
		resolved.PromptTemplate = prompt.Content
	}
	if strings.TrimSpace(resolved.APIKey) == "" || strings.TrimSpace(resolved.BaseURL) == "" || strings.TrimSpace(resolved.ModelName) == "" {
		return ResolvedLLMConfig{}, errors.New("翻译LLM未配置，请在系统设置中配置翻译模型或设置服务默认LLM")
	}
	return resolved, nil
}

// ResolveLLMProfile 返回当前用户拥有的某个 LLM profile，包含内部调用所需的密钥明文。
// 该方法只用于服务间调用，例如创建 Agent 时引用保存好的模型配置，不应直接暴露给浏览器。
func (s *settingsServiceImpl) ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (ResolvedLLMConfig, error) {
	if ownerID <= 0 || profileID <= 0 {
		return ResolvedLLMConfig{}, errors.New("用户和LLM配置不能为空")
	}
	profile, err := s.repo.GetLLMProfile(ctx, profileID)
	if err != nil {
		return ResolvedLLMConfig{}, err
	}
	if profile == nil || profile.Scope != model.ScopeUser || profile.OwnerID != ownerID {
		return ResolvedLLMConfig{}, errors.New("LLM配置不存在或无权使用")
	}
	if !profile.Enabled {
		return ResolvedLLMConfig{}, errors.New("LLM配置已停用")
	}
	if strings.TrimSpace(profile.APIKey) == "" || strings.TrimSpace(profile.BaseURL) == "" || strings.TrimSpace(profile.ModelName) == "" {
		return ResolvedLLMConfig{}, errors.New("LLM配置不完整")
	}
	return ResolvedLLMConfig{
		ProfileID:    profile.ID,
		APIKey:       profile.APIKey,
		BaseURL:      profile.BaseURL,
		ModelName:    profile.ModelName,
		ProviderType: profile.ProviderType,
	}, nil
}

// sanitizeLLMProfile 将数据库模型转换为脱敏 DTO。
func sanitizeLLMProfile(profile *model.LLMProfile) *settingsclient.LLMProfile {
	if profile == nil {
		return nil
	}
	return &settingsclient.LLMProfile{
		ID:           profile.ID,
		Name:         profile.Name,
		ProviderType: profile.ProviderType,
		BaseURL:      profile.BaseURL,
		ModelName:    profile.ModelName,
		UsageType:    profile.UsageType,
		IsDefault:    profile.IsDefault,
		Enabled:      profile.Enabled,
		HasAPIKey:    strings.TrimSpace(profile.APIKey) != "",
	}
}

// promptToDTO 将 prompt 数据库模型转换为客户端 DTO。
func promptToDTO(prompt *model.PromptTemplate) *settingsclient.PromptTemplate {
	if prompt == nil {
		return nil
	}
	return &settingsclient.PromptTemplate{
		ID:        prompt.ID,
		Type:      prompt.Type,
		Name:      prompt.Name,
		Content:   prompt.Content,
		IsDefault: prompt.IsDefault,
		Enabled:   prompt.Enabled,
	}
}

// defaultString 去除空白后在 value 为空时返回 fallback。
func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
