package component

import (
	"context"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
)

// NewChatModel 创建兼容 OpenAI 协议的 Eino ChatModel。
//
// baseURL 会去掉末尾斜杠，避免 Eino/OpenAI 客户端拼接接口路径时出现双斜杠。
// 该函数同时支持官方 OpenAI 地址和本地/第三方 OpenAI-compatible 网关。
func NewChatModel(ctx context.Context, apiKey, baseURL, modelName string) (*openai.ChatModel, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	modelConfig := &openai.ChatModelConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
	}

	chatModel, err := openai.NewChatModel(ctx, modelConfig)
	if err != nil {
		return nil, err
	}

	return chatModel, nil
}
