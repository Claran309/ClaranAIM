package main

import (
	"ClaranAIM/internal/msg-history-service/dao"
	"ClaranAIM/internal/msg-history-service/handler"
	"ClaranAIM/internal/msg-history-service/service"
	"ClaranAIM/kitex_gen/message/historyservice"
	"ClaranAIM/pkg/config"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	cfg, err := config.Load("config/msg-history-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库（msg-history-service 独立管理自己的表）
	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("msg-history-service 数据库初始化成功")

	historyRepo := dao.NewHistoryRepo(db)
	historyService := service.NewHistoryService(historyRepo)
	historyHandler := handler.NewHistoryServiceImpl(historyService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		log.Fatal("创建etcd注册中心失败:", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
	}

	svr := historyservice.NewServer(
		historyHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: cfg.Service.Name,
		}),
		server.WithRegistry(r),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)

	log.Printf("msg-history-service 启动在 %s", cfg.Service.Address)
	if err := svr.Run(); err != nil {
		log.Fatal("msg-history-service 启动失败:", err)
	}
}
