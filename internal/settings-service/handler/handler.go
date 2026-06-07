// Package handler 实现 settings-service 的 Kitex RPC 入口。
package handler

import (
	settingssvc "ClaranAIM/internal/settings-service/service"
	"ClaranAIM/kitex_gen/settings"
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
	profile, err := h.svc.SaveLLMProfile(ctx, req.UserId, settingssvc.SaveLLMProfileInput{
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

// TestLLMProfile 对用户填写或已保存的模型配置做一次最小连通测试。
func (h *SettingsServiceImpl) TestLLMProfile(ctx context.Context, req *settings.TestLLMProfileReq) (*settings.TestLLMProfileResp, error) {
	result, err := h.svc.TestLLMProfile(ctx, req.UserId, settingssvc.TestLLMProfileInput{
		ProfileID:    req.ProfileId,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseUrl,
		APIKey:       req.ApiKey,
		ModelName:    req.ModelName,
		UsageType:    req.UsageType,
		UseBuiltin:   req.UseBuiltin,
	})
	if err != nil {
		return &settings.TestLLMProfileResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.TestLLMProfileResp{
		Success:      true,
		Ok:           result.OK,
		Msg:          result.Msg,
		LatencyMs:    result.LatencyMS,
		ProviderType: result.ProviderType,
		ModelName:    result.ModelName,
	}, nil
}

// SavePrompt 保存用户 Prompt 模板。
func (h *SettingsServiceImpl) SavePrompt(ctx context.Context, req *settings.SavePromptReq) (*settings.SavePromptResp, error) {
	prompt, err := h.svc.SavePrompt(ctx, req.UserId, settingssvc.SavePromptInput{
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
	skill, err := h.svc.SaveSkill(ctx, req.UserId, settingssvc.SaveSkillInput{
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
		Username:    req.Username,
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

// GetSkill 读取用户拥有的 Skill 详情和入口文件内容。
func (h *SettingsServiceImpl) GetSkill(ctx context.Context, req *settings.GetSkillReq) (*settings.GetSkillResp, error) {
	skill, err := h.svc.GetSkill(ctx, req.UserId, req.SkillId)
	if err != nil {
		return &settings.GetSkillResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.GetSkillResp{Success: true, Skill: toRPCAgentSkill(skill)}, nil
}

// UpdateSkillContent 保存用户在前端编辑后的 SKILL.md 正文。
func (h *SettingsServiceImpl) UpdateSkillContent(ctx context.Context, req *settings.UpdateSkillContentReq) (*settings.UpdateSkillContentResp, error) {
	skill, err := h.svc.UpdateSkillContent(ctx, req.UserId, req.SkillId, req.Name, req.Description, req.Content)
	if err != nil {
		return &settings.UpdateSkillContentResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.UpdateSkillContentResp{Success: true, Skill: toRPCAgentSkill(skill)}, nil
}

// DeleteSkill 删除当前用户拥有的 Skill 元数据。
func (h *SettingsServiceImpl) DeleteSkill(ctx context.Context, req *settings.DeleteSkillReq) (*settings.DeleteSkillResp, error) {
	if err := h.svc.DeleteSkill(ctx, req.UserId, req.SkillId); err != nil {
		return &settings.DeleteSkillResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.DeleteSkillResp{Success: true, Msg: "删除成功"}, nil
}

// SaveMCPServer 保存用户外部 MCP Server 配置。
func (h *SettingsServiceImpl) SaveMCPServer(ctx context.Context, req *settings.SaveMCPServerReq) (*settings.SaveMCPServerResp, error) {
	server, err := h.svc.SaveMCPServer(ctx, req.UserId, settingssvc.SaveMCPServerInput{
		ID:             req.Id,
		AgentID:        req.AgentId,
		ConversationID: req.ConversationId,
		Scope:          req.Scope,
		Name:           req.Name,
		Description:    req.Description,
		Transport:      req.Transport,
		EndpointURL:    req.EndpointUrl,
		Command:        req.Command,
		ArgsJSON:       req.ArgsJson,
		EnvJSON:        req.EnvJson,
		HeadersJSON:    req.HeadersJson,
		AuthType:       req.AuthType,
		Secret:         req.Secret,
		SecretAction:   req.SecretAction,
		Enabled:        optionalBool(req.Enabled, req.EnabledSet),
		TrustLevel:     req.TrustLevel,
		AllowToolsJSON: req.AllowToolsJson,
		DenyToolsJSON:  req.DenyToolsJson,
	})
	if err != nil {
		return &settings.SaveMCPServerResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.SaveMCPServerResp{Success: true, Server: toRPCMCPServer(server)}, nil
}

// ListMCPServers 返回用户配置的 MCP Server 列表，Secret 不回显。
func (h *SettingsServiceImpl) ListMCPServers(ctx context.Context, req *settings.ListMCPServersReq) (*settings.ListMCPServersResp, error) {
	servers, err := h.svc.ListMCPServers(ctx, req.UserId, req.Scope, req.AgentId, req.ConversationId, req.IncludeDisabled)
	if err != nil {
		return &settings.ListMCPServersResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ListMCPServersResp{Success: true, Servers: toRPCMCPServers(servers)}, nil
}

// ResolveMCPServers 返回某次 Agent 运行可用的 MCP 配置，服务间调用可携带 Secret。
func (h *SettingsServiceImpl) ResolveMCPServers(ctx context.Context, req *settings.ResolveMCPServersReq) (*settings.ResolveMCPServersResp, error) {
	servers, err := h.svc.ResolveMCPServers(ctx, req.UserId, req.AgentId, req.ConversationId)
	if err != nil {
		return &settings.ResolveMCPServersResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.ResolveMCPServersResp{Success: true, Servers: toRPCMCPServers(servers)}, nil
}

// DeleteMCPServer 删除当前用户拥有的 MCP Server 配置。
func (h *SettingsServiceImpl) DeleteMCPServer(ctx context.Context, req *settings.DeleteMCPServerReq) (*settings.DeleteMCPServerResp, error) {
	if err := h.svc.DeleteMCPServer(ctx, req.UserId, req.ServerId); err != nil {
		return &settings.DeleteMCPServerResp{Success: false, Msg: err.Error()}, nil
	}
	return &settings.DeleteMCPServerResp{Success: true, Msg: "删除成功"}, nil
}

func toRPCLLMProfile(profile *settingssvc.LLMProfile) *settings.LLMProfile {
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

func toRPCLLMProfiles(profiles []settingssvc.LLMProfile) []*settings.LLMProfile {
	out := make([]*settings.LLMProfile, 0, len(profiles))
	for i := range profiles {
		out = append(out, toRPCLLMProfile(&profiles[i]))
	}
	return out
}

func toRPCPrompt(prompt *settingssvc.PromptTemplate) *settings.PromptTemplate {
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

func toRPCPrompts(prompts []settingssvc.PromptTemplate) []*settings.PromptTemplate {
	out := make([]*settings.PromptTemplate, 0, len(prompts))
	for i := range prompts {
		out = append(out, toRPCPrompt(&prompts[i]))
	}
	return out
}

func toClientSkillFiles(files []*settings.SkillFile) []settingssvc.SkillFileInput {
	out := make([]settingssvc.SkillFileInput, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		out = append(out, settingssvc.SkillFileInput{Path: file.Path, Content: file.Content})
	}
	return out
}

func toRPCAgentSkill(skill *settingssvc.AgentSkill) *settings.AgentSkill {
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
		Summary:     skill.Summary,
		Content:     skill.Content,
	}
}

func toRPCAgentSkills(skills []settingssvc.AgentSkill) []*settings.AgentSkill {
	out := make([]*settings.AgentSkill, 0, len(skills))
	for i := range skills {
		out = append(out, toRPCAgentSkill(&skills[i]))
	}
	return out
}

func toRPCMCPServer(server *settingssvc.MCPServerConfig) *settings.MCPServerConfig {
	if server == nil {
		return nil
	}
	return &settings.MCPServerConfig{
		Id:             server.ID,
		OwnerId:        server.OwnerID,
		AgentId:        server.AgentID,
		ConversationId: server.ConversationID,
		Scope:          server.Scope,
		Name:           server.Name,
		Description:    server.Description,
		Transport:      server.Transport,
		EndpointUrl:    server.EndpointURL,
		Command:        server.Command,
		ArgsJson:       server.ArgsJSON,
		EnvJson:        server.EnvJSON,
		HeadersJson:    server.HeadersJSON,
		AuthType:       server.AuthType,
		Enabled:        server.Enabled,
		TrustLevel:     server.TrustLevel,
		AllowToolsJson: server.AllowToolsJSON,
		DenyToolsJson:  server.DenyToolsJSON,
		HasSecret:      server.HasSecret,
		Secret:         server.Secret,
	}
}

func toRPCMCPServers(servers []settingssvc.MCPServerConfig) []*settings.MCPServerConfig {
	out := make([]*settings.MCPServerConfig, 0, len(servers))
	for i := range servers {
		out = append(out, toRPCMCPServer(&servers[i]))
	}
	return out
}

func optionalBool(value bool, set bool) *bool {
	if !set {
		return nil
	}
	return &value
}
