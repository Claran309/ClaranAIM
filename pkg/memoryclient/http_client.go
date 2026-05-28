package memoryclient

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

// CreateMemory 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) CreateMemory(ctx context.Context, input CreateMemoryInput) (*MemoryFact, error) {
	var out struct {
		Success bool        `json:"success"`
		Memory  *MemoryFact `json:"memory"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/memory/create", input, &out)
	return out.Memory, err
}

// ListMemories 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) ListMemories(ctx context.Context, viewerID int64, filter Filter) ([]MemoryFact, int64, error) {
	var out struct {
		Success  bool         `json:"success"`
		Memories []MemoryFact `json:"memories"`
		Total    int64        `json:"total"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/memory/list", map[string]interface{}{"viewer_id": viewerID, "filter": filter}, &out)
	return out.Memories, out.Total, err
}

// Recall 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) Recall(ctx context.Context, input RecallInput) (RecallResult, error) {
	var out struct {
		Success bool         `json:"success"`
		Result  RecallResult `json:"result"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/memory/recall", input, &out)
	return out.Result, err
}

// UpdateMemory 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) UpdateMemory(ctx context.Context, viewerID, memoryID int64, input UpdateMemoryInput) (*MemoryFact, error) {
	var out struct {
		Success bool        `json:"success"`
		Memory  *MemoryFact `json:"memory"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/memory/update", map[string]interface{}{"viewer_id": viewerID, "memory_id": memoryID, "input": input}, &out)
	return out.Memory, err
}

// DeleteMemory 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) DeleteMemory(ctx context.Context, viewerID, memoryID int64) error {
	return servicehttp.Post(ctx, c.client, c.baseURL, "/internal/memory/delete", map[string]interface{}{"viewer_id": viewerID, "memory_id": memoryID}, nil)
}
