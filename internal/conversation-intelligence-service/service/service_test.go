package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProcessDigestJobArchivesConversationArtifactsToRAGAndMemoryCandidates(t *testing.T) {
	repo := newFakeConversationRepo()
	msgs := []ConversationMessage{
		{ID: 1, ConversationID: 100, SenderID: 10, Content: "我们先做 conversation intelligence，不要每条消息都入 Milvus。", MsgType: "text", CreatedAt: time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)},
		{ID: 2, ConversationID: 100, SenderID: 11, Content: "决定：先把一小时或一百条消息聚合成摘要，再入 strict rag。", MsgType: "text", CreatedAt: time.Date(2026, 6, 2, 10, 1, 0, 0, time.UTC)},
		{ID: 3, ConversationID: 100, SenderID: 10, Content: "任务：实现 digest job worker，负责人 Codex。", MsgType: "text", CreatedAt: time.Date(2026, 6, 2, 10, 2, 0, 0, time.UTC)},
		{ID: 4, ConversationID: 100, SenderID: 10, Content: "我希望先进入 pending memory candidate，不要直接写正式记忆。", MsgType: "text", CreatedAt: time.Date(2026, 6, 2, 10, 3, 0, 0, time.UTC)},
		{ID: 5, ConversationID: 100, SenderID: 11, Content: "好", MsgType: "text", CreatedAt: time.Date(2026, 6, 2, 10, 4, 0, 0, time.UTC)},
	}
	fetcher := &fakeMessageWindowFetcher{messages: msgs, participants: []int64{10, 11}}
	rag := &fakeConversationRAGSink{}
	memory := &fakeConversationMemorySink{}
	extractor := RuleArtifactExtractor{}
	svc := NewConversationIntelligenceService(repo, fetcher, rag, memory, extractor, ConversationIntelligenceOptions{MinValuableMessages: 2})

	job, err := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 100, ViewerID: 10, AgentID: 9001, Reason: "test"})
	if err != nil {
		t.Fatalf("CreateDigestJob returned error: %v", err)
	}
	result, err := svc.ProcessDigestJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ProcessDigestJob returned error: %v", err)
	}
	if result.Job.Status != JobStatusCompleted {
		t.Fatalf("expected completed job, got %s", result.Job.Status)
	}
	if result.Summary == nil || !strings.Contains(result.Summary.Summary, "conversation intelligence") {
		t.Fatalf("expected conversation summary, got %#v", result.Summary)
	}
	if len(result.Decisions) == 0 || !strings.Contains(result.Decisions[0].DecisionText, "一小时或一百条消息聚合") {
		t.Fatalf("expected decision artifact, got %#v", result.Decisions)
	}
	if len(result.Tasks) == 0 || result.Tasks[0].Assignee != "Codex" {
		t.Fatalf("expected task artifact with assignee Codex, got %#v", result.Tasks)
	}
	if len(result.Topics) == 0 || !strings.Contains(strings.Join(result.Topics[0].Keywords, ","), "conversation") {
		t.Fatalf("expected topic artifact, got %#v", result.Topics)
	}
	if len(result.MemoryCandidates) == 0 || !strings.Contains(result.MemoryCandidates[0].Content, "pending memory candidate") {
		t.Fatalf("expected pending memory candidate, got %#v", result.MemoryCandidates)
	}
	if len(rag.ingested) == 0 {
		t.Fatalf("expected summary/topic artifacts to be sent to RAG")
	}
	if strings.Contains(rag.ingested[0].Content, "好") {
		t.Fatalf("expected noisy short chat not to be archived into RAG content")
	}
	if len(memory.candidates) != len(result.MemoryCandidates) {
		t.Fatalf("expected memory candidates to be sent pending, got %d", len(memory.candidates))
	}
}

func TestProcessDigestJobSkipsNoisyWindowWithoutRAGOrMemoryWrites(t *testing.T) {
	repo := newFakeConversationRepo()
	fetcher := &fakeMessageWindowFetcher{messages: []ConversationMessage{
		{ID: 1, ConversationID: 200, SenderID: 10, Content: "好", MsgType: "text"},
		{ID: 2, ConversationID: 200, SenderID: 11, Content: "嗯", MsgType: "text"},
		{ID: 3, ConversationID: 200, SenderID: 10, Content: "可以", MsgType: "text"},
	}}
	rag := &fakeConversationRAGSink{}
	memory := &fakeConversationMemorySink{}
	svc := NewConversationIntelligenceService(repo, fetcher, rag, memory, RuleArtifactExtractor{}, ConversationIntelligenceOptions{MinValuableMessages: 2})

	job, err := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 200, ViewerID: 10, AgentID: 9001})
	if err != nil {
		t.Fatalf("CreateDigestJob returned error: %v", err)
	}
	result, err := svc.ProcessDigestJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ProcessDigestJob returned error: %v", err)
	}
	if result.Job.Status != JobStatusSkipped {
		t.Fatalf("expected skipped noisy job, got %s", result.Job.Status)
	}
	if len(rag.ingested) != 0 {
		t.Fatalf("expected no RAG writes for noisy window")
	}
	if len(memory.candidates) != 0 {
		t.Fatalf("expected no memory candidates for noisy window")
	}
}

func TestProcessDigestJobFiltersLowValueMemoryCandidates(t *testing.T) {
	repo := newFakeConversationRepo()
	fetcher := &fakeMessageWindowFetcher{messages: []ConversationMessage{
		{ID: 1, ConversationID: 210, SenderID: 10, Content: "用户正在集成 RAG 和 Milvus，希望先保证主链路稳定。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 2, ConversationID: 210, SenderID: 10, Content: "这条消息足够长，用来触发会话归档窗口处理。", MsgType: "text", CreatedAt: time.Now()},
	}}
	memory := &fakeConversationMemorySink{}
	extractor := fixedArtifactExtractor{bundle: ArtifactBundle{
		Summary: &ConversationSummary{ConversationID: 210, Summary: "讨论了 RAG 集成。", SourceMsgIDs: []int64{1, 2}},
		MemoryCandidates: []MemoryCandidate{
			{Title: "噪声", Content: "好的", Confidence: 0.9, Importance: 0.9},
			{Title: "低置信", Content: "用户可能有一点点关注 RAG。", Confidence: 0.4, Importance: 0.9},
			{Title: "长期项目状态", Content: "用户正在集成 RAG 和 Milvus，希望先保证主链路稳定。", Evidence: "用户明确说明", Confidence: 0.92, Importance: 0.82},
			{Title: "重复候选", Content: "用户正在集成 RAG 和 Milvus，希望先保证主链路稳定。", Evidence: "重复", Confidence: 0.88, Importance: 0.78},
		},
	}}
	svc := NewConversationIntelligenceService(repo, fetcher, nil, memory, extractor, ConversationIntelligenceOptions{MinValuableMessages: 1})

	job, err := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 210, ViewerID: 10, AgentID: 9001})
	if err != nil {
		t.Fatalf("CreateDigestJob returned error: %v", err)
	}
	result, err := svc.ProcessDigestJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ProcessDigestJob returned error: %v", err)
	}
	if len(result.MemoryCandidates) != 1 {
		t.Fatalf("expected one high-value memory candidate, got %#v", result.MemoryCandidates)
	}
	if len(memory.candidates) != 1 || !strings.Contains(memory.candidates[0].Content, "Milvus") {
		t.Fatalf("expected only durable memory candidate to be archived, got %#v", memory.candidates)
	}
}

func TestProcessDigestJobRejectsMismatchedViewer(t *testing.T) {
	repo := newFakeConversationRepo()
	fetcher := &fakeMessageWindowFetcher{messages: []ConversationMessage{
		{ID: 1, ConversationID: 100, SenderID: 10, Content: "决定：只能由任务所属用户处理会话归档。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 2, ConversationID: 100, SenderID: 11, Content: "任务：补齐 viewer_id 校验，避免越权处理。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 3, ConversationID: 100, SenderID: 10, Content: "这条消息足够长，用来确保窗口能被归档提炼。", MsgType: "text", CreatedAt: time.Now()},
	}}
	rag := &fakeConversationRAGSink{}
	svc := NewConversationIntelligenceService(repo, fetcher, rag, nil, RuleArtifactExtractor{}, ConversationIntelligenceOptions{MinValuableMessages: 1})

	job, err := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 100, ViewerID: 1001, AgentID: 9001})
	if err != nil {
		t.Fatalf("CreateDigestJob returned error: %v", err)
	}

	_, err = svc.ProcessDigestJob(context.Background(), job.ID, 2002)
	if err == nil || !strings.Contains(err.Error(), "无权处理") {
		t.Fatalf("ProcessDigestJob error = %v, want permission error", err)
	}
	if len(repo.artifacts) != 0 || len(rag.ingested) != 0 {
		t.Fatalf("mismatched viewer must not produce side effects, artifacts=%d rag=%d", len(repo.artifacts), len(rag.ingested))
	}
}

func TestSchedulerProcessesActiveConversationWhenMessageThresholdReached(t *testing.T) {
	repo := newFakeConversationRepo()
	fetcher := &fakeMessageWindowFetcher{messages: []ConversationMessage{
		{ID: 1, ConversationID: 300, SenderID: 10, Content: "决定：聊天记录只归档摘要和主题块，不逐条入库。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 2, ConversationID: 300, SenderID: 11, Content: "任务：把摘要写入 RAG，把候选记忆写入 pending。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 3, ConversationID: 300, SenderID: 10, Content: "长期记住：用户希望聊天记录归档要有权限裁剪。", MsgType: "text", CreatedAt: time.Now()},
	}, participants: []int64{10, 11}}
	rag := &fakeConversationRAGSink{}
	memory := &fakeConversationMemorySink{}
	svc := NewConversationIntelligenceService(repo, fetcher, rag, memory, RuleArtifactExtractor{}, ConversationIntelligenceOptions{MinValuableMessages: 2})

	if err := svc.RecordActivity(context.Background(), ConversationActivityInput{
		ConversationID:    300,
		ViewerID:          10,
		AgentID:           9001,
		MessageID:         3,
		MessageCountDelta: 3,
		OccurredAt:        time.Now(),
	}); err != nil {
		t.Fatalf("RecordActivity returned error: %v", err)
	}
	results, err := svc.RunDueDigestJobs(context.Background(), DigestScheduleOptions{MessageThreshold: 3, WindowMinutes: 60, Limit: 10})
	if err != nil {
		t.Fatalf("RunDueDigestJobs returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one scheduled digest result, got %d", len(results))
	}
	if results[0].Job.Status != JobStatusCompleted {
		t.Fatalf("scheduled job status=%s", results[0].Job.Status)
	}
	if len(rag.ingested) == 0 {
		t.Fatal("scheduled digest should archive summary/topic into RAG")
	}
}

func TestFailedDigestJobRecordsRetryStateAndCanBeRetried(t *testing.T) {
	repo := newFakeConversationRepo()
	failing := &fakeMessageWindowFetcher{err: errors.New("msg-core unavailable")}
	svc := NewConversationIntelligenceService(repo, failing, nil, nil, RuleArtifactExtractor{}, ConversationIntelligenceOptions{MinValuableMessages: 1, MaxRetries: 2, RetryDelaySeconds: 30})

	job, err := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 400, ViewerID: 10})
	if err != nil {
		t.Fatalf("CreateDigestJob returned error: %v", err)
	}
	_, err = svc.ProcessDigestJob(context.Background(), job.ID, 10)
	if err == nil {
		t.Fatal("ProcessDigestJob should return fetcher error")
	}
	failed, _ := repo.GetJob(context.Background(), job.ID)
	if failed.Status != JobStatusFailed || failed.RetryCount != 1 || failed.NextRunAt == nil {
		t.Fatalf("failed job retry state = %#v", failed)
	}

	failing.err = nil
	failing.messages = []ConversationMessage{
		{ID: 1, ConversationID: 400, SenderID: 10, Content: "决定：失败归档任务应该能由本人重试。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 2, ConversationID: 400, SenderID: 11, Content: "任务：重试时要保留 viewer_id 权限校验。", MsgType: "text", CreatedAt: time.Now()},
		{ID: 3, ConversationID: 400, SenderID: 10, Content: "这条足够长的消息用于满足最小有效消息数量。", MsgType: "text", CreatedAt: time.Now()},
	}
	result, err := svc.RetryDigestJob(context.Background(), job.ID, 10)
	if err != nil {
		t.Fatalf("RetryDigestJob returned error: %v", err)
	}
	if result.Job.Status != JobStatusCompleted || result.Job.RetryCount != 1 {
		t.Fatalf("retry result job = %#v", result.Job)
	}
}

func TestListDigestJobsScopesToViewerAndReturnsLatestFirst(t *testing.T) {
	repo := newFakeConversationRepo()
	svc := NewConversationIntelligenceService(repo, &fakeMessageWindowFetcher{}, nil, nil, RuleArtifactExtractor{}, ConversationIntelligenceOptions{})
	first, _ := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 500, ViewerID: 10, Reason: "first"})
	second, _ := svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 501, ViewerID: 10, Reason: "second"})
	_, _ = svc.CreateDigestJob(context.Background(), CreateDigestJobInput{ConversationID: 500, ViewerID: 11, Reason: "other"})

	jobs, total, err := svc.ListDigestJobs(context.Background(), ListDigestJobsInput{ViewerID: 10, Limit: 20})
	if err != nil {
		t.Fatalf("ListDigestJobs returned error: %v", err)
	}
	if total != 2 || len(jobs) != 2 {
		t.Fatalf("jobs len=%d total=%d", len(jobs), total)
	}
	if jobs[0].ID != second.ID || jobs[1].ID != first.ID {
		t.Fatalf("jobs order = %#v", jobs)
	}
	for _, job := range jobs {
		if job.ViewerID != 10 {
			t.Fatalf("listed job leaks another viewer: %#v", job)
		}
	}
}

func TestMessageWindowFilteringKeepsRequestedMessageAndTimeRange(t *testing.T) {
	start := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	messages := []ConversationMessage{
		{ID: 1, Content: "before id", CreatedAt: start.Add(-time.Minute)},
		{ID: 2, Content: "in range", CreatedAt: start.Add(time.Minute)},
		{ID: 3, Content: "also in range", CreatedAt: start.Add(2 * time.Minute)},
		{ID: 4, Content: "after id", CreatedAt: start.Add(3 * time.Minute)},
	}
	filtered := filterWindowMessages(messages, FetchWindowInput{
		StartMessageID: 2,
		EndMessageID:   3,
		StartTime:      start,
		EndTime:        start.Add(2 * time.Minute),
	})
	if len(filtered) != 2 || filtered[0].ID != 2 || filtered[1].ID != 3 {
		t.Fatalf("filtered messages = %#v", filtered)
	}
}

func TestLLMArtifactExtractorParsesStructuredConversationArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		payload := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": `{
					"summary": {"summary": "讨论了会话归档和候选记忆。"},
					"decisions": [{"decision_text": "先做 LLM 提炼器。", "reason": "规则提炼不够准确", "decided_by": 10, "source_message_ids": [1], "confidence": 0.82}],
					"tasks": [{"task_title": "补归档状态页", "assignee": "Codex", "status": "todo", "source_message_ids": [2], "confidence": 0.76}],
					"topics": [{"topic": "Conversation Intelligence", "keywords": ["RAG", "Memory"], "content": "会话归档主题块", "source_message_ids": [1,2], "confidence": 0.8}],
					"quotes": [{"quote": "不要每条短消息都入向量库。", "reason": "架构原则", "source_message_ids": [1], "confidence": 0.7}],
					"memory_candidates": [{"title": "用户偏好", "content": "用户希望先候选再确认。", "evidence": "用户明确说明", "source_message_ids": [2], "confidence": 0.9, "importance": 0.6}]
				}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()
	extractor := NewLLMArtifactExtractor(LLMArtifactExtractorConfig{BaseURL: server.URL, APIKey: "test-key", Model: "small-model"})
	window := MessageWindow{Messages: []ConversationMessage{
		{ID: 1, ConversationID: 600, SenderID: 10, Content: "决定：先做 LLM 提炼器。", CreatedAt: time.Now()},
		{ID: 2, ConversationID: 600, SenderID: 11, Content: "任务：补归档状态页。", CreatedAt: time.Now()},
	}, Participants: []int64{10, 11}}

	bundle, err := extractor.Extract(context.Background(), window)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if bundle.Summary == nil || bundle.Summary.ConversationID != 600 || !strings.Contains(bundle.Summary.Summary, "会话归档") {
		t.Fatalf("summary = %#v", bundle.Summary)
	}
	if len(bundle.Decisions) != 1 || bundle.Decisions[0].DecisionText != "先做 LLM 提炼器。" {
		t.Fatalf("decisions = %#v", bundle.Decisions)
	}
	if len(bundle.MemoryCandidates) != 1 || bundle.MemoryCandidates[0].Importance != 0.6 {
		t.Fatalf("memory candidates = %#v", bundle.MemoryCandidates)
	}
}

func TestFilterMemoryCandidatesDropsRecentConversationNoise(t *testing.T) {
	candidates := filterMemoryCandidates([]MemoryCandidate{
		{
			Title:      "最近会话",
			Content:    "用户最近会话中提到了上传文档和查看知识图谱，这类短期上下文应在调用时召回，不应进入长期记忆。",
			Evidence:   "最近会话提到上传文档",
			Confidence: 0.95,
			Importance: 0.9,
		},
		{
			Title:      "长期偏好",
			Content:    "用户长期偏好先修复核心功能链路，再打磨前端视觉和动效。",
			Evidence:   "用户多次强调先解决 bug 和功能链路",
			Confidence: 0.9,
			Importance: 0.86,
		},
	})
	if len(candidates) != 1 {
		t.Fatalf("candidates=%#v, want only durable long-term candidate", candidates)
	}
	if !strings.Contains(candidates[0].Content, "长期偏好") {
		t.Fatalf("candidate=%#v, want long-term preference kept", candidates[0])
	}
}

type fakeMessageWindowFetcher struct {
	messages     []ConversationMessage
	participants []int64
	err          error
}

func (f *fakeMessageWindowFetcher) FetchWindow(ctx context.Context, input FetchWindowInput) (MessageWindow, error) {
	_ = ctx
	_ = input
	if f.err != nil {
		return MessageWindow{}, f.err
	}
	return MessageWindow{Messages: f.messages, Participants: f.participants}, nil
}

type fakeConversationRAGSink struct {
	ingested []RAGArchiveInput
}

func (s *fakeConversationRAGSink) Archive(ctx context.Context, input RAGArchiveInput) error {
	_ = ctx
	s.ingested = append(s.ingested, input)
	return nil
}

type fakeConversationMemorySink struct {
	candidates []MemoryCandidateArchiveInput
}

func (s *fakeConversationMemorySink) CreateCandidate(ctx context.Context, input MemoryCandidateArchiveInput) error {
	_ = ctx
	s.candidates = append(s.candidates, input)
	return nil
}

type fixedArtifactExtractor struct {
	bundle ArtifactBundle
	err    error
}

func (e fixedArtifactExtractor) Extract(ctx context.Context, window MessageWindow) (ArtifactBundle, error) {
	_ = ctx
	_ = window
	return e.bundle, e.err
}
