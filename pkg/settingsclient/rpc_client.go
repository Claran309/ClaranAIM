package settingsclient

import (
	"ClaranAIM/kitex_gen/settings"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"context"
	"errors"
)

// RPCClient 用 Kitex 调用 settings-service。
// 它实现 Service 接口，调用方无需感知底层传输从内部 HTTP 切换到了 RPC。
type RPCClient struct {
	client settingsservice.Client
}

// NewRPCClient 包装已由服务启动逻辑创建好的 Kitex 客户端。
func NewRPCClient(client settingsservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

// SaveLLMProfile 创建或更新用户的 LLM 预设。
func (c *RPCClient) SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*LLMProfile, error) {
	resp, err := c.client.SaveLLMProfile(ctx, &settings.SaveLLMProfileReq{
		UserId:       ownerID,
		Id:           input.ID,
		Name:         input.Name,
		ProviderType: input.ProviderType,
		BaseUrl:      input.BaseURL,
		ApiKey:       input.APIKey,
		ModelName:    input.ModelName,
		UsageType:    input.UsageType,
		IsDefault:    input.IsDefault,
		Enabled:      boolValue(input.Enabled, true),
		ApiKeyAction: input.APIKeyAction,
		EnabledSet:   input.Enabled != nil,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCLLMProfile(resp.GetProfile()), nil
}

// ListLLMProfiles 查询用户启用中的 LLM 预设。
func (c *RPCClient) ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]LLMProfile, error) {
	resp, err := c.client.ListLLMProfiles(ctx, &settings.ListLLMProfilesReq{UserId: ownerID, UsageType: usageType})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCLLMProfiles(resp.GetProfiles()), nil
}

// DeleteLLMProfile 删除当前用户拥有的 LLM 预设。
func (c *RPCClient) DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error {
	resp, err := c.client.DeleteLLMProfile(ctx, &settings.DeleteLLMProfileReq{UserId: ownerID, ProfileId: profileID})
	if err != nil {
		return err
	}
	return rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

// SavePrompt 创建或更新用户 Prompt 模板。
func (c *RPCClient) SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*PromptTemplate, error) {
	resp, err := c.client.SavePrompt(ctx, &settings.SavePromptReq{
		UserId:     ownerID,
		Id:         input.ID,
		Type:       input.Type,
		Name:       input.Name,
		Content:    input.Content,
		IsDefault:  input.IsDefault,
		Enabled:    boolValue(input.Enabled, true),
		EnabledSet: input.Enabled != nil,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCPrompt(resp.GetPrompt()), nil
}

// ListPrompts 返回用户启用中的 Prompt 模板。
func (c *RPCClient) ListPrompts(ctx context.Context, ownerID int64) ([]PromptTemplate, error) {
	resp, err := c.client.ListPrompts(ctx, &settings.ListPromptsReq{UserId: ownerID})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCPrompts(resp.GetPrompts()), nil
}

// ResolveTranslationConfig 解析消息翻译使用的 LLM 配置。
func (c *RPCClient) ResolveTranslationConfig(ctx context.Context, ownerID int64) (ResolvedLLMConfig, error) {
	resp, err := c.client.ResolveTranslationConfig(ctx, &settings.ResolveTranslationConfigReq{UserId: ownerID})
	if err != nil {
		return ResolvedLLMConfig{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return ResolvedLLMConfig{}, err
	}
	return ResolvedLLMConfig{
		ProfileID:      resp.GetProfileId(),
		APIKey:         resp.GetApiKey(),
		BaseURL:        resp.GetBaseUrl(),
		ModelName:      resp.GetModelName(),
		ProviderType:   resp.GetProviderType(),
		PromptTemplate: resp.GetPromptTemplate(),
	}, nil
}

// ResolveLLMProfile 解析某个用户拥有的 LLM 预设。
func (c *RPCClient) ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (ResolvedLLMConfig, error) {
	resp, err := c.client.ResolveLLMProfile(ctx, &settings.ResolveLLMProfileReq{UserId: ownerID, ProfileId: profileID})
	if err != nil {
		return ResolvedLLMConfig{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return ResolvedLLMConfig{}, err
	}
	return ResolvedLLMConfig{
		ProfileID:      resp.GetProfileId(),
		APIKey:         resp.GetApiKey(),
		BaseURL:        resp.GetBaseUrl(),
		ModelName:      resp.GetModelName(),
		ProviderType:   resp.GetProviderType(),
		PromptTemplate: resp.GetPromptTemplate(),
	}, nil
}

// SaveSkill 保存用户上传后的 Skill 包元数据和文件内容。
func (c *RPCClient) SaveSkill(ctx context.Context, ownerID int64, input SaveSkillInput) (*AgentSkill, error) {
	resp, err := c.client.SaveSkill(ctx, &settings.SaveSkillReq{
		UserId:      ownerID,
		Id:          input.ID,
		Name:        input.Name,
		Description: input.Description,
		Scope:       input.Scope,
		AgentId:     input.AgentID,
		FileName:    input.FileName,
		Content:     input.Content,
		Files:       toRPCSkillFiles(input.Files),
		IsDefault:   input.IsDefault,
		Enabled:     boolValue(input.Enabled, true),
		EnabledSet:  input.Enabled != nil,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCAgentSkill(resp.GetSkill()), nil
}

// ListSkills 查询当前用户可用的 Skill。
func (c *RPCClient) ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]AgentSkill, error) {
	resp, err := c.client.ListSkills(ctx, &settings.ListSkillsReq{UserId: ownerID, Scope: scope, AgentId: agentID})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCAgentSkills(resp.GetSkills()), nil
}

// DeleteSkill 删除当前用户拥有的 Skill。
func (c *RPCClient) DeleteSkill(ctx context.Context, ownerID, skillID int64) error {
	resp, err := c.client.DeleteSkill(ctx, &settings.DeleteSkillReq{UserId: ownerID, SkillId: skillID})
	if err != nil {
		return err
	}
	return rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

func fromRPCLLMProfile(profile *settings.LLMProfile) *LLMProfile {
	if profile == nil {
		return nil
	}
	return &LLMProfile{
		ID:           profile.GetId(),
		Name:         profile.GetName(),
		ProviderType: profile.GetProviderType(),
		BaseURL:      profile.GetBaseUrl(),
		ModelName:    profile.GetModelName(),
		UsageType:    profile.GetUsageType(),
		IsDefault:    profile.GetIsDefault(),
		Enabled:      profile.GetEnabled(),
		HasAPIKey:    profile.GetHasApiKey(),
	}
}

func fromRPCLLMProfiles(profiles []*settings.LLMProfile) []LLMProfile {
	out := make([]LLMProfile, 0, len(profiles))
	for _, profile := range profiles {
		item := fromRPCLLMProfile(profile)
		if item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func fromRPCPrompt(prompt *settings.PromptTemplate) *PromptTemplate {
	if prompt == nil {
		return nil
	}
	return &PromptTemplate{
		ID:        prompt.GetId(),
		Type:      prompt.GetType(),
		Name:      prompt.GetName(),
		Content:   prompt.GetContent(),
		IsDefault: prompt.GetIsDefault(),
		Enabled:   prompt.GetEnabled(),
	}
}

func fromRPCPrompts(prompts []*settings.PromptTemplate) []PromptTemplate {
	out := make([]PromptTemplate, 0, len(prompts))
	for _, prompt := range prompts {
		item := fromRPCPrompt(prompt)
		if item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func toRPCSkillFiles(files []SkillFileInput) []*settings.SkillFile {
	out := make([]*settings.SkillFile, 0, len(files))
	for _, file := range files {
		out = append(out, &settings.SkillFile{Path: file.Path, Content: file.Content})
	}
	return out
}

func fromRPCAgentSkill(skill *settings.AgentSkill) *AgentSkill {
	if skill == nil {
		return nil
	}
	return &AgentSkill{
		ID:          skill.GetId(),
		OwnerID:     skill.GetOwnerId(),
		AgentID:     skill.GetAgentId(),
		Scope:       skill.GetScope(),
		Name:        skill.GetName(),
		Description: skill.GetDescription(),
		SkillsDir:   skill.GetSkillsDir(),
		EntryFile:   skill.GetEntryFile(),
		SourceType:  skill.GetSourceType(),
		IsDefault:   skill.GetIsDefault(),
		Enabled:     skill.GetEnabled(),
	}
}

func fromRPCAgentSkills(skills []*settings.AgentSkill) []AgentSkill {
	out := make([]AgentSkill, 0, len(skills))
	for _, skill := range skills {
		item := fromRPCAgentSkill(skill)
		if item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	if msg == "" {
		msg = "settings-service RPC调用失败"
	}
	return errors.New(msg)
}
