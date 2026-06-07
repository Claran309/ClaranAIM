package main

import (
	"ClaranAIM/internal/agent-runtime-service/handler"
	"ClaranAIM/internal/agent-runtime-service/logic"
	"ClaranAIM/internal/agent-runtime-service/service"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"net"

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

	runtimeSvc := service.NewAgentRuntimeService(service.RuntimeConfig{
		SessionDir:          cfg.Agent.SessionDir,
		DefaultWorkspaceDir: cfg.Agent.AgentRoot,
		CozeloopToken:       cfg.Agent.CozeloopToken,
		CozeloopWorkspaceID: cfg.Agent.CozeloopWSID,
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
