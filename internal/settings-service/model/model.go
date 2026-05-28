// Package model 定义系统级和用户级设置的持久化模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// 下面这组常量定义当前包使用的固定取值，集中声明可以避免业务代码中散落魔法字符串或魔法数字。
const (
	ScopeSystem       = "system"
	ScopeUser         = "user"
	ProviderTranslate = "translation"

	DefaultTranslatePrompt = "请将下面内容翻译成中文。只输出译文，保留代码、链接、数字、专有名词和 Markdown 结构。"
)

// LLMProfile 保存一份可复用的 OpenAI 兼容模型供应商配置。
type LLMProfile struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Scope        string    `json:"scope" gorm:"size:32;uniqueIndex:uk_llm_profile_owner_name,priority:1;not null"`
	OwnerID      int64     `json:"owner_id" gorm:"uniqueIndex:uk_llm_profile_owner_name,priority:2;not null;default:0"`
	Name         string    `json:"name" gorm:"size:80;uniqueIndex:uk_llm_profile_owner_name,priority:3;not null"`
	ProviderType string    `json:"provider_type" gorm:"size:40;not null;default:openai_compatible"`
	BaseURL      string    `json:"base_url" gorm:"size:255"`
	APIKey       string    `json:"-" gorm:"type:text"`
	ModelName    string    `json:"model_name" gorm:"size:120"`
	UsageType    string    `json:"usage_type" gorm:"size:64;index"`
	IsDefault    bool      `json:"is_default" gorm:"not null;default:false;index"`
	Enabled      bool      `json:"enabled" gorm:"not null;default:true;index"`
	HasAPIKey    bool      `json:"has_api_key" gorm:"-"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (LLMProfile) TableName() string {
	return "llm_profiles"
}

// BeforeCreate 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (p *LLMProfile) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&p.ID)
}

// PromptTemplate 保存翻译和未来任务使用的用户/系统 prompt。
type PromptTemplate struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Scope     string    `json:"scope" gorm:"size:32;uniqueIndex:uk_prompt_owner_type,priority:1;not null"`
	OwnerID   int64     `json:"owner_id" gorm:"uniqueIndex:uk_prompt_owner_type,priority:2;not null;default:0"`
	Type      string    `json:"type" gorm:"size:64;uniqueIndex:uk_prompt_owner_type,priority:3;not null"`
	Name      string    `json:"name" gorm:"size:80"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	IsDefault bool      `json:"is_default" gorm:"not null;default:false;index"`
	Enabled   bool      `json:"enabled" gorm:"not null;default:true;index"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (PromptTemplate) TableName() string {
	return "prompt_templates"
}

// BeforeCreate 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (p *PromptTemplate) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&p.ID)
}

// fillSnowflakeID 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func fillSnowflakeID(id *int64) error {
	if *id != 0 {
		return nil
	}
	nextID, err := idgen.NextID()
	if err != nil {
		return err
	}
	*id = nextID
	return nil
}
