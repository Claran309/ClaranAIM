package main

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/internal/group-service/handler"
	"ClaranAIM/internal/group-service/service"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/outbox"
	"context"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	logger.InitService("group-service")

	cfg, err := config.Load("config/group-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "group-service")

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Warn("Redis连接失败，将不使用缓存", "error", err)
			redisClient = nil
		} else {
			health.CheckRedis(redisClient.GetInnerClient(), "group-service")
		}
	}

	groupRepo := dao.NewGroupRepo(db)
	var publisher eventbus.Publisher
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		publisher = eventbus.NewKafkaPublisher(cfg.Kafka.Brokers, cfg.Kafka.ClientID)
		defer publisher.Close()
		outboxWorker := outbox.NewWorker(outbox.NewGormStore(db), publisher)
		go outboxWorker.Run(context.Background())
		logger.Info("Kafka事件发布已启用", "brokers", cfg.Kafka.Brokers)
	}
	groupService := service.NewGroupService(groupRepo, redisClient)
	groupHandler := handler.NewGroupServiceImpl(groupService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "group-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := groupservice.NewServer(
		groupHandler,
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
		Name:    "group-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
