package service

import (
	"ClaranAIM/pkg/settingsclient"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleTranslator 调用 OpenAI 兼容供应商的 /chat/completions 接口。
type OpenAICompatibleTranslator struct {
	client *http.Client
}

// NewOpenAICompatibleTranslator 创建翻译用 LLM 客户端。
func NewOpenAICompatibleTranslator() *OpenAICompatibleTranslator {
	return &OpenAICompatibleTranslator{client: &http.Client{Timeout: 60 * time.Second}}
}

// Translate 发送单次翻译 prompt，并返回模型回复文本。
func (t *OpenAICompatibleTranslator) Translate(ctx context.Context, cfg settingsclient.ResolvedLLMConfig, prompt string) (string, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" || cfg.APIKey == "" || cfg.ModelName == "" {
		return "", errors.New("翻译LLM配置不完整")
	}
	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	body := map[string]interface{}{
		"model": cfg.ModelName,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业翻译引擎。严格遵循用户的翻译要求，只输出译文。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error.Message != "" {
			return "", errors.New(decoded.Error.Message)
		}
		return "", fmt.Errorf("翻译LLM请求失败: HTTP %d", resp.StatusCode)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", errors.New("翻译LLM未返回内容")
	}
	return decoded.Choices[0].Message.Content, nil
}
