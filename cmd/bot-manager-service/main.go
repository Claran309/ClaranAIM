package main

import (
	"ClaranAIM/internal/bot-manager-service/dao"
	"ClaranAIM/internal/bot-manager-service/handler"
	"ClaranAIM/internal/bot-manager-service/service"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	logger.InitService("bot-manager-service")

	cfg, err := config.Load("config/bot-manager-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "bot-manager-service")

	botRepo := dao.NewBotRepo(db)
	routeRepo := dao.NewRouteRepo(db)
	billingRepo := dao.NewBillingRepo(db)

	botService := service.NewBotService(botRepo, routeRepo, billingRepo)
	botHandler := handler.NewBotServiceImpl(botService, cfg)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "bot-manager-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := botservice.NewServer(
		botHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: cfg.Service.Name,
		}),
		server.WithRegistry(r),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)

	health.LogStartup(health.ServiceInfo{
		Name:    "bot-manager-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
