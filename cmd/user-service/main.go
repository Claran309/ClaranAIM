package main

import (
	"ClaranAIM/internal/user-service/dao"
	"ClaranAIM/internal/user-service/handler"
	"ClaranAIM/internal/user-service/service"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动用户服务：初始化用户/好友数据表、Redis 缓存、JWT 签发配置，并把 Kitex 服务注册到 Etcd。
func main() {
	logger.InitService("user-service")

	cfg, err := config.Load("config/user-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "user-service")

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Warn("Redis连接失败，将仅使用数据库", "error", err)
		} else {
			health.CheckRedis(redisClient.GetInnerClient(), "user-service")
		}
	}

	userRepo := dao.NewUserRepo(db)
	userService := service.NewUserService(userRepo, redisClient)
	userHandler := handler.NewUserServiceImpl(userService)

	handler.InitJWTConfigWithRefresh(cfg.JWT.SecretKey, cfg.JWT.AccessExpiration, cfg.JWT.RefreshExpiration)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}

	health.CheckEtcd(cfg.Etcd.Endpoints, "user-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := userservice.NewServer(
		userHandler,
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
		Name:    "user-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
