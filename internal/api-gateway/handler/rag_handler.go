package handler

import (
	"ClaranAIM/pkg/documentparser"
	"ClaranAIM/pkg/ragclient"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const maxRAGUploadBytes int64 = 20 << 20

// RAGHandler 暴露知识库入库、RAG 搜索和知识图谱查询接口。
type RAGHandler struct {
	svc ragclient.Service
}

// gatewayRAGService 是 api-gateway 到 rag-service 的 RPC 门面。
var gatewayRAGService ragclient.Service
var gatewayDocumentOCR documentparser.OCRProvider

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
	results := make([]*ragUploadResult, 0, len(headers))
	for _, header := range headers {
		result := h.handleOneRAGUpload(ctx, userID, header, c)
		results = append(results, result)
	}
	response.Success(c, map[string]interface{}{"success": true, "files": results})
}

func (h *RAGHandler) handleOneRAGUpload(ctx context.Context, userID int64, header *multipart.FileHeader, c *app.RequestContext) *ragUploadResult {
	result := &ragUploadResult{FileName: header.Filename}
	if header.Size > maxRAGUploadBytes {
		result.Msg = "文件不能超过20MB"
		return result
	}
	file, err := header.Open()
	if err != nil {
		result.Msg = "读取文件失败"
		return result
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxRAGUploadBytes+1))
	_ = file.Close()
	if readErr != nil {
		result.Msg = "读取文件失败"
		return result
	}
	if int64(len(data)) > maxRAGUploadBytes {
		result.Msg = "文件不能超过20MB"
		return result
	}
	parsed, err := documentparser.ParseWithOptions(ctx, header.Filename, header.Header.Get("Content-Type"), data, documentparser.ParseOptions{
		OCR: gatewayDocumentOCR,
	})
	if err != nil {
		result.Msg = err.Error()
		return result
	}
	title := strings.TrimSpace(string(c.FormValue("title")))
	if title == "" {
		title = parsed.Title
	}
	resp, err := h.svc.IngestDocument(ctx, userID, ragclient.IngestInput{
		Title:          title,
		Content:        parsed.Content,
		Source:         header.Filename,
		SourceType:     parsed.SourceType,
		Visibility:     defaultRAGString(string(c.FormValue("visibility")), ragclient.VisibilityPrivate),
		GroupID:        parseRAGIntString(string(c.FormValue("group_id"))),
		ConversationID: parseRAGIntString(string(c.FormValue("conversation_id"))),
	})
	if err != nil {
		result.Msg = err.Error()
		return result
	}
	result.Success = resp.GetSuccess()
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
	})
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
		Query: strings.TrimSpace(c.DefaultQuery("query", "")),
		Limit: int(limit),
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
}

type ragUploadResult struct {
	Success       bool        `json:"success"`
	FileName      string      `json:"file_name"`
	Document      interface{} `json:"document,omitempty"`
	ChunkCount    int64       `json:"chunk_count"`
	EntityCount   int64       `json:"entity_count"`
	RelationCount int64       `json:"relation_count"`
	Msg           string      `json:"msg"`
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
