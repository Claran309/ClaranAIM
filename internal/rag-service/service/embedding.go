package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EmbeddingProvider 抽象文本向量生成能力。
// 生产可使用 GLM embedding-3，测试和离线开发可回退到 hash embedding。
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// GLMEmbeddingProvider 调用智谱 GLM embedding-3 接口。
type GLMEmbeddingProvider struct {
	URL        string
	APIKey     string
	Model      string
	Dimensions int
	Client     *http.Client
}

// NewGLMEmbeddingProvider 创建 GLM embedding provider。
func NewGLMEmbeddingProvider(url, apiKey, model string, dimensions int) *GLMEmbeddingProvider {
	return &GLMEmbeddingProvider{
		URL:        strings.TrimSpace(url),
		APIKey:     strings.TrimSpace(apiKey),
		Model:      defaultString(model, "embedding-3"),
		Dimensions: dimensions,
		Client:     &http.Client{Timeout: 20 * time.Second},
	}
}

// Embed 调用外部 embedding 接口，并返回 float32 向量。
func (p *GLMEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p == nil || p.URL == "" || p.APIKey == "" {
		return nil, errors.New("embedding provider未配置")
	}
	body := map[string]interface{}{
		"model": p.Model,
		"input": text,
	}
	if p.Dimensions > 0 {
		body["dimensions"] = p.Dimensions
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding接口返回状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return nil, errors.New("embedding接口未返回向量")
	}
	vector := make([]float32, len(decoded.Data[0].Embedding))
	for i, value := range decoded.Data[0].Embedding {
		vector[i] = float32(value)
	}
	return vector, nil
}
