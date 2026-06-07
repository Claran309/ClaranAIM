package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"ClaranAIM/internal/rag-service/graphstore"
	"ClaranAIM/internal/rag-service/model"
)

func TestRuleGraphExtractorProducesSpecificRelations(t *testing.T) {
	text := `api-gateway 调用 rag-service 和 memory-service，并写入 message_user_states。rag-service 读取 rag_chunks，生成知识图谱。agent-runtime-service 消费 agent.events，并发布 mcp.events。agent-manager-service 依赖 settings-service。`

	entities := extractGraphEntities(text)
	relations := extractGraphRelationships(text, entities)
	types := map[string]int{}
	for _, relation := range relations {
		types[normalizeRelationType(relation.Type)]++
	}

	for _, want := range []string{"CALLS", "WRITES", "READS", "生成", "CONSUMES", "PUBLISHES", "DEPENDS_ON"} {
		if types[want] == 0 {
			t.Fatalf("expected relation type %s, got types=%v relations=%v entities=%v", want, sortedRelationTypes(types), relations, entities)
		}
	}
	if types["RELATED_TO"] >= len(relations) {
		t.Fatalf("expected specific relations, got only generic relations: %v", relations)
	}
}

func TestFallbackGraphExtractorMergesRuleRelations(t *testing.T) {
	text := `api-gateway 调用 rag-service 和 memory-service，并写入 message_user_states。rag-service 读取 rag_chunks，生成知识图谱。`
	primary := graphExtractResult{
		Entities: []extractedEntity{
			{Name: "api-gateway", Type: "服务"},
			{Name: "rag-service", Type: "服务"},
			{Name: "memory-service", Type: "服务"},
			{Name: "message_user_states", Type: "数据库表"},
			{Name: "rag_chunks", Type: "数据库表"},
			{Name: "知识图谱", Type: "技术概念"},
		},
	}
	extractor := fallbackGraphExtractor{
		primary:  staticGraphExtractor{result: primary},
		fallback: ruleGraphExtractor{},
	}
	result, err := extractor.Extract(context.Background(), graphExtractInput{
		Chunk: model.Chunk{Content: text},
	})
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	for _, relation := range result.Relationships {
		types[normalizeRelationType(relation.Type)]++
	}
	for _, want := range []string{"CALLS", "WRITES", "READS", "生成"} {
		if types[want] == 0 {
			t.Fatalf("expected merged fallback relation type %s, got types=%v relations=%v", want, sortedRelationTypes(types), result.Relationships)
		}
	}
}

func TestEnsureDisplayEntityRelationsDoesNotSynthesizeGenericEdges(t *testing.T) {
	entities := []model.Entity{
		{ID: 1, OwnerID: 7, Name: "api-gateway", Type: "服务"},
		{ID: 2, OwnerID: 7, Name: "rag-service", Type: "服务"},
	}

	relations := ensureDisplayEntityRelations(entities, nil, 99)

	if len(relations) != 0 {
		t.Fatalf("expected display layer not to synthesize generic edges, got %v", relations)
	}
}

func TestFilterGraphExtractResultKeepsFreeSpecificRelations(t *testing.T) {
	result := graphExtractResult{
		Entities: []extractedEntity{
			{Name: "知识图谱", Type: "技术概念", Description: "用于展示文档实体关系"},
			{Name: "关系筛选", Type: "交互能力", Description: "用于控制可视化范围"},
		},
		Relationships: []extractedRelationship{
			{
				Source:      "知识图谱",
				Target:      "关系筛选",
				Type:        "面向",
				Description: "知识图谱面向关系筛选提供可视化控制",
				Evidence:    "知识图谱面向关系筛选提供可视化控制。",
				Confidence:  0.72,
			},
		},
	}

	filtered := filterGraphExtractResult(result, "知识图谱面向关系筛选提供可视化控制。")

	if len(filtered.Relationships) != 1 {
		t.Fatalf("expected free specific relation to survive filtering, got %+v", filtered.Relationships)
	}
	if filtered.Relationships[0].Type != "面向" {
		t.Fatalf("expected relation type 面向, got %q", filtered.Relationships[0].Type)
	}
}

func TestFilterGraphExtractResultKeepsUsefulWeakSuffixEntitiesAndLowerConfidenceRelations(t *testing.T) {
	result := graphExtractResult{
		Entities: []extractedEntity{
			{Name: "订单信息", Type: "业务对象", Description: "订单的业务信息"},
			{Name: "风险规则", Type: "业务规则", Description: "识别订单风险的规则"},
		},
		Relationships: []extractedRelationship{
			{
				Source:      "风险规则",
				Target:      "订单信息",
				Type:        "校验",
				Description: "风险规则校验订单信息",
				Evidence:    "风险规则校验订单信息并生成审核结果。",
				Confidence:  0.5,
			},
		},
	}

	filtered := filterGraphExtractResult(result, "风险规则校验订单信息并生成审核结果。")

	if len(filtered.Entities) != 2 {
		t.Fatalf("expected useful weak-suffix entities to survive, got %+v", filtered.Entities)
	}
	if len(filtered.Relationships) != 1 {
		t.Fatalf("expected lower-confidence evidenced relation to survive, got %+v", filtered.Relationships)
	}
}

func TestRuleGraphExtractorKeepsMoreChineseBusinessEntitiesAndRelations(t *testing.T) {
	text := `上传流程使用权限规则，并面向关系筛选能力。检索策略约束召回策略。图谱页面展示关系标签。`

	entities := extractGraphEntities(text)
	names := map[string]bool{}
	for _, entity := range entities {
		names[entity.Name] = true
	}
	for _, want := range []string{"上传流程", "权限规则", "关系筛选能力", "检索策略", "召回策略", "图谱页面", "关系标签"} {
		if !names[want] {
			t.Fatalf("expected entity %s, got entities=%v", want, entities)
		}
	}

	relations := extractGraphRelationships(text, entities)
	types := map[string]int{}
	for _, relation := range relations {
		types[normalizeRelationType(relation.Type)]++
	}
	for _, want := range []string{"使用", "面向", "约束", "展示"} {
		if types[want] == 0 {
			t.Fatalf("expected relation type %s, got types=%v relations=%v", want, sortedRelationTypes(types), relations)
		}
	}
}

func TestFilterGraphExtractResultPreservesLLMFreeEntityAndRelationTypes(t *testing.T) {
	result := graphExtractResult{
		Entities: []extractedEntity{
			{
				Name:        "图谱上传质量保障策略",
				Type:        "由 LLM 自由命名且不受固定枚举约束的长业务策略类型",
				Description: "用于保障上传后图谱抽取质量",
			},
			{
				Name:        "关系标签可视化行为",
				Type:        "由 LLM 自由命名且不受固定枚举约束的长交互行为类型",
				Description: "用于展示关系标签的前端行为",
			},
		},
		Relationships: []extractedRelationship{
			{
				Source:      "图谱上传质量保障策略",
				Target:      "关系标签可视化行为",
				Type:        "由 LLM 自由命名且不受固定枚举约束的长关系类型：保障并驱动展示",
				Description: "图谱上传质量保障策略保障并驱动关系标签可视化行为",
				Evidence:    "图谱上传质量保障策略保障并驱动关系标签可视化行为。",
				Confidence:  0.7,
			},
		},
	}

	filtered := filterGraphExtractResult(result, "图谱上传质量保障策略保障并驱动关系标签可视化行为。")

	if len(filtered.Entities) != 2 {
		t.Fatalf("expected free entity types to survive, got %+v", filtered.Entities)
	}
	if filtered.Entities[0].Type != "由 LLM 自由命名且不受固定枚举约束的长业务策略类型" {
		t.Fatalf("expected free entity type to be preserved, got %q", filtered.Entities[0].Type)
	}
	if len(filtered.Relationships) != 1 {
		t.Fatalf("expected free relation type to survive, got %+v", filtered.Relationships)
	}
	if filtered.Relationships[0].Type != "由 LLM 自由命名且不受固定枚举约束的长关系类型：保障并驱动展示" {
		t.Fatalf("expected free relation type to be preserved, got %q", filtered.Relationships[0].Type)
	}
}

func TestParseGraphExtractResultPreservesLLMRelationTypeText(t *testing.T) {
	content := `{
		"entities": [
			{"name": "api-gateway", "type": "LLM 自由服务类型", "description": "入口服务", "aliases": []},
			{"name": "rag-service", "type": "LLM 自由 RAG 服务类型", "description": "检索增强服务", "aliases": []}
		],
		"relationships": [
			{"source": "api-gateway", "target": "rag-service", "type": "calls", "description": "api-gateway calls rag-service", "evidence": "api-gateway calls rag-service.", "confidence": 0.8}
		]
	}`

	result, err := parseGraphExtractResult(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relationships) != 1 {
		t.Fatalf("expected one relationship, got %+v", result.Relationships)
	}
	if result.Relationships[0].Type != "calls" {
		t.Fatalf("expected LLM relation type calls to be preserved, got %q", result.Relationships[0].Type)
	}
}

func TestLLMGraphExtractorRunsRelationSupplementWhenRelationsAreSparse(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entities\":[{\"name\":\"产品愿景\",\"type\":\"自由类型\",\"description\":\"产品方向\",\"aliases\":[]},{\"name\":\"用户需求\",\"type\":\"自由类型\",\"description\":\"用户诉求\",\"aliases\":[]},{\"name\":\"交付节奏\",\"type\":\"自由类型\",\"description\":\"交付安排\",\"aliases\":[]}],\"relationships\":[]}"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entities\":[{\"name\":\"产品愿景\",\"type\":\"自由类型\",\"description\":\"产品方向\",\"aliases\":[]},{\"name\":\"用户需求\",\"type\":\"自由类型\",\"description\":\"用户诉求\",\"aliases\":[]},{\"name\":\"交付节奏\",\"type\":\"自由类型\",\"description\":\"交付安排\",\"aliases\":[]}],\"relationships\":[{\"source\":\"产品愿景\",\"target\":\"用户需求\",\"type\":\"回应\",\"description\":\"产品愿景回应用户需求\",\"evidence\":\"产品愿景回应用户需求并影响交付节奏。\",\"confidence\":0.86},{\"source\":\"用户需求\",\"target\":\"交付节奏\",\"type\":\"影响\",\"description\":\"用户需求影响交付节奏\",\"evidence\":\"产品愿景回应用户需求并影响交付节奏。\",\"confidence\":0.84}]}"}}]}`))
	}))
	defer server.Close()

	extractor := NewLLMGraphExtractor("test-key", server.URL, "test-model")
	extractor.Client = server.Client()
	result, err := extractor.Extract(context.Background(), graphExtractInput{
		Chunk: model.Chunk{Content: "产品愿景回应用户需求并影响交付节奏。"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("expected sparse first pass to trigger a second relation extraction request, got %d requests", requestCount)
	}
	if len(result.Relationships) < 2 {
		t.Fatalf("expected supplemental LLM relationships to be merged, got %+v", result.Relationships)
	}
}

func TestLLMGraphExtractorRefinesGenericRelationsWithSupplement(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entities\":[{\"name\":\"产品愿景\",\"type\":\"自由类型\",\"description\":\"产品方向\",\"aliases\":[]},{\"name\":\"用户需求\",\"type\":\"自由类型\",\"description\":\"用户诉求\",\"aliases\":[]},{\"name\":\"交付节奏\",\"type\":\"自由类型\",\"description\":\"交付安排\",\"aliases\":[]}],\"relationships\":[{\"source\":\"产品愿景\",\"target\":\"用户需求\",\"type\":\"同章共现\",\"description\":\"产品愿景和用户需求在同一章节出现\",\"evidence\":\"产品愿景回应用户需求并影响交付节奏。\",\"confidence\":0.7},{\"source\":\"用户需求\",\"target\":\"交付节奏\",\"type\":\"同章共现\",\"description\":\"用户需求和交付节奏在同一章节出现\",\"evidence\":\"产品愿景回应用户需求并影响交付节奏。\",\"confidence\":0.7}]}"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"entities\":[{\"name\":\"产品愿景\",\"type\":\"自由类型\",\"description\":\"产品方向\",\"aliases\":[]},{\"name\":\"用户需求\",\"type\":\"自由类型\",\"description\":\"用户诉求\",\"aliases\":[]},{\"name\":\"交付节奏\",\"type\":\"自由类型\",\"description\":\"交付安排\",\"aliases\":[]}],\"relationships\":[{\"source\":\"产品愿景\",\"target\":\"用户需求\",\"type\":\"回应\",\"description\":\"产品愿景回应用户需求\",\"evidence\":\"产品愿景回应用户需求并影响交付节奏。\",\"confidence\":0.86},{\"source\":\"用户需求\",\"target\":\"交付节奏\",\"type\":\"影响\",\"description\":\"用户需求影响交付节奏\",\"evidence\":\"产品愿景回应用户需求并影响交付节奏。\",\"confidence\":0.84}]}"}}]}`))
	}))
	defer server.Close()

	extractor := NewLLMGraphExtractor("test-key", server.URL, "test-model")
	extractor.Client = server.Client()
	result, err := extractor.Extract(context.Background(), graphExtractInput{
		Chunk: model.Chunk{Content: "产品愿景回应用户需求并影响交付节奏。"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("expected generic first pass to trigger relation refinement, got %d requests", requestCount)
	}
	types := map[string]int{}
	for _, relation := range result.Relationships {
		types[relation.Type]++
	}
	if types["回应"] == 0 || types["影响"] == 0 {
		t.Fatalf("expected refined concrete relations to be merged, got types=%v relations=%v", types, result.Relationships)
	}
	if types["同章共现"] > 0 {
		t.Fatalf("expected refined relations to replace same-pair generic relations, got %v", result.Relationships)
	}
}

func TestBuildGraphFallsBackWhenConfiguredExtractorReturnsEmptyShortDocument(t *testing.T) {
	store := graphstore.NewMemoryStore()
	svc := &ragServiceImpl{
		graphStore:      store,
		graphExtractor:  staticGraphExtractor{result: graphExtractResult{}},
		graphSummarizer: ruleGraphCommunitySummarizer{},
		llmGraphEnabled: true,
	}
	chunks := []model.Chunk{
		{
			ID:           1,
			DocumentID:   88,
			ChunkLevel:   model.ChunkLevelParent,
			Content:      "api-gateway 调用 rag-service 和 memory-service，并写入 message_user_states。rag-service 读取 rag_chunks，生成知识图谱。",
			QualityScore: 1,
		},
	}

	entityCount, relationCount := svc.buildGraph(context.Background(), 7, 88, "GraphRAG 诊断", "upload", chunks, "balanced")
	entities, relations, err := store.ListOwnerGraph(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}

	if entityCount == 0 || relationCount == 0 {
		t.Fatalf("expected fallback graph to be persisted, got entityCount=%d relationCount=%d entities=%v relations=%v", entityCount, relationCount, entities, relations)
	}
	types := map[string]int{}
	for _, relation := range relations {
		types[normalizeRelationType(relation.Relation)]++
	}
	for _, want := range []string{"CALLS", "WRITES", "READS", "生成"} {
		if types[want] == 0 {
			t.Fatalf("expected fallback relation type %s, got types=%v relations=%v", want, sortedRelationTypes(types), relations)
		}
	}
}

func TestBuildGraphBackfillsSpecificRelationsWhenExtractorOnlyReturnsEntities(t *testing.T) {
	store := graphstore.NewMemoryStore()
	svc := &ragServiceImpl{
		graphStore: store,
		graphExtractor: staticGraphExtractor{result: graphExtractResult{
			Entities: []extractedEntity{
				{Name: "api-gateway", Type: "服务", Description: "API 网关服务"},
				{Name: "rag-service", Type: "服务", Description: "RAG 服务"},
				{Name: "memory-service", Type: "服务", Description: "记忆服务"},
				{Name: "message_user_states", Type: "数据库表", Description: "用户消息状态表"},
				{Name: "rag_chunks", Type: "数据库表", Description: "RAG 分块表"},
				{Name: "知识图谱", Type: "技术概念", Description: "GraphRAG 关系图谱"},
			},
		}},
		graphSummarizer: ruleGraphCommunitySummarizer{},
		llmGraphEnabled: true,
	}
	chunks := []model.Chunk{
		{
			ID:           2,
			DocumentID:   89,
			ChunkLevel:   model.ChunkLevelParent,
			Content:      "api-gateway 调用 rag-service 和 memory-service，并写入 message_user_states。rag-service 读取 rag_chunks，生成知识图谱。",
			QualityScore: 1,
		},
	}

	_, _ = svc.buildGraph(context.Background(), 7, 89, "GraphRAG 关系回填", "upload", chunks, "balanced")
	_, relations, err := store.ListOwnerGraph(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	for _, relation := range relations {
		types[normalizeRelationType(relation.Relation)]++
	}

	for _, want := range []string{"CALLS", "WRITES", "READS", "生成"} {
		if types[want] == 0 {
			t.Fatalf("expected backfilled relation type %s, got types=%v relations=%v", want, sortedRelationTypes(types), relations)
		}
	}
}

func TestBuildGraphCreatesVisibleEntityRelationsWhenExtractorReturnsOnlyLooseEntities(t *testing.T) {
	store := graphstore.NewMemoryStore()
	svc := &ragServiceImpl{
		graphStore: store,
		graphExtractor: staticGraphExtractor{result: graphExtractResult{
			Entities: []extractedEntity{
				{Name: "产品愿景", Type: "LLM 自由实体类型", Description: "产品希望达成的方向"},
				{Name: "用户需求", Type: "LLM 自由实体类型", Description: "用户在文档中表达的需求"},
				{Name: "交付节奏", Type: "LLM 自由实体类型", Description: "项目推进的节奏"},
			},
		}},
		graphSummarizer: ruleGraphCommunitySummarizer{},
		llmGraphEnabled: true,
	}
	chunks := []model.Chunk{
		{
			ID:           3,
			DocumentID:   90,
			ChunkLevel:   model.ChunkLevelParent,
			Content:      "本节围绕产品愿景、用户需求和交付节奏展开，说明团队接下来需要关注的几个方向。",
			QualityScore: 1,
		},
	}

	_, _ = svc.buildGraph(context.Background(), 7, 90, "GraphRAG 宽松实体", "upload", chunks, "balanced")
	_, relations, err := store.ListOwnerGraph(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	visibleEntityRelations := 0
	for _, relation := range relations {
		relationName := normalizeRelationType(relation.Relation)
		if relationName != "CONTAINS" && !isGenericRelatedRelation(relationName) {
			visibleEntityRelations++
		}
	}
	if visibleEntityRelations == 0 {
		t.Fatalf("expected at least one visible entity-to-entity relation, got relations=%v", relations)
	}
}

func TestVisibleEntityCooccurrenceFallbackUsesSparseAdaptiveSkeleton(t *testing.T) {
	store := graphstore.NewMemoryStore()
	svc := &ragServiceImpl{graphStore: store}
	entityByCanonical := map[string]*model.Entity{}
	extracted := make([]extractedEntity, 0, 10)
	for _, name := range []string{"实体一", "实体二", "实体三", "实体四", "实体五", "实体六", "实体七", "实体八", "实体九", "实体十"} {
		entity := svc.upsertGraphEntity(context.Background(), 7, extractedEntity{Name: name, Type: "自由类型", Description: name})
		if entity == nil {
			t.Fatalf("failed to upsert entity %s", name)
		}
		entityByCanonical[canonicalEntityKey(name)] = entity
		extracted = append(extracted, extractedEntity{Name: name, Type: "自由类型", Description: name})
	}

	count := svc.saveVisibleEntityCooccurrenceRelations(context.Background(), 7, 91, "稀疏骨架", model.Chunk{ID: 4, Content: "实体一到实体十共同说明上下文。"}, extracted, entityByCanonical, map[string]bool{}, map[int64]int{})

	if count <= 0 {
		t.Fatalf("expected sparse cooccurrence skeleton")
	}
	if count >= int64(len(extracted)-1) {
		t.Fatalf("expected cooccurrence fallback to stay sparse, got %d relations for %d entities", count, len(extracted))
	}
}

type staticGraphExtractor struct {
	result graphExtractResult
	err    error
}

func (e staticGraphExtractor) Extract(context.Context, graphExtractInput) (graphExtractResult, error) {
	return e.result, e.err
}

func sortedRelationTypes(types map[string]int) []string {
	out := make([]string, 0, len(types))
	for relationType, count := range types {
		for i := 0; i < count; i++ {
			out = append(out, relationType)
		}
	}
	sort.Strings(out)
	return out
}
