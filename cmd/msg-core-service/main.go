package main

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/handler"
	"ClaranAIM/internal/msg-core-service/push"
	"ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/config"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// msg-core-service 启动入口
// 消息核心服务，负责会话管理、消息收发、消息搜索等核心功能
// 是即时通讯系统的核心服务，集成了Redis缓存和WebSocket推送
//
// 启动流程：加载配置 → 初始化数据库 → 连接Redis → 创建推送客户端 → 组装依赖 → 注册Etcd → 启动RPC服务
func main() {
	// 加载配置文件（config/msg-core-service.yaml + 环境变量）
	cfg, err := config.Load("config/msg-core-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库（自动创建 conversations、conversation_participants、messages 表）
	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("msg-core-service 数据库初始化成功")

	// DAO层
	msgRepo := dao.NewMessageRepo(db)

	// 连接Redis（用于缓存会话列表和最近消息）
	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Redis连接失败，将仅使用数据库: %v", err)
		} else {
			log.Println("Redis连接成功")
		}
	}

	// 创建WebSocket推送客户端
	// 指向websocket-gateway的地址，发送消息时通过HTTP请求推送给在线用户
	pushClient := push.NewPushClient("127.0.0.1:8081")

	// 依赖注入：DAO + PushClient + Redis → Service → Handler
	msgService := service.NewMessageService(msgRepo, pushClient, redisClient)
	msgHandler := handler.NewMessageServiceImpl(msgService)

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
	svr := messageservice.NewServer(
		msgHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: cfg.Service.Name,
		}),
		server.WithRegistry(r),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)

	log.Printf("msg-core-service 启动在 %s", cfg.Service.Address)
	if err := svr.Run(); err != nil {
		log.Fatal("msg-core-service 启动失败:", err)
	}
}
