package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/graphstore"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"strings"
)

func NewRAGService(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string) RAGService {
	if embeddingDim <= 0 {
		embeddingDim = 256
	}
	if vectorIndex == nil {
		vectorIndex = NewLocalVectorIndex()
	}
	if strings.TrimSpace(defaultMode) == "" {
		defaultMode = "adaptive"
	}
	return &ragServiceImpl{repo: repo, graphStore: defaultGraphStore(repo), vectorIndex: vectorIndex, router: HybridAdaptiveRouter{}, graphExtractor: ruleGraphExtractor{}, graphSummarizer: ruleGraphCommunitySummarizer{}, embeddingDim: embeddingDim, defaultMode: defaultMode}
}

// NewRAGServiceWithGraphStore creates a RAG service with an explicit GraphRAG persistence backend.
func NewRAGServiceWithGraphStore(repo dao.Repository, graphStore graphstore.GraphStore, vectorIndex VectorIndex, embeddingDim int, defaultMode string) RAGService {
	svc := NewRAGService(repo, vectorIndex, embeddingDim, defaultMode).(*ragServiceImpl)
	if graphStore != nil {
		svc.graphStore = graphStore
	}
	return svc
}

func defaultGraphStore(repo dao.Repository) graphstore.GraphStore {
	if marker, ok := any(repo).(interface{ UsesExternalGraphStore() bool }); ok && marker.UsesExternalGraphStore() {
		return graphstore.NewMemoryStore()
	}
	if store, ok := any(repo).(graphstore.GraphStore); ok {
		return store
	}
	return graphstore.NewMemoryStore()
}

// NewRAGServiceWithEmbedding 创建带外部 embedding provider 的 RAG 服务。
// provider 不可用时每次向量生成都会自动降级到 hash embedding，保证入库链路不中断。
func NewRAGServiceWithEmbedding(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider) RAGService {
	svc := NewRAGService(repo, vectorIndex, embeddingDim, defaultMode).(*ragServiceImpl)
	svc.embedder = embedder
	return svc
}

func NewRAGServiceWithGraphStoreAndEmbedding(repo dao.Repository, graphStore graphstore.GraphStore, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider) RAGService {
	svc := NewRAGServiceWithGraphStore(repo, graphStore, vectorIndex, embeddingDim, defaultMode).(*ragServiceImpl)
	svc.embedder = embedder
	return svc
}

// NewRAGServiceWithRouter 创建带 embedding provider 和 RAG router 的服务。
// router 通常是轻量 LLM；router 失败时 Search 会回退到规则路由，避免外部模型故障阻断检索。
func NewRAGServiceWithRouter(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter) RAGService {
	svc := NewRAGServiceWithEmbedding(repo, vectorIndex, embeddingDim, defaultMode, embedder).(*ragServiceImpl)
	if router != nil {
		svc.router = HybridAdaptiveRouter{LLM: router}
	}
	return svc
}

func NewRAGServiceWithGraphStoreAndRouter(repo dao.Repository, graphStore graphstore.GraphStore, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter) RAGService {
	svc := NewRAGServiceWithGraphStoreAndEmbedding(repo, graphStore, vectorIndex, embeddingDim, defaultMode, embedder).(*ragServiceImpl)
	if router != nil {
		svc.router = HybridAdaptiveRouter{LLM: router}
	}
	return svc
}

func NewRAGServiceWithRouterAndReranker(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker) RAGService {
	return NewRAGServiceWithRouterRerankerAndCRAG(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, nil)
}

func NewRAGServiceWithRouterRerankerAndCRAG(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator) RAGService {
	return NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, nil)
}

func NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge) RAGService {
	svc := NewRAGServiceWithRouter(repo, vectorIndex, embeddingDim, defaultMode, embedder, router).(*ragServiceImpl)
	svc.reranker = reranker
	if cragEvaluator == nil {
		cragEvaluator = RuleCRAGEvaluator{}
	}
	svc.cragEvaluator = cragEvaluator
	if selfJudge == nil {
		selfJudge = RuleSelfRAGJudge{}
	}
	svc.selfJudge = selfJudge
	return svc
}

// NewRAGServiceWithGraphExtractor 创建带 GraphRAG LLM 抽取器/社区摘要器的服务。
// 抽取器或摘要器失败时会自动降级到规则实现，避免外部模型故障阻断知识入库。
func NewRAGServiceWithGraphExtractor(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, extractor graphExtractor, summarizer graphCommunitySummarizer) RAGService {
	svc := NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge).(*ragServiceImpl)
	if extractor != nil {
		svc.graphExtractor = fallbackGraphExtractor{primary: extractor, fallback: ruleGraphExtractor{}}
		svc.llmGraphEnabled = true
	}
	if summarizer != nil {
		svc.graphSummarizer = fallbackGraphCommunitySummarizer{primary: summarizer, fallback: ruleGraphCommunitySummarizer{}}
	}
	return svc
}

func NewRAGServiceWithGraphStoreAndGraphExtractor(repo dao.Repository, graphStore graphstore.GraphStore, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, extractor graphExtractor, summarizer graphCommunitySummarizer) RAGService {
	svc := NewRAGServiceWithGraphStoreAndRouter(repo, graphStore, vectorIndex, embeddingDim, defaultMode, embedder, router).(*ragServiceImpl)
	svc.reranker = reranker
	if cragEvaluator == nil {
		cragEvaluator = RuleCRAGEvaluator{}
	}
	svc.cragEvaluator = cragEvaluator
	if selfJudge == nil {
		selfJudge = RuleSelfRAGJudge{}
	}
	svc.selfJudge = selfJudge
	if extractor != nil {
		svc.graphExtractor = fallbackGraphExtractor{primary: extractor, fallback: ruleGraphExtractor{}}
		svc.llmGraphEnabled = true
	}
	if summarizer != nil {
		svc.graphSummarizer = fallbackGraphCommunitySummarizer{primary: summarizer, fallback: ruleGraphCommunitySummarizer{}}
	}
	return svc
}

// NewRAGServiceWithRouterProviderAndGraphExtractor 同时启用用户级 RAG Router 和 GraphRAG LLM 抽取/社区摘要。
// 这避免 settings-service 覆盖 Router 时把 GraphRAG 又退回纯规则抽取。
func NewRAGServiceWithRouterProviderAndGraphExtractor(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, settings settingsservice.Client, routerFactory func(resolvedLLMConfig) RAGRouter, extractor graphExtractor, summarizer graphCommunitySummarizer) RAGService {
	svc := NewRAGServiceWithGraphExtractor(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge, extractor, summarizer).(*ragServiceImpl)
	svc.settings = settings
	svc.routerFactory = routerFactory
	return svc
}

func NewRAGServiceWithGraphStoreRouterProviderAndGraphExtractor(repo dao.Repository, graphStore graphstore.GraphStore, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, settings settingsservice.Client, routerFactory func(resolvedLLMConfig) RAGRouter, extractor graphExtractor, summarizer graphCommunitySummarizer) RAGService {
	svc := NewRAGServiceWithGraphStoreAndGraphExtractor(repo, graphStore, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge, extractor, summarizer).(*ragServiceImpl)
	svc.settings = settings
	svc.routerFactory = routerFactory
	return svc
}

// NewRAGServiceWithRouterProvider 创建支持“用户 RAG 路由小模型覆盖”的 RAG 服务。
// Search 时会先读取当前 viewer 的 settings-service rag_router 默认预设；如果没有配置、配置不完整、
// settings-service 暂不可用或用户小模型调用失败，就回退到启动时注入的项目内置 router。
func NewRAGServiceWithRouterProvider(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, settings settingsservice.Client, routerFactory func(resolvedLLMConfig) RAGRouter) RAGService {
	svc := NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge).(*ragServiceImpl)
	svc.settings = settings
	if routerFactory != nil {
		svc.routerFactory = routerFactory
	} else {
		svc.routerFactory = func(cfg resolvedLLMConfig) RAGRouter {
			return NewLLMRouter(cfg.APIKey, cfg.BaseURL, cfg.ModelName)
		}
	}
	return svc
}

// IngestDocument 写入文档、分块、向量索引和 GraphRAG 图谱。
