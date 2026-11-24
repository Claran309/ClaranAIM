package main

import (
	"ClaranAIM/internal/agent-runtime-service/handler"
	"ClaranAIM/internal/agent-runtime-service/logic"
	"ClaranAIM/internal/agent-runtime-service/reflection"
	"ClaranAIM/internal/agent-runtime-service/service"
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"ClaranAIM/pkg/agentkit/component"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"context"
	"net"
	"strings"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 Agent 运行时服务。
// runtime 负责 Eino DeepAgent、长会话 JSONL、工作目录和工具/Skill 执行，不直接管理 Agent 权限。
func main() {
	logger.InitService("agent-runtime-service")

	cfg, err := config.Load("config/agent-runtime-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	var sessionStore store.SessionStore
	var runStore store.RunStore
	var checkpointStore store.CheckpointStore
	if cfg.MySQL.DSN != "" {
		db, dbErr := store.InitDB(cfg.MySQL.DSN)
		if dbErr != nil {
			logger.Warn("agent-runtime-service MySQL初始化失败，回退JSONL/内存存储", "error", dbErr)
		} else {
			gormStore := store.NewGormStore(db)
			sessionStore = gormStore
			runStore = gormStore
			checkpointStore = gormStore
			if sqlDB, sqlErr := db.DB(); sqlErr == nil {
				observability.RegisterDBStats(sqlDB, cfg.Service.Name)
			}
			logger.Info("agent-runtime-service 会话/运行/审批checkpoint已使用MySQL持久化")
		}
	}
	runtimeSvc := service.NewAgentRuntimeService(service.RuntimeConfig{
		SessionDir:          cfg.Agent.SessionDir,
		DefaultWorkspaceDir: cfg.Agent.AgentRoot,
		CozeloopToken:       cfg.Agent.CozeloopToken,
		CozeloopWorkspaceID: cfg.Agent.CozeloopWSID,
		SessionStore:        sessionStore,
		RunStore:            runStore,
		CheckpointStore:     checkpointStore,
		Reflection:          buildReflectionConfig(cfg),
	})
	runtimeHandler := handler.NewAgentRuntimeServiceImpl(runtimeSvc)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "agent-runtime-service")
	if resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints); err == nil {
		ragRPCClient, cliErr := ragservice.NewClient(
			"rag-service",
			append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)...,
		)
		if cliErr != nil {
			logger.Warn("创建rag-service客户端失败，Agent知识库工具将不可用", "error", cliErr)
		} else {
			logic.SetRAGService(ragRPCClient)
			logger.Info("Agent知识库工具已连接rag-service")
		}
		webSearchRPCClient, cliErr := websearchservice.NewClient(
			"web-search-service",
			append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)...,
		)
		if cliErr != nil {
			logger.Warn("创建web-search-service客户端失败，Agent联网搜索工具将不可用", "error", cliErr)
		} else {
			logic.SetWebSearchService(webSearchRPCClient)
			logger.Info("Agent联网搜索工具已连接web-search-service")
		}
		mcpGatewayRPCClient, cliErr := mcpgatewayservice.NewClient(
			"mcp-gateway-service",
			append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)...,
		)
		if cliErr != nil {
			logger.Warn("创建mcp-gateway-service客户端失败，Agent MCP工具将不可用", "error", cliErr)
		} else {
			logic.SetMCPService(mcpGatewayRPCClient)
			logger.Info("Agent MCP工具已连接mcp-gateway-service")
		}
	} else {
		logger.Warn("创建etcd resolver失败，Agent知识库工具将不可用", "error", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := botruntimeservice.NewServer(
		runtimeHandler,
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
				ServiceName: cfg.Service.Name,
			}),
			server.WithRegistry(r),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)

	health.LogStartup(health.ServiceInfo{
		Name:    "agent-runtime-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}

// buildReflectionConfig 构造 critic 自检配置。
// critic 默认复用 Agent 模型；配置独立 APIKey/BaseURL/Model 时可使用更便宜的模型。
func buildReflectionConfig(cfg *config.Config) reflection.Config {
	if cfg == nil {
		return reflection.Config{}
	}
	result := reflection.Config{
		Enabled:   cfg.Agent.ReflectionEnabled,
		ModelName: cfg.Agent.ReflectionCriticModel,
		Budget: reflection.Budget{
			MaxRounds:      1,
			MaxTokens:      cfg.Agent.ReflectionMaxTokens,
			Timeout:        time.Duration(cfg.Agent.ReflectionTimeoutMS) * time.Millisecond,
			MaxCostUSD:     cfg.Agent.ReflectionMaxCostUSD,
			MinAnswerRunes: cfg.Agent.ReflectionMinAnswerRune,
		},
	}
	apiKey := firstNonEmpty(cfg.Agent.ReflectionCriticAPIKey, cfg.LLM.DefaultAPIKey)
	baseURL := firstNonEmpty(cfg.Agent.ReflectionCriticBaseURL, cfg.LLM.DefaultBaseURL)
	modelName := firstNonEmpty(cfg.Agent.ReflectionCriticModel, cfg.LLM.DefaultModel)
	if apiKey == "" || baseURL == "" || modelName == "" {
		logger.Warn("Reflection已禁用：缺少critic模型配置",
			"api_key_configured", apiKey != "",
			"base_url_configured", baseURL != "",
			"model", modelName,
		)
		result.Enabled = false
		return result
	}
	chatModel, err := component.NewChatModel(context.Background(), apiKey, baseURL, modelName, cfg.Agent.ReflectionMaxTokens, 0)
	if err != nil {
		logger.Warn("Reflection已禁用：critic模型初始化失败", "error", err)
		result.Enabled = false
		return result
	}
	result.ModelName = modelName
	result.ChatModel = chatModel
	logger.Info("Agent Reflection已启用", "critic_model", modelName, "max_rounds", 1)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
