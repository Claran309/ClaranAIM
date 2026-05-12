package main

import (
	"ClaranAIM/internal/bot-manager-service/dao"
	"ClaranAIM/internal/bot-manager-service/handler"
	"ClaranAIM/internal/bot-manager-service/service"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/pkg/config"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	cfg, err := config.Load("config/bot-manager-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("bot-manager-service 数据库初始化成功")

	botRepo := dao.NewBotRepo(db)
	routeRepo := dao.NewRouteRepo(db)
	billingRepo := dao.NewBillingRepo(db)

	botService := service.NewBotService(botRepo, routeRepo, billingRepo)
	botHandler := handler.NewBotServiceImpl(botService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		log.Fatal("创建etcd注册中心失败:", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
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

	log.Printf("bot-manager-service 启动在 %s", cfg.Service.Address)
	if err := svr.Run(); err != nil {
		log.Fatal("bot-manager-service 启动失败:", err)
	}
}
