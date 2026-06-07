package logic

import (
	"ClaranAIM/kitex_gen/web_search"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"context"
	"fmt"
	"strings"
	"sync"
)

var (
	webSearchServiceMu sync.RWMutex
	webSearchService   websearchservice.Client
)

// SetWebSearchService 注入 web-search-service RPC 客户端。
func SetWebSearchService(svc websearchservice.Client) {
	webSearchServiceMu.Lock()
	defer webSearchServiceMu.Unlock()
	webSearchService = svc
}

// WebSearchParams 是 Agent 联网搜索增强工具的入参。
type WebSearchParams struct {
	Query       string `json:"query" jsonschema:"description=需要联网搜索的问题或关键词"`
	Limit       int    `json:"limit" jsonschema:"description=返回来源数量，建议 3 到 5；为空时使用 5"`
	MaxFetch    int    `json:"max_fetch" jsonschema:"description=最多抓取多少个网页正文，建议 3 到 5；为空时使用 5"`
	MaxPassages int    `json:"max_passages" jsonschema:"description=每个来源最多截取多少段相关正文，建议 2 到 3；为空时使用 3"`
}

// SearchWeb 调用 web-search-service 做一次性搜索增强。
func SearchWeb(ctx context.Context, input *WebSearchParams) (string, error) {
	if input == nil {
		input = &WebSearchParams{}
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return "联网搜索失败：query不能为空。", nil
	}
	webSearchServiceMu.RLock()
	svc := webSearchService
	webSearchServiceMu.RUnlock()
	if svc == nil {
		return "联网搜索不可用：agent-runtime-service 尚未连接 web-search-service。", nil
	}
	result, err := svc.Augment(ctx, &web_search.AugmentReq{
		Query:       query,
		Limit:       int64(input.Limit),
		MaxFetch:    int64(input.MaxFetch),
		MaxPassages: int64(input.MaxPassages),
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("## 联网搜索增强结果\n")
	b.WriteString(result.GetAnswerContext())
	b.WriteString("\n\n## 来源\n")
	if len(result.GetSources()) == 0 {
		b.WriteString("- 未找到可用网页来源。\n")
	} else {
		for i, source := range result.GetSources() {
			b.WriteString(fmt.Sprintf("- [%d] %s\n  URL: %s\n  trusted=%t score=%.3f status=%s\n", i+1, firstNonEmptyTool(source.GetTitle(), source.GetUrl()), source.GetUrl(), source.GetTrusted(), source.GetScore(), source.GetFetchStatus()))
		}
	}
	return b.String(), nil
}

func firstNonEmptyTool(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
