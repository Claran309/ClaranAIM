package service

const (
	ProviderTranslate      = "translation"
	ProviderRAGRouter      = "rag_router"
	ProviderAgent          = "agent"
	ProviderGeneral        = "general"
	ProviderEmbedding      = "embedding"
	ProviderOCR            = "ocr"
	ProviderRerank         = "rerank"
	DefaultTranslatePrompt = "请将下面内容翻译成中文。只输出译文，保留代码、链接、数字、专有名词和 Markdown 结构。"

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

	APIKeyActionKeep  = "keep"
	APIKeyActionSet   = "set"
	APIKeyActionClear = "clear"
)

type LLMProfile struct {
	ID           int64
	Name         string
	ProviderType string
	BaseURL      string
	ModelName    string
	UsageType    string
	IsDefault    bool
	Enabled      bool
	HasAPIKey    bool
}

type PromptTemplate struct {
	ID        int64
	Type      string
	Name      string
	Content   string
	IsDefault bool
	Enabled   bool
}

type AgentSkill struct {
	ID          int64
	OwnerID     int64
	AgentID     int64
	Scope       string
	Name        string
	Description string
	SkillsDir   string
	EntryFile   string
	SourceType  string
	IsDefault   bool
	Enabled     bool
	Summary     string
	Content     string
}

type MCPServerConfig struct {
	ID             int64
	OwnerID        int64
	AgentID        int64
	ConversationID int64
	Scope          string
	Name           string
	Description    string
	Transport      string
	EndpointURL    string
	Command        string
	ArgsJSON       string
	EnvJSON        string
	HeadersJSON    string
	AuthType       string
	Secret         string
	Enabled        bool
	TrustLevel     string
	AllowToolsJSON string
	DenyToolsJSON  string
	HasSecret      bool
}

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

type TestLLMProfileInput struct {
	ProfileID    int64
	ProviderType string
	BaseURL      string
	APIKey       string
	ModelName    string
	UsageType    string
	UseBuiltin   bool
}

type TestLLMProfileResult struct {
	OK           bool
	Msg          string
	LatencyMS    int64
	ProviderType string
	ModelName    string
}

type SavePromptInput struct {
	ID        int64
	Type      string
	Name      string
	Content   string
	IsDefault bool
	Enabled   *bool
}

type SaveSkillInput struct {
	ID          int64
	Username    string
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

type SaveMCPServerInput struct {
	ID             int64
	AgentID        int64
	ConversationID int64
	Scope          string
	Name           string
	Description    string
	Transport      string
	EndpointURL    string
	Command        string
	ArgsJSON       string
	EnvJSON        string
	HeadersJSON    string
	AuthType       string
	Secret         string
	SecretAction   string
	Enabled        *bool
	TrustLevel     string
	AllowToolsJSON string
	DenyToolsJSON  string
}

type SkillFileInput struct {
	Path    string
	Content []byte
}

type ResolvedLLMConfig struct {
	ProfileID      int64
	APIKey         string
	BaseURL        string
	ModelName      string
	ProviderType   string
	PromptTemplate string
}
