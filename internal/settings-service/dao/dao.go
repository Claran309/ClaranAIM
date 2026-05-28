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
	if err := db.AutoMigrate(&model.LLMProfile{}, &model.PromptTemplate{}); err != nil {
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
}

// settingsRepositoryImpl 是基于 GORM 的系统设置仓储实现。
type settingsRepositoryImpl struct {
	db *gorm.DB
}

// NewSettingsRepo 创建基于 GORM 的设置仓储。
func NewSettingsRepo(db *gorm.DB) SettingsRepository {
	return &settingsRepositoryImpl{db: db}
}

// SaveLLMProfile 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *settingsRepositoryImpl) SaveLLMProfile(ctx context.Context, profile *model.LLMProfile) error {
	return r.db.WithContext(ctx).Save(profile).Error
}

// GetLLMProfile 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *settingsRepositoryImpl) GetLLMProfile(ctx context.Context, id int64) (*model.LLMProfile, error) {
	var profile model.LLMProfile
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &profile, err
}

// GetLLMProfileByName 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *settingsRepositoryImpl) GetLLMProfileByName(ctx context.Context, scope string, ownerID int64, name string) (*model.LLMProfile, error) {
	var profile model.LLMProfile
	err := r.db.WithContext(ctx).Where("scope = ? AND owner_id = ? AND name = ?", scope, ownerID, name).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &profile, err
}

// ListLLMProfiles 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
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

// DeleteLLMProfile 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *settingsRepositoryImpl) DeleteLLMProfile(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.LLMProfile{}, id).Error
}

// SavePrompt 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *settingsRepositoryImpl) SavePrompt(ctx context.Context, prompt *model.PromptTemplate) error {
	return r.db.WithContext(ctx).Save(prompt).Error
}

// GetPromptByType 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *settingsRepositoryImpl) GetPromptByType(ctx context.Context, scope string, ownerID int64, promptType string) (*model.PromptTemplate, error) {
	var prompt model.PromptTemplate
	err := r.db.WithContext(ctx).Where("scope = ? AND owner_id = ? AND type = ?", scope, ownerID, promptType).First(&prompt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &prompt, err
}

// ListPrompts 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
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
