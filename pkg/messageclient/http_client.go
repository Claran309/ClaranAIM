package messageclient

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
	return &HTTPClient{baseURL: baseURL, client: &http.Client{Timeout: 70 * time.Second}}
}

// TranslateMessage 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (c *HTTPClient) TranslateMessage(ctx context.Context, input TranslateMessageInput) (TranslateMessageResult, error) {
	var out struct {
		Success     bool                   `json:"success"`
		Translation TranslateMessageResult `json:"translation"`
	}
	err := servicehttp.Post(ctx, c.client, c.baseURL, "/internal/message/translate", input, &out)
	return out.Translation, err
}
