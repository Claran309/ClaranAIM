// Package dao 负责 settings-service 的持久化访问。
package dao

import (
	"ClaranAIM/internal/settings-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 打开 MySQL，并对设置表执行非破坏性迁移。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.LLMProfile{}, &model.PromptTemplate{}, &model.AgentSkill{}, &model.MCPServerConfig{}); err != nil {
		return nil, err
	}
	return db, nil
}

// LLMProfileFilter 限定 LLM profile 查询条件。
type LLMProfileFilter struct {
	Scope     string
	OwnerID   int64
	UsageType string
	Enabled   *bool
}

// PromptFilter 限定 prompt 模板查询条件。
type PromptFilter struct {
	Scope   string
	OwnerID int64
	Type    string
	Enabled *bool
}

// SkillFilter 限定 Agent Skill 查询条件。
type SkillFilter struct {
	OwnerID int64
	Scope   string
	AgentID int64
	Enabled *bool
}

// MCPServerFilter 限定外部 MCP Server 配置查询条件。
type MCPServerFilter struct {
	OwnerID        int64
	Scope          string
	AgentID        int64
	ConversationID int64
	Enabled        *bool
}

// SettingsRepository 定义 settings-service 使用的存储操作。
type SettingsRepository interface {
	SaveLLMProfile(ctx context.Context, profile *model.LLMProfile) error
	GetLLMProfile(ctx context.Context, id int64) (*model.LLMProfile, error)
	GetLLMProfileByName(ctx context.Context, scope string, ownerID int64, name string) (*model.LLMProfile, error)
	ListLLMProfiles(ctx context.Context, filter LLMProfileFilter) ([]model.LLMProfile, error)
	DeleteLLMProfile(ctx context.Context, id int64) error
	SavePrompt(ctx context.Context, prompt *model.PromptTemplate) error
	GetPromptByType(ctx context.Context, scope string, ownerID int64, promptType string) (*model.PromptTemplate, error)
	ListPrompts(ctx context.Context, filter PromptFilter) ([]model.PromptTemplate, error)
	SaveSkill(ctx context.Context, skill *model.AgentSkill) error
	GetSkill(ctx context.Context, id int64) (*model.AgentSkill, error)
	ListSkills(ctx context.Context, filter SkillFilter) ([]model.AgentSkill, error)
	DeleteSkill(ctx context.Context, id int64) error
	SaveMCPServer(ctx context.Context, server *model.MCPServerConfig) error
	GetMCPServer(ctx context.Context, id int64) (*model.MCPServerConfig, error)
	ListMCPServers(ctx context.Context, filter MCPServerFilter) ([]model.MCPServerConfig, error)
	DeleteMCPServer(ctx context.Context, id int64) error
}

// settingsRepositoryImpl 是基于 GORM 的系统设置仓储实现。
type settingsRepositoryImpl struct {
	db *gorm.DB
}

// NewSettingsRepo 创建基于 GORM 的设置仓储。
func NewSettingsRepo(db *gorm.DB) SettingsRepository {
	return &settingsRepositoryImpl{db: db}
}

// SaveLLMProfile 使用 GORM Save 实现创建或覆盖更新。
// service 层会先做所有权和 API Key 行为校验，dao 层只负责持久化。
func (r *settingsRepositoryImpl) SaveLLMProfile(ctx context.Context, profile *model.LLMProfile) error {
	return r.db.WithContext(ctx).Save(profile).Error
}

// GetLLMProfile 按主键读取 LLM 预设，不存在时返回 nil 而不是 gorm.ErrRecordNotFound。
func (r *settingsRepositoryImpl) GetLLMProfile(ctx context.Context, id int64) (*model.LLMProfile, error) {
	var profile model.LLMProfile
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &profile, err
}

// GetLLMProfileByName 用于“同一用户同名预设覆盖更新”的查重逻辑。
func (r *settingsRepositoryImpl) GetLLMProfileByName(ctx context.Context, scope string, ownerID int64, name string) (*model.LLMProfile, error) {
	var profile model.LLMProfile
	err := r.db.WithContext(ctx).Where("scope = ? AND owner_id = ? AND name = ?", scope, ownerID, name).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &profile, err
}

// ListLLMProfiles 根据 filter 逐项拼接查询条件。
// 结果优先返回默认预设，再按最近更新时间排列，方便前端下拉框直接展示。
func (r *settingsRepositoryImpl) ListLLMProfiles(ctx context.Context, filter LLMProfileFilter) ([]model.LLMProfile, error) {
	query := r.db.WithContext(ctx).Model(&model.LLMProfile{})
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.OwnerID >= 0 {
		query = query.Where("owner_id = ?", filter.OwnerID)
	}
	if filter.UsageType != "" {
		query = query.Where("usage_type = ?", filter.UsageType)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	var profiles []model.LLMProfile
	err := query.Order("is_default DESC, updated_at DESC, id DESC").Find(&profiles).Error
	return profiles, err
}

// DeleteLLMProfile 按主键删除 LLM 预设；所有权判断在 service 层完成。
func (r *settingsRepositoryImpl) DeleteLLMProfile(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.LLMProfile{}, id).Error
}

// SavePrompt 创建或覆盖更新 Prompt 模板。
func (r *settingsRepositoryImpl) SavePrompt(ctx context.Context, prompt *model.PromptTemplate) error {
	return r.db.WithContext(ctx).Save(prompt).Error
}

// GetPromptByType 按 scope、owner 和类型读取唯一 Prompt，用于保存前覆盖更新。
func (r *settingsRepositoryImpl) GetPromptByType(ctx context.Context, scope string, ownerID int64, promptType string) (*model.PromptTemplate, error) {
	var prompt model.PromptTemplate
	err := r.db.WithContext(ctx).Where("scope = ? AND owner_id = ? AND type = ?", scope, ownerID, promptType).First(&prompt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &prompt, err
}

// ListPrompts 查询当前用户可用 Prompt，默认模板排在前面。
func (r *settingsRepositoryImpl) ListPrompts(ctx context.Context, filter PromptFilter) ([]model.PromptTemplate, error) {
	query := r.db.WithContext(ctx).Model(&model.PromptTemplate{})
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.OwnerID >= 0 {
		query = query.Where("owner_id = ?", filter.OwnerID)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	var prompts []model.PromptTemplate
	err := query.Order("is_default DESC, updated_at DESC, id DESC").Find(&prompts).Error
	return prompts, err
}

// SaveSkill 保存 Agent Skill 元数据。
func (r *settingsRepositoryImpl) SaveSkill(ctx context.Context, skill *model.AgentSkill) error {
	return r.db.WithContext(ctx).Save(skill).Error
}

// GetSkill 按 ID 读取 Agent Skill 元数据，不存在时返回 nil。
func (r *settingsRepositoryImpl) GetSkill(ctx context.Context, id int64) (*model.AgentSkill, error) {
	var skill model.AgentSkill
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &skill, err
}

// ListSkills 按用户、作用域、Agent 和启用状态查询 Agent Skill。
func (r *settingsRepositoryImpl) ListSkills(ctx context.Context, filter SkillFilter) ([]model.AgentSkill, error) {
	query := r.db.WithContext(ctx).Model(&model.AgentSkill{})
	if filter.OwnerID >= 0 {
		query = query.Where("owner_id = ?", filter.OwnerID)
	}
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.AgentID >= 0 {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	var skills []model.AgentSkill
	err := query.Order("is_default DESC, updated_at DESC, id DESC").Find(&skills).Error
	return skills, err
}

// DeleteSkill 删除一条 Agent Skill 元数据。
func (r *settingsRepositoryImpl) DeleteSkill(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.AgentSkill{}, id).Error
}

// SaveMCPServer 保存一份外部 MCP Server 配置。
func (r *settingsRepositoryImpl) SaveMCPServer(ctx context.Context, server *model.MCPServerConfig) error {
	return r.db.WithContext(ctx).Save(server).Error
}

// GetMCPServer 按 ID 读取 MCP 配置，不存在时返回 nil。
func (r *settingsRepositoryImpl) GetMCPServer(ctx context.Context, id int64) (*model.MCPServerConfig, error) {
	var server model.MCPServerConfig
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&server).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &server, err
}

// ListMCPServers 按 owner、scope、Agent、会话和启用状态查询 MCP 配置。
func (r *settingsRepositoryImpl) ListMCPServers(ctx context.Context, filter MCPServerFilter) ([]model.MCPServerConfig, error) {
	query := r.db.WithContext(ctx).Model(&model.MCPServerConfig{})
	if filter.OwnerID >= 0 {
		query = query.Where("owner_id = ?", filter.OwnerID)
	}
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.AgentID >= 0 {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.ConversationID >= 0 {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	var servers []model.MCPServerConfig
	err := query.Order("updated_at DESC, id DESC").Find(&servers).Error
	return servers, err
}

// DeleteMCPServer 删除一条 MCP 配置。
func (r *settingsRepositoryImpl) DeleteMCPServer(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.MCPServerConfig{}, id).Error
}
