// Package websearchclient 定义其他服务调用 web-search-service 的稳定客户端契约。
package websearchclient

import "context"

// Source 表示一次搜索增强返回的网页来源。
type Source struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Snippet     string   `json:"snippet"`
	Source      string   `json:"source"`
	Trusted     bool     `json:"trusted"`
	Score       float64  `json:"score"`
	FetchStatus string   `json:"fetch_status"`
	Passages    []string `json:"passages"`
}

// SearchInput 表示只搜索、不抓取正文的请求。
type SearchInput struct {
	Query string
	Limit int
}

// SearchResult 表示纯搜索响应。
type SearchResult struct {
	Success bool     `json:"success"`
	Query   string   `json:"query"`
	Results []Source `json:"results"`
	Msg     string   `json:"msg"`
}

// AugmentInput 表示轻量 Web Search Augmentation 请求。
type AugmentInput struct {
	Query       string
	Limit       int
	MaxFetch    int
	MaxPassages int
}

// AugmentResult 表示搜索增强响应。
type AugmentResult struct {
	Success       bool     `json:"success"`
	Query         string   `json:"query"`
	AnswerContext string   `json:"answer_context"`
	Sources       []Source `json:"sources"`
	SearchTime    string   `json:"search_time"`
	Msg           string   `json:"msg"`
}

// Service 是 api-gateway、Agent 服务调用 web-search-service 的最小接口。
type Service interface {
	Search(ctx context.Context, input SearchInput) (SearchResult, error)
	Augment(ctx context.Context, input AugmentInput) (AugmentResult, error)
}
