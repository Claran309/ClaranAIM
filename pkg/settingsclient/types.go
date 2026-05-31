package settingsclient

import "context"

// 这些取值是 settings-service 与调用方共享的协议值。
// 其中 APIKeyAction* 不能随意改名，前端和网关依赖它区分“保留原密钥、覆盖密钥、清空密钥”三种敏感操作。
const (
	ProviderTranslate      = "translation"
	DefaultTranslatePrompt = "请将下面内容翻译成中文。只输出译文，保留代码、链接、数字、专有名词和 Markdown 结构。"

	SkillScopeGlobal = "global"
	SkillScopeAgent  = "agent"

	APIKeyActionKeep  = "keep"
	APIKeyActionSet   = "set"
	APIKeyActionClear = "clear"
)

// LLMProfile 是跨服务传输的 LLM 供应商配置 DTO。
// API Key 不会放在该结构中返回给浏览器，只通过 HasAPIKey 告知前端是否已配置。
type LLMProfile struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	BaseURL      string `json:"base_url"`
	ModelName    string `json:"model_name"`
	UsageType    string `json:"usage_type"`
	IsDefault    bool   `json:"is_default"`
	Enabled      bool   `json:"enabled"`
	HasAPIKey    bool   `json:"has_api_key"`
}

// PromptTemplate 是用户可配置 prompt 模板的跨服务 DTO。
type PromptTemplate struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	IsDefault bool   `json:"is_default"`
	Enabled   bool   `json:"enabled"`
}

// AgentSkill 是 settings-service 保存的 Skill 包元数据。
// SkillsDir 是后端校验后的本地目录，可作为 Agent 的 skills_dir 注入 runtime。
type AgentSkill struct {
	ID          int64  `json:"id"`
	OwnerID     int64  `json:"owner_id"`
	AgentID     int64  `json:"agent_id"`
	Scope       string `json:"scope"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SkillsDir   string `json:"skills_dir"`
	EntryFile   string `json:"entry_file"`
	SourceType  string `json:"source_type"`
	IsDefault   bool   `json:"is_default"`
	Enabled     bool   `json:"enabled"`
}

// SaveLLMProfileInput 表示保存 LLM profile 的入参。
// APIKeyAction 用于区分保留、设置、清空密钥，避免空字符串语义不清。
type SaveLLMProfileInput struct {
	ID           int64
	Name         string
	ProviderType string
	BaseURL      string
	APIKey       string
	ModelName    string
	UsageType    string
	IsDefault    bool
	Enabled      *bool
	APIKeyAction string
}

// SavePromptInput 表示保存 prompt 模板的入参。
type SavePromptInput struct {
	ID        int64
	Type      string
	Name      string
	Content   string
	IsDefault bool
	Enabled   *bool
}

// SaveSkillInput 描述一次 Skill 上传后的保存请求。
// Content 用于单文件 SKILL.md，Files 用于 zip 或文件夹展开后的多文件 Skill 包。
type SaveSkillInput struct {
	ID          int64
	Name        string
	Description string
	Scope       string
	AgentID     int64
	FileName    string
	Content     []byte
	Files       []SkillFileInput
	IsDefault   bool
	Enabled     *bool
}

// SkillFileInput 表示 Skill 包内的一个相对路径文件。
type SkillFileInput struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

// ResolvedLLMConfig 是 settings-service 解析后的可执行 LLM 配置。
// 该结构可能包含 API Key，只能用于服务间调用，不能原样返回给浏览器。
type ResolvedLLMConfig struct {
	ProfileID      int64  `json:"profile_id"`
	APIKey         string `json:"api_key,omitempty"`
	BaseURL        string `json:"base_url"`
	ModelName      string `json:"model_name"`
	ProviderType   string `json:"provider_type"`
	PromptTemplate string `json:"prompt_template"`
}

// Service 是 settings-service 对其他服务暴露的最小客户端契约。
type Service interface {
	SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*LLMProfile, error)
	ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]LLMProfile, error)
	DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error
	SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*PromptTemplate, error)
	ListPrompts(ctx context.Context, ownerID int64) ([]PromptTemplate, error)
	ResolveTranslationConfig(ctx context.Context, ownerID int64) (ResolvedLLMConfig, error)
	ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (ResolvedLLMConfig, error)
	SaveSkill(ctx context.Context, ownerID int64, input SaveSkillInput) (*AgentSkill, error)
	ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]AgentSkill, error)
	DeleteSkill(ctx context.Context, ownerID, skillID int64) error
}
