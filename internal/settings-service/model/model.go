// Package model 定义系统级和用户级设置的持久化模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// 固定作用域和用途值会同时写入数据库、settings-service RPC DTO 和前端筛选项。
// 集中声明可以避免 LLM 预设、Prompt 和 Skill 在不同层使用不一致的字符串。
const (
	ScopeSystem       = "system"
	ScopeUser         = "user"
	ProviderTranslate = "translation"
	ProviderRAGRouter = "rag_router"

	SkillScopeGlobal = "global"
	SkillScopeAgent  = "agent"

	MCPScopeGlobal       = "global"
	MCPScopeUser         = "user"
	MCPScopeAgent        = "agent"
	MCPScopeConversation = "conversation"

	MCPTransportStreamableHTTP = "streamable_http"
	MCPTransportSSE            = "sse"
	MCPTransportStdio          = "stdio"

	MCPTrustLow    = "low"
	MCPTrustNormal = "normal"
	MCPTrustHigh   = "high"

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

// TableName 固定 LLM 预设表名，避免 GORM 复数化策略变化影响迁移。
func (LLMProfile) TableName() string {
	return "llm_profiles"
}

// BeforeCreate 使用项目统一雪花 ID，避免多服务写库时依赖 MySQL 自增序列。
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

// TableName 固定 Prompt 模板表名，便于文档和迁移脚本直接引用。
func (PromptTemplate) TableName() string {
	return "prompt_templates"
}

// BeforeCreate 为 Prompt 模板分配雪花 ID；调用方传入 ID 时保持原值。
func (p *PromptTemplate) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&p.ID)
}

// AgentSkill 记录用户上传并可注入 Agent 运行时的 Eino Skill 包。
//
// Scope=global 表示当前用户创建 Agent 时可复用的全局 Skill；
// Scope=agent 表示只绑定到某个 Agent 的专属 Skill。SkillsDir 指向经过
// settings-service 校验和落盘后的本地目录，runtime 只读取该目录，不直接信任浏览器上传路径。
type AgentSkill struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OwnerID     int64     `json:"owner_id" gorm:"index:idx_agent_skill_owner_scope;not null"`
	AgentID     int64     `json:"agent_id" gorm:"index;not null;default:0"`
	Scope       string    `json:"scope" gorm:"size:32;index:idx_agent_skill_owner_scope;not null"`
	Name        string    `json:"name" gorm:"size:120;not null"`
	Description string    `json:"description" gorm:"size:255"`
	SkillsDir   string    `json:"skills_dir" gorm:"size:500;not null"`
	EntryFile   string    `json:"entry_file" gorm:"size:255;not null;default:SKILL.md"`
	SourceType  string    `json:"source_type" gorm:"size:32;not null;default:markdown"`
	IsDefault   bool      `json:"is_default" gorm:"not null;default:false;index"`
	Enabled     bool      `json:"enabled" gorm:"not null;default:true;index"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回 Agent Skill 元数据表名。
func (AgentSkill) TableName() string {
	return "agent_skills"
}

// BeforeCreate 为 Agent Skill 写入分布式 ID。
func (s *AgentSkill) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&s.ID)
}

// MCPServerConfig 保存用户或 Agent 可接入的外部 MCP Server 配置。
//
// settings-service 只负责持久化和敏感字段脱敏；实际连接、工具发现和调用由
// mcp-gateway-service 执行。Secret 当前以服务间明文字段保存和解析，后续应接入
// KMS/secret-store 后改为 encrypted_secret_ref。
type MCPServerConfig struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OwnerID        int64     `json:"owner_id" gorm:"index:idx_mcp_owner_scope;not null"`
	AgentID        int64     `json:"agent_id" gorm:"index;not null;default:0"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null;default:0"`
	Scope          string    `json:"scope" gorm:"size:32;index:idx_mcp_owner_scope;not null"`
	Name           string    `json:"name" gorm:"size:120;not null"`
	Description    string    `json:"description" gorm:"size:255"`
	Transport      string    `json:"transport" gorm:"size:32;not null;default:streamable_http"`
	EndpointURL    string    `json:"endpoint_url" gorm:"size:500"`
	Command        string    `json:"command" gorm:"size:500"`
	ArgsJSON       string    `json:"args_json" gorm:"type:text"`
	EnvJSON        string    `json:"env_json" gorm:"type:text"`
	HeadersJSON    string    `json:"headers_json" gorm:"type:text"`
	AuthType       string    `json:"auth_type" gorm:"size:32"`
	Secret         string    `json:"-" gorm:"type:text"`
	Enabled        bool      `json:"enabled" gorm:"not null;default:true;index"`
	TrustLevel     string    `json:"trust_level" gorm:"size:32;not null;default:low"`
	AllowToolsJSON string    `json:"allow_tools_json" gorm:"type:text"`
	DenyToolsJSON  string    `json:"deny_tools_json" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回 MCP server 配置表名。
func (MCPServerConfig) TableName() string {
	return "mcp_server_configs"
}

// BeforeCreate 为 MCP 配置分配雪花 ID。
func (c *MCPServerConfig) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&c.ID)
}

// fillSnowflakeID 只在模型创建钩子中使用。
// 如果上游已经设置 ID，说明是在恢复或测试场景，函数会保留该值。
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
