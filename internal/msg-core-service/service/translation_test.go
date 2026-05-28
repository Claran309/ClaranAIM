package service

import (
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"strings"
	"testing"
)

func TestTranslateMessageUsesSettingsAndCachesPerUserLanguage(t *testing.T) {
	repo := newFakeMessageRepo()
	conv := &model.Conversation{ID: 10, Type: "private"}
	msg := &model.Message{ID: 99, ConversationID: 10, SenderID: 2002, Content: "hello world", MsgType: "text", Status: MessageStatusSent}
	repo.conversations[10] = conv
	repo.messages[10] = []model.Message{*msg}
	repo.participants[10] = map[int64]model.ConversationParticipant{1001: {ConversationID: 10, UserID: 1001}, 2002: {ConversationID: 10, UserID: 2002}}
	settings := &fakeTranslationSettings{resolved: settingsclient.ResolvedLLMConfig{APIKey: "key", BaseURL: "https://llm.example/v1", ModelName: "glm-4.7", PromptTemplate: "翻译成{{target_language}}：{{text}}"}}
	llm := &fakeTranslatorLLM{reply: "你好，世界"}
	svc := NewMessageServiceForTest(repo)
	svc.SetTranslationDependencies(settings, llm)

	result, err := svc.TranslateMessage(context.Background(), TranslateMessageInput{MessageID: 99, UserID: 1001, TargetLanguage: "中文"})
	if err != nil {
		t.Fatalf("TranslateMessage returned error: %v", err)
	}
	if result.TranslatedText != "你好，世界" {
		t.Fatalf("translated = %q", result.TranslatedText)
	}
	if !strings.Contains(llm.lastPrompt, "hello world") || !strings.Contains(llm.lastPrompt, "中文") {
		t.Fatalf("prompt = %q, want text and target language", llm.lastPrompt)
	}
	if len(repo.translations) != 1 {
		t.Fatalf("translations stored = %d, want 1", len(repo.translations))
	}

	second, err := svc.TranslateMessage(context.Background(), TranslateMessageInput{MessageID: 99, UserID: 1001, TargetLanguage: "中文"})
	if err != nil {
		t.Fatalf("second TranslateMessage returned error: %v", err)
	}
	if second.TranslatedText != "你好，世界" || llm.calls != 1 {
		t.Fatalf("cache miss: second=%#v calls=%d", second, llm.calls)
	}
}

func TestTranslateMessageRejectsInvisibleMessage(t *testing.T) {
	repo := newFakeMessageRepo()
	repo.conversations[10] = &model.Conversation{ID: 10, Type: "private"}
	repo.messages[10] = []model.Message{{ID: 99, ConversationID: 10, SenderID: 2002, Content: "hello", MsgType: "text", Status: MessageStatusSent}}
	repo.participants[10] = map[int64]model.ConversationParticipant{2002: {ConversationID: 10, UserID: 2002}}
	svc := NewMessageServiceForTest(repo)
	svc.SetTranslationDependencies(&fakeTranslationSettings{resolved: settingsclient.ResolvedLLMConfig{APIKey: "key", BaseURL: "https://llm.example/v1", ModelName: "m", PromptTemplate: "{{text}}"}}, &fakeTranslatorLLM{reply: "x"})

	_, err := svc.TranslateMessage(context.Background(), TranslateMessageInput{MessageID: 99, UserID: 1001, TargetLanguage: "中文"})
	if err == nil {
		t.Fatal("expected invisible message translation to fail")
	}
}

type fakeTranslationSettings struct {
	resolved settingsclient.ResolvedLLMConfig
}

func (s *fakeTranslationSettings) ResolveTranslationConfig(ctx context.Context, ownerID int64) (settingsclient.ResolvedLLMConfig, error) {
	return s.resolved, nil
}

type fakeTranslatorLLM struct {
	reply      string
	calls      int
	lastPrompt string
}

func (l *fakeTranslatorLLM) Translate(ctx context.Context, cfg settingsclient.ResolvedLLMConfig, prompt string) (string, error) {
	l.calls++
	l.lastPrompt = prompt
	return l.reply, nil
}
