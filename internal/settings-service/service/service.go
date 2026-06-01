// Package service 实现系统级和用户级可复用设置。
package service

import (
	"ClaranAIM/internal/settings-service/dao"
	"ClaranAIM/internal/settings-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// APIKeyAction* 复用 pkg/settingsclient 的协议值，避免 service 层和 HTTP 客户端对密钥操作语义不一致。
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
	SaveSkill(ctx context.Context, ownerID int64, input SaveSkillInput) (*settingsclient.AgentSkill, error)
	GetSkill(ctx context.Context, ownerID, skillID int64) (*settingsclient.AgentSkill, error)
	UpdateSkillContent(ctx context.Context, ownerID, skillID int64, name, description string, content []byte) (*settingsclient.AgentSkill, error)
	ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]settingsclient.AgentSkill, error)
	DeleteSkill(ctx context.Context, ownerID, skillID int64) error
}

// SaveLLMProfileInput 描述一份可复用模型供应商配置。
type SaveLLMProfileInput = settingsclient.SaveLLMProfileInput

// SavePromptInput 描述一次 prompt 模板保存请求。
type SavePromptInput = settingsclient.SavePromptInput

// SaveSkillInput 描述一次 Skill 包上传保存请求。
type SaveSkillInput = settingsclient.SaveSkillInput

// ResolvedLLMConfig 是为某个任务解析出的模型供应商和 prompt 配置。
type ResolvedLLMConfig = settingsclient.ResolvedLLMConfig

// settingsServiceImpl 是 SettingsService 的默认实现，负责密钥治理、默认配置回退和 DTO 脱敏。
type settingsServiceImpl struct {
	repo             dao.SettingsRepository
	defaultLLM       DefaultLLMConfig
	skillStorageRoot string
}

// Option 调整 settings-service 的可选运行配置。
type Option func(*settingsServiceImpl)

// WithSkillStorageRoot 配置 Skill 包安全落盘根目录。
func WithSkillStorageRoot(root string) Option {
	return func(s *settingsServiceImpl) {
		s.skillStorageRoot = root
	}
}

// NewSettingsService 创建设置业务服务。
func NewSettingsService(repo dao.SettingsRepository, defaultLLM DefaultLLMConfig, opts ...Option) SettingsService {
	svc := &settingsServiceImpl{repo: repo, defaultLLM: defaultLLM, skillStorageRoot: "storage/agent/skills"}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
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

// SaveSkill 保存用户上传的 Skill 包并写入元数据。
//
// 浏览器不能直接指定 runtime 使用的目录；settings-service 会把单个 SKILL.md、
// zip 或文件夹展开后的文件写入受控根目录，然后返回安全的 SkillsDir。
func (s *settingsServiceImpl) SaveSkill(ctx context.Context, ownerID int64, input SaveSkillInput) (*settingsclient.AgentSkill, error) {
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
	if !containsSkillMarkdown(files) {
		return nil, errors.New("Skill包必须包含SKILL.md")
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
		EntryFile:   "SKILL.md",
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
	dir, err := s.writeSkillFiles(ownerID, skill.AgentID, scope, name, files)
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
func (s *settingsServiceImpl) ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]settingsclient.AgentSkill, error) {
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
	out := make([]settingsclient.AgentSkill, 0, len(skills))
	for i := range skills {
		out = append(out, *skillToDTO(&skills[i]))
	}
	return out, nil
}

// GetSkill 读取当前用户拥有的 Skill 元数据和入口 SKILL.md 内容。
// 列表接口只返回摘要，编辑器需要用该方法按需读取正文，避免每次打开设置页都传输大文本。
func (s *settingsServiceImpl) GetSkill(ctx context.Context, ownerID, skillID int64) (*settingsclient.AgentSkill, error) {
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
func (s *settingsServiceImpl) UpdateSkillContent(ctx context.Context, ownerID, skillID int64, name, description string, content []byte) (*settingsclient.AgentSkill, error) {
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
	skill.EntryFile = "SKILL.md"
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

// skillToDTO 将数据库模型转换为跨服务 DTO。
func skillToDTO(skill *model.AgentSkill) *settingsclient.AgentSkill {
	if skill == nil {
		return nil
	}
	dto := &settingsclient.AgentSkill{
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
func normalizeSkillFiles(input SaveSkillInput) ([]settingsclient.SkillFileInput, string, error) {
	if len(input.Files) > 0 {
		files := make([]settingsclient.SkillFileInput, 0, len(input.Files))
		for _, f := range input.Files {
			clean, err := cleanSkillRelativePath(f.Path)
			if err != nil {
				return nil, "", err
			}
			files = append(files, settingsclient.SkillFileInput{Path: clean, Content: f.Content})
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
	return []settingsclient.SkillFileInput{{Path: clean, Content: input.Content}}, "markdown", nil
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

// containsSkillMarkdown 判断 Skill 包根目录是否包含入口文件。
func containsSkillMarkdown(files []settingsclient.SkillFileInput) bool {
	for _, f := range files {
		if strings.EqualFold(filepath.ToSlash(f.Path), "SKILL.md") {
			return true
		}
	}
	return false
}

// writeSkillFiles 将 Skill 包写入受控存储根目录。
func (s *settingsServiceImpl) writeSkillFiles(ownerID, agentID int64, scope, name string, files []settingsclient.SkillFileInput) (string, error) {
	root := strings.TrimSpace(s.skillStorageRoot)
	if root == "" {
		root = "storage/agent/skills"
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	slug := safeSkillSlug(name)
	if slug == "" {
		slug = "skill"
	}
	var target string
	if scope == model.SkillScopeAgent {
		target = filepath.Join(absRoot, "agents", fmt.Sprintf("%d", agentID), slug)
	} else {
		target = filepath.Join(absRoot, "global", fmt.Sprintf("%d", ownerID), slug)
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

// defaultString 去除空白后在 value 为空时返回 fallback。
func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
