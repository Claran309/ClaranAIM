package service

import (
	"context"
	"strings"
	"testing"
)

func TestAugmentSearchPrefersTrustedPagesAndExtractsRelevantPassages(t *testing.T) {
	svc := NewWebSearchServiceWithDeps(
		&fakeSearchProvider{results: []SearchResult{
			{Title: "个人博客介绍", URL: "https://random.example/post", Snippet: "泛泛提到了 ClaranAIM"},
			{Title: "官方文档", URL: "https://docs.example.com/claran/web-search", Snippet: "官方说明 web search augmentation"},
		}},
		&fakePageFetcher{pages: map[string]string{
			"https://random.example/post": `<html><body>这里只有很短的泛泛介绍。</body></html>`,
			"https://docs.example.com/claran/web-search": `<html><head><script>bad()</script></head><body>
				<h1>Web Search Augmentation</h1>
				<p>无关段落：这个页面先介绍项目背景。</p>
				<p>轻量 Web RAG 的流程是搜索、抓取网页正文、清洗正文、截取相关段落，然后给 LLM 回答。</p>
				<p>它不需要 Milvus、embedding、长期索引，只在一次请求内临时检索。</p>
			</body></html>`,
		}},
		WebSearchOptions{TrustedDomains: []string{"docs.example.com"}, MaxResults: 2, MaxFetch: 2, MaxPassages: 3, MaxCharsPerPage: 2000},
	)

	result, err := svc.Augment(context.Background(), AugmentInput{Query: "轻量 Web RAG 怎么做", Limit: 2})
	if err != nil {
		t.Fatalf("Augment returned error: %v", err)
	}
	if len(result.Sources) == 0 {
		t.Fatalf("expected sources")
	}
	if result.Sources[0].URL != "https://docs.example.com/claran/web-search" {
		t.Fatalf("expected trusted official source first, got %s", result.Sources[0].URL)
	}
	if !result.Sources[0].Trusted {
		t.Fatalf("expected first source to be trusted")
	}
	if strings.Contains(result.AnswerContext, "bad()") || strings.Contains(result.AnswerContext, "<script>") {
		t.Fatalf("expected cleaned context without scripts, got %q", result.AnswerContext)
	}
	if !strings.Contains(result.AnswerContext, "搜索、抓取网页正文、清洗正文、截取相关段落") {
		t.Fatalf("expected relevant passage in answer context, got %q", result.AnswerContext)
	}
	if strings.Contains(result.AnswerContext, "这里只有很短的泛泛介绍") {
		t.Fatalf("expected weak unrelated blog content to be filtered from context")
	}
}

func TestAugmentSearchFallsBackToSnippetWhenPageFetchFails(t *testing.T) {
	svc := NewWebSearchServiceWithDeps(
		&fakeSearchProvider{results: []SearchResult{
			{Title: "搜索结果摘要", URL: "https://example.com/missing", Snippet: "这个摘要解释了临时检索，不做长期索引。"},
		}},
		&fakePageFetcher{errors: map[string]error{"https://example.com/missing": errFakeFetch}},
		WebSearchOptions{MaxResults: 1, MaxFetch: 1, MaxPassages: 2},
	)

	result, err := svc.Augment(context.Background(), AugmentInput{Query: "临时检索 长期索引", Limit: 1})
	if err != nil {
		t.Fatalf("Augment returned error: %v", err)
	}
	if len(result.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(result.Sources))
	}
	if result.Sources[0].FetchStatus != "snippet_fallback" {
		t.Fatalf("expected snippet fallback, got %s", result.Sources[0].FetchStatus)
	}
	if !strings.Contains(result.AnswerContext, "这个摘要解释了临时检索") {
		t.Fatalf("expected snippet in context, got %q", result.AnswerContext)
	}
}

type fakeSearchProvider struct {
	results []SearchResult
	err     error
}

func (p *fakeSearchProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	_ = ctx
	_ = query
	_ = limit
	return p.results, p.err
}

type fakePageFetcher struct {
	pages  map[string]string
	errors map[string]error
}

func (f *fakePageFetcher) Fetch(ctx context.Context, targetURL string) (FetchedPage, error) {
	_ = ctx
	if err := f.errors[targetURL]; err != nil {
		return FetchedPage{}, err
	}
	return FetchedPage{URL: targetURL, Content: f.pages[targetURL]}, nil
}

type fakeFetchError string

func (e fakeFetchError) Error() string { return string(e) }

var errFakeFetch = fakeFetchError("fetch failed")
