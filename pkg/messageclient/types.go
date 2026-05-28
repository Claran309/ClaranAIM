package messageclient

import "context"

// TranslateMessageInput 是跨服务调用消息翻译功能的入参。
type TranslateMessageInput struct {
	MessageID      int64  `json:"message_id"`
	UserID         int64  `json:"user_id"`
	TargetLanguage string `json:"target_language"`
	Force          bool   `json:"force"`
}

// TranslateMessageResult 是跨服务返回的消息翻译结果。
type TranslateMessageResult struct {
	MessageID      int64  `json:"message_id"`
	TargetLanguage string `json:"target_language"`
	TranslatedText string `json:"translated_text"`
	Cached         bool   `json:"cached"`
	ModelName      string `json:"model_name"`
}

// TranslationService 是 msg-core-service 翻译能力暴露给其他服务的最小客户端契约。
type TranslationService interface {
	TranslateMessage(ctx context.Context, input TranslateMessageInput) (TranslateMessageResult, error)
}
