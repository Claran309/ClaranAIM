package main

import (
	"ClaranAIM/internal/msg-history-service/dao"
	"ClaranAIM/internal/msg-history-service/handler"
	"ClaranAIM/internal/msg-history-service/service"
	"ClaranAIM/kitex_gen/message/historyservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动消息历史服务：连接独立历史库表、注册 Kitex RPC，并提供归档/离线消息查询能力。
func main() {
	logger.InitService("msg-history-service")

	cfg, err := config.Load("config/msg-history-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "msg-history-service")

	historyRepo := dao.NewHistoryRepo(db)
	historyService := service.NewHistoryService(historyRepo)
	historyHandler := handler.NewHistoryServiceImpl(historyService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "msg-history-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := historyservice.NewServer(
		historyHandler,
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
		Name:    "msg-history-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
