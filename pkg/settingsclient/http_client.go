package settingsclient

import (
	"ClaranAIM/pkg/servicehttp"
	"context"
	"net/http"
	"time"
)

// HTTPClient 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type HTTPClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPClient 是当前包对外暴露的函数，负责承接对应的业务流程、参数校验或适配逻辑。
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{baseURL: baseURL, client: &http.Client{Timeout: 10 * time.Second}}
}

// SaveLLMProfile 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) SaveLLMProfile(ctx context.Context, ownerID int64, input SaveLLMProfileInput) (*LLMProfile, error) {
	var out struct {
		Success bool        `json:"success"`
		Profile *LLMProfile `json:"profile"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/llm-profiles/save", map[string]interface{}{"user_id": ownerID, "input": input}, &out)
	return out.Profile, err
}

// ListLLMProfiles 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]LLMProfile, error) {
	var out struct {
		Success  bool         `json:"success"`
		Profiles []LLMProfile `json:"profiles"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/llm-profiles/list", map[string]interface{}{"user_id": ownerID, "usage_type": usageType}, &out)
	return out.Profiles, err
}

// DeleteLLMProfile 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error {
	return servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/llm-profiles/delete", map[string]interface{}{"user_id": ownerID, "profile_id": profileID}, nil)
}

// SavePrompt 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) SavePrompt(ctx context.Context, ownerID int64, input SavePromptInput) (*PromptTemplate, error) {
	var out struct {
		Success bool            `json:"success"`
		Prompt  *PromptTemplate `json:"prompt"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/prompts/save", map[string]interface{}{"user_id": ownerID, "input": input}, &out)
	return out.Prompt, err
}

// ListPrompts 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) ListPrompts(ctx context.Context, ownerID int64) ([]PromptTemplate, error) {
	var out struct {
		Success bool             `json:"success"`
		Prompts []PromptTemplate `json:"prompts"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/prompts/list", map[string]interface{}{"user_id": ownerID}, &out)
	return out.Prompts, err
}

// ResolveTranslationConfig 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) ResolveTranslationConfig(ctx context.Context, ownerID int64) (ResolvedLLMConfig, error) {
	var out struct {
		Success bool              `json:"success"`
		Config  ResolvedLLMConfig `json:"config"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/translation/resolve", map[string]interface{}{"user_id": ownerID}, &out)
	return out.Config, err
}

// ResolveLLMProfile 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (ResolvedLLMConfig, error) {
	var out struct {
		Success bool              `json:"success"`
		Config  ResolvedLLMConfig `json:"config"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/settings/llm-profiles/resolve", map[string]interface{}{"user_id": ownerID, "profile_id": profileID}, &out)
	return out.Config, err
}
