package handler

import (
	"ClaranAIM/pkg/documentparser"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/ragclient"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

const maxRAGUploadBytes int64 = 120 << 20

const (
	ragUploadStatusPending    = "pending"
	ragUploadStatusProcessing = "processing"
	ragUploadStatusCompleted  = "completed"
	ragUploadStatusFailed     = "failed"
)

// RAGHandler 暴露知识库入库、RAG 搜索和知识图谱查询接口。
type RAGHandler struct {
	svc ragclient.Service
}

// gatewayRAGService 是 api-gateway 到 rag-service 的 RPC 门面。
var gatewayRAGService ragclient.Service
var gatewayDocumentOCR documentparser.OCRProvider
var gatewayRAGUploadJobs = newRAGUploadJobStore()

// InitRAGService 注册 rag-service RPC 客户端。
func InitRAGService(svc ragclient.Service) {
	gatewayRAGService = svc
}

// InitDocumentOCR 注册上传文档解析使用的 OCR provider。
func InitDocumentOCR(provider documentparser.OCRProvider) {
	gatewayDocumentOCR = provider
}

// NewRAGHandler 创建 RAG HTTP handler。
func NewRAGHandler() *RAGHandler {
	return &RAGHandler{svc: gatewayRAGService}
}

func (h *RAGHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "rag-service未初始化")
		return false
	}
	return true
}

// IngestDocument 接收用户输入的知识文本并写入 rag-service。
func (h *RAGHandler) IngestDocument(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req ragIngestReq
	if err := bindRAGJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		response.BadRequest(c, "知识内容不能为空")
		return
	}
	resp, err := h.svc.IngestDocument(ctx, userID, ragclient.IngestInput{
		Title:          strings.TrimSpace(req.Title),
		Content:        content,
		Source:         strings.TrimSpace(req.Source),
		SourceType:     defaultRAGString(req.SourceType, "text"),
		Visibility:     defaultRAGString(req.Visibility, ragclient.VisibilityPrivate),
		GroupID:        parseRAGNumberOrZero(req.GroupID),
		ConversationID: parseRAGNumberOrZero(req.ConversationID),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UploadDocument 接收 txt、md、pdf 等文件，解析为文本后写入 RAG 知识库并构建图谱。
func (h *RAGHandler) UploadDocument(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil || form == nil {
		response.BadRequest(c, "请上传知识库文件")
		return
	}
	headers := form.File["file"]
	if len(headers) == 0 {
		response.BadRequest(c, "请上传知识库文件")
		return
	}
	title := strings.TrimSpace(string(c.FormValue("title")))
	visibility := defaultRAGString(string(c.FormValue("visibility")), ragclient.VisibilityPrivate)
	groupID := parseRAGIntString(string(c.FormValue("group_id")))
	conversationID := parseRAGIntString(string(c.FormValue("conversation_id")))
	items := make([]ragUploadWorkItem, 0, len(headers))
	results := make([]*ragUploadResult, 0, len(headers))
	for _, header := range headers {
		item, result := readRAGUploadWorkItem(header)
		items = append(items, item)
		results = append(results, result)
	}
	jobID := gatewayRAGUploadJobs.create(userID, results)
	go h.processRAGUploadJob(jobID, userID, items, ragUploadOptions{
		Title:          title,
		Visibility:     visibility,
		GroupID:        groupID,
		ConversationID: conversationID,
	})
	response.Success(c, map[string]interface{}{"success": true, "async": true, "job_id": jobID, "status": ragUploadStatusPending, "files": results})
}

// GetUploadJob 返回 RAG 文件上传后台任务状态。
func (h *RAGHandler) GetUploadJob(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	jobID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "任务ID无效")
		return
	}
	job, ok := gatewayRAGUploadJobs.get(jobID, userID)
	if !ok {
		response.BadRequest(c, "任务不存在或无权查看")
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "job": job})
}

func readRAGUploadWorkItem(header *multipart.FileHeader) (ragUploadWorkItem, *ragUploadResult) {
	item := ragUploadWorkItem{FileName: header.Filename, ContentType: header.Header.Get("Content-Type")}
	result := &ragUploadResult{FileName: header.Filename, Status: ragUploadStatusPending}
	if header.Size > maxRAGUploadBytes {
		result.Status = ragUploadStatusFailed
		result.Msg = "文件不能超过120MB"
		return item, result
	}
	file, err := header.Open()
	if err != nil {
		result.Status = ragUploadStatusFailed
		result.Msg = "读取文件失败"
		return item, result
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxRAGUploadBytes+1))
	_ = file.Close()
	if readErr != nil {
		result.Status = ragUploadStatusFailed
		result.Msg = "读取文件失败"
		return item, result
	}
	if int64(len(data)) > maxRAGUploadBytes {
		result.Status = ragUploadStatusFailed
		result.Msg = "文件不能超过120MB"
		return item, result
	}
	item.Data = data
	return item, result
}

func (h *RAGHandler) processRAGUploadJob(jobID, userID int64, items []ragUploadWorkItem, opts ragUploadOptions) {
	gatewayRAGUploadJobs.setStatus(jobID, ragUploadStatusProcessing, "")
	for idx, item := range items {
		gatewayRAGUploadJobs.updateFile(jobID, idx, func(result *ragUploadResult) {
			if result.Status != ragUploadStatusFailed {
				result.Status = ragUploadStatusProcessing
				result.Msg = "正在解析和入库"
			}
		})
		if len(item.Data) == 0 {
			gatewayRAGUploadJobs.updateFile(jobID, idx, func(result *ragUploadResult) {
				result.Status = ragUploadStatusFailed
				if result.Msg == "" {
					result.Msg = "文件内容为空或读取失败"
				}
			})
			continue
		}
		result := h.handleOneRAGUpload(context.Background(), userID, item, opts)
		gatewayRAGUploadJobs.replaceFile(jobID, idx, result)
	}
	gatewayRAGUploadJobs.finish(jobID)
}

func (h *RAGHandler) handleOneRAGUpload(ctx context.Context, userID int64, item ragUploadWorkItem, opts ragUploadOptions) *ragUploadResult {
	result := &ragUploadResult{FileName: item.FileName, Status: ragUploadStatusProcessing}
	parsed, err := documentparser.ParseWithOptions(ctx, item.FileName, item.ContentType, item.Data, documentparser.ParseOptions{
		OCR: gatewayDocumentOCR,
	})
	if err != nil {
		result.Status = ragUploadStatusFailed
		result.Msg = err.Error()
		return result
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = parsed.Title
	}
	resp, err := h.svc.IngestDocument(ctx, userID, ragclient.IngestInput{
		Title:          title,
		Content:        parsed.Content,
		Source:         item.FileName,
		SourceType:     parsed.SourceType,
		Visibility:     opts.Visibility,
		GroupID:        opts.GroupID,
		ConversationID: opts.ConversationID,
	})
	if err != nil {
		result.Status = ragUploadStatusFailed
		result.Msg = err.Error()
		return result
	}
	result.Success = resp.GetSuccess()
	result.Status = ragUploadStatusCompleted
	result.Document = resp.GetDocument()
	result.ChunkCount = resp.GetChunkCount()
	result.EntityCount = resp.GetEntityCount()
	result.RelationCount = resp.GetRelationCount()
	result.Msg = resp.GetMsg()
	return result
}

// Search 执行 Adaptive、Hybrid 和 GraphRAG 路由检索。
func (h *RAGHandler) Search(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req ragSearchReq
	if err := bindRAGJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		response.BadRequest(c, "问题不能为空")
		return
	}
	resp, err := h.svc.Search(ctx, userID, ragclient.SearchInput{
		Query:          req.Query,
		Mode:           req.Mode,
		Limit:          int(parseRAGNumberOrDefault(req.Limit, 8)),
		GroupID:        parseRAGNumberOrZero(req.GroupID),
		ConversationID: parseRAGNumberOrZero(req.ConversationID),
		DocumentID:     parseRAGNumberOrZero(req.DocumentID),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteDocument 删除当前用户拥有的知识文档、分块和该文档图谱。
func (h *RAGHandler) DeleteDocument(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	documentID := parseRAGInt64(c.Param("id"), 0)
	if documentID <= 0 {
		response.BadRequest(c, "文档ID无效")
		return
	}
	resp, err := h.svc.DeleteDocument(ctx, userID, documentID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteDocumentGraph 只删除某篇文档对应的知识图谱，保留文档与检索 chunk。
func (h *RAGHandler) DeleteDocumentGraph(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	documentID := parseRAGInt64(c.Param("id"), 0)
	if documentID <= 0 {
		response.BadRequest(c, "文档ID无效")
		return
	}
	resp, err := h.svc.DeleteDocumentGraph(ctx, userID, documentID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetGraph 返回知识图谱节点、边和社区摘要。
func (h *RAGHandler) GetGraph(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "80"), 10, 64)
	resp, err := h.svc.GetGraph(ctx, userID, ragclient.GraphInput{
		Query:      strings.TrimSpace(c.DefaultQuery("query", "")),
		Limit:      int(limit),
		DocumentID: parseRAGInt64(c.DefaultQuery("document_id", "0"), 0),
		Hops:       int(parseRAGInt64(c.DefaultQuery("hops", "1"), 1)),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListDocuments 返回当前用户可见的知识文档。
func (h *RAGHandler) ListDocuments(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	resp, err := h.svc.ListDocuments(ctx, userID, int(limit), int(offset))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, resp)
}

type ragIngestReq struct {
	Title          string      `json:"title"`
	Content        string      `json:"content"`
	Source         string      `json:"source"`
	SourceType     string      `json:"source_type"`
	Visibility     string      `json:"visibility"`
	GroupID        json.Number `json:"group_id"`
	ConversationID json.Number `json:"conversation_id"`
}

type ragSearchReq struct {
	Query          string      `json:"query"`
	Mode           string      `json:"mode"`
	Limit          json.Number `json:"limit"`
	GroupID        json.Number `json:"group_id"`
	ConversationID json.Number `json:"conversation_id"`
	DocumentID     json.Number `json:"document_id"`
}

type ragUploadResult struct {
	Success       bool        `json:"success"`
	Status        string      `json:"status"`
	FileName      string      `json:"file_name"`
	Document      interface{} `json:"document,omitempty"`
	ChunkCount    int64       `json:"chunk_count"`
	EntityCount   int64       `json:"entity_count"`
	RelationCount int64       `json:"relation_count"`
	Msg           string      `json:"msg"`
}

type ragUploadWorkItem struct {
	FileName    string
	ContentType string
	Data        []byte
}

type ragUploadOptions struct {
	Title          string
	Visibility     string
	GroupID        int64
	ConversationID int64
}

type ragUploadJob struct {
	ID          int64              `json:"id"`
	UserID      int64              `json:"user_id"`
	Status      string             `json:"status"`
	Msg         string             `json:"msg"`
	Files       []*ragUploadResult `json:"files"`
	Total       int                `json:"total"`
	Completed   int                `json:"completed"`
	Failed      int                `json:"failed"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
	CompletedAt string             `json:"completed_at,omitempty"`
}

type ragUploadJobStore struct {
	mu   sync.RWMutex
	jobs map[int64]*ragUploadJob
}

func newRAGUploadJobStore() *ragUploadJobStore {
	return &ragUploadJobStore{jobs: map[int64]*ragUploadJob{}}
}

func (s *ragUploadJobStore) create(userID int64, files []*ragUploadResult) int64 {
	jobID, err := idgen.NextID()
	if err != nil {
		jobID = time.Now().UnixNano()
	}
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[jobID] = &ragUploadJob{
		ID:        jobID,
		UserID:    userID,
		Status:    ragUploadStatusPending,
		Files:     files,
		Total:     len(files),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.cleanupLocked()
	return jobID
}

func (s *ragUploadJobStore) get(jobID, userID int64) (*ragUploadJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok || job.UserID != userID {
		return nil, false
	}
	cp := *job
	cp.Files = make([]*ragUploadResult, 0, len(job.Files))
	for _, file := range job.Files {
		if file == nil {
			continue
		}
		item := *file
		cp.Files = append(cp.Files, &item)
	}
	return &cp, true
}

func (s *ragUploadJobStore) setStatus(jobID int64, status, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.jobs[jobID]; job != nil {
		job.Status = status
		job.Msg = msg
		job.UpdatedAt = time.Now().Format(time.RFC3339)
	}
}

func (s *ragUploadJobStore) updateFile(jobID int64, index int, fn func(*ragUploadResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil || index < 0 || index >= len(job.Files) || job.Files[index] == nil {
		return
	}
	fn(job.Files[index])
	job.UpdatedAt = time.Now().Format(time.RFC3339)
	s.recountLocked(job)
}

func (s *ragUploadJobStore) replaceFile(jobID int64, index int, result *ragUploadResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil || index < 0 || index >= len(job.Files) {
		return
	}
	job.Files[index] = result
	job.UpdatedAt = time.Now().Format(time.RFC3339)
	s.recountLocked(job)
}

func (s *ragUploadJobStore) finish(jobID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return
	}
	s.recountLocked(job)
	if job.Failed > 0 && job.Completed == 0 {
		job.Status = ragUploadStatusFailed
		job.Msg = "所有文件处理失败"
	} else {
		job.Status = ragUploadStatusCompleted
		job.Msg = "文件处理完成"
	}
	now := time.Now().Format(time.RFC3339)
	job.UpdatedAt = now
	job.CompletedAt = now
}

func (s *ragUploadJobStore) recountLocked(job *ragUploadJob) {
	job.Completed = 0
	job.Failed = 0
	for _, file := range job.Files {
		if file == nil {
			continue
		}
		switch file.Status {
		case ragUploadStatusCompleted:
			job.Completed++
		case ragUploadStatusFailed:
			job.Failed++
		}
	}
}

func (s *ragUploadJobStore) cleanupLocked() {
	cutoff := time.Now().Add(-6 * time.Hour)
	for id, job := range s.jobs {
		if parsed, err := time.Parse(time.RFC3339, job.UpdatedAt); err == nil && parsed.Before(cutoff) {
			delete(s.jobs, id)
		}
	}
}

func bindRAGJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseRAGNumberOrZero(value json.Number) int64 {
	return parseRAGNumberOrDefault(value, 0)
}

func parseRAGNumberOrDefault(value json.Number, fallback int64) int64 {
	if strings.TrimSpace(value.String()) == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultRAGString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseRAGIntString(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func parseRAGInt64(value string, fallback int64) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
