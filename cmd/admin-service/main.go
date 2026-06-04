package main

import (
	admindao "ClaranAIM/internal/admin-service/dao"
	adminhandler "ClaranAIM/internal/admin-service/handler"
	adminsvc "ClaranAIM/internal/admin-service/service"
	"ClaranAIM/kitex_gen/admin/adminservice"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"log"
	"net"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 admin-service。它拥有管理公告/管理审计表，并通过 RPC 聚合其他微服务的管理视图。
func main() {
	logger.InitService("admin-service")

	cfg, err := config.Load("config/admin-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	db, err := admindao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "admin-service")

	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	clientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)
	deps := adminsvc.Dependencies{
		Users:     must(userservice.NewClient("user-service", clientOptions...)),
		Groups:    must(groupservice.NewClient("group-service", clientOptions...)),
		Files:     must(fileservice.NewClient("file-service", clientOptions...)),
		Agents:    must(botservice.NewClient("agent-manager-service", clientOptions...)),
		Memory:    must(memoryservice.NewClient("memory-service", clientOptions...)),
		Knowledge: must(knowledgeservice.NewClient("knowledge-service", clientOptions...)),
		MCP:       must(mcpgatewayservice.NewClient("mcp-gateway-service", clientOptions...)),
		RAG:       must(ragservice.NewClient("rag-service", clientOptions...)),
	}

	svc := adminsvc.NewAdminService(admindao.NewRepository(db), deps)
	h := adminhandler.NewAdminServiceImpl(svc)

	registry, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "admin-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := adminservice.NewServer(
		h,
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(registry),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)

	health.LogStartup(health.ServiceInfo{Name: "admin-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return value
}
