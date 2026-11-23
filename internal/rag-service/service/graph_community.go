package service

import (
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/pkg/idgen"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *ragServiceImpl) rebuildGraphCommunities(ctx context.Context, ownerID int64) {
	if s == nil || s.graphStore == nil || ownerID <= 0 {
		return
	}
	entities, relations, err := s.graphStore.ListOwnerGraph(ctx, ownerID)
	if err != nil || len(entities) == 0 {
		return
	}
	assignments := leidenLikeCommunities(entities, relations)
	communities := s.buildCommunityModels(ctx, ownerID, entities, relations, assignments)
	entityCommunity := map[int64]int64{}
	idx := 0
	for _, community := range communities {
		for _, entityID := range communityEntityIDs(assignments, idx) {
			entityCommunity[entityID] = community.ID
		}
		idx++
	}
	_ = s.graphStore.ReplaceOwnerCommunities(ctx, ownerID, communities, entityCommunity)
}

func leidenLikeCommunities(entities []model.Entity, relations []model.Relation) map[int64]int {
	if len(entities) == 0 {
		return map[int64]int{}
	}
	nodeSet := map[int64]bool{}
	for idx, entity := range entities {
		nodeSet[entity.ID] = true
		_ = idx
	}
	graph := weightedAdjacency(nodeSet, relations)
	assignments := strongEdgeComponents(entities, relations, graph)
	for iter := 0; iter < 8; iter++ {
		moved := false
		ordered := append([]model.Entity(nil), entities...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Score > ordered[j].Score })
		for _, entity := range ordered {
			current := assignments[entity.ID]
			bestCommunity := current
			bestGain := 0.0
			neighborCommunityWeight := map[int]float64{}
			for neighborID, weight := range graph[entity.ID] {
				neighborCommunityWeight[assignments[neighborID]] += weight
			}
			for community, weight := range neighborCommunityWeight {
				if community == current {
					continue
				}
				gain := weight - 0.05*communitySize(assignments, community)
				if gain > bestGain {
					bestGain = gain
					bestCommunity = community
				}
			}
			if bestCommunity != current && bestGain > 0 {
				assignments[entity.ID] = bestCommunity
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	return compactCommunityIDs(refineDisconnectedCommunities(assignments, graph))
}

func strongEdgeComponents(entities []model.Entity, relations []model.Relation, graph map[int64]map[int64]float64) map[int64]int {
	parent := map[int64]int64{}
	for _, entity := range entities {
		parent[entity.ID] = entity.ID
	}
	var find func(int64) int64
	find = func(id int64) int64 {
		if parent[id] != id {
			parent[id] = find(parent[id])
		}
		return parent[id]
	}
	union := func(a, b int64) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	threshold := strongEdgeThreshold(relations)
	for sourceID, neighbors := range graph {
		for targetID, weight := range neighbors {
			if sourceID < targetID && weight >= threshold {
				union(sourceID, targetID)
			}
		}
	}
	roots := map[int64]int{}
	next := 0
	assignments := map[int64]int{}
	for _, entity := range entities {
		root := find(entity.ID)
		if _, ok := roots[root]; !ok {
			roots[root] = next
			next++
		}
		assignments[entity.ID] = roots[root]
	}
	return assignments
}

func strongEdgeThreshold(relations []model.Relation) float64 {
	if len(relations) == 0 {
		return 0.15
	}
	total := 0.0
	count := 0
	for _, relation := range relations {
		weight := relation.Weight
		if weight <= 0 {
			weight = relation.Confidence
		}
		if weight <= 0 {
			weight = 0.5
		}
		total += weight
		count++
	}
	avg := total / float64(count)
	threshold := avg * 0.25
	if threshold < 0.15 {
		return 0.15
	}
	if threshold > 0.6 {
		return 0.6
	}
	return threshold
}

func weightedAdjacency(nodeSet map[int64]bool, relations []model.Relation) map[int64]map[int64]float64 {
	graph := map[int64]map[int64]float64{}
	for id := range nodeSet {
		graph[id] = map[int64]float64{}
	}
	for _, relation := range relations {
		if !nodeSet[relation.SourceID] || !nodeSet[relation.TargetID] || relation.SourceID == relation.TargetID {
			continue
		}
		weight := relation.Weight
		if weight <= 0 {
			weight = relation.Confidence
		}
		if weight <= 0 {
			weight = 0.5
		}
		graph[relation.SourceID][relation.TargetID] += weight
		graph[relation.TargetID][relation.SourceID] += weight
	}
	return graph
}

func communitySize(assignments map[int64]int, community int) float64 {
	count := 0
	for _, value := range assignments {
		if value == community {
			count++
		}
	}
	return float64(count)
}

func refineDisconnectedCommunities(assignments map[int64]int, graph map[int64]map[int64]float64) map[int64]int {
	nextCommunity := 0
	for _, community := range assignments {
		if community >= nextCommunity {
			nextCommunity = community + 1
		}
	}
	refined := map[int64]int{}
	visited := map[int64]bool{}
	communityNodes := map[int][]int64{}
	for nodeID, community := range assignments {
		communityNodes[community] = append(communityNodes[community], nodeID)
	}
	for community, nodes := range communityNodes {
		nodeSet := map[int64]bool{}
		for _, nodeID := range nodes {
			nodeSet[nodeID] = true
		}
		componentIndex := 0
		for _, nodeID := range nodes {
			if visited[nodeID] {
				continue
			}
			targetCommunity := community
			if componentIndex > 0 {
				targetCommunity = nextCommunity
				nextCommunity++
			}
			componentIndex++
			queue := []int64{nodeID}
			visited[nodeID] = true
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				refined[current] = targetCommunity
				for neighborID := range graph[current] {
					if !nodeSet[neighborID] || visited[neighborID] {
						continue
					}
					visited[neighborID] = true
					queue = append(queue, neighborID)
				}
			}
		}
	}
	return refined
}

func compactCommunityIDs(assignments map[int64]int) map[int64]int {
	seen := map[int]int{}
	next := 0
	out := map[int64]int{}
	ids := make([]int64, 0, len(assignments))
	for id := range assignments {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		old := assignments[id]
		if _, ok := seen[old]; !ok {
			seen[old] = next
			next++
		}
		out[id] = seen[old]
	}
	return out
}

type graphCommunitySummaryInput struct {
	CommunityIndex int
	Entities       []model.Entity
	Relations      []model.Relation
}

type graphCommunitySummary struct {
	Title   string
	Summary string
}

func (s *ragServiceImpl) buildCommunityModels(ctx context.Context, ownerID int64, entities []model.Entity, relations []model.Relation, assignments map[int64]int) []model.Community {
	grouped := map[int][]model.Entity{}
	for _, entity := range entities {
		grouped[assignments[entity.ID]] = append(grouped[assignments[entity.ID]], entity)
	}
	indexes := make([]int, 0, len(grouped))
	for index := range grouped {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	summarizer := s.graphSummarizer
	if summarizer == nil {
		summarizer = ruleGraphCommunitySummarizer{}
	}
	out := make([]model.Community, 0, len(indexes))
	for _, index := range indexes {
		items := grouped[index]
		sort.Slice(items, func(i, j int) bool {
			if items[i].Score == items[j].Score {
				return items[i].Name < items[j].Name
			}
			return items[i].Score > items[j].Score
		})
		communityRelations := relationsForCommunity(relations, assignments, index)
		summary, err := summarizer.Summarize(ctx, graphCommunitySummaryInput{CommunityIndex: index, Entities: items, Relations: communityRelations})
		if err != nil || strings.TrimSpace(summary.Summary) == "" {
			summary, _ = ruleGraphCommunitySummarizer{}.Summarize(ctx, graphCommunitySummaryInput{CommunityIndex: index, Entities: items, Relations: communityRelations})
		}
		communityID, _ := idgen.NextID()
		if communityID == 0 {
			communityID = int64(index + 1)
		}
		out = append(out, model.Community{
			ID:              communityID,
			OwnerID:         ownerID,
			Name:            defaultString(summary.Title, communityTitle(items)),
			Summary:         summary.Summary,
			KeyEntitiesJSON: encodeCommunityEntityNames(items),
			Level:           1,
		})
	}
	return out
}

func communityEntityIDs(assignments map[int64]int, community int) []int64 {
	var ids []int64
	for id, value := range assignments {
		if value == community {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func relationsForCommunity(relations []model.Relation, assignments map[int64]int, community int) []model.Relation {
	out := make([]model.Relation, 0)
	for _, relation := range relations {
		if assignments[relation.SourceID] == community && assignments[relation.TargetID] == community {
			out = append(out, relation)
		}
	}
	return out
}

func encodeCommunityEntityNames(entities []model.Entity) string {
	names := make([]string, 0, minInt(len(entities), 12))
	for i, entity := range entities {
		if i >= 12 {
			break
		}
		names = append(names, entity.Name)
	}
	data, _ := json.Marshal(names)
	return string(data)
}

func communityTitle(entities []model.Entity) string {
	if len(entities) == 0 {
		return "知识社区"
	}
	typeCount := map[string]int{}
	for _, entity := range entities {
		typeCount[entity.Type]++
	}
	bestType := "主题"
	bestCount := -1
	for typ, count := range typeCount {
		if count > bestCount {
			bestType = typ
			bestCount = count
		}
	}
	return fmt.Sprintf("%s 社区", bestType)
}

type ruleGraphCommunitySummarizer struct{}

func (ruleGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	_ = ctx
	names := make([]string, 0, minInt(len(input.Entities), 8))
	for i, entity := range input.Entities {
		if i >= 8 {
			break
		}
		names = append(names, entity.Name)
	}
	title := communityTitle(input.Entities)
	summary := fmt.Sprintf("%s包含 %s 等实体，共 %d 个实体、%d 条内部关系。该社区用于 GraphRAG 查询时提供局部结构和证据入口。", title, strings.Join(names, "、"), len(input.Entities), len(input.Relations))
	return graphCommunitySummary{Title: title, Summary: summary}, nil
}

type fallbackGraphCommunitySummarizer struct {
	primary  graphCommunitySummarizer
	fallback graphCommunitySummarizer
}

func (s fallbackGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	if s.primary != nil {
		result, err := s.primary.Summarize(ctx, input)
		if err == nil && strings.TrimSpace(result.Summary) != "" {
			return result, nil
		}
	}
	if s.fallback == nil {
		return graphCommunitySummary{}, nil
	}
	return s.fallback.Summarize(ctx, input)
}

type LLMGraphCommunitySummarizer struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewLLMGraphCommunitySummarizer(apiKey, baseURL, model string) *LLMGraphCommunitySummarizer {
	return &LLMGraphCommunitySummarizer{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:   defaultString(model, "glm-4-flash"),
		Client:  &http.Client{Timeout: 16 * time.Second},
	}
}

func (s *LLMGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	if s == nil || s.APIKey == "" || s.BaseURL == "" {
		return graphCommunitySummary{}, errors.New("llm graph community summarizer未配置")
	}
	payload := map[string]interface{}{
		"model": s.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是GraphRAG community summarizer。只输出JSON，不要解释。根据社区实体和关系生成短标题和摘要。JSON字段: title, summary。摘要要说明该社区表达的系统链路、关键实体和关系用途。"},
			{"role": "user", "content": buildCommunitySummaryPrompt(input)},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return graphCommunitySummary{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return graphCommunitySummary{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return graphCommunitySummary{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return graphCommunitySummary{}, fmt.Errorf("llm graph community summarizer状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return graphCommunitySummary{}, err
	}
	if len(decoded.Choices) == 0 {
		return graphCommunitySummary{}, errors.New("llm graph community summarizer未返回结果")
	}
	return parseCommunitySummary(decoded.Choices[0].Message.Content)
}

func buildCommunitySummaryPrompt(input graphCommunitySummaryInput) string {
	var b strings.Builder
	b.WriteString("实体:\n")
	for _, entity := range input.Entities {
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", entity.Name, entity.Type, truncate(entity.Summary, 180)))
	}
	b.WriteString("\n关系:\n")
	for i, relation := range input.Relations {
		if i >= 30 {
			break
		}
		b.WriteString(fmt.Sprintf("- %d -> %d %s: %s\n", relation.SourceID, relation.TargetID, relation.Relation, truncate(relation.Description, 180)))
	}
	return b.String()
}

func parseCommunitySummary(content string) (graphCommunitySummary, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return graphCommunitySummary{}, err
	}
	if strings.TrimSpace(parsed.Summary) == "" {
		return graphCommunitySummary{}, errors.New("社区摘要为空")
	}
	return graphCommunitySummary{Title: strings.TrimSpace(parsed.Title), Summary: strings.TrimSpace(parsed.Summary)}, nil
}
