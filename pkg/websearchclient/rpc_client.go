package websearchclient

import (
	"ClaranAIM/kitex_gen/web_search"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"context"
	"errors"
)

// RPCClient 使用 Kitex 调用 web-search-service。
type RPCClient struct {
	client websearchservice.Client
}

// NewRPCClient 包装已经创建好的 Kitex 客户端。
func NewRPCClient(client websearchservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

// Search 执行纯搜索，不抓取网页正文。
func (c *RPCClient) Search(ctx context.Context, input SearchInput) (SearchResult, error) {
	resp, err := c.client.Search(ctx, &web_search.SearchReq{Query: input.Query, Limit: int64(input.Limit)})
	if err != nil {
		return SearchResult{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Success: resp.GetSuccess(), Query: resp.GetQuery(), Results: toClientSources(resp.GetResults()), Msg: resp.GetMsg()}, nil
}

// Augment 执行一次性 Web Search Augmentation。
func (c *RPCClient) Augment(ctx context.Context, input AugmentInput) (AugmentResult, error) {
	resp, err := c.client.Augment(ctx, &web_search.AugmentReq{
		Query:       input.Query,
		Limit:       int64(input.Limit),
		MaxFetch:    int64(input.MaxFetch),
		MaxPassages: int64(input.MaxPassages),
	})
	if err != nil {
		return AugmentResult{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return AugmentResult{}, err
	}
	return AugmentResult{
		Success:       resp.GetSuccess(),
		Query:         resp.GetQuery(),
		AnswerContext: resp.GetAnswerContext(),
		Sources:       toClientSources(resp.GetSources()),
		SearchTime:    resp.GetSearchTime(),
		Msg:           resp.GetMsg(),
	}, nil
}

func toClientSources(sources []*web_search.WebSearchSource) []Source {
	out := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		out = append(out, Source{
			Title:       source.GetTitle(),
			URL:         source.GetUrl(),
			Snippet:     source.GetSnippet(),
			Source:      source.GetSource(),
			Trusted:     source.GetTrusted(),
			Score:       source.GetScore(),
			FetchStatus: source.GetFetchStatus(),
			Passages:    source.GetPassages(),
		})
	}
	return out
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	if msg == "" {
		msg = "web-search-service RPC调用失败"
	}
	return errors.New(msg)
}
