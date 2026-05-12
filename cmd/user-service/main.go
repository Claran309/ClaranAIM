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

// user-service 启动入口
// 用户服务，负责用户注册、登录、信息管理、好友关系等
// 使用Kitex框架提供RPC服务，通过Etcd注册实现服务发现
//
// 启动流程：加载配置 → 初始化数据库 → 连接Redis → 组装依赖 → 注册Etcd → 启动RPC服务
func main() {
	// 加载配置文件（config/user-service.yaml + 环境变量）
	cfg, err := config.Load("config/user-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库（自动创建 users、friends 表）
	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("user-service 数据库初始化成功")

	// 连接Redis（可选，连接失败则仅使用数据库，不缓存）
	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Redis连接失败，将仅使用数据库: %v", err)
		} else {
			log.Println("Redis连接成功")
		}
	}

	// 依赖注入：DAO → Service → Handler
	userRepo := dao.NewUserRepo(db)
	userService := service.NewUserService(userRepo, redisClient)
	userHandler := handler.NewUserServiceImpl(userService)

	// 初始化JWT配置（Handler层生成Token时使用）
	handler.InitJWTConfig(cfg.JWT.SecretKey, cfg.JWT.Expiration)

	// 创建Etcd注册中心（用于服务发现，api-gateway通过Etcd找到本服务）
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
	// 配置项：
	//   - WithServiceAddr: 服务监听地址
	//   - WithServerBasicInfo: 服务名称（用于Etcd注册）
	//   - WithRegistry: Etcd注册中心
	//   - WithMetaHandler: TTHeader传输协议（与客户端保持一致）
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
