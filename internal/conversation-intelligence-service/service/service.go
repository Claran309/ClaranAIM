// Package service 实现会话智能归档：把聊天窗口提炼为摘要、决策、任务、主题和候选记忆。
package service

import (
	"ClaranAIM/internal/conversation-intelligence-service/dao"
	"ClaranAIM/internal/conversation-intelligence-service/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	JobStatusPending    = model.JobStatusPending
	JobStatusProcessing = model.JobStatusProcessing
	JobStatusCompleted  = model.JobStatusCompleted
	JobStatusSkipped    = model.JobStatusSkipped
	JobStatusFailed     = model.JobStatusFailed
)

// ConversationIntelligenceService 暴露会话智能归档能力。
type ConversationIntelligenceService interface {
	CreateDigestJob(ctx context.Context, input CreateDigestJobInput) (*DigestJob, error)
	ProcessDigestJob(ctx context.Context, jobID int64, viewerID ...int64) (DigestResult, error)
	RetryDigestJob(ctx context.Context, jobID, viewerID int64) (DigestResult, error)
	ListDigestJobs(ctx context.Context, input ListDigestJobsInput) ([]DigestJob, int64, error)
	ListArtifacts(ctx context.Context, input ListArtifactsInput) ([]ArtifactDTO, int64, error)
	RecordActivity(ctx context.Context, input ConversationActivityInput) error
	RunDueDigestJobs(ctx context.Context, options DigestScheduleOptions) ([]DigestResult, error)
}

// CreateDigestJobInput 创建一次会话归档任务。
type CreateDigestJobInput struct {
	ConversationID int64
	ViewerID       int64
	AgentID        int64
	StartMessageID int64
	EndMessageID   int64
	StartTime      time.Time
	EndTime        time.Time
	Reason         string
}

type ListArtifactsInput struct {
	ConversationID int64
	ViewerID       int64
	ArtifactType   string
	Limit          int
	Offset         int
}

// ListDigestJobsInput 查询当前用户可见的归档任务状态。
type ListDigestJobsInput struct {
	ConversationID int64
	ViewerID       int64
	Status         string
	Limit          int
	Offset         int
}

// ConversationActivityInput 表示会话中出现了可归档的新消息活动。
type ConversationActivityInput struct {
	ConversationID    int64
	ViewerID          int64
	AgentID           int64
	MessageID         int64
	MessageCountDelta int
	OccurredAt        time.Time
}

// DigestScheduleOptions 控制调度器何时把活跃会话窗口转为归档任务。
type DigestScheduleOptions struct {
	MessageThreshold int
	WindowMinutes    int
	Limit            int
	Now              time.Time
}

type DigestJob struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversation_id"`
	ViewerID       int64  `json:"viewer_id"`
	AgentID        int64  `json:"agent_id"`
	Status         string `json:"status"`
	MessageCount   int    `json:"message_count"`
	ValuableCount  int    `json:"valuable_count"`
	ErrorMessage   string `json:"error_message"`
	RetryCount     int    `json:"retry_count"`
	MaxRetries     int    `json:"max_retries"`
	NextRunAt      string `json:"next_run_at"`
	LastAttemptAt  string `json:"last_attempt_at"`
	CompletedAt    string `json:"completed_at"`
	Reason         string `json:"reason"`
}

type ArtifactDTO struct {
	ID             int64   `json:"id"`
	JobID          int64   `json:"job_id"`
	ConversationID int64   `json:"conversation_id"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Metadata       string  `json:"metadata"`
	SourceMsgIDs   []int64 `json:"source_message_ids"`
	Confidence     float64 `json:"confidence"`
}

type ConversationMessage struct {
	ID             int64
	ConversationID int64
	SenderID       int64
	Content        string
	MsgType        string
	CreatedAt      time.Time
	ReplyToID      int64
}

type FetchWindowInput struct {
	ConversationID int64
	ViewerID       int64
	Limit          int
	BeforeID       int64
	StartMessageID int64
	EndMessageID   int64
	StartTime      time.Time
	EndTime        time.Time
}

type MessageWindow struct {
	Messages     []ConversationMessage
	Participants []int64
}

// MessageWindowFetcher 从 msg-core-service 拉取当前 viewer 可见的消息窗口。
type MessageWindowFetcher interface {
	FetchWindow(ctx context.Context, input FetchWindowInput) (MessageWindow, error)
}

// RAGSink 把有价值的会话摘要/主题块送入 strict RAG。
type RAGSink interface {
	Archive(ctx context.Context, input RAGArchiveInput) error
}

type RAGArchiveInput struct {
	OwnerID        int64
	Title          string
	Content        string
	Source         string
	GroupID        int64
	ConversationID int64
}

// MemorySink 把用户画像类信息写入 pending memory candidate。
type MemorySink interface {
	CreateCandidate(ctx context.Context, input MemoryCandidateArchiveInput) error
}

type MemoryCandidateArchiveInput struct {
	AgentID        int64
	UserID         int64
	OwnerUserID    int64
	ConversationID int64
	Title          string
	Content        string
	Evidence       string
	Importance     float64
	Confidence     float64
}

type ArtifactBundle struct {
	Summary          *ConversationSummary
	Decisions        []Decision
	Tasks            []Task
	Topics           []TopicChunk
	Quotes           []Quote
	MemoryCandidates []MemoryCandidate
}

type ConversationSummary struct {
	ConversationID   int64
	StartMessageID   int64
	EndMessageID     int64
	StartTime        time.Time
	EndTime          time.Time
	Summary          string
	Participants     []int64
	SourceMsgIDs     []int64
	ValuableMsgCount int
}

type Decision struct {
	DecisionText string
	Reason       string
	DecidedBy    int64
	SourceMsgIDs []int64
	Confidence   float64
}

type Task struct {
	TaskTitle    string
	Assignee     string
	DueTime      string
	Status       string
	SourceMsgIDs []int64
	Confidence   float64
}

type TopicChunk struct {
	Topic        string
	Keywords     []string
	Content      string
	SourceMsgIDs []int64
	Confidence   float64
}

type Quote struct {
	Quote        string
	Reason       string
	SourceMsgIDs []int64
	Confidence   float64
}

type MemoryCandidate struct {
	Title        string
	Content      string
	Evidence     string
	SourceMsgIDs []int64
	Confidence   float64
	Importance   float64
}

type DigestResult struct {
	Job              DigestJob
	Summary          *ConversationSummary
	Decisions        []Decision
	Tasks            []Task
	Topics           []TopicChunk
	Quotes           []Quote
	MemoryCandidates []MemoryCandidate
}

// ArtifactExtractor 从清洗后的消息窗口中提炼结构化产物。
type ArtifactExtractor interface {
	Extract(ctx context.Context, window MessageWindow) (ArtifactBundle, error)
}

type ConversationIntelligenceOptions struct {
	WindowMessageLimit  int
	MinValuableMessages int
	MaxRetries          int
	RetryDelaySeconds   int
}

type conversationIntelligenceServiceImpl struct {
	repo      dao.Repository
	fetcher   MessageWindowFetcher
	rag       RAGSink
	memory    MemorySink
	extractor ArtifactExtractor
	options   ConversationIntelligenceOptions
}

// NewConversationIntelligenceService 创建会话智能归档服务。
func NewConversationIntelligenceService(repo dao.Repository, fetcher MessageWindowFetcher, rag RAGSink, memory MemorySink, extractor ArtifactExtractor, options ConversationIntelligenceOptions) ConversationIntelligenceService {
	options = normalizeOptions(options)
	if extractor == nil {
		extractor = RuleArtifactExtractor{}
	}
	return &conversationIntelligenceServiceImpl{repo: repo, fetcher: fetcher, rag: rag, memory: memory, extractor: extractor, options: options}
}

func (s *conversationIntelligenceServiceImpl) CreateDigestJob(ctx context.Context, input CreateDigestJobInput) (*DigestJob, error) {
	if s.repo == nil {
		return nil, errors.New("conversation intelligence repository未配置")
	}
	if input.ConversationID <= 0 || input.ViewerID <= 0 {
		return nil, errors.New("conversation_id和viewer_id不能为空")
	}
	job := &model.DigestJob{
		ConversationID: input.ConversationID,
		ViewerID:       input.ViewerID,
		AgentID:        input.AgentID,
		StartMessageID: input.StartMessageID,
		EndMessageID:   input.EndMessageID,
		Reason:         strings.TrimSpace(input.Reason),
		Status:         model.JobStatusPending,
		MaxRetries:     s.options.MaxRetries,
	}
	if !input.StartTime.IsZero() {
		job.StartTime = &input.StartTime
	}
	if !input.EndTime.IsZero() {
		job.EndTime = &input.EndTime
	}
	if err := s.repo.CreateJob(ctx, job); err != nil {
		return nil, err
	}
	dto := jobToDTO(job)
	return &dto, nil
}

func (s *conversationIntelligenceServiceImpl) ProcessDigestJob(ctx context.Context, jobID int64, viewerID ...int64) (DigestResult, error) {
	if s.repo == nil || s.fetcher == nil {
		return DigestResult{}, errors.New("conversation intelligence依赖未配置")
	}
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return DigestResult{}, err
	}
	if job == nil {
		return DigestResult{}, errors.New("digest job不存在")
	}
	if len(viewerID) > 0 && viewerID[0] > 0 && job.ViewerID != viewerID[0] {
		return DigestResult{Job: jobToDTO(job)}, errors.New("无权处理该会话归档任务")
	}
	now := time.Now()
	job.Status = model.JobStatusProcessing
	job.LastAttemptAt = &now
	job.NextRunAt = nil
	_ = s.repo.UpdateJob(ctx, job)
	window, err := s.fetcher.FetchWindow(ctx, FetchWindowInput{
		ConversationID: job.ConversationID,
		ViewerID:       job.ViewerID,
		Limit:          s.options.WindowMessageLimit,
		StartMessageID: job.StartMessageID,
		EndMessageID:   job.EndMessageID,
		StartTime:      optionalTimeValue(job.StartTime),
		EndTime:        optionalTimeValue(job.EndTime),
	})
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	job.MessageCount = len(window.Messages)
	cleaned := MessageWindow{Messages: filterValuableMessages(window.Messages), Participants: window.Participants}
	job.ValuableCount = len(cleaned.Messages)
	if len(cleaned.Messages) < s.options.MinValuableMessages {
		job.Status = model.JobStatusSkipped
		_ = s.repo.UpdateJob(ctx, job)
		return DigestResult{Job: jobToDTO(job)}, nil
	}
	bundle, err := s.extractor.Extract(ctx, cleaned)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	artifacts := artifactsFromBundle(job, bundle)
	if err := s.repo.CreateArtifacts(ctx, artifacts); err != nil {
		return s.failJob(ctx, job, err)
	}
	s.archiveToRAG(ctx, job, bundle)
	s.archiveMemoryCandidates(ctx, job, bundle)
	job.Status = model.JobStatusCompleted
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.NextRunAt = nil
	job.ErrorMessage = ""
	if err := s.repo.UpdateJob(ctx, job); err != nil {
		return DigestResult{}, err
	}
	result := DigestResult{Job: jobToDTO(job), Summary: bundle.Summary, Decisions: bundle.Decisions, Tasks: bundle.Tasks, Topics: bundle.Topics, Quotes: bundle.Quotes, MemoryCandidates: bundle.MemoryCandidates}
	return result, nil
}

func (s *conversationIntelligenceServiceImpl) RetryDigestJob(ctx context.Context, jobID, viewerID int64) (DigestResult, error) {
	if s.repo == nil {
		return DigestResult{}, errors.New("conversation intelligence repository未配置")
	}
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return DigestResult{}, err
	}
	if job == nil {
		return DigestResult{}, errors.New("digest job不存在")
	}
	if viewerID <= 0 || job.ViewerID != viewerID {
		return DigestResult{Job: jobToDTO(job)}, errors.New("无权重试该会话归档任务")
	}
	if job.Status != model.JobStatusFailed {
		return DigestResult{Job: jobToDTO(job)}, errors.New("只有失败的归档任务可以重试")
	}
	if job.RetryCount >= job.MaxRetries && job.MaxRetries > 0 {
		return DigestResult{Job: jobToDTO(job)}, errors.New("归档任务已达到最大重试次数")
	}
	job.Status = model.JobStatusPending
	job.NextRunAt = nil
	if err := s.repo.UpdateJob(ctx, job); err != nil {
		return DigestResult{}, err
	}
	return s.ProcessDigestJob(ctx, job.ID, viewerID)
}

func (s *conversationIntelligenceServiceImpl) ListDigestJobs(ctx context.Context, input ListDigestJobsInput) ([]DigestJob, int64, error) {
	if s.repo == nil {
		return nil, 0, errors.New("conversation intelligence repository未配置")
	}
	if input.ViewerID <= 0 {
		return nil, 0, errors.New("viewer_id不能为空")
	}
	rows, total, err := s.repo.ListJobs(ctx, dao.JobFilter{
		ConversationID: input.ConversationID,
		ViewerID:       input.ViewerID,
		Status:         strings.TrimSpace(input.Status),
		Limit:          input.Limit,
		Offset:         input.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]DigestJob, 0, len(rows))
	for i := range rows {
		out = append(out, jobToDTO(&rows[i]))
	}
	return out, total, nil
}

func (s *conversationIntelligenceServiceImpl) ListArtifacts(ctx context.Context, input ListArtifactsInput) ([]ArtifactDTO, int64, error) {
	if s.repo == nil {
		return nil, 0, errors.New("conversation intelligence repository未配置")
	}
	if input.ViewerID <= 0 {
		return nil, 0, errors.New("viewer_id不能为空")
	}
	rows, total, err := s.repo.ListArtifacts(ctx, dao.ArtifactFilter{
		ConversationID: input.ConversationID,
		ViewerID:       input.ViewerID,
		ArtifactType:   input.ArtifactType,
		Limit:          input.Limit,
		Offset:         input.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]ArtifactDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, artifactToDTO(row))
	}
	return out, total, nil
}

func (s *conversationIntelligenceServiceImpl) RecordActivity(ctx context.Context, input ConversationActivityInput) error {
	if s.repo == nil {
		return errors.New("conversation intelligence repository未配置")
	}
	if input.ConversationID <= 0 || input.ViewerID <= 0 {
		return errors.New("conversation_id和viewer_id不能为空")
	}
	return s.repo.UpsertActivity(ctx, dao.ActivityUpsert{
		ConversationID:    input.ConversationID,
		ViewerID:          input.ViewerID,
		AgentID:           input.AgentID,
		MessageID:         input.MessageID,
		MessageCountDelta: input.MessageCountDelta,
		OccurredAt:        input.OccurredAt,
	})
}

func (s *conversationIntelligenceServiceImpl) RunDueDigestJobs(ctx context.Context, options DigestScheduleOptions) ([]DigestResult, error) {
	if s.repo == nil {
		return nil, errors.New("conversation intelligence repository未配置")
	}
	cursors, err := s.repo.ListDueActivities(ctx, dao.DueActivityOptions{
		MessageThreshold: options.MessageThreshold,
		WindowMinutes:    options.WindowMinutes,
		Limit:            options.Limit,
		Now:              options.Now,
	})
	if err != nil {
		return nil, err
	}
	results := make([]DigestResult, 0, len(cursors))
	for _, cursor := range cursors {
		job, err := s.CreateDigestJob(ctx, CreateDigestJobInput{
			ConversationID: cursor.ConversationID,
			ViewerID:       cursor.ViewerID,
			AgentID:        cursor.AgentID,
			StartMessageID: cursor.FirstMessageID,
			EndMessageID:   cursor.LastMessageID,
			StartTime:      cursor.WindowStartedAt,
			EndTime:        cursor.LastActivityAt,
			Reason:         "scheduled_conversation_digest",
		})
		if err != nil {
			return results, err
		}
		result, err := s.ProcessDigestJob(ctx, job.ID)
		if err != nil {
			return results, err
		}
		results = append(results, result)
		_ = s.repo.MarkActivityDigested(ctx, cursor.ID, job.ID, time.Now())
	}
	remainingLimit := options.Limit
	if remainingLimit <= 0 {
		remainingLimit = 20
	}
	remainingLimit -= len(results)
	if remainingLimit > 0 {
		retryable, err := s.repo.ListRetryableJobs(ctx, dao.RetryableJobOptions{Limit: remainingLimit, Now: options.Now})
		if err != nil {
			return results, err
		}
		for _, job := range retryable {
			result, err := s.ProcessDigestJob(ctx, job.ID, job.ViewerID)
			results = append(results, result)
			if err != nil {
				continue
			}
		}
	}
	return results, nil
}

func (s *conversationIntelligenceServiceImpl) failJob(ctx context.Context, job *model.DigestJob, err error) (DigestResult, error) {
	job.Status = model.JobStatusFailed
	job.ErrorMessage = err.Error()
	now := time.Now()
	job.LastAttemptAt = &now
	if job.MaxRetries <= 0 {
		job.MaxRetries = s.options.MaxRetries
	}
	if job.RetryCount < job.MaxRetries {
		job.RetryCount++
		next := now.Add(time.Duration(s.options.RetryDelaySeconds) * time.Second)
		job.NextRunAt = &next
	} else {
		job.NextRunAt = nil
	}
	_ = s.repo.UpdateJob(ctx, job)
	return DigestResult{Job: jobToDTO(job)}, err
}

func (s *conversationIntelligenceServiceImpl) archiveToRAG(ctx context.Context, job *model.DigestJob, bundle ArtifactBundle) {
	if s.rag == nil {
		return
	}
	if bundle.Summary != nil {
		_ = s.rag.Archive(ctx, RAGArchiveInput{OwnerID: job.ViewerID, Title: "会话摘要", Content: bundle.Summary.Summary, Source: fmt.Sprintf("conversation:%d:job:%d", job.ConversationID, job.ID), ConversationID: job.ConversationID})
	}
	for _, topic := range bundle.Topics {
		_ = s.rag.Archive(ctx, RAGArchiveInput{OwnerID: job.ViewerID, Title: topic.Topic, Content: topic.Content, Source: fmt.Sprintf("conversation:%d:job:%d:topic", job.ConversationID, job.ID), ConversationID: job.ConversationID})
	}
}

func (s *conversationIntelligenceServiceImpl) archiveMemoryCandidates(ctx context.Context, job *model.DigestJob, bundle ArtifactBundle) {
	if s.memory == nil {
		return
	}
	for _, candidate := range bundle.MemoryCandidates {
		_ = s.memory.CreateCandidate(ctx, MemoryCandidateArchiveInput{
			AgentID:        job.AgentID,
			UserID:         job.ViewerID,
			OwnerUserID:    job.ViewerID,
			ConversationID: job.ConversationID,
			Title:          candidate.Title,
			Content:        candidate.Content,
			Evidence:       candidate.Evidence,
			Importance:     candidate.Importance,
			Confidence:     candidate.Confidence,
		})
	}
}

func normalizeOptions(options ConversationIntelligenceOptions) ConversationIntelligenceOptions {
	if options.WindowMessageLimit <= 0 {
		options.WindowMessageLimit = 100
	}
	if options.MinValuableMessages <= 0 {
		options.MinValuableMessages = 3
	}
	if options.MaxRetries <= 0 {
		options.MaxRetries = 3
	}
	if options.RetryDelaySeconds <= 0 {
		options.RetryDelaySeconds = 60
	}
	return options
}

func filterValuableMessages(messages []ConversationMessage) []ConversationMessage {
	out := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		if isValuableMessage(msg) {
			out = append(out, msg)
		}
	}
	return out
}

func isValuableMessage(msg ConversationMessage) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return false
	}
	if msg.MsgType != "" && msg.MsgType != "text" {
		return true
	}
	if len([]rune(content)) >= 18 {
		return true
	}
	keywords := []string{"决定", "任务", "负责人", "结论", "原则", "需求", "实现", "bug", "代码", "文件", "链接", "http", "RAG", "Milvus", "Kafka"}
	lower := strings.ToLower(content)
	for _, keyword := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// RuleArtifactExtractor 是不用 LLM 的保底提炼器，生产可替换为 LLM extractor。
type RuleArtifactExtractor struct{}

func (RuleArtifactExtractor) Extract(ctx context.Context, window MessageWindow) (ArtifactBundle, error) {
	_ = ctx
	if len(window.Messages) == 0 {
		return ArtifactBundle{}, nil
	}
	start := window.Messages[0]
	end := window.Messages[len(window.Messages)-1]
	ids := messageIDs(window.Messages)
	joined := joinMessages(window.Messages)
	bundle := ArtifactBundle{
		Summary: &ConversationSummary{
			ConversationID:   start.ConversationID,
			StartMessageID:   start.ID,
			EndMessageID:     end.ID,
			StartTime:        start.CreatedAt,
			EndTime:          end.CreatedAt,
			Summary:          buildRuleSummary(joined),
			Participants:     window.Participants,
			SourceMsgIDs:     ids,
			ValuableMsgCount: len(window.Messages),
		},
	}
	for _, msg := range window.Messages {
		content := strings.TrimSpace(msg.Content)
		if decision := extractAfterPrefix(content, "决定"); decision != "" {
			bundle.Decisions = append(bundle.Decisions, Decision{DecisionText: decision, Reason: "消息中显式出现决策表达", DecidedBy: msg.SenderID, SourceMsgIDs: []int64{msg.ID}, Confidence: 0.8})
		}
		if task := extractAfterPrefix(content, "任务"); task != "" {
			bundle.Tasks = append(bundle.Tasks, parseTask(task, msg.ID))
		}
		if strings.Contains(content, "pending memory candidate") || strings.Contains(content, "候选记忆") || strings.Contains(content, "长期记住") {
			bundle.MemoryCandidates = append(bundle.MemoryCandidates, MemoryCandidate{Title: "会话偏好", Content: content, Evidence: content, SourceMsgIDs: []int64{msg.ID}, Confidence: 0.75, Importance: 0.6})
		}
		if looksLikeQuote(content) {
			bundle.Quotes = append(bundle.Quotes, Quote{Quote: content, Reason: "包含原则/定义/明确结论", SourceMsgIDs: []int64{msg.ID}, Confidence: 0.7})
		}
	}
	topic := buildTopic(window.Messages)
	bundle.Topics = append(bundle.Topics, topic)
	return bundle, nil
}

func buildRuleSummary(joined string) string {
	return truncate("这段会话主要讨论了："+joined, 500)
}

func parseTask(raw string, msgID int64) Task {
	assignee := ""
	match := regexp.MustCompile(`负责人\s*[:：]?\s*([^，。,.\s]+)`).FindStringSubmatch(raw)
	if len(match) > 1 {
		assignee = match[1]
	}
	return Task{TaskTitle: strings.TrimSpace(raw), Assignee: assignee, Status: "todo", SourceMsgIDs: []int64{msgID}, Confidence: 0.75}
}

func buildTopic(messages []ConversationMessage) TopicChunk {
	keywords := extractKeywords(joinMessages(messages))
	topic := "会话主题"
	if len(keywords) > 0 {
		topic = strings.Join(keywords[:min(3, len(keywords))], " / ")
	}
	return TopicChunk{Topic: topic, Keywords: keywords, Content: joinMessages(messages), SourceMsgIDs: messageIDs(messages), Confidence: 0.7}
}

func extractAfterPrefix(content, prefix string) string {
	re := regexp.MustCompile(prefix + `\s*[:：]\s*(.+)`)
	match := re.FindStringSubmatch(content)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func looksLikeQuote(content string) bool {
	return strings.Contains(content, "原则") || strings.Contains(content, "结论") || strings.Contains(content, "不应该") || strings.Contains(content, "应该")
}

func extractKeywords(text string) []string {
	candidates := []string{"conversation", "intelligence", "RAG", "Milvus", "memory", "candidate", "digest", "job", "worker", "summary", "decision", "task", "topic", "Kafka"}
	lower := strings.ToLower(text)
	out := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, keyword := range candidates {
		if strings.Contains(lower, strings.ToLower(keyword)) && !seen[keyword] {
			seen[keyword] = true
			out = append(out, keyword)
		}
	}
	return out
}

func artifactsFromBundle(job *model.DigestJob, bundle ArtifactBundle) []model.ConversationArtifact {
	var out []model.ConversationArtifact
	if bundle.Summary != nil {
		out = append(out, newArtifact(job, model.ArtifactSummary, "会话摘要", bundle.Summary.Summary, bundle.Summary.SourceMsgIDs, bundle.Summary))
	}
	for _, item := range bundle.Decisions {
		out = append(out, newArtifact(job, model.ArtifactDecision, "决策", item.DecisionText, item.SourceMsgIDs, item))
	}
	for _, item := range bundle.Tasks {
		out = append(out, newArtifact(job, model.ArtifactTask, item.TaskTitle, item.TaskTitle, item.SourceMsgIDs, item))
	}
	for _, item := range bundle.Topics {
		out = append(out, newArtifact(job, model.ArtifactTopic, item.Topic, item.Content, item.SourceMsgIDs, item))
	}
	for _, item := range bundle.Quotes {
		out = append(out, newArtifact(job, model.ArtifactQuote, "金句/原则", item.Quote, item.SourceMsgIDs, item))
	}
	for _, item := range bundle.MemoryCandidates {
		out = append(out, newArtifact(job, model.ArtifactMemoryCandidate, item.Title, item.Content, item.SourceMsgIDs, item))
	}
	return out
}

func newArtifact(job *model.DigestJob, artifactType, title, content string, sourceIDs []int64, metadata interface{}) model.ConversationArtifact {
	data, _ := json.Marshal(metadata)
	return model.ConversationArtifact{JobID: job.ID, ConversationID: job.ConversationID, ViewerID: job.ViewerID, AgentID: job.AgentID, ArtifactType: artifactType, Title: title, Content: content, MetadataJSON: string(data), SourceMsgIDs: encodeIDs(sourceIDs), Confidence: 0.7}
}

func jobToDTO(job *model.DigestJob) DigestJob {
	if job == nil {
		return DigestJob{}
	}
	return DigestJob{
		ID:             job.ID,
		ConversationID: job.ConversationID,
		ViewerID:       job.ViewerID,
		AgentID:        job.AgentID,
		Status:         job.Status,
		MessageCount:   job.MessageCount,
		ValuableCount:  job.ValuableCount,
		ErrorMessage:   job.ErrorMessage,
		RetryCount:     job.RetryCount,
		MaxRetries:     job.MaxRetries,
		NextRunAt:      formatOptionalTime(job.NextRunAt),
		LastAttemptAt:  formatOptionalTime(job.LastAttemptAt),
		CompletedAt:    formatOptionalTime(job.CompletedAt),
		Reason:         job.Reason,
	}
}

func artifactToDTO(row model.ConversationArtifact) ArtifactDTO {
	return ArtifactDTO{ID: row.ID, JobID: row.JobID, ConversationID: row.ConversationID, Type: row.ArtifactType, Title: row.Title, Content: row.Content, Metadata: row.MetadataJSON, SourceMsgIDs: decodeIDs(row.SourceMsgIDs), Confidence: row.Confidence}
}

func optionalTimeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func messageIDs(messages []ConversationMessage) []int64 {
	ids := make([]int64, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}
	return ids
}

func joinMessages(messages []ConversationMessage) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, strings.TrimSpace(msg.Content))
	}
	return strings.Join(parts, "\n")
}

func encodeIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func decodeIDs(raw string) []int64 {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func filterWindowMessages(messages []ConversationMessage, input FetchWindowInput) []ConversationMessage {
	out := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		if input.StartMessageID > 0 && msg.ID < input.StartMessageID {
			continue
		}
		if input.EndMessageID > 0 && msg.ID > input.EndMessageID {
			continue
		}
		if !input.StartTime.IsZero() && msg.CreatedAt.Before(input.StartTime) {
			continue
		}
		if !input.EndTime.IsZero() && msg.CreatedAt.After(input.EndTime) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func truncate(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
