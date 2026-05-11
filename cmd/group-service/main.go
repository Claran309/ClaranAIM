package main

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/internal/group-service/handler"
	"ClaranAIM/internal/group-service/service"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/pkg/config"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	cfg, err := config.Load("config/group-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库（group-service 独立管理自己的表）
	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("group-service 数据库初始化成功")

	groupRepo := dao.NewGroupRepo(db)
	groupService := service.NewGroupService(groupRepo)
	groupHandler := handler.NewGroupServiceImpl(groupService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		log.Fatal("创建etcd注册中心失败:", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
	}

	svr := groupservice.NewServer(
		groupHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: cfg.Service.Name,
		}),
		server.WithRegistry(r),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)

	log.Printf("group-service 启动在 %s", cfg.Service.Address)
	if err := svr.Run(); err != nil {
		log.Fatal("group-service 启动失败:", err)
	}
}
