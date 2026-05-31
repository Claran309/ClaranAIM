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

	db, err := memorydao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "memory-service")
	memoryService := memorysvc.NewMemoryService(memorydao.NewMemoryRepo(db))

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
