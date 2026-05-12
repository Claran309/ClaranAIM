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

// msg-history-service 启动入口
// 消息历史服务，负责消息归档存储、历史查询、离线消息管理、未读计数等
// 与 msg-core-service 分离，实现消息的长期归档和离线消息功能
//
// 启动流程：加载配置 → 初始化数据库 → 组装依赖 → 注册Etcd → 启动RPC服务
func main() {
	// 加载配置文件（config/msg-history-service.yaml + 环境变量）
	cfg, err := config.Load("config/msg-history-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库（自动创建 message_history、offline_messages 表）
	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("msg-history-service 数据库初始化成功")

	// 依赖注入：DAO → Service → Handler
	historyRepo := dao.NewHistoryRepo(db)
	historyService := service.NewHistoryService(historyRepo)
	historyHandler := handler.NewHistoryServiceImpl(historyService)

	// 创建Etcd注册中心
	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		log.Fatal("创建etcd注册中心失败:", err)
	}

	// 解析服务监听地址
	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
	}

	// 创建Kitex RPC服务器
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
