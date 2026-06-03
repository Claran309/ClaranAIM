package main

import (
	mcpdao "ClaranAIM/internal/mcp-gateway-service/dao"
	mcphandler "ClaranAIM/internal/mcp-gateway-service/handler"
	mcpsvc "ClaranAIM/internal/mcp-gateway-service/service"
	"ClaranAIM/kitex_gen/conversation_intelligence/conversationintelligenceservice"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/conversationintelclient"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/knowledgeclient"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/ragclient"
	"ClaranAIM/pkg/settingsclient"
	"ClaranAIM/pkg/websearchclient"
	"net"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 MCP Gateway。
// Gateway 负责把内部服务和用户配置的远程 MCP Server 统一暴露为 Agent 可调用工具，
// 同时记录工具调用审计；它不直接实现 RAG/Memory/WebSearch 的业务逻辑。
func main() {
	logger.InitService("mcp-gateway-service")

	cfg, err := config.Load("config/mcp-gateway-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := mcpdao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "mcp-gateway-service")

	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	clientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)
	longRPC := cfg.Governance.RPC
	longRPC.TimeoutMS = 0
	longClientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.LongRunningClientOptions(longRPC)...)

	settingsRPCClient, err := settingsservice.NewClient("settings-service", clientOptions...)
	if err != nil {
		logger.Fatal("创建settings-service客户端失败", "error", err)
	}
	webSearchRPCClient, err := websearchservice.NewClient("web-search-service", longClientOptions...)
	if err != nil {
		logger.Warn("创建web-search-service客户端失败，web_search工具将不可用", "error", err)
	}
	memoryRPCClient, err := memoryservice.NewClient("memory-service", clientOptions...)
	if err != nil {
		logger.Warn("创建memory-service客户端失败，search_memory工具将不可用", "error", err)
	}
	ragRPCClient, err := ragservice.NewClient("rag-service", clientOptions...)
	if err != nil {
		logger.Warn("创建rag-service客户端失败，search_knowledge工具将不可用", "error", err)
	}
	knowledgeRPCClient, err := knowledgeservice.NewClient("knowledge-service", clientOptions...)
	if err != nil {
		logger.Warn("创建knowledge-service客户端失败，query_knowledge_graph工具将不可用", "error", err)
	}
	conversationRPCClient, err := conversationintelligenceservice.NewClient("conversation-intelligence-service", longClientOptions...)
	if err != nil {
		logger.Warn("创建conversation-intelligence-service客户端失败，summarize_conversation工具将不可用", "error", err)
	}

	var webSearchSvc websearchclient.Service
	if webSearchRPCClient != nil {
		webSearchSvc = websearchclient.NewRPCClient(webSearchRPCClient)
	}
	var memorySvc memoryclient.Service
	if memoryRPCClient != nil {
		memorySvc = memoryclient.NewRPCClient(memoryRPCClient)
	}
	var ragSvc ragclient.Service
	if ragRPCClient != nil {
		ragSvc = ragclient.NewRPCClient(ragRPCClient)
	}
	var knowledgeSvc knowledgeclient.Service
	if knowledgeRPCClient != nil {
		knowledgeSvc = knowledgeclient.NewRPCClient(knowledgeRPCClient)
	}
	var conversationSvc conversationintelclient.Service
	if conversationRPCClient != nil {
		conversationSvc = conversationintelclient.NewRPCClient(conversationRPCClient)
	}

	mcpGateway := mcpsvc.NewMCPGatewayService(mcpsvc.Dependencies{
		Repo:         mcpdao.NewRepository(db),
		Settings:     settingsclient.NewRPCClient(settingsRPCClient),
		WebSearch:    webSearchSvc,
		Memory:       memorySvc,
		RAG:          ragSvc,
		Knowledge:    knowledgeSvc,
		Conversation: conversationSvc,
	})

	registry, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "mcp-gateway-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}
	svr := mcpgatewayservice.NewServer(
		mcphandler.NewMCPGatewayServiceImpl(mcpGateway),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(registry),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)

	health.LogStartup(health.ServiceInfo{Name: "mcp-gateway-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := svr.Run(); err != nil {
		logger.Fatal("mcp-gateway-service RPC服务停止", "error", err)
	}
}
