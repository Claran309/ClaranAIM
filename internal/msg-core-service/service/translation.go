package service

import (
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// TranslateMessageInput 描述一次用户手动触发的消息翻译请求。
type TranslateMessageInput struct {
	MessageID      int64
	UserID         int64
	TargetLanguage string
	Force          bool
}

// TranslateMessageResult 是返回给 api-gateway 和前端的翻译结果。
type TranslateMessageResult struct {
	MessageID      int64  `json:"message_id"`
	TargetLanguage string `json:"target_language"`
	TranslatedText string `json:"translated_text"`
	Cached         bool   `json:"cached"`
	ModelName      string `json:"model_name"`
}

// TranslationSettings 解析用户或系统级翻译 LLM 与 prompt 配置。
type TranslationSettings interface {
	ResolveTranslationConfig(ctx context.Context, ownerID int64) (settingsclient.ResolvedLLMConfig, error)
}

// TranslationLLM 使用解析出的 LLM 配置执行翻译请求。
type TranslationLLM interface {
	Translate(ctx context.Context, cfg settingsclient.ResolvedLLMConfig, prompt string) (string, error)
}

// SetTranslationDependencies 将 settings-service 和 LLM 执行器注入 msg-core-service。
func (s *messageServiceImpl) SetTranslationDependencies(settings TranslationSettings, llm TranslationLLM) {
	s.translationSettings = settings
	s.translationLLM = llm
}

// TranslateMessage 手动翻译一条当前用户可见的文本消息，并按源文本 hash 缓存结果。
func (s *messageServiceImpl) TranslateMessage(ctx context.Context, input TranslateMessageInput) (TranslateMessageResult, error) {
	if input.MessageID <= 0 || input.UserID <= 0 {
		return TranslateMessageResult{}, errors.New("message_id和user_id不能为空")
	}
	targetLanguage := strings.TrimSpace(input.TargetLanguage)
	if targetLanguage == "" {
		targetLanguage = "中文"
	}
	msg, err := s.repo.GetMessageByID(ctx, input.MessageID)
	if err != nil {
		return TranslateMessageResult{}, err
	}
	if msg == nil {
		return TranslateMessageResult{}, errors.New("消息不存在")
	}
	if msg.Status == MessageStatusRecalled {
		return TranslateMessageResult{}, errors.New("已撤回消息不能翻译")
	}
	if strings.TrimSpace(msg.Content) == "" || msg.MsgType != "text" {
		return TranslateMessageResult{}, errors.New("当前只支持翻译文本消息")
	}
	conv, err := s.repo.GetConversationByID(ctx, msg.ConversationID)
	if err != nil {
		return TranslateMessageResult{}, err
	}
	if conv == nil {
		return TranslateMessageResult{}, errors.New("会话不存在")
	}
	if err := s.ensureConversationParticipant(ctx, conv, input.UserID); err != nil {
		return TranslateMessageResult{}, err
	}
	sourceHash := hashTranslationSource(msg.Content)
	if !input.Force {
		cached, err := s.repo.GetTranslation(ctx, msg.ID, input.UserID, targetLanguage, sourceHash)
		if err != nil {
			return TranslateMessageResult{}, err
		}
		if cached != nil {
			return TranslateMessageResult{MessageID: msg.ID, TargetLanguage: targetLanguage, TranslatedText: cached.TranslatedText, Cached: true, ModelName: cached.ModelName}, nil
		}
	}
	if s.translationSettings == nil || s.translationLLM == nil {
		return TranslateMessageResult{}, errors.New("消息翻译服务未配置")
	}
	cfg, err := s.translationSettings.ResolveTranslationConfig(ctx, input.UserID)
	if err != nil {
		return TranslateMessageResult{}, err
	}
	prompt := renderTranslationPrompt(cfg.PromptTemplate, targetLanguage, msg.Content)
	translated, err := s.translationLLM.Translate(ctx, cfg, prompt)
	if err != nil {
		return TranslateMessageResult{}, describeTranslationLLMError(cfg, err)
	}
	translated = strings.TrimSpace(translated)
	if translated == "" {
		return TranslateMessageResult{}, errors.New("翻译结果为空")
	}
	record := &model.MessageTranslation{
		MessageID:      msg.ID,
		UserID:         input.UserID,
		SourceTextHash: sourceHash,
		TargetLanguage: targetLanguage,
		TranslatedText: translated,
		Provider:       cfg.ProviderType,
		ModelName:      cfg.ModelName,
		PromptVersion:  "user-current",
	}
	_ = s.repo.SaveTranslation(ctx, record)
	return TranslateMessageResult{MessageID: msg.ID, TargetLanguage: targetLanguage, TranslatedText: translated, Cached: false, ModelName: cfg.ModelName}, nil
}

func describeTranslationLLMError(cfg settingsclient.ResolvedLLMConfig, err error) error {
	if err == nil {
		return nil
	}
	raw := strings.TrimSpace(err.Error())
	lower := strings.ToLower(raw)
	provider := defaultTranslationErrorField(cfg.ProviderType, "unknown")
	model := defaultTranslationErrorField(cfg.ModelName, "unknown")
	baseURL := defaultTranslationErrorField(cfg.BaseURL, "未配置")
	prefix := "第三方翻译模型调用失败"
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") || strings.Contains(raw, "速率限制") || strings.Contains(raw, "达到速率") || strings.Contains(raw, "429") {
		prefix = "第三方翻译模型限流"
	}
	return fmt.Errorf("%s：provider=%s，model=%s，base_url=%s，错误=%s", prefix, provider, model, baseURL, raw)
}

func defaultTranslationErrorField(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

// renderTranslationPrompt 将用户配置的模板渲染为实际翻译 prompt。
// 模板未包含 {{text}} 时会把原文追加在末尾，避免用户模板写漏占位符导致模型拿不到待翻译内容。
func renderTranslationPrompt(template, targetLanguage, text string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = "请将下面内容翻译成{{target_language}}。只输出译文。\n\n{{text}}"
	}
	replacer := strings.NewReplacer(
		"{{target_language}}", targetLanguage,
		"{{text}}", text,
	)
	rendered := replacer.Replace(template)
	if !strings.Contains(template, "{{text}}") {
		rendered += "\n\n待翻译内容：\n" + text
	}
	return rendered
}

// hashTranslationSource 计算源文本 hash，用于判断同一消息内容的翻译缓存是否仍然有效。
func hashTranslationSource(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
