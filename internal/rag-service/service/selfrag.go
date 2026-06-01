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

type SelfRAGJudgeInput struct {
	Query  string
	Answer string
	Chunks []rankedChunk
	CRAG   CRAGEvaluation
	Route  string
}

type SelfRAGJudgement struct {
	IsRel  bool
	IsSup  bool
	IsUse  bool
	Reason string
}

type SelfRAGJudge interface {
	Judge(ctx context.Context, input SelfRAGJudgeInput) (SelfRAGJudgement, error)
}

type RuleSelfRAGJudge struct{}

func (RuleSelfRAGJudge) Judge(ctx context.Context, input SelfRAGJudgeInput) (SelfRAGJudgement, error) {
	_ = ctx
	hasSources := len(input.Chunks) > 0
	return SelfRAGJudgement{
		IsRel:  input.CRAG.Relevance >= 0.50,
		IsSup:  input.CRAG.Label == CRAGLabelCorrect || input.CRAG.Label == CRAGLabelAmbiguous,
		IsUse:  hasSources && strings.TrimSpace(input.Answer) != "",
		Reason: "规则Self-RAG评估",
	}, nil
}

type LLMSelfRAGJudge struct {
	APIKey   string
	BaseURL  string
	Model    string
	Fallback SelfRAGJudge
	Client   *http.Client
}

func NewLLMSelfRAGJudge(apiKey, baseURL, model string, fallback SelfRAGJudge) *LLMSelfRAGJudge {
	return &LLMSelfRAGJudge{
		APIKey:   strings.TrimSpace(apiKey),
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:    defaultString(model, "glm-4-flash"),
		Fallback: fallback,
		Client:   &http.Client{Timeout: 12 * time.Second},
	}
}

func (j *LLMSelfRAGJudge) Judge(ctx context.Context, input SelfRAGJudgeInput) (SelfRAGJudgement, error) {
	if j == nil || j.APIKey == "" || j.BaseURL == "" {
		return fallbackSelfRAGJudge(ctx, j, input, errors.New("self-rag judge未配置"))
	}
	payload := map[string]interface{}{
		"model": j.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是Self-RAG judge。只输出JSON，不要解释。你不执行工具，不决定权限，只判断已有回答和来源。字段: is_rel(boolean), is_sup(boolean), is_use(boolean), reason(string)。is_rel表示检索资料是否相关；is_sup表示答案是否被资料支撑；is_use表示答案是否对用户有用。",
			},
			{
				"role":    "user",
				"content": buildSelfRAGJudgePrompt(input),
			},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fallbackSelfRAGJudge(ctx, j, input, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fallbackSelfRAGJudge(ctx, j, input, err)
	}
	req.Header.Set("Authorization", "Bearer "+j.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := j.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fallbackSelfRAGJudge(ctx, j, input, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fallbackSelfRAGJudge(ctx, j, input, fmt.Errorf("self-rag judge状态码%d", resp.StatusCode))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return fallbackSelfRAGJudge(ctx, j, input, err)
	}
	if len(decoded.Choices) == 0 {
		return fallbackSelfRAGJudge(ctx, j, input, errors.New("self-rag judge未返回结果"))
	}
	judgement, err := parseSelfRAGJudgement(decoded.Choices[0].Message.Content)
	if err != nil {
		return fallbackSelfRAGJudge(ctx, j, input, err)
	}
	return judgement, nil
}

func fallbackSelfRAGJudge(ctx context.Context, judge *LLMSelfRAGJudge, input SelfRAGJudgeInput, cause error) (SelfRAGJudgement, error) {
	if judge != nil && judge.Fallback != nil {
		result, err := judge.Fallback.Judge(ctx, input)
		if err == nil {
			result.Reason = strings.TrimSpace(result.Reason + "；LLM Self-RAG降级: " + cause.Error())
		}
		return result, err
	}
	return SelfRAGJudgement{}, cause
}

func buildSelfRAGJudgePrompt(input SelfRAGJudgeInput) string {
	var b strings.Builder
	b.WriteString("用户问题:\n")
	b.WriteString(input.Query)
	b.WriteString("\n\n回答:\n")
	b.WriteString(input.Answer)
	b.WriteString("\n\n来源:\n")
	for i, chunk := range input.Chunks {
		if i >= 6 {
			break
		}
		b.WriteString(fmt.Sprintf("[%d] %s\n%s\n\n", i+1, chunk.Document.Title, truncate(chunk.Chunk.Content, 700)))
	}
	b.WriteString("\nCRAG:\n")
	b.WriteString(cragNote(input.CRAG))
	return b.String()
}

func parseSelfRAGJudgement(content string) (SelfRAGJudgement, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		IsRel  bool   `json:"is_rel"`
		IsSup  bool   `json:"is_sup"`
		IsUse  bool   `json:"is_use"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return SelfRAGJudgement{}, err
	}
	return SelfRAGJudgement{IsRel: parsed.IsRel, IsSup: parsed.IsSup, IsUse: parsed.IsUse, Reason: strings.TrimSpace(parsed.Reason)}, nil
}

func selfRAGNote(judgement SelfRAGJudgement) string {
	return fmt.Sprintf("Self-RAG Judge: IsRel=%t IsSup=%t IsUse=%t reason=%s", judgement.IsRel, judgement.IsSup, judgement.IsUse, judgement.Reason)
}
