// Package service 实现一次请求内的 Web Search Augmentation。
//
// 这个服务不是完整 Web RAG：它不写向量库、不做长期索引，也不把网页内容沉淀到知识库。
// 它只在一次请求中完成搜索、抓正文、清洗、相关段落截取和来源整理，供 Agent 或网关把结果交给 LLM。
package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// WebSearchService 是 web-search-service 暴露的业务接口。
type WebSearchService interface {
	Search(ctx context.Context, input SearchInput) (SearchOutput, error)
	Augment(ctx context.Context, input AugmentInput) (AugmentOutput, error)
}

// SearchInput 表示只搜索、不抓正文的轻量请求。
type SearchInput struct {
	Query string
	Limit int
}

// AugmentInput 表示搜索增强请求。
type AugmentInput struct {
	Query       string
	Limit       int
	MaxFetch    int
	MaxPassages int
}

// SearchResult 是搜索引擎返回的一条候选网页。
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
	Source  string
}

// FetchedPage 保存单页抓取结果。
type FetchedPage struct {
	URL     string
	Content string
}

// WebSource 是最终返回给调用方展示和审计的来源。
type WebSource struct {
	Title       string
	URL         string
	Snippet     string
	Source      string
	Trusted     bool
	Score       float64
	FetchStatus string
	Passages    []string
}

// SearchOutput 是纯搜索接口响应。
type SearchOutput struct {
	Success bool
	Query   string
	Results []WebSource
	Msg     string
}

// AugmentOutput 是一次性 Web Search Augmentation 响应。
type AugmentOutput struct {
	Success       bool
	Query         string
	AnswerContext string
	Sources       []WebSource
	SearchTime    string
	Msg           string
}

// SearchProvider 抽象搜索源。默认实现按 DuckDuckGo HTML、SearxNG、Wikipedia 降级。
type SearchProvider interface {
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
}

// PageFetcher 抽象网页抓取，便于测试和替换抓取策略。
type PageFetcher interface {
	Fetch(ctx context.Context, targetURL string) (FetchedPage, error)
}

// WebSearchOptions 控制搜索增强的数量、可信域名和 HTTP 行为。
type WebSearchOptions struct {
	MaxResults      int
	MaxFetch        int
	MaxPassages     int
	MaxCharsPerPage int
	TrustedDomains  []string
	UserAgent       string
	Timeout         time.Duration
}

type webSearchServiceImpl struct {
	searcher SearchProvider
	fetcher  PageFetcher
	options  WebSearchOptions
}

// NewWebSearchService 创建默认 Web Search Augmentation 服务。
func NewWebSearchService(options WebSearchOptions) WebSearchService {
	options = normalizeOptions(options)
	client := &http.Client{Timeout: options.Timeout}
	return NewWebSearchServiceWithDeps(
		NewFallbackSearchProvider(client, options.UserAgent),
		NewHTTPPageFetcher(client, options.UserAgent, options.MaxCharsPerPage),
		options,
	)
}

// NewWebSearchServiceWithDeps 创建带依赖注入的服务，主要供测试或替换搜索源使用。
func NewWebSearchServiceWithDeps(searcher SearchProvider, fetcher PageFetcher, options WebSearchOptions) WebSearchService {
	options = normalizeOptions(options)
	if searcher == nil {
		searcher = NewFallbackSearchProvider(&http.Client{Timeout: options.Timeout}, options.UserAgent)
	}
	if fetcher == nil {
		fetcher = NewHTTPPageFetcher(&http.Client{Timeout: options.Timeout}, options.UserAgent, options.MaxCharsPerPage)
	}
	return &webSearchServiceImpl{searcher: searcher, fetcher: fetcher, options: options}
}

// Search 只返回搜索结果和可信源排序，不抓取网页正文。
func (s *webSearchServiceImpl) Search(ctx context.Context, input SearchInput) (SearchOutput, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return SearchOutput{}, errors.New("搜索关键词不能为空")
	}
	limit := normalizeLimit(input.Limit, s.options.MaxResults)
	searchCtx, cancel := context.WithTimeout(ctx, searchProviderBudget(s.options.Timeout))
	results, err := s.searcher.Search(searchCtx, query, limit)
	cancel()
	if err != nil {
		return SearchOutput{Success: true, Query: query, Results: nil, Msg: "搜索源暂时不可用：" + err.Error()}, nil
	}
	sources := make([]WebSource, 0, len(results))
	for _, result := range results {
		source := s.resultToSource(result, nil, "search_only", query)
		sources = append(sources, source)
	}
	sortSources(sources)
	if len(sources) > limit {
		sources = sources[:limit]
	}
	return SearchOutput{Success: true, Query: query, Results: sources}, nil
}

// Augment 执行轻量 Web Search Augmentation。
func (s *webSearchServiceImpl) Augment(ctx context.Context, input AugmentInput) (AugmentOutput, error) {
	start := time.Now()
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return AugmentOutput{}, errors.New("搜索问题不能为空")
	}
	limit := normalizeLimit(input.Limit, s.options.MaxResults)
	maxFetch := normalizeLimit(input.MaxFetch, s.options.MaxFetch)
	maxPassages := normalizeLimit(input.MaxPassages, s.options.MaxPassages)
	searchCtx, cancel := context.WithTimeout(ctx, searchProviderBudget(s.options.Timeout))
	results, err := s.searcher.Search(searchCtx, query, max(limit, maxFetch))
	cancel()
	if err != nil {
		return AugmentOutput{
			Success:       true,
			Query:         query,
			AnswerContext: "联网搜索增强暂时没有拿到可用搜索结果。请检查 web-search-service 网络连通性或稍后重试。用户问题：" + query,
			Sources:       nil,
			SearchTime:    time.Since(start).Round(time.Millisecond).String(),
			Msg:           "搜索源暂时不可用：" + err.Error(),
		}, nil
	}
	results = rankResults(results, query, s.options.TrustedDomains)
	if len(results) > maxFetch {
		results = results[:maxFetch]
	}
	sources := make([]WebSource, 0, len(results))
	deadline := time.Now().Add(s.options.Timeout)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline.Add(-500 * time.Millisecond)
	}
	for _, result := range results {
		if time.Now().After(deadline) {
			source := s.resultToSource(result, []string{strings.TrimSpace(result.Snippet)}, "deadline_snippet_fallback", query)
			sources = append(sources, source)
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, perPageFetchBudget(s.options.Timeout, time.Until(deadline), len(results)))
		page, fetchErr := s.fetcher.Fetch(fetchCtx, result.URL)
		cancel()
		if fetchErr != nil || strings.TrimSpace(page.Content) == "" {
			source := s.resultToSource(result, []string{strings.TrimSpace(result.Snippet)}, "snippet_fallback", query)
			if fetchErr != nil {
				source.FetchStatus = "snippet_fallback"
			}
			sources = append(sources, source)
			continue
		}
		cleaned := CleanWebText(page.Content)
		passages := ExtractRelevantPassages(query, cleaned, maxPassages)
		source := s.resultToSource(result, passages, "fetched", query)
		sources = append(sources, source)
	}
	sortSources(sources)
	sources = filterUsefulSources(sources, limit)
	if len(sources) == 0 && len(results) > 0 {
		for _, result := range results {
			source := s.resultToSource(result, []string{strings.TrimSpace(result.Snippet)}, "snippet_fallback", query)
			if len(source.Passages) == 0 && strings.TrimSpace(source.Snippet) != "" {
				source.Passages = []string{strings.TrimSpace(source.Snippet)}
			}
			sources = append(sources, source)
			if len(sources) >= limit {
				break
			}
		}
	}
	return AugmentOutput{
		Success:       true,
		Query:         query,
		AnswerContext: BuildAnswerContext(query, sources),
		Sources:       sources,
		SearchTime:    time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

func perPageFetchBudget(totalBudget, remain time.Duration, resultCount int) time.Duration {
	if totalBudget <= 0 {
		totalBudget = 10 * time.Second
	}
	if remain <= 0 {
		return 500 * time.Millisecond
	}
	if resultCount <= 0 {
		resultCount = 1
	}
	budget := remain / time.Duration(resultCount)
	if budget < 800*time.Millisecond {
		return 800 * time.Millisecond
	}
	if budget > 3500*time.Millisecond {
		return 3500 * time.Millisecond
	}
	return budget
}

func searchProviderBudget(totalBudget time.Duration) time.Duration {
	if totalBudget <= 0 {
		return 6 * time.Second
	}
	budget := totalBudget / 2
	if budget < 2500*time.Millisecond {
		return 2500 * time.Millisecond
	}
	if budget > 6*time.Second {
		return 6 * time.Second
	}
	return budget
}

func (s *webSearchServiceImpl) resultToSource(result SearchResult, passages []string, status, query string) WebSource {
	source := WebSource{
		Title:       strings.TrimSpace(result.Title),
		URL:         strings.TrimSpace(result.URL),
		Snippet:     strings.TrimSpace(result.Snippet),
		Source:      strings.TrimSpace(result.Source),
		Trusted:     isTrustedURL(result.URL, s.options.TrustedDomains),
		FetchStatus: status,
		Passages:    compactPassages(passages),
	}
	source.Score = scoreSource(source, query)
	return source
}

// BuildAnswerContext 把来源和相关段落整理成适合喂给 LLM 的临时上下文。
func BuildAnswerContext(query string, sources []WebSource) string {
	if len(sources) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("以下是一次性 Web Search Augmentation 检索到的临时网页资料。\n")
	b.WriteString("这些资料不是长期记忆；如果来源与当前问题无关，不要强行使用。\n")
	b.WriteString("用户问题：")
	b.WriteString(strings.TrimSpace(query))
	b.WriteString("\n\n")
	for i, source := range sources {
		if len(source.Passages) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("[%d] %s\nURL: %s\n", i+1, firstNonEmpty(source.Title, source.URL), source.URL))
		if source.Trusted {
			b.WriteString("可信度提示：命中可信/官方域名。\n")
		}
		for _, passage := range source.Passages {
			b.WriteString("- ")
			b.WriteString(passage)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// CleanWebText 清理网页正文中的脚本、样式、HTML 标签和多余空白。
func CleanWebText(raw string) string {
	text := raw
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<!--.*?-->`),
	} {
		text = re.ReplaceAllString(text, " ")
	}
	text = regexp.MustCompile(`(?is)<(p|div|br|li|h[1-6]|section|article|tr)[^>]*>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = regexp.MustCompile(`[ \t\r\f\v]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n\s*\n+`).ReplaceAllString(text, "\n")
	return strings.TrimSpace(text)
}

// ExtractRelevantPassages 通过词项重叠从正文中抽取相关段落。
func ExtractRelevantPassages(query, content string, limit int) []string {
	limit = normalizeLimit(limit, 3)
	paragraphs := splitParagraphs(content)
	queryTerms := tokenize(query)
	type scored struct {
		text  string
		score float64
		index int
	}
	rows := make([]scored, 0, len(paragraphs))
	for i, paragraph := range paragraphs {
		score := lexicalScore(queryTerms, paragraph)
		if score <= 0 {
			continue
		}
		rows = append(rows, scored{text: paragraph, score: score, index: i})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score == rows[j].score {
			return rows[i].index < rows[j].index
		}
		return rows[i].score > rows[j].score
	})
	out := make([]string, 0, limit)
	for _, row := range rows {
		out = append(out, truncateText(row.text, 600))
		if len(out) >= limit {
			break
		}
	}
	return out
}

type fallbackSearchProvider struct {
	client    *http.Client
	userAgent string
}

// NewFallbackSearchProvider 创建默认搜索源，按 DuckDuckGo HTML、SearxNG、Wikipedia 降级。
func NewFallbackSearchProvider(client *http.Client, userAgent string) SearchProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &fallbackSearchProvider{client: client, userAgent: userAgent}
}

func (p *fallbackSearchProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if results, err := p.searchDuckDuckGo(ctx, query, limit); err == nil && len(results) > 0 {
		return results, nil
	}
	if results, err := p.searchSearxNG(ctx, query, limit); err == nil && len(results) > 0 {
		return results, nil
	}
	if results, err := p.searchWikipedia(ctx, query, limit); err == nil && len(results) > 0 {
		return results, nil
	}
	return nil, errors.New("未找到可用搜索结果")
}

func (p *fallbackSearchProvider) searchDuckDuckGo(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	body, err := p.get(ctx, searchURL)
	if err != nil {
		return nil, err
	}
	return parseDuckDuckGoHTML(body, limit), nil
}

func (p *fallbackSearchProvider) searchSearxNG(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	searchURL := "https://searx.be/search?q=" + url.QueryEscape(query) + "&format=json"
	body, err := p.get(ctx, searchURL)
	if err != nil {
		return nil, err
	}
	return parseSearxJSON(body, limit), nil
}

func (p *fallbackSearchProvider) searchWikipedia(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	searchURL := "https://en.wikipedia.org/w/api.php?action=query&list=search&format=json&srsearch=" + url.QueryEscape(query)
	body, err := p.get(ctx, searchURL)
	if err != nil {
		return nil, err
	}
	return parseWikipediaJSON(body, limit), nil
}

func (p *fallbackSearchProvider) get(ctx context.Context, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "text/html,application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("搜索请求状态码%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type httpPageFetcher struct {
	client       *http.Client
	userAgent    string
	maxChars     int
	allowedTypes []string
}

// NewHTTPPageFetcher 创建网页正文抓取器。
func NewHTTPPageFetcher(client *http.Client, userAgent string, maxChars int) PageFetcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if maxChars <= 0 {
		maxChars = 12000
	}
	return &httpPageFetcher{client: client, userAgent: userAgent, maxChars: maxChars}
}

func (f *httpPageFetcher) Fetch(ctx context.Context, targetURL string) (FetchedPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return FetchedPage{}, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,text/plain,application/xhtml+xml")
	resp, err := f.client.Do(req)
	if err != nil {
		return FetchedPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchedPage{}, fmt.Errorf("网页抓取状态码%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(f.maxChars)+1))
	if err != nil {
		return FetchedPage{}, err
	}
	return FetchedPage{URL: targetURL, Content: string(data)}, nil
}

func parseDuckDuckGoHTML(raw string, limit int) []SearchResult {
	linkRe := regexp.MustCompile(`(?is)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	matches := linkRe.FindAllStringSubmatch(raw, limit)
	results := make([]SearchResult, 0, len(matches))
	for i, match := range matches {
		link := cleanDuckDuckGoURL(html.UnescapeString(match[1]))
		title := CleanWebText(match[2])
		if link == "" || title == "" {
			continue
		}
		results = append(results, SearchResult{Title: title, URL: link, Snippet: duckSnippet(raw, i), Source: "duckduckgo"})
	}
	return results
}

func parseSearxJSON(raw string, limit int) []SearchResult {
	itemRe := regexp.MustCompile(`(?is)\{[^{}]*"url"\s*:\s*"([^"]+)"[^{}]*"title"\s*:\s*"([^"]*)"[^{}]*?(?:"content"\s*:\s*"([^"]*)")?[^{}]*\}`)
	matches := itemRe.FindAllStringSubmatch(raw, limit)
	results := make([]SearchResult, 0, len(matches))
	for _, match := range matches {
		results = append(results, SearchResult{
			Title:   cleanJSONText(match[2]),
			URL:     cleanJSONText(match[1]),
			Snippet: cleanJSONText(match[3]),
			Source:  "searxng",
		})
	}
	return results
}

func parseWikipediaJSON(raw string, limit int) []SearchResult {
	itemRe := regexp.MustCompile(`(?is)"title"\s*:\s*"([^"]+)".*?"snippet"\s*:\s*"([^"]*)"`)
	matches := itemRe.FindAllStringSubmatch(raw, limit)
	results := make([]SearchResult, 0, len(matches))
	for _, match := range matches {
		title := cleanJSONText(match[1])
		results = append(results, SearchResult{
			Title:   title,
			URL:     "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(url.PathEscape(title), "%20", "_"),
			Snippet: CleanWebText(cleanJSONText(match[2])),
			Source:  "wikipedia",
		})
	}
	return results
}

func duckSnippet(raw string, index int) string {
	re := regexp.MustCompile(`(?is)<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(raw, -1)
	if index >= 0 && index < len(matches) {
		return CleanWebText(matches[index][1])
	}
	return ""
}

func cleanDuckDuckGoURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil {
		if uddg := parsed.Query().Get("uddg"); uddg != "" {
			return uddg
		}
	}
	return raw
}

func cleanJSONText(raw string) string {
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, `\"`, `"`)
	raw = strings.ReplaceAll(raw, `\n`, " ")
	return html.UnescapeString(raw)
}

func normalizeOptions(options WebSearchOptions) WebSearchOptions {
	if options.MaxResults <= 0 {
		options.MaxResults = 5
	}
	if options.MaxFetch <= 0 {
		options.MaxFetch = 5
	}
	if options.MaxPassages <= 0 {
		options.MaxPassages = 3
	}
	if options.MaxCharsPerPage <= 0 {
		options.MaxCharsPerPage = 12000
	}
	if options.Timeout <= 0 {
		options.Timeout = 10 * time.Second
	}
	if strings.TrimSpace(options.UserAgent) == "" {
		options.UserAgent = "ClaranAIM-WebSearch/1.0"
	}
	return options
}

func normalizeLimit(value, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value <= 0 {
		return 1
	}
	if value > 20 {
		return 20
	}
	return value
}

func rankResults(results []SearchResult, query string, trustedDomains []string) []SearchResult {
	out := append([]SearchResult(nil), results...)
	sort.SliceStable(out, func(i, j int) bool {
		left := scoreSearchResult(out[i], query, trustedDomains)
		right := scoreSearchResult(out[j], query, trustedDomains)
		return left > right
	})
	return out
}

func scoreSearchResult(result SearchResult, query string, trustedDomains []string) float64 {
	score := lexicalScore(tokenize(query), result.Title+" "+result.Snippet)
	if isTrustedURL(result.URL, trustedDomains) {
		score += 1.5
	}
	return score
}

func scoreSource(source WebSource, query string) float64 {
	text := source.Title + " " + source.Snippet + " " + strings.Join(source.Passages, " ")
	score := lexicalScore(tokenize(query), text)
	if source.Trusted {
		score += 1.5
	}
	if source.FetchStatus == "fetched" && len(source.Passages) > 0 {
		score += 0.5
	}
	return math.Round(score*1000) / 1000
}

func sortSources(sources []WebSource) {
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Score > sources[j].Score
	})
}

func filterUsefulSources(sources []WebSource, limit int) []WebSource {
	out := make([]WebSource, 0, min(limit, len(sources)))
	for _, source := range sources {
		if len(source.Passages) == 0 {
			continue
		}
		if source.Score <= 0 && !source.Trusted {
			continue
		}
		out = append(out, source)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func isTrustedURL(raw string, domains []string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func splitParagraphs(content string) []string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len([]rune(line)) < 12 {
			continue
		}
		out = append(out, line)
	}
	return out
}

func tokenize(text string) []string {
	raw := regexp.MustCompile(`[^\p{L}\p{N}_]+`).Split(strings.ToLower(text), -1)
	terms := make([]string, 0, len(raw))
	for _, term := range raw {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		terms = append(terms, term)
	}
	return terms
}

func lexicalScore(terms []string, text string) float64 {
	if len(terms) == 0 || strings.TrimSpace(text) == "" {
		return 0
	}
	lower := strings.ToLower(text)
	var hits float64
	for _, term := range terms {
		if strings.Contains(lower, term) {
			hits++
		}
	}
	return hits / float64(len(terms))
}

func compactPassages(passages []string) []string {
	out := make([]string, 0, len(passages))
	seen := map[string]bool{}
	for _, passage := range passages {
		passage = strings.TrimSpace(passage)
		if passage == "" || seen[passage] {
			continue
		}
		seen[passage] = true
		out = append(out, passage)
	}
	return out
}

func truncateText(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
