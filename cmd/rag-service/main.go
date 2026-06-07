package main

import (
	ragdao "ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/graphstore"
	"ClaranAIM/internal/rag-service/handler"
	ragsvc "ClaranAIM/internal/rag-service/service"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"context"
	"net"
	"strings"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 RAG RPC 服务。
// 服务负责知识文档入库、Hybrid Search、GraphRAG 子图、CRAG/Self-RAG/Adaptive RAG 的基础执行链路。
func main() {
	logger.InitService("rag-service")

	cfg, err := config.Load("config/rag-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	db, err := ragdao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "rag-service")

	vectorIndex := ragsvc.VectorIndex(ragsvc.NewLocalVectorIndex())
	if cfg.Milvus.Enabled {
		milvusIndex, err := ragsvc.NewMilvusVectorIndex(context.Background(), cfg.Milvus.Address, cfg.Milvus.Collection, cfg.RAG.EmbeddingDim)
		if err != nil {
			logger.Warn("Milvus连接失败，rag-service降级使用本地向量索引", "error", err, "address", cfg.Milvus.Address, "collection", cfg.Milvus.Collection)
		} else {
			vectorIndex = milvusIndex
			logger.Info("Milvus向量后端已启用", "address", cfg.Milvus.Address, "collection", cfg.Milvus.Collection)
		}
	} else {
		logger.Info("Milvus未启用，rag-service使用本地向量降级索引")
	}
	var embedder ragsvc.EmbeddingProvider
	if cfg.RAG.EmbeddingProvider == "glm" && cfg.RAG.EmbeddingAPIKey != "" {
		embedder = ragsvc.NewGLMEmbeddingProvider(cfg.RAG.EmbeddingURL, cfg.RAG.EmbeddingAPIKey, cfg.RAG.EmbeddingModel, cfg.RAG.EmbeddingDimension)
		logger.Info("GLM Embedding已启用", "model", cfg.RAG.EmbeddingModel)
	} else {
		logger.Info("使用hash embedding降级模式")
	}
	var router ragsvc.RAGRouter
	if cfg.RAG.RouterProvider == "llm" {
		routerAPIKey := firstNonEmpty(cfg.RAG.RouterAPIKey, cfg.LLM.DefaultAPIKey)
		routerBaseURL := firstNonEmpty(cfg.RAG.RouterBaseURL, cfg.LLM.DefaultBaseURL)
		routerModel := firstNonEmpty(cfg.RAG.RouterModel, cfg.LLM.DefaultModel, "glm-4-flash")
		if routerAPIKey != "" && routerBaseURL != "" {
			router = ragsvc.NewLLMRouter(routerAPIKey, routerBaseURL, routerModel)
			logger.Info("RAG项目内置LLM Router已启用", "model", routerModel)
		} else {
			logger.Warn("RAG Router配置不完整，已降级规则路由", "provider", cfg.RAG.RouterProvider)
		}
	}
	var cragEvaluator ragsvc.CRAGEvaluator = ragsvc.RuleCRAGEvaluator{}
	var selfJudge ragsvc.SelfRAGJudge = ragsvc.RuleSelfRAGJudge{}
	if cfg.RAG.RouterProvider == "llm" {
		cragAPIKey := firstNonEmpty(cfg.RAG.RouterAPIKey, cfg.LLM.DefaultAPIKey)
		cragBaseURL := firstNonEmpty(cfg.RAG.RouterBaseURL, cfg.LLM.DefaultBaseURL)
		cragModel := firstNonEmpty(cfg.RAG.RouterModel, cfg.LLM.DefaultModel, "glm-4-flash")
		if cragAPIKey != "" && cragBaseURL != "" {
			cragEvaluator = ragsvc.NewLLMCRAGEvaluator(cragAPIKey, cragBaseURL, cragModel, ragsvc.RuleCRAGEvaluator{})
			selfJudge = ragsvc.NewLLMSelfRAGJudge(cragAPIKey, cragBaseURL, cragModel, ragsvc.RuleSelfRAGJudge{})
			logger.Info("RAG LLM CRAG Evaluator已启用", "model", cragModel)
			logger.Info("RAG LLM Self-RAG Judge已启用", "model", cragModel)
		} else {
			logger.Warn("RAG CRAG/Self-RAG小模型配置不完整，已降级规则评估")
		}
	}
	var graphExtractor *ragsvc.LLMGraphExtractor
	var graphSummarizer *ragsvc.LLMGraphCommunitySummarizer
	if cfg.RAG.RouterProvider == "llm" {
		graphAPIKey := firstNonEmpty(cfg.RAG.RouterAPIKey, cfg.LLM.DefaultAPIKey)
		graphBaseURL := firstNonEmpty(cfg.RAG.RouterBaseURL, cfg.LLM.DefaultBaseURL)
		graphModel := firstNonEmpty(cfg.RAG.RouterModel, cfg.LLM.DefaultModel, "glm-4-flash")
		if graphAPIKey != "" && graphBaseURL != "" {
			graphExtractor = ragsvc.NewLLMGraphExtractor(graphAPIKey, graphBaseURL, graphModel)
			graphSummarizer = ragsvc.NewLLMGraphCommunitySummarizer(graphAPIKey, graphBaseURL, graphModel)
			logger.Info("GraphRAG LLM实体/关系抽取和社区摘要已启用", "model", graphModel)
		} else {
			logger.Warn("GraphRAG LLM配置不完整，已降级规则抽取和本地社区摘要")
		}
	}
	var reranker ragsvc.Reranker
	if cfg.RAG.RerankProvider == "glm" {
		if cfg.RAG.RerankAPIKey != "" && cfg.RAG.RerankURL != "" {
			reranker = ragsvc.NewGLMReranker(cfg.RAG.RerankURL, cfg.RAG.RerankAPIKey, cfg.RAG.RerankModel)
			logger.Info("RAG模型Reranker已启用", "model", cfg.RAG.RerankModel)
		} else {
			logger.Warn("RAG Reranker配置不完整，已降级本地轻量rerank", "provider", cfg.RAG.RerankProvider)
		}
	}
	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	settingsRPCClient, err := settingsservice.NewClient(
		"settings-service",
		append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)...,
	)
	if err != nil {
		logger.Warn("创建settings-service客户端失败，RAG将只使用项目内置Router", "error", err)
	}
	repo := ragdao.NewRepository(db)
	graphStore := graphstore.GraphStore(graphstore.NewMemoryStore())
	if cfg.Neo4j.Enabled {
		neo4jStore, err := graphstore.NewNeo4jStoreFromConfig(cfg.Neo4j.URI, cfg.Neo4j.Username, cfg.Neo4j.Password, cfg.Neo4j.Database)
		if err != nil {
			logger.Fatal("初始化Neo4j图谱后端失败", "error", err, "uri", cfg.Neo4j.URI)
		}
		defer func() {
			if err := neo4jStore.Close(context.Background()); err != nil {
				logger.Warn("关闭Neo4j图谱后端失败", "error", err)
			}
		}()
		if err := neo4jStore.EnsureSchema(context.Background()); err != nil {
			logger.Fatal("初始化Neo4j图谱Schema失败", "error", err)
		}
		graphStore = neo4jStore
		logger.Info("Neo4j GraphRAG后端已启用", "uri", cfg.Neo4j.URI, "database", cfg.Neo4j.Database)
	} else {
		logger.Warn("Neo4j未启用，GraphRAG使用进程内内存图谱后端；该模式不适合生产部署")
	}
	ragService := ragsvc.NewRAGServiceWithGraphStoreAndGraphExtractor(
		repo,
		graphStore,
		vectorIndex,
		cfg.RAG.EmbeddingDim,
		cfg.RAG.DefaultMode,
		embedder,
		router,
		reranker,
		cragEvaluator,
		selfJudge,
		graphExtractor,
		graphSummarizer,
	)
	if settingsRPCClient != nil {
		ragService = ragsvc.NewRAGServiceWithGraphStoreRouterProviderAndGraphExtractor(
			repo,
			graphStore,
			vectorIndex,
			cfg.RAG.EmbeddingDim,
			cfg.RAG.DefaultMode,
			embedder,
			router,
			reranker,
			cragEvaluator,
			selfJudge,
			settingsRPCClient,
			nil,
			graphExtractor,
			graphSummarizer,
		)
		logger.Info("RAG用户级Router配置解析已启用", "usage_type", "rag_router")
	}

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "rag-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}
	svr := ragservice.NewServer(
		handler.NewRAGServiceImpl(ragService),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(r),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)
	health.LogStartup(health.ServiceInfo{
		Name:    "rag-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("rag-service RPC服务停止", "error", err)
	}
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
