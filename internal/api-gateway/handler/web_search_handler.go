package handler

import (
	"ClaranAIM/kitex_gen/web_search"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// WebSearchHandler 暴露一次性 Web Search Augmentation 接口。
type WebSearchHandler struct {
	svc websearchservice.Client
}

var gatewayWebSearchService websearchservice.Client

// InitWebSearchService 注册 web-search-service RPC 客户端。
func InitWebSearchService(svc websearchservice.Client) {
	gatewayWebSearchService = svc
}

// NewWebSearchHandler 创建 HTTP handler。
func NewWebSearchHandler() *WebSearchHandler {
	return &WebSearchHandler{svc: gatewayWebSearchService}
}

func (h *WebSearchHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "web-search-service未初始化")
		return false
	}
	return true
}

// Search 只返回搜索结果列表，不抓正文。
func (h *WebSearchHandler) Search(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	query := strings.TrimSpace(c.DefaultQuery("query", ""))
	if query == "" {
		response.BadRequest(c, "搜索关键词不能为空")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	result, err := h.svc.Search(ctx, &web_search.SearchReq{Query: query, Limit: int64(limit)})
	if err != nil || !result.GetSuccess() {
		if err == nil {
			err = webSearchStatusError(result.GetSuccess(), result.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}

// Augment 搜索、抓取网页正文、清洗并返回相关段落。
func (h *WebSearchHandler) Augment(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	var req webSearchAugmentReq
	if err := bindWebSearchJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		response.BadRequest(c, "搜索问题不能为空")
		return
	}
	result, err := h.svc.Augment(ctx, &web_search.AugmentReq{
		Query:       req.Query,
		Limit:       parseWebSearchNumberOrDefault(req.Limit, 5),
		MaxFetch:    parseWebSearchNumberOrDefault(req.MaxFetch, 5),
		MaxPassages: parseWebSearchNumberOrDefault(req.MaxPassages, 3),
	})
	if err != nil || !result.GetSuccess() {
		if err == nil {
			err = webSearchStatusError(result.GetSuccess(), result.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}

type webSearchAugmentReq struct {
	Query       string      `json:"query"`
	Limit       json.Number `json:"limit"`
	MaxFetch    json.Number `json:"max_fetch"`
	MaxPassages json.Number `json:"max_passages"`
}

func bindWebSearchJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseWebSearchNumberOrDefault(value json.Number, fallback int64) int64 {
	if strings.TrimSpace(value.String()) == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func webSearchStatusError(success bool, msg string) error {
	if success {
		return nil
	}
	if strings.TrimSpace(msg) == "" {
		msg = "web-search-service RPC调用失败"
	}
	return errors.New(msg)
}
