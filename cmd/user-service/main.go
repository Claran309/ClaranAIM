package main

import (
	"ClaranAIM/internal/user-service/dao"
	"ClaranAIM/internal/user-service/handler"
	"ClaranAIM/internal/user-service/service"
	"ClaranAIM/kitex_gen/user/userservice"
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
	cfg, err := config.Load("config/user-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("user-service 数据库初始化成功")

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Redis连接失败，将仅使用数据库: %v", err)
		} else {
			log.Println("Redis连接成功")
		}
	}

	userRepo := dao.NewUserRepo(db)
	userService := service.NewUserService(userRepo, redisClient)
	userHandler := handler.NewUserServiceImpl(userService)

	handler.InitJWTConfig(cfg.JWT.SecretKey, cfg.JWT.Expiration)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		log.Fatal("创建etcd注册中心失败:", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
	}

	svr := userservice.NewServer(
		userHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: cfg.Service.Name,
		}),
		server.WithRegistry(r),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)

	log.Printf("user-service 启动在 %s", cfg.Service.Address)
	if err := svr.Run(); err != nil {
		log.Fatal("user-service 启动失败:", err)
	}
}
