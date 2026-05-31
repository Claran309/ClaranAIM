// Package handler 实现 settings-service 的 Kitex RPC 入口。
package handler

import (
	settingssvc "ClaranAIM/internal/settings-service/service"
	"ClaranAIM/kitex_gen/settings"
	"ClaranAIM/pkg/settingsclient"
	"context"
)

// SettingsServiceImpl 只负责 RPC DTO 与 service DTO 之间的转换。
// LLM 配置所有权、API Key 处理、Skill 文件落盘等业务规则都留在 service 层。
type SettingsServiceImpl struct {
	svc settingssvc.SettingsService
}

// NewSettingsServiceImpl 创建 settings-service 的 Kitex handler。
func NewSettingsServiceImpl(svc settingssvc.SettingsService) settings.SettingsService {
	return &SettingsServiceImpl{svc: svc}
}

// SaveLLMProfile 保存用户的 LLM 预设，并返回脱敏后的配置。
func (h *SettingsServiceImpl) SaveLLMProfile(ctx context.Context, req *settings.SaveLLMProfileReq) (*settings.SaveLLMProfileResp, error) {
	profile, err := h.svc.SaveLLMProfile(ctx, req.UserId, settingsclient.SaveLLMProfileInput{
		ID:           req.Id,
		Name:         req.Name,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseUrl,
		APIKey:       req.ApiKey,
		ModelName:    req.ModelName,
		UsageType:    req.UsageType,
		IsDefault:    req.IsDefault,
		Enabled:      optionalBool(req.Enabled, req.EnabledSet),
		APIKeyAction: req.ApiKeyAction,
	})
	if err != nil {
		return &settings.SaveLLMProfileResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.SaveLLMProfileResp{Success: true, Profile: toRPCLLMProfile(profile)}, nil
}

// ListLLMProfiles 查询当前用户启用的 LLM 预设。
func (h *SettingsServiceImpl) ListLLMProfiles(ctx context.Context, req *settings.ListLLMProfilesReq) (*settings.ListLLMProfilesResp, error) {
	profiles, err := h.svc.ListLLMProfiles(ctx, req.UserId, req.UsageType)
	if err != nil {
		return &settings.ListLLMProfilesResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ListLLMProfilesResp{Success: true, Profiles: toRPCLLMProfiles(profiles)}, nil
}

// DeleteLLMProfile 删除当前用户拥有的 LLM 预设。
func (h *SettingsServiceImpl) DeleteLLMProfile(ctx context.Context, req *settings.DeleteLLMProfileReq) (*settings.DeleteLLMProfileResp, error) {
	if err := h.svc.DeleteLLMProfile(ctx, req.UserId, req.ProfileId); err != nil {
		return &settings.DeleteLLMProfileResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.DeleteLLMProfileResp{Success: true, Msg: "删除成功"}, nil
}

// SavePrompt 保存用户 Prompt 模板。
func (h *SettingsServiceImpl) SavePrompt(ctx context.Context, req *settings.SavePromptReq) (*settings.SavePromptResp, error) {
	prompt, err := h.svc.SavePrompt(ctx, req.UserId, settingsclient.SavePromptInput{
		ID:        req.Id,
		Type:      req.Type,
		Name:      req.Name,
		Content:   req.Content,
		IsDefault: req.IsDefault,
		Enabled:   optionalBool(req.Enabled, req.EnabledSet),
	})
	if err != nil {
		return &settings.SavePromptResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.SavePromptResp{Success: true, Prompt: toRPCPrompt(prompt)}, nil
}

// ListPrompts 返回用户启用的 Prompt 模板。
func (h *SettingsServiceImpl) ListPrompts(ctx context.Context, req *settings.ListPromptsReq) (*settings.ListPromptsResp, error) {
	prompts, err := h.svc.ListPrompts(ctx, req.UserId)
	if err != nil {
		return &settings.ListPromptsResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ListPromptsResp{Success: true, Prompts: toRPCPrompts(prompts)}, nil
}

// ResolveTranslationConfig 返回翻译功能实际使用的模型配置。
// 响应可能包含 API Key 明文，只允许服务间 RPC 使用。
func (h *SettingsServiceImpl) ResolveTranslationConfig(ctx context.Context, req *settings.ResolveTranslationConfigReq) (*settings.ResolveTranslationConfigResp, error) {
	cfg, err := h.svc.ResolveTranslationConfig(ctx, req.UserId)
	if err != nil {
		return &settings.ResolveTranslationConfigResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ResolveTranslationConfigResp{
		Success:        true,
		ProfileId:      cfg.ProfileID,
		ApiKey:         cfg.APIKey,
		BaseUrl:        cfg.BaseURL,
		ModelName:      cfg.ModelName,
		ProviderType:   cfg.ProviderType,
		PromptTemplate: cfg.PromptTemplate,
	}, nil
}

// ResolveLLMProfile 返回创建 Agent 等内部流程可使用的模型配置。
func (h *SettingsServiceImpl) ResolveLLMProfile(ctx context.Context, req *settings.ResolveLLMProfileReq) (*settings.ResolveLLMProfileResp, error) {
	cfg, err := h.svc.ResolveLLMProfile(ctx, req.UserId, req.ProfileId)
	if err != nil {
		return &settings.ResolveLLMProfileResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ResolveLLMProfileResp{
		Success:        true,
		ProfileId:      cfg.ProfileID,
		ApiKey:         cfg.APIKey,
		BaseUrl:        cfg.BaseURL,
		ModelName:      cfg.ModelName,
		ProviderType:   cfg.ProviderType,
		PromptTemplate: cfg.PromptTemplate,
	}, nil
}

// SaveSkill 保存用户上传的 Agent Skill 包元数据。
func (h *SettingsServiceImpl) SaveSkill(ctx context.Context, req *settings.SaveSkillReq) (*settings.SaveSkillResp, error) {
	skill, err := h.svc.SaveSkill(ctx, req.UserId, settingsclient.SaveSkillInput{
		ID:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Scope:       req.Scope,
		AgentID:     req.AgentId,
		FileName:    req.FileName,
		Content:     req.Content,
		Files:       toClientSkillFiles(req.Files),
		IsDefault:   req.IsDefault,
		Enabled:     optionalBool(req.Enabled, req.EnabledSet),
	})
	if err != nil {
		return &settings.SaveSkillResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.SaveSkillResp{Success: true, Skill: toRPCAgentSkill(skill)}, nil
}

// ListSkills 查询当前用户可用的 Agent Skill。
func (h *SettingsServiceImpl) ListSkills(ctx context.Context, req *settings.ListSkillsReq) (*settings.ListSkillsResp, error) {
	skills, err := h.svc.ListSkills(ctx, req.UserId, req.Scope, req.AgentId)
	if err != nil {
		return &settings.ListSkillsResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ListSkillsResp{Success: true, Skills: toRPCAgentSkills(skills)}, nil
}

// DeleteSkill 删除当前用户拥有的 Skill 元数据。
func (h *SettingsServiceImpl) DeleteSkill(ctx context.Context, req *settings.DeleteSkillReq) (*settings.DeleteSkillResp, error) {
	if err := h.svc.DeleteSkill(ctx, req.UserId, req.SkillId); err != nil {
		return &settings.DeleteSkillResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.DeleteSkillResp{Success: true, Msg: "删除成功"}, nil
}

func toRPCLLMProfile(profile *settingsclient.LLMProfile) *settings.LLMProfile {
	if profile == nil {
		return nil
	}
	return &settings.LLMProfile{
		Id:           profile.ID,
		Name:         profile.Name,
		ProviderType: profile.ProviderType,
		BaseUrl:      profile.BaseURL,
		ModelName:    profile.ModelName,
		UsageType:    profile.UsageType,
		IsDefault:    profile.IsDefault,
		Enabled:      profile.Enabled,
		HasApiKey:    profile.HasAPIKey,
	}
}

func toRPCLLMProfiles(profiles []settingsclient.LLMProfile) []*settings.LLMProfile {
	out := make([]*settings.LLMProfile, 0, len(profiles))
	for i := range profiles {
		out = append(out, toRPCLLMProfile(&profiles[i]))
	}
	return out
}

func toRPCPrompt(prompt *settingsclient.PromptTemplate) *settings.PromptTemplate {
	if prompt == nil {
		return nil
	}
	return &settings.PromptTemplate{
		Id:        prompt.ID,
		Type:      prompt.Type,
		Name:      prompt.Name,
		Content:   prompt.Content,
		IsDefault: prompt.IsDefault,
		Enabled:   prompt.Enabled,
	}
}

func toRPCPrompts(prompts []settingsclient.PromptTemplate) []*settings.PromptTemplate {
	out := make([]*settings.PromptTemplate, 0, len(prompts))
	for i := range prompts {
		out = append(out, toRPCPrompt(&prompts[i]))
	}
	return out
}

func toClientSkillFiles(files []*settings.SkillFile) []settingsclient.SkillFileInput {
	out := make([]settingsclient.SkillFileInput, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		out = append(out, settingsclient.SkillFileInput{Path: file.Path, Content: file.Content})
	}
	return out
}

func toRPCAgentSkill(skill *settingsclient.AgentSkill) *settings.AgentSkill {
	if skill == nil {
		return nil
	}
	return &settings.AgentSkill{
		Id:          skill.ID,
		OwnerId:     skill.OwnerID,
		AgentId:     skill.AgentID,
		Scope:       skill.Scope,
		Name:        skill.Name,
		Description: skill.Description,
		SkillsDir:   skill.SkillsDir,
		EntryFile:   skill.EntryFile,
		SourceType:  skill.SourceType,
		IsDefault:   skill.IsDefault,
		Enabled:     skill.Enabled,
	}
}

func toRPCAgentSkills(skills []settingsclient.AgentSkill) []*settings.AgentSkill {
	out := make([]*settings.AgentSkill, 0, len(skills))
	for i := range skills {
		out = append(out, toRPCAgentSkill(&skills[i]))
	}
	return out
}

func optionalBool(value bool, set bool) *bool {
	if !set {
		return nil
	}
	return &value
}
