// Package service 实现系统级和用户级可复用设置。
package service

import (
	"ClaranAIM/internal/settings-service/dao"
	"ClaranAIM/internal/settings-service/model"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultLLMConfig 保存平台默认 OpenAI 兼容模型供应商配置。
type DefaultLLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// SettingsService 管理用户/系统 LLM profile 和 prompt 模板。
type SettingsService interface {
	SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*LLMProfile, error)
	ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]LLMProfile, error)
	DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error
	TestLLMProfile(ctx context.Context, ownerID int64, input TestLLMProfileInput) (TestLLMProfileResult, error)
	SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*PromptTemplate, error)
	ListPrompts(ctx context.Context, ownerID int64) ([]PromptTemplate, error)
	ResolveTranslationConfig(ctx context.Context, ownerID int64) (ResolvedLLMConfig, error)
	ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (ResolvedLLMConfig, error)
	SaveSkill(ctx context.Context, ownerID int64, input SaveSkillInput) (*AgentSkill, error)
	GetSkill(ctx context.Context, ownerID, skillID int64) (*AgentSkill, error)
	UpdateSkillContent(ctx context.Context, ownerID, skillID int64, name, description string, content []byte) (*AgentSkill, error)
	ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]AgentSkill, error)
	DeleteSkill(ctx context.Context, ownerID, skillID int64) error
	SaveMCPServer(ctx context.Context, ownerID int64, input SaveMCPServerInput) (*MCPServerConfig, error)
	ListMCPServers(ctx context.Context, ownerID int64, scope string, agentID, conversationID int64, includeDisabled bool) ([]MCPServerConfig, error)
	ResolveMCPServers(ctx context.Context, ownerID, agentID, conversationID int64) ([]MCPServerConfig, error)
	DeleteMCPServer(ctx context.Context, ownerID, serverID int64) error
}

// settingsServiceImpl 是 SettingsService 的默认实现，负责密钥治理、默认配置回退和 DTO 脱敏。
type settingsServiceImpl struct {
	repo             dao.SettingsRepository
	defaultLLM       DefaultLLMConfig
	skillStorageRoot string
	secretCodec      secretCodec
}

// Option 调整 settings-service 的可选运行配置。
type Option func(*settingsServiceImpl)

// WithSkillStorageRoot 配置 Skill 包安全落盘根目录。
func WithSkillStorageRoot(root string) Option {
	return func(s *settingsServiceImpl) {
		s.skillStorageRoot = root
	}
}

// WithSecretEncryptionKey 配置 settings-service 用于保护 API Key / MCP Secret 的本地加密密钥。
// 空字符串表示保持兼容模式：保存和读取都按明文处理，便于测试或旧部署临时过渡。
func WithSecretEncryptionKey(secret string) Option {
	return func(s *settingsServiceImpl) {
		codec, err := newAESGCMSecretCodec(secret)
		if err != nil {
			s.secretCodec = noopSecretCodec{}
			return
		}
		s.secretCodec = codec
	}
}

// NewSettingsService 创建设置业务服务。
func NewSettingsService(repo dao.SettingsRepository, defaultLLM DefaultLLMConfig, opts ...Option) SettingsService {
	svc := &settingsServiceImpl{repo: repo, defaultLLM: defaultLLM, skillStorageRoot: "storage/agent/skills", secretCodec: noopSecretCodec{}}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	if svc.secretCodec == nil {
		svc.secretCodec = noopSecretCodec{}
	}
	return svc
}

// SaveLLMProfile 保存一份可复用 LLM 供应商配置。
// API Key 保存后不再返回给浏览器，响应中只暴露 has_api_key 供前端展示是否已配置。
func (s *settingsServiceImpl) SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*LLMProfile, error) {
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
		encrypted, err := s.encryptSecret(input.APIKey)
		if err != nil {
			return nil, err
		}
		profile.APIKey = encrypted
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
func (s *settingsServiceImpl) ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]LLMProfile, error) {
	enabled := true
	profiles, err := s.repo.ListLLMProfiles(ctx, dao.LLMProfileFilter{Scope: model.ScopeUser, OwnerID: ownerID, UsageType: usageType, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	out := make([]LLMProfile, 0, len(profiles))
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

// TestLLMProfile 用最小请求验证用户填写或已保存的模型配置是否可用。
// 该方法不保存任何密钥；错误信息会脱敏后返回给前端，便于用户定位 BaseURL、模型名或 API Key 问题。
func (s *settingsServiceImpl) TestLLMProfile(ctx context.Context, ownerID int64, input TestLLMProfileInput) (TestLLMProfileResult, error) {
	if ownerID <= 0 {
		return TestLLMProfileResult{}, errors.New("用户未登录")
	}
	cfg, err := s.resolveTestLLMConfig(ctx, ownerID, input)
	if err != nil {
		return TestLLMProfileResult{}, err
	}
	start := time.Now()
	msg, err := testLLMEndpoint(ctx, cfg)
	result := TestLLMProfileResult{
		OK:           err == nil,
		Msg:          defaultString(msg, "连通测试通过"),
		LatencyMS:    time.Since(start).Milliseconds(),
		ProviderType: cfg.ProviderType,
		ModelName:    cfg.ModelName,
	}
	if err != nil {
		result.Msg = sanitizeConnectivityError(err)
	}
	return result, nil
}

func (s *settingsServiceImpl) resolveTestLLMConfig(ctx context.Context, ownerID int64, input TestLLMProfileInput) (ResolvedLLMConfig, error) {
	if input.UseBuiltin {
		cfg := ResolvedLLMConfig{
			APIKey:       strings.TrimSpace(s.defaultLLM.APIKey),
			BaseURL:      strings.TrimSpace(s.defaultLLM.BaseURL),
			ModelName:    strings.TrimSpace(s.defaultLLM.Model),
			ProviderType: defaultString(input.ProviderType, "openai_compatible"),
		}
		switch input.UsageType {
		case ProviderEmbedding:
			cfg.APIKey = firstNonEmpty(os.Getenv("RAG_EMBEDDING_API_KEY"), cfg.APIKey)
			cfg.BaseURL = firstNonEmpty(os.Getenv("RAG_EMBEDDING_URL"), cfg.BaseURL)
			cfg.ModelName = firstNonEmpty(os.Getenv("RAG_EMBEDDING_MODEL"), "embedding-3")
		case ProviderOCR:
			cfg.APIKey = firstNonEmpty(os.Getenv("DOCUMENT_OCR_API_KEY"), cfg.APIKey)
			cfg.BaseURL = firstNonEmpty(os.Getenv("DOCUMENT_OCR_URL"), cfg.BaseURL)
			cfg.ModelName = firstNonEmpty(os.Getenv("DOCUMENT_OCR_MODEL"), "glm-ocr")
		case ProviderRerank:
			cfg.APIKey = firstNonEmpty(os.Getenv("RAG_RERANK_API_KEY"), cfg.APIKey)
			cfg.BaseURL = firstNonEmpty(os.Getenv("RAG_RERANK_URL"), cfg.BaseURL)
			cfg.ModelName = firstNonEmpty(os.Getenv("RAG_RERANK_MODEL"), "rerank")
		case ProviderRAGRouter:
			cfg.BaseURL = firstNonEmpty(os.Getenv("RAG_ROUTER_BASE_URL"), cfg.BaseURL)
			cfg.ModelName = firstNonEmpty(os.Getenv("RAG_ROUTER_MODEL"), cfg.ModelName)
		}
		if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.ModelName == "" {
			return ResolvedLLMConfig{}, errors.New("项目内置默认模型未配置，请检查 LLM_DEFAULT_API_KEY、LLM_DEFAULT_BASE_URL 和 LLM_DEFAULT_MODEL")
		}
		cfg.PromptTemplate = strings.TrimSpace(input.UsageType)
		return cfg, nil
	}
	if input.ProfileID > 0 {
		cfg, err := s.ResolveLLMProfile(ctx, ownerID, input.ProfileID)
		if err != nil {
			return ResolvedLLMConfig{}, err
		}
		cfg.PromptTemplate = strings.TrimSpace(input.UsageType)
		return cfg, nil
	}
	cfg := ResolvedLLMConfig{
		APIKey:       strings.TrimSpace(input.APIKey),
		BaseURL:      strings.TrimSpace(input.BaseURL),
		ModelName:    strings.TrimSpace(input.ModelName),
		ProviderType: defaultString(input.ProviderType, "openai_compatible"),
	}
	if cfg.ModelName == "" {
		switch input.UsageType {
		case ProviderEmbedding:
			cfg.ModelName = "embedding-3"
		case ProviderOCR:
			cfg.ModelName = "glm-ocr"
		case ProviderRerank:
			cfg.ModelName = "rerank"
		}
	}
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.ModelName == "" {
		return ResolvedLLMConfig{}, errors.New("模型配置不完整，请填写 BaseURL、API Key 和模型名")
	}
	cfg.PromptTemplate = strings.TrimSpace(input.UsageType)
	return cfg, nil
}

func testLLMEndpoint(ctx context.Context, cfg ResolvedLLMConfig) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	usage := strings.TrimSpace(cfg.PromptTemplate)
	var payload map[string]interface{}
	switch usage {
	case ProviderEmbedding:
		if !strings.HasSuffix(endpoint, "/embeddings") {
			endpoint += "/embeddings"
		}
		payload = map[string]interface{}{"model": cfg.ModelName, "input": "ClaranAIM 连通测试", "dimensions": 16}
	case ProviderOCR:
		if !strings.HasSuffix(endpoint, "/layout_parsing") {
			endpoint += "/layout_parsing"
		}
		payload = map[string]interface{}{"model": cfg.ModelName, "file": "https://cdn.bigmodel.cn/static/logo/introduction.png"}
	case ProviderRerank:
		if !strings.HasSuffix(endpoint, "/rerank") {
			endpoint += "/rerank"
		}
		payload = map[string]interface{}{"model": cfg.ModelName, "query": "ClaranAIM", "documents": []string{"ClaranAIM 是 Agent Native IM 项目。", "无关文本"}}
	default:
		if !strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint += "/chat/completions"
		}
		payload = map[string]interface{}{
			"model": cfg.ModelName,
			"messages": []map[string]string{
				{"role": "system", "content": "你是连通测试助手，只输出 OK。"},
				{"role": "user", "content": "请回复 OK"},
			},
			"temperature": 0,
			"max_tokens":  8,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("模型服务返回 HTTP %d", resp.StatusCode)
	}
	return "连通测试通过", nil
}

func sanitizeConnectivityError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len([]rune(msg)) > 180 {
		msg = string([]rune(msg)[:180]) + "..."
	}
	return "连通测试失败：" + msg
}

// SavePrompt 保存当前用户拥有的 prompt 模板。
func (s *settingsServiceImpl) SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*PromptTemplate, error) {
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
func (s *settingsServiceImpl) ListPrompts(ctx context.Context, ownerID int64) ([]PromptTemplate, error) {
	enabled := true
	prompts, err := s.repo.ListPrompts(ctx, dao.PromptFilter{Scope: model.ScopeUser, OwnerID: ownerID, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	out := make([]PromptTemplate, 0, len(prompts))
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
		apiKey, err := s.decryptSecret(selected.APIKey)
		if err != nil {
			return ResolvedLLMConfig{}, err
		}
		resolved.ProfileID = selected.ID
		resolved.APIKey = apiKey
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
	apiKey, err := s.decryptSecret(profile.APIKey)
	if err != nil {
		return ResolvedLLMConfig{}, err
	}
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(profile.BaseURL) == "" || strings.TrimSpace(profile.ModelName) == "" {
		return ResolvedLLMConfig{}, errors.New("LLM配置不完整")
	}
	return ResolvedLLMConfig{
		ProfileID:    profile.ID,
		APIKey:       apiKey,
		BaseURL:      profile.BaseURL,
		ModelName:    profile.ModelName,
		ProviderType: profile.ProviderType,
	}, nil
}

// SaveSkill 保存用户上传的 Skill 包并写入元数据。
//
// 浏览器不能直接指定 runtime 使用的目录；settings-service 会把单个 SKILL.md、
// zip 或文件夹展开后的文件写入受控根目录，然后返回安全的 SkillsDir。
func (s *settingsServiceImpl) SaveSkill(ctx context.Context, ownerID int64, input SaveSkillInput) (*AgentSkill, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("Skill名称不能为空")
	}
	scope := defaultString(input.Scope, model.SkillScopeGlobal)
	if scope != model.SkillScopeGlobal && scope != model.SkillScopeAgent {
		return nil, errors.New("无效的Skill作用域")
	}
	if scope == model.SkillScopeAgent && input.AgentID <= 0 {
		return nil, errors.New("Agent专属Skill必须指定Agent ID")
	}
	files, sourceType, err := normalizeSkillFiles(input)
	if err != nil {
		return nil, err
	}
	files, entryFile, packageDir, err := normalizeSkillPackageRoot(files)
	if err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	skill := &model.AgentSkill{
		OwnerID:     ownerID,
		AgentID:     input.AgentID,
		Scope:       scope,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		EntryFile:   entryFile,
		SourceType:  sourceType,
		IsDefault:   input.IsDefault,
		Enabled:     enabled,
	}
	if input.ID > 0 {
		existing, err := s.repo.GetSkill(ctx, input.ID)
		if err != nil {
			return nil, err
		}
		if existing == nil || existing.OwnerID != ownerID {
			return nil, errors.New("Skill不存在或无权修改")
		}
		skill.ID = existing.ID
	}
	dir, err := s.writeSkillFiles(ownerID, input.Username, skill.AgentID, scope, name, packageDir, files)
	if err != nil {
		return nil, err
	}
	skill.SkillsDir = dir
	if err := s.repo.SaveSkill(ctx, skill); err != nil {
		return nil, err
	}
	return skillToDTO(skill), nil
}

// ListSkills 返回当前用户可用的 Skill 列表。
func (s *settingsServiceImpl) ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]AgentSkill, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	enabled := true
	skills, err := s.repo.ListSkills(ctx, dao.SkillFilter{
		OwnerID: ownerID,
		Scope:   strings.TrimSpace(scope),
		AgentID: agentID,
		Enabled: &enabled,
	})
	if err != nil {
		return nil, err
	}
	out := make([]AgentSkill, 0, len(skills))
	for i := range skills {
		out = append(out, *skillToDTO(&skills[i]))
	}
	return out, nil
}

// GetSkill 读取当前用户拥有的 Skill 元数据和入口 SKILL.md 内容。
// 列表接口只返回摘要，编辑器需要用该方法按需读取正文，避免每次打开设置页都传输大文本。
func (s *settingsServiceImpl) GetSkill(ctx context.Context, ownerID, skillID int64) (*AgentSkill, error) {
	if ownerID <= 0 || skillID <= 0 {
		return nil, errors.New("用户和Skill不能为空")
	}
	skill, err := s.repo.GetSkill(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if skill == nil || skill.OwnerID != ownerID {
		return nil, errors.New("Skill不存在或无权访问")
	}
	dto := skillToDTO(skill)
	content, err := readSkillEntryContent(skill)
	if err != nil {
		return nil, err
	}
	dto.Content = string(content)
	dto.Summary = extractSkillSummary(dto.Content, dto.Description)
	return dto, nil
}

// UpdateSkillContent 覆盖当前 Skill 的入口 SKILL.md 并同步基础元数据。
// 只允许改入口文件正文，zip 包中的其他辅助文件保留不动；这样前端可以安全编辑核心指令，同时不破坏资源目录。
func (s *settingsServiceImpl) UpdateSkillContent(ctx context.Context, ownerID, skillID int64, name, description string, content []byte) (*AgentSkill, error) {
	if ownerID <= 0 || skillID <= 0 {
		return nil, errors.New("用户和Skill不能为空")
	}
	if len(content) == 0 {
		return nil, errors.New("Skill内容不能为空")
	}
	if !strings.Contains(strings.ToLower(string(content)), "skill") && !strings.HasPrefix(strings.TrimSpace(string(content)), "#") {
		return nil, errors.New("Skill内容应为有效的Markdown指令")
	}
	skill, err := s.repo.GetSkill(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if skill == nil || skill.OwnerID != ownerID {
		return nil, errors.New("Skill不存在或无权修改")
	}
	entryPath, err := safeSkillEntryPath(skill)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(entryPath, content, 0644); err != nil {
		return nil, err
	}
	if trimmedName := strings.TrimSpace(name); trimmedName != "" {
		skill.Name = trimmedName
	}
	skill.Description = strings.TrimSpace(description)
	if err := s.repo.SaveSkill(ctx, skill); err != nil {
		return nil, err
	}
	dto := skillToDTO(skill)
	dto.Content = string(content)
	dto.Summary = extractSkillSummary(dto.Content, dto.Description)
	return dto, nil
}

// DeleteSkill 删除当前用户拥有的 Skill 元数据；已落盘文件保留，避免误删正在运行中的目录。
func (s *settingsServiceImpl) DeleteSkill(ctx context.Context, ownerID, skillID int64) error {
	if ownerID <= 0 || skillID <= 0 {
		return errors.New("用户和Skill不能为空")
	}
	skill, err := s.repo.GetSkill(ctx, skillID)
	if err != nil {
		return err
	}
	if skill == nil {
		return nil
	}
	if skill.OwnerID != ownerID {
		return errors.New("只能删除自己的Skill")
	}
	return s.repo.DeleteSkill(ctx, skillID)
}

// SaveMCPServer 保存用户自定义外部 MCP Server 配置。
// 列表接口会脱敏 Secret；只有 ResolveMCPServers 会把 Secret 返回给 mcp-gateway 做服务间调用。
func (s *settingsServiceImpl) SaveMCPServer(ctx context.Context, ownerID int64, input SaveMCPServerInput) (*MCPServerConfig, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("MCP名称不能为空")
	}
	scope := defaultString(input.Scope, model.MCPScopeUser)
	if err := validateMCPScope(scope, input.AgentID, input.ConversationID); err != nil {
		return nil, err
	}
	transport := defaultString(input.Transport, model.MCPTransportStreamableHTTP)
	if err := validateMCPTransport(transport, input.EndpointURL, input.Command); err != nil {
		return nil, err
	}
	trust := defaultString(input.TrustLevel, model.MCPTrustLow)
	if trust != model.MCPTrustLow && trust != model.MCPTrustNormal && trust != model.MCPTrustHigh {
		return nil, errors.New("无效的MCP信任级别")
	}
	if err := validateMaybeJSON(input.ArgsJSON, "args_json"); err != nil {
		return nil, err
	}
	if err := validateMaybeJSON(input.EnvJSON, "env_json"); err != nil {
		return nil, err
	}
	if err := validateMaybeJSON(input.HeadersJSON, "headers_json"); err != nil {
		return nil, err
	}
	if err := validateMaybeJSON(input.AllowToolsJSON, "allow_tools_json"); err != nil {
		return nil, err
	}
	if err := validateMaybeJSON(input.DenyToolsJSON, "deny_tools_json"); err != nil {
		return nil, err
	}
	enabled := true
	var existing *model.MCPServerConfig
	if input.ID > 0 {
		loaded, err := s.repo.GetMCPServer(ctx, input.ID)
		if err != nil {
			return nil, err
		}
		if loaded == nil || loaded.OwnerID != ownerID {
			return nil, errors.New("MCP配置不存在或无权修改")
		}
		existing = loaded
		enabled = loaded.Enabled
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	server := &model.MCPServerConfig{}
	if existing != nil {
		*server = *existing
	}
	server.OwnerID = ownerID
	server.AgentID = input.AgentID
	server.ConversationID = input.ConversationID
	server.Scope = scope
	server.Name = name
	server.Description = strings.TrimSpace(input.Description)
	server.Transport = transport
	server.EndpointURL = strings.TrimSpace(input.EndpointURL)
	server.Command = strings.TrimSpace(input.Command)
	server.ArgsJSON = strings.TrimSpace(input.ArgsJSON)
	server.EnvJSON = strings.TrimSpace(input.EnvJSON)
	server.HeadersJSON = strings.TrimSpace(input.HeadersJSON)
	server.AuthType = strings.TrimSpace(input.AuthType)
	server.Enabled = enabled
	server.TrustLevel = trust
	server.AllowToolsJSON = strings.TrimSpace(input.AllowToolsJSON)
	server.DenyToolsJSON = strings.TrimSpace(input.DenyToolsJSON)
	switch defaultString(input.SecretAction, APIKeyActionKeep) {
	case APIKeyActionSet:
		encrypted, err := s.encryptSecret(input.Secret)
		if err != nil {
			return nil, err
		}
		server.Secret = encrypted
	case APIKeyActionClear:
		server.Secret = ""
	case APIKeyActionKeep:
	default:
		return nil, errors.New("无效的MCP Secret操作")
	}
	if err := s.repo.SaveMCPServer(ctx, server); err != nil {
		return nil, err
	}
	return mcpServerToDTO(server, false), nil
}

// ListMCPServers 返回当前用户可见的 MCP 配置列表，默认只返回启用项并脱敏。
func (s *settingsServiceImpl) ListMCPServers(ctx context.Context, ownerID int64, scope string, agentID, conversationID int64, includeDisabled bool) ([]MCPServerConfig, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	var enabled *bool
	if !includeDisabled {
		v := true
		enabled = &v
	}
	servers, err := s.repo.ListMCPServers(ctx, dao.MCPServerFilter{
		OwnerID:        ownerID,
		Scope:          strings.TrimSpace(scope),
		AgentID:        agentID,
		ConversationID: conversationID,
		Enabled:        enabled,
	})
	if err != nil {
		return nil, err
	}
	return mcpServersToDTO(servers, false), nil
}

// ResolveMCPServers 解析某次 Agent 运行可用的 MCP Server，包含全局、用户、Agent、会话四个作用域。
func (s *settingsServiceImpl) ResolveMCPServers(ctx context.Context, ownerID, agentID, conversationID int64) ([]MCPServerConfig, error) {
	if ownerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	enabled := true
	var all []model.MCPServerConfig
	for _, filter := range []dao.MCPServerFilter{
		{OwnerID: ownerID, Scope: model.MCPScopeGlobal, AgentID: 0, ConversationID: 0, Enabled: &enabled},
		{OwnerID: ownerID, Scope: model.MCPScopeUser, AgentID: 0, ConversationID: 0, Enabled: &enabled},
		{OwnerID: ownerID, Scope: model.MCPScopeAgent, AgentID: agentID, ConversationID: 0, Enabled: &enabled},
		{OwnerID: ownerID, Scope: model.MCPScopeConversation, AgentID: agentID, ConversationID: conversationID, Enabled: &enabled},
	} {
		items, err := s.repo.ListMCPServers(ctx, filter)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return s.mcpServersToDTO(all, true), nil
}

// DeleteMCPServer 删除当前用户拥有的 MCP 配置。
func (s *settingsServiceImpl) DeleteMCPServer(ctx context.Context, ownerID, serverID int64) error {
	if ownerID <= 0 || serverID <= 0 {
		return errors.New("用户和MCP配置不能为空")
	}
	server, err := s.repo.GetMCPServer(ctx, serverID)
	if err != nil {
		return err
	}
	if server == nil {
		return nil
	}
	if server.OwnerID != ownerID {
		return errors.New("只能删除自己的MCP配置")
	}
	return s.repo.DeleteMCPServer(ctx, serverID)
}

func (s *settingsServiceImpl) encryptSecret(value string) (string, error) {
	if s.secretCodec == nil {
		return value, nil
	}
	return s.secretCodec.Encrypt(value)
}

func (s *settingsServiceImpl) decryptSecret(value string) (string, error) {
	if s.secretCodec == nil {
		return value, nil
	}
	return s.secretCodec.Decrypt(value)
}

// sanitizeLLMProfile 将数据库模型转换为脱敏 DTO。
func sanitizeLLMProfile(profile *model.LLMProfile) *LLMProfile {
	if profile == nil {
		return nil
	}
	return &LLMProfile{
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
func promptToDTO(prompt *model.PromptTemplate) *PromptTemplate {
	if prompt == nil {
		return nil
	}
	return &PromptTemplate{
		ID:        prompt.ID,
		Type:      prompt.Type,
		Name:      prompt.Name,
		Content:   prompt.Content,
		IsDefault: prompt.IsDefault,
		Enabled:   prompt.Enabled,
	}
}

// skillToDTO 将数据库模型转换为跨服务 DTO。
func skillToDTO(skill *model.AgentSkill) *AgentSkill {
	if skill == nil {
		return nil
	}
	dto := &AgentSkill{
		ID:          skill.ID,
		OwnerID:     skill.OwnerID,
		AgentID:     skill.AgentID,
		Scope:       skill.Scope,
		Name:        skill.Name,
		Description: skill.Description,
		SkillsDir:   skill.SkillsDir,
		EntryFile:   skill.EntryFile,
		SourceType:  skill.SourceType,
		IsDefault:   skill.IsDefault,
		Enabled:     skill.Enabled,
	}
	if content, err := readSkillEntryContent(skill); err == nil {
		dto.Summary = extractSkillSummary(string(content), dto.Description)
	} else {
		dto.Summary = strings.TrimSpace(dto.Description)
	}
	return dto
}

func mcpServerToDTO(server *model.MCPServerConfig, includeSecret bool) *MCPServerConfig {
	if server == nil {
		return nil
	}
	dto := &MCPServerConfig{
		ID:             server.ID,
		OwnerID:        server.OwnerID,
		AgentID:        server.AgentID,
		ConversationID: server.ConversationID,
		Scope:          server.Scope,
		Name:           server.Name,
		Description:    server.Description,
		Transport:      server.Transport,
		EndpointURL:    server.EndpointURL,
		Command:        server.Command,
		ArgsJSON:       server.ArgsJSON,
		EnvJSON:        server.EnvJSON,
		HeadersJSON:    server.HeadersJSON,
		AuthType:       server.AuthType,
		Enabled:        server.Enabled,
		TrustLevel:     server.TrustLevel,
		AllowToolsJSON: server.AllowToolsJSON,
		DenyToolsJSON:  server.DenyToolsJSON,
		HasSecret:      strings.TrimSpace(server.Secret) != "",
	}
	if includeSecret {
		dto.Secret = server.Secret
	}
	return dto
}

func mcpServersToDTO(servers []model.MCPServerConfig, includeSecret bool) []MCPServerConfig {
	out := make([]MCPServerConfig, 0, len(servers))
	for i := range servers {
		dto := mcpServerToDTO(&servers[i], includeSecret)
		if dto != nil {
			out = append(out, *dto)
		}
	}
	return out
}

func (s *settingsServiceImpl) mcpServerToDTO(server *model.MCPServerConfig, includeSecret bool) *MCPServerConfig {
	dto := mcpServerToDTO(server, includeSecret)
	if dto == nil || !includeSecret {
		return dto
	}
	secret, err := s.decryptSecret(dto.Secret)
	if err != nil {
		dto.Secret = ""
		return dto
	}
	dto.Secret = secret
	return dto
}

func (s *settingsServiceImpl) mcpServersToDTO(servers []model.MCPServerConfig, includeSecret bool) []MCPServerConfig {
	out := make([]MCPServerConfig, 0, len(servers))
	for i := range servers {
		dto := s.mcpServerToDTO(&servers[i], includeSecret)
		if dto != nil {
			out = append(out, *dto)
		}
	}
	return out
}

func validateMCPScope(scope string, agentID, conversationID int64) error {
	switch scope {
	case model.MCPScopeUser, model.MCPScopeGlobal:
		return nil
	case model.MCPScopeAgent:
		if agentID <= 0 {
			return errors.New("Agent级MCP必须指定Agent ID")
		}
		return nil
	case model.MCPScopeConversation:
		if agentID <= 0 || conversationID <= 0 {
			return errors.New("会话级MCP必须指定Agent ID和会话ID")
		}
		return nil
	default:
		return errors.New("无效的MCP作用域")
	}
}

func validateMCPTransport(transport, endpointURL, command string) error {
	switch transport {
	case model.MCPTransportStreamableHTTP, model.MCPTransportSSE:
		if strings.TrimSpace(endpointURL) == "" {
			return errors.New("远程MCP必须配置endpoint_url")
		}
	case model.MCPTransportStdio:
		if strings.TrimSpace(command) == "" {
			return errors.New("stdio MCP必须配置command")
		}
	default:
		return errors.New("无效的MCP传输类型")
	}
	return nil
}

func validateMaybeJSON(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !(strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")) {
		return fmt.Errorf("%s必须是JSON对象或数组", field)
	}
	return nil
}

// readSkillEntryContent 读取入口文件正文；入口文件固定限制在 Skill 目录内，防止元数据污染导致越界读取。
func readSkillEntryContent(skill *model.AgentSkill) ([]byte, error) {
	entryPath, err := safeSkillEntryPath(skill)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(entryPath)
}

// safeSkillEntryPath 解析 SKILL.md 的绝对路径，并确认它仍位于 settings-service 管理的 Skill 目录中。
func safeSkillEntryPath(skill *model.AgentSkill) (string, error) {
	if skill == nil || strings.TrimSpace(skill.SkillsDir) == "" {
		return "", errors.New("Skill目录为空")
	}
	entry := strings.TrimSpace(skill.EntryFile)
	if entry == "" {
		entry = "SKILL.md"
	}
	rel, err := cleanSkillRelativePath(entry)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(skill.SkillsDir)
	if err != nil {
		return "", err
	}
	absEntry, err := filepath.Abs(filepath.Join(absDir, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if !isPathInside(absDir, absEntry) {
		return "", errors.New("Skill入口文件越界")
	}
	return absEntry, nil
}

// extractSkillSummary 从 SKILL.md 中提取适合列表展示的一句话摘要。
// 优先使用用户填写的说明；否则跳过标题、空行和列表符号，取第一段有信息量的正文。
func extractSkillSummary(content string, fallback string) string {
	if summary := strings.TrimSpace(fallback); summary != "" {
		return summary
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		line = strings.TrimLeft(line, "-*0123456789.、) ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 90 {
			line = string(runes[:90]) + "..."
		}
		return line
	}
	return "暂无摘要"
}

// normalizeSkillFiles 统一单文件和多文件 Skill 包输入。
func normalizeSkillFiles(input SaveSkillInput) ([]SkillFileInput, string, error) {
	if len(input.Files) > 0 {
		files := make([]SkillFileInput, 0, len(input.Files))
		for _, f := range input.Files {
			clean, err := cleanSkillRelativePath(f.Path)
			if err != nil {
				return nil, "", err
			}
			files = append(files, SkillFileInput{Path: clean, Content: f.Content})
		}
		return files, "package", nil
	}
	if len(input.Content) == 0 {
		return nil, "", errors.New("Skill文件不能为空")
	}
	clean, err := cleanSkillRelativePath(input.FileName)
	if err != nil {
		return nil, "", err
	}
	if !strings.EqualFold(clean, "SKILL.md") {
		return nil, "", errors.New("单文件上传必须命名为SKILL.md")
	}
	return []SkillFileInput{{Path: clean, Content: input.Content}}, "markdown", nil
}

// cleanSkillRelativePath 校验 Skill 包内部路径，禁止绝对路径和路径穿越。
func cleanSkillRelativePath(path string) (string, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "" || filepath.IsAbs(normalized) || strings.HasPrefix(normalized, "/") {
		return "", errors.New("无效的Skill文件路径")
	}
	clean := filepath.Clean(normalized)
	clean = filepath.ToSlash(clean)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", errors.New("Skill文件路径不能包含路径穿越")
	}
	return clean, nil
}

// normalizeSkillPackageRoot 找到上传包中的 SKILL.md，并把所有文件裁剪到同一个包根目录下。
// 例如浏览器目录上传 Skill/SKILL.md 和 Skill/references/a.md 时，落盘目录就是 .../Skill，
// EntryFile 仍是相对包根的 SKILL.md，runtime 不需要理解浏览器原始目录层级。
func normalizeSkillPackageRoot(files []SkillFileInput) ([]SkillFileInput, string, string, error) {
	entryPath := ""
	for _, f := range files {
		normalized := filepath.ToSlash(f.Path)
		if strings.EqualFold(filepath.Base(normalized), "SKILL.md") {
			entryPath = normalized
			break
		}
	}
	if entryPath == "" {
		return nil, "", "", errors.New("Skill包必须包含SKILL.md")
	}
	packageDir := filepath.ToSlash(filepath.Dir(entryPath))
	if packageDir == "." || packageDir == "/" {
		packageDir = ""
	}
	out := make([]SkillFileInput, 0, len(files))
	for _, f := range files {
		path := filepath.ToSlash(f.Path)
		rel := path
		if packageDir != "" {
			prefix := packageDir + "/"
			if path != packageDir && !strings.HasPrefix(path, prefix) {
				continue
			}
			rel = strings.TrimPrefix(path, prefix)
		}
		clean, err := cleanSkillRelativePath(rel)
		if err != nil {
			return nil, "", "", err
		}
		out = append(out, SkillFileInput{Path: clean, Content: f.Content})
	}
	if len(out) == 0 {
		return nil, "", "", errors.New("Skill包为空")
	}
	return out, "SKILL.md", packageDir, nil
}

// writeSkillFiles 将 Skill 包写入受控存储根目录。
func (s *settingsServiceImpl) writeSkillFiles(ownerID int64, username string, agentID int64, scope, name, packageDir string, files []SkillFileInput) (string, error) {
	root := strings.TrimSpace(s.skillStorageRoot)
	if root == "" {
		root = "storage/agent/skills"
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	userFolder := safeUserFolder(username, ownerID)
	packageFolder := safeSkillPackageFolder(packageDir)
	var target string
	if scope == model.SkillScopeAgent {
		target = filepath.Join(absRoot, "agents", userFolder, fmt.Sprintf("%d", agentID), "skill", packageFolder)
	} else {
		target = filepath.Join(absRoot, "global", userFolder, "skill", packageFolder)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !isPathInside(absRoot, absTarget) {
		return "", errors.New("Skill目录越界")
	}
	if err := os.MkdirAll(absTarget, 0755); err != nil {
		return "", err
	}
	for _, f := range files {
		rel, err := cleanSkillRelativePath(f.Path)
		if err != nil {
			return "", err
		}
		dst := filepath.Join(absTarget, filepath.FromSlash(rel))
		absDst, err := filepath.Abs(dst)
		if err != nil {
			return "", err
		}
		if !isPathInside(absTarget, absDst) {
			return "", errors.New("Skill文件写入路径越界")
		}
		if err := os.MkdirAll(filepath.Dir(absDst), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(absDst, f.Content, 0644); err != nil {
			return "", err
		}
	}
	return absTarget, nil
}

// isPathInside 判断 child 是否位于 root 内部或等于 root。
func isPathInside(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// safeSkillSlug 生成适合目录名的短标识。
func safeSkillSlug(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("skill_%d", len([]rune(name)))
	}
	return b.String()
}

// safeUserFolder 生成用户级 Skill 存储目录名。
// 用户名为空时回退到 user_<id>，保证服务间旧调用不会落到共享目录。
func safeUserFolder(username string, ownerID int64) string {
	folder := safePathSegment(username)
	if folder == "" {
		folder = fmt.Sprintf("user_%d", ownerID)
	}
	return folder
}

// safeSkillPackageFolder 保留上传包最外层目录名；单文件上传没有包目录时使用固定 skill 目录。
func safeSkillPackageFolder(packageDir string) string {
	packageDir = filepath.ToSlash(strings.TrimSpace(packageDir))
	packageDir = strings.Trim(packageDir, "/")
	if packageDir == "" || packageDir == "." {
		return "skill"
	}
	return defaultString(safePathSegment(filepath.Base(packageDir)), "skill")
}

// safePathSegment 过滤 Windows 和类 Unix 路径中有特殊含义的字符，保留中文等普通可读字符。
func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		case r < 32:
			continue
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(strings.TrimSpace(b.String()), ". ")
	if out == "" || out == "." || out == ".." {
		return ""
	}
	return out
}

// defaultString 去除空白后在 value 为空时返回 fallback。
func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
