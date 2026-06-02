package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// LLMArtifactExtractorConfig 配置会话归档使用的小模型提炼器。
type LLMArtifactExtractorConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// LLMArtifactExtractor 调用 OpenAI 兼容 chat completions 接口，将聊天窗口提炼为结构化归档产物。
type LLMArtifactExtractor struct {
	config LLMArtifactExtractorConfig
	client *http.Client
}

// FallbackArtifactExtractor 在主提炼器失败时降级到保底提炼器。
// 这样 LLM 服务抖动只会降低归档质量，不会让会话归档队列整体卡死。
type FallbackArtifactExtractor struct {
	Primary  ArtifactExtractor
	Fallback ArtifactExtractor
}

func NewFallbackArtifactExtractor(primary, fallback ArtifactExtractor) FallbackArtifactExtractor {
	return FallbackArtifactExtractor{Primary: primary, Fallback: fallback}
}

func (e FallbackArtifactExtractor) Extract(ctx context.Context, window MessageWindow) (ArtifactBundle, error) {
	if e.Primary != nil {
		bundle, err := e.Primary.Extract(ctx, window)
		if err == nil {
			return bundle, nil
		}
	}
	if e.Fallback != nil {
		return e.Fallback.Extract(ctx, window)
	}
	return ArtifactBundle{}, errors.New("conversation intelligence extractor未配置")
}

func NewLLMArtifactExtractor(config LLMArtifactExtractorConfig) *LLMArtifactExtractor {
	if config.Timeout <= 0 {
		config.Timeout = 20 * time.Second
	}
	return &LLMArtifactExtractor{config: config, client: &http.Client{Timeout: config.Timeout}}
}

func (e *LLMArtifactExtractor) Extract(ctx context.Context, window MessageWindow) (ArtifactBundle, error) {
	if e == nil || strings.TrimSpace(e.config.BaseURL) == "" || strings.TrimSpace(e.config.Model) == "" {
		return ArtifactBundle{}, errors.New("conversation intelligence LLM未配置")
	}
	reqBody := map[string]interface{}{
		"model": e.config.Model,
		"messages": []map[string]string{
			{"role": "system", "content": conversationDigestSystemPrompt()},
			{"role": "user", "content": buildConversationDigestPrompt(window)},
		},
		"temperature": 0.1,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeChatCompletionsURL(e.config.BaseURL), bytes.NewReader(data))
	if err != nil {
		return ArtifactBundle{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(e.config.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(e.config.APIKey))
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return ArtifactBundle{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ArtifactBundle{}, errors.New("conversation intelligence LLM调用失败")
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ArtifactBundle{}, err
	}
	if len(payload.Choices) == 0 || strings.TrimSpace(payload.Choices[0].Message.Content) == "" {
		return ArtifactBundle{}, errors.New("conversation intelligence LLM未返回内容")
	}
	return parseLLMArtifactBundle(payload.Choices[0].Message.Content, window)
}

func conversationDigestSystemPrompt() string {
	return `你是 ClaranAIM 的会话智能归档器。请只输出合法 JSON，不要输出 Markdown。
目标：从聊天窗口中提炼 conversation_summary、decision、task、topic、quote 和 memory_candidate。
不要把寒暄、短噪声和没有长期价值的消息写入记忆。候选记忆只保存长期有用的用户偏好、目标、项目状态或反复困惑。
字段格式：
{
  "summary": {"summary": "..."},
  "decisions": [{"decision_text": "...", "reason": "...", "decided_by": 0, "source_message_ids": [1], "confidence": 0.8}],
  "tasks": [{"task_title": "...", "assignee": "...", "due_time": "", "status": "todo", "source_message_ids": [1], "confidence": 0.7}],
  "topics": [{"topic": "...", "keywords": ["..."], "content": "...", "source_message_ids": [1], "confidence": 0.7}],
  "quotes": [{"quote": "...", "reason": "...", "source_message_ids": [1], "confidence": 0.7}],
  "memory_candidates": [{"title": "...", "content": "...", "evidence": "...", "source_message_ids": [1], "confidence": 0.7, "importance": 0.5}]
}`
}

func buildConversationDigestPrompt(window MessageWindow) string {
	var b strings.Builder
	b.WriteString("参与者：")
	b.WriteString(encodeIDs(window.Participants))
	b.WriteString("\n消息：\n")
	for _, msg := range window.Messages {
		b.WriteString("[")
		b.WriteString(formatInt(msg.ID))
		b.WriteString("] sender=")
		b.WriteString(formatInt(msg.SenderID))
		b.WriteString(" time=")
		if !msg.CreatedAt.IsZero() {
			b.WriteString(msg.CreatedAt.Format(time.RFC3339))
		}
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(msg.Content))
		b.WriteString("\n")
	}
	return b.String()
}

type llmArtifactJSON struct {
	Summary struct {
		Summary string `json:"summary"`
	} `json:"summary"`
	Decisions []struct {
		DecisionText string  `json:"decision_text"`
		Reason       string  `json:"reason"`
		DecidedBy    int64   `json:"decided_by"`
		SourceMsgIDs []int64 `json:"source_message_ids"`
		Confidence   float64 `json:"confidence"`
	} `json:"decisions"`
	Tasks []struct {
		TaskTitle    string  `json:"task_title"`
		Assignee     string  `json:"assignee"`
		DueTime      string  `json:"due_time"`
		Status       string  `json:"status"`
		SourceMsgIDs []int64 `json:"source_message_ids"`
		Confidence   float64 `json:"confidence"`
	} `json:"tasks"`
	Topics []struct {
		Topic        string   `json:"topic"`
		Keywords     []string `json:"keywords"`
		Content      string   `json:"content"`
		SourceMsgIDs []int64  `json:"source_message_ids"`
		Confidence   float64  `json:"confidence"`
	} `json:"topics"`
	Quotes []struct {
		Quote        string  `json:"quote"`
		Reason       string  `json:"reason"`
		SourceMsgIDs []int64 `json:"source_message_ids"`
		Confidence   float64 `json:"confidence"`
	} `json:"quotes"`
	MemoryCandidates []struct {
		Title        string  `json:"title"`
		Content      string  `json:"content"`
		Evidence     string  `json:"evidence"`
		SourceMsgIDs []int64 `json:"source_message_ids"`
		Confidence   float64 `json:"confidence"`
		Importance   float64 `json:"importance"`
	} `json:"memory_candidates"`
}

func parseLLMArtifactBundle(raw string, window MessageWindow) (ArtifactBundle, error) {
	raw = extractJSONObject(raw)
	var parsed llmArtifactJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ArtifactBundle{}, err
	}
	if len(window.Messages) == 0 {
		return ArtifactBundle{}, nil
	}
	start := window.Messages[0]
	end := window.Messages[len(window.Messages)-1]
	bundle := ArtifactBundle{}
	if strings.TrimSpace(parsed.Summary.Summary) != "" {
		bundle.Summary = &ConversationSummary{
			ConversationID:   start.ConversationID,
			StartMessageID:   start.ID,
			EndMessageID:     end.ID,
			StartTime:        start.CreatedAt,
			EndTime:          end.CreatedAt,
			Summary:          strings.TrimSpace(parsed.Summary.Summary),
			Participants:     window.Participants,
			SourceMsgIDs:     messageIDs(window.Messages),
			ValuableMsgCount: len(window.Messages),
		}
	}
	for _, item := range parsed.Decisions {
		if strings.TrimSpace(item.DecisionText) != "" {
			bundle.Decisions = append(bundle.Decisions, Decision{DecisionText: item.DecisionText, Reason: item.Reason, DecidedBy: item.DecidedBy, SourceMsgIDs: item.SourceMsgIDs, Confidence: normalizedConfidence(item.Confidence)})
		}
	}
	for _, item := range parsed.Tasks {
		if strings.TrimSpace(item.TaskTitle) != "" {
			status := defaultText(item.Status, "todo")
			bundle.Tasks = append(bundle.Tasks, Task{TaskTitle: item.TaskTitle, Assignee: item.Assignee, DueTime: item.DueTime, Status: status, SourceMsgIDs: item.SourceMsgIDs, Confidence: normalizedConfidence(item.Confidence)})
		}
	}
	for _, item := range parsed.Topics {
		if strings.TrimSpace(item.Topic) != "" || strings.TrimSpace(item.Content) != "" {
			bundle.Topics = append(bundle.Topics, TopicChunk{Topic: defaultText(item.Topic, "会话主题"), Keywords: item.Keywords, Content: item.Content, SourceMsgIDs: item.SourceMsgIDs, Confidence: normalizedConfidence(item.Confidence)})
		}
	}
	for _, item := range parsed.Quotes {
		if strings.TrimSpace(item.Quote) != "" {
			bundle.Quotes = append(bundle.Quotes, Quote{Quote: item.Quote, Reason: item.Reason, SourceMsgIDs: item.SourceMsgIDs, Confidence: normalizedConfidence(item.Confidence)})
		}
	}
	for _, item := range parsed.MemoryCandidates {
		if strings.TrimSpace(item.Content) != "" {
			bundle.MemoryCandidates = append(bundle.MemoryCandidates, MemoryCandidate{Title: defaultText(item.Title, "候选记忆"), Content: item.Content, Evidence: item.Evidence, SourceMsgIDs: item.SourceMsgIDs, Confidence: normalizedConfidence(item.Confidence), Importance: normalizedConfidence(item.Importance)})
		}
	}
	return bundle, nil
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

func normalizeChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func normalizedConfidence(v float64) float64 {
	if v <= 0 {
		return 0.7
	}
	if v > 1 {
		return 1
	}
	return v
}

func defaultText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func formatInt(v int64) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(jsonNumber(v), ".0"), "+"))
}

func jsonNumber(v int64) string {
	data, _ := json.Marshal(v)
	return string(data)
}
