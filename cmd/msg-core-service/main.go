package main

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/handler"
	"ClaranAIM/internal/msg-core-service/push"
	"ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/pkg/cache/redis"
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
	logger.InitService("msg-core-service")

	cfg, err := config.Load("config/msg-core-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "msg-core-service")

	msgRepo := dao.NewMessageRepo(db)

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Warn("Redis连接失败，将仅使用数据库", "error", err)
		} else {
			health.CheckRedis(redisClient.GetInnerClient(), "msg-core-service")
		}
	}

	pushClient := push.NewPushClient("127.0.0.1:8081")

	msgService := service.NewMessageService(msgRepo, pushClient, redisClient, cfg.Etcd.Endpoints)
	msgHandler := handler.NewMessageServiceImpl(msgService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "msg-core-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
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

	health.LogStartup(health.ServiceInfo{
		Name:    "msg-core-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
