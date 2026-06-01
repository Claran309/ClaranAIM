package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	CRAGLabelCorrect   = "correct"
	CRAGLabelIncorrect = "incorrect"
	CRAGLabelAmbiguous = "ambiguous"
)

type CRAGEvaluateInput struct {
	Query  string
	Chunks []rankedChunk
}

type CRAGEvaluation struct {
	Label       string
	Score       float64
	Relevance   float64
	Coverage    float64
	Specificity float64
	Conflict    float64
	Reason      string
}

type CRAGEvaluator interface {
	Evaluate(ctx context.Context, input CRAGEvaluateInput) (CRAGEvaluation, error)
}

type RuleCRAGEvaluator struct{}

func (RuleCRAGEvaluator) Evaluate(ctx context.Context, input CRAGEvaluateInput) (CRAGEvaluation, error) {
	_ = ctx
	quality := retrievalQuality(input.Chunks)
	coverage := math.Min(1, float64(len(input.Chunks))/3)
	specificity := averageSpecificity(input.Chunks)
	conflict := conflictScore(input.Chunks)
	score := clamp01(0.40*quality + 0.25*coverage + 0.25*specificity + 0.10*(1-conflict))
	label := CRAGLabelCorrect
	if score < 0.32 || quality < 0.18 {
		label = CRAGLabelIncorrect
	} else if score < 0.62 || coverage < 0.45 || conflict > 0.55 {
		label = CRAGLabelAmbiguous
	}
	return CRAGEvaluation{
		Label:       label,
		Score:       score,
		Relevance:   clamp01(quality),
		Coverage:    clamp01(coverage),
		Specificity: clamp01(specificity),
		Conflict:    clamp01(conflict),
		Reason:      "规则CRAG评估",
	}, nil
}

type LLMCRAGEvaluator struct {
	APIKey   string
	BaseURL  string
	Model    string
	Fallback CRAGEvaluator
	Client   *http.Client
}

func NewLLMCRAGEvaluator(apiKey, baseURL, model string, fallback CRAGEvaluator) *LLMCRAGEvaluator {
	return &LLMCRAGEvaluator{
		APIKey:   strings.TrimSpace(apiKey),
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:    defaultString(model, "glm-4-flash"),
		Fallback: fallback,
		Client:   &http.Client{Timeout: 12 * time.Second},
	}
}

func (e *LLMCRAGEvaluator) Evaluate(ctx context.Context, input CRAGEvaluateInput) (CRAGEvaluation, error) {
	if e == nil || e.APIKey == "" || e.BaseURL == "" {
		return fallbackCRAGEvaluate(ctx, e, input, errors.New("llm crag evaluator未配置"))
	}
	payload := map[string]interface{}{
		"model": e.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是CRAG evaluator。只输出JSON，不要解释。根据用户问题和检索资料评估四项: relevance, coverage, specificity, conflict，取值0到1。label只能是correct、incorrect、ambiguous。correct表示内部资料足够回答；incorrect表示内部资料明显不相关或不足，需要Web/询问用户兜底；ambiguous表示部分可用但覆盖不足、过泛或存在冲突，应内部+外部合并。JSON字段: label, score, relevance, coverage, specificity, conflict, reason。",
			},
			{
				"role":    "user",
				"content": buildCRAGEvaluatorPrompt(input),
			},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fallbackCRAGEvaluate(ctx, e, input, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fallbackCRAGEvaluate(ctx, e, input, err)
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fallbackCRAGEvaluate(ctx, e, input, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fallbackCRAGEvaluate(ctx, e, input, fmt.Errorf("llm crag evaluator状态码%d", resp.StatusCode))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return fallbackCRAGEvaluate(ctx, e, input, err)
	}
	if len(decoded.Choices) == 0 {
		return fallbackCRAGEvaluate(ctx, e, input, errors.New("llm crag evaluator未返回结果"))
	}
	evaluation, err := parseCRAGEvaluation(decoded.Choices[0].Message.Content)
	if err != nil {
		return fallbackCRAGEvaluate(ctx, e, input, err)
	}
	return evaluation, nil
}

func fallbackCRAGEvaluate(ctx context.Context, evaluator *LLMCRAGEvaluator, input CRAGEvaluateInput, cause error) (CRAGEvaluation, error) {
	if evaluator != nil && evaluator.Fallback != nil {
		evaluation, err := evaluator.Fallback.Evaluate(ctx, input)
		if err == nil {
			evaluation.Reason = strings.TrimSpace(evaluation.Reason + "；LLM CRAG降级: " + cause.Error())
		}
		return evaluation, err
	}
	return CRAGEvaluation{}, cause
}

func buildCRAGEvaluatorPrompt(input CRAGEvaluateInput) string {
	var b strings.Builder
	b.WriteString("用户问题:\n")
	b.WriteString(input.Query)
	b.WriteString("\n\n检索资料:\n")
	for i, chunk := range input.Chunks {
		if i >= 8 {
			break
		}
		b.WriteString(fmt.Sprintf("[%d] 标题: %s\n内容: %s\n\n", i+1, chunk.Document.Title, truncate(chunk.Chunk.Content, 900)))
	}
	return b.String()
}

func parseCRAGEvaluation(content string) (CRAGEvaluation, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed CRAGEvaluation
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return CRAGEvaluation{}, err
	}
	parsed.Label = normalizeCRAGLabel(parsed.Label)
	parsed.Score = clamp01(parsed.Score)
	parsed.Relevance = clamp01(parsed.Relevance)
	parsed.Coverage = clamp01(parsed.Coverage)
	parsed.Specificity = clamp01(parsed.Specificity)
	parsed.Conflict = clamp01(parsed.Conflict)
	if parsed.Label == "" {
		return CRAGEvaluation{}, errors.New("CRAG结果缺少label")
	}
	return parsed, nil
}

func normalizeCRAGLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "correct", "use_internal":
		return CRAGLabelCorrect
	case "incorrect", "web_fallback":
		return CRAGLabelIncorrect
	case "ambiguous", "merge_internal_and_web":
		return CRAGLabelAmbiguous
	default:
		return ""
	}
}

func cragActionForLabel(label string) string {
	switch normalizeCRAGLabel(label) {
	case CRAGLabelCorrect:
		return CRAGLabelCorrect
	case CRAGLabelIncorrect:
		return CRAGLabelIncorrect
	case CRAGLabelAmbiguous:
		return CRAGLabelAmbiguous
	default:
		return CRAGLabelAmbiguous
	}
}

func cragNote(evaluation CRAGEvaluation) string {
	return fmt.Sprintf(
		"CRAG Evaluator: label=%s score=%.2f Relevance=%.2f Coverage=%.2f Specificity=%.2f Conflict=%.2f reason=%s",
		evaluation.Label,
		evaluation.Score,
		evaluation.Relevance,
		evaluation.Coverage,
		evaluation.Specificity,
		evaluation.Conflict,
		evaluation.Reason,
	)
}

func averageSpecificity(chunks []rankedChunk) float64 {
	if len(chunks) == 0 {
		return 0
	}
	total := 0.0
	for _, chunk := range chunks {
		keywords := extractKeywords(chunk.Chunk.Content, 16)
		total += math.Min(1, float64(len(keywords))/10)
	}
	return total / float64(len(chunks))
}

func conflictScore(chunks []rankedChunk) float64 {
	combined := strings.ToLower(chunksText(chunks))
	conflictMarkers := []string{"矛盾", "冲突", "相反", "不一致", "deprecated", "obsolete", "however", "but"}
	count := 0
	for _, marker := range conflictMarkers {
		if strings.Contains(combined, strings.ToLower(marker)) {
			count++
		}
	}
	return math.Min(1, float64(count)/3)
}

func chunksText(chunks []rankedChunk) string {
	var b strings.Builder
	for _, chunk := range chunks {
		b.WriteString(chunk.Chunk.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
