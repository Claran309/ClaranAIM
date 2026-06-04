package main

import (
	memorydao "ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/internal/memory-service/handler"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"context"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动记忆服务的 Kitex RPC API。
// 记忆能力由 api-gateway 和 Agent 服务通过 memoryclient.RPCClient 调用，暂不暴露浏览器直连端口。
func main() {
	logger.InitService("memory-service")

	cfg, err := config.Load("config/memory-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	db, err := memorydao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "memory-service")
	embedder := memorysvc.MemoryEmbeddingProvider(memorysvc.HashMemoryEmbeddingProvider{Dim: cfg.MemoryRAG.EmbeddingDim})
	if cfg.MemoryRAG.EmbeddingProvider == "glm" && cfg.MemoryRAG.EmbeddingAPIKey != "" {
		embedder = memorysvc.NewGLMMemoryEmbeddingProvider(cfg.MemoryRAG.EmbeddingURL, cfg.MemoryRAG.EmbeddingAPIKey, cfg.MemoryRAG.EmbeddingModel, cfg.MemoryRAG.EmbeddingDimension)
		logger.Info("Memory GLM Embedding已启用", "model", cfg.MemoryRAG.EmbeddingModel)
	} else {
		logger.Info("Memory 使用hash embedding降级模式")
	}
	vectorIndex := memorysvc.MemoryVectorIndex(memorysvc.NewLocalMemoryVectorIndex(embedder))
	if cfg.MemoryRAG.MilvusEnabled {
		milvusIndex, err := memorysvc.NewMilvusMemoryVectorIndex(context.Background(), cfg.MemoryRAG.MilvusAddress, cfg.MemoryRAG.MilvusCollection, cfg.MemoryRAG.EmbeddingDim, embedder)
		if err != nil {
			logger.Warn("Memory Milvus连接失败，已降级本地向量索引", "error", err, "address", cfg.MemoryRAG.MilvusAddress, "collection", cfg.MemoryRAG.MilvusCollection)
		} else {
			vectorIndex = milvusIndex
			logger.Info("Memory Milvus向量后端已启用", "address", cfg.MemoryRAG.MilvusAddress, "collection", cfg.MemoryRAG.MilvusCollection)
		}
	}
	var relevanceFilter memorysvc.MemoryRelevanceFilter
	if cfg.MemoryRAG.LLMFilterEnabled {
		apiKey := firstNonEmpty(cfg.MemoryRAG.LLMFilterAPIKey, cfg.LLM.DefaultAPIKey)
		baseURL := firstNonEmpty(cfg.MemoryRAG.LLMFilterBaseURL, cfg.LLM.DefaultBaseURL)
		modelName := firstNonEmpty(cfg.MemoryRAG.LLMFilterModel, cfg.LLM.DefaultModel, "glm-4-flash")
		if apiKey != "" && baseURL != "" {
			relevanceFilter = memorysvc.NewLLMMemoryRelevanceFilter(apiKey, baseURL, modelName)
			logger.Info("Memory LLM Relevance Filter已启用", "model", modelName)
		} else {
			logger.Warn("Memory LLM Relevance Filter配置不完整，已关闭")
		}
	}
	memoryService := memorysvc.NewMemoryServiceWithRAG(memorydao.NewMemoryRepo(db), vectorIndex, relevanceFilter, memorysvc.MemoryRAGOptions{
		UseVector:          cfg.MemoryRAG.Enabled,
		VectorCandidateK:   cfg.MemoryRAG.VectorCandidateK,
		MinScore:           cfg.MemoryRAG.MinScore,
		VectorWeight:       cfg.MemoryRAG.VectorWeight,
		ImportanceWeight:   cfg.MemoryRAG.ImportanceWeight,
		RecencyWeight:      cfg.MemoryRAG.RecencyWeight,
		ScopeWeight:        cfg.MemoryRAG.ScopeWeight,
		EnableLLMFiltering: cfg.MemoryRAG.LLMFilterEnabled,
	})

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "memory-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}
	svr := memoryservice.NewServer(
		handler.NewMemoryServiceImpl(memoryService),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(r),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)
	health.LogStartup(health.ServiceInfo{
		Name:    "memory-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("memory-service RPC服务停止", "error", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
