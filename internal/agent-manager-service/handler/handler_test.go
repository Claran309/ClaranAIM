package handler

import (
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/pkg/config"
	"testing"
)

func TestBotConfigForDisplayUsesLatestDefaultModelForInternalAgent(t *testing.T) {
	h := &agentServiceImpl{cfg: &config.Config{LLM: config.LLMConfig{
		DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4",
		DefaultModel:   "glm-4.6v",
	}}}
	info := h.botConfigForDisplay(&model.Bot{
		ID:        1,
		Name:      "assistant",
		Type:      "internal",
		ModelName: "glm-4.7",
		BaseURL:   "https://open.bigmodel.cn/api/paas/v4/",
		IsActive:  true,
	})
	if info.ModelName != "glm-4.6v" {
		t.Fatalf("display model = %q, want latest default model", info.ModelName)
	}
	if info.BaseUrl != "https://open.bigmodel.cn/api/paas/v4" {
		t.Fatalf("display base url = %q, want latest default base url", info.BaseUrl)
	}
}

func TestBotConfigForDisplayKeepsCustomEndpointModel(t *testing.T) {
	h := &agentServiceImpl{cfg: &config.Config{LLM: config.LLMConfig{
		DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4",
		DefaultModel:   "glm-4.6v",
	}}}
	info := h.botConfigForDisplay(&model.Bot{
		ID:        1,
		Name:      "assistant",
		Type:      "internal",
		ModelName: "custom-model",
		BaseURL:   "https://custom.example/v1",
		IsActive:  true,
	})
	if info.ModelName != "custom-model" {
		t.Fatalf("display model = %q, want custom model preserved", info.ModelName)
	}
	if info.BaseUrl != "https://open.bigmodel.cn/api/paas/v4" {
		t.Fatalf("display base url = %q, want latest default base url for internal agent", info.BaseUrl)
	}
}
