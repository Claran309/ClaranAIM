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

func main() {
	cfg, err := config.Load("config/msg-core-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("msg-core-service 数据库初始化成功")

	msgRepo := dao.NewMessageRepo(db)

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Redis连接失败，将仅使用数据库: %v", err)
		} else {
			log.Println("Redis连接成功")
		}
	}

	pushClient := push.NewPushClient("127.0.0.1:8081")
	msgService := service.NewMessageService(msgRepo, pushClient, redisClient)
	msgHandler := handler.NewMessageServiceImpl(msgService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		log.Fatal("创建etcd注册中心失败:", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
	}

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
