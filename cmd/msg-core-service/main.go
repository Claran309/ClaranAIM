package main

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/dtmbranch"
	"ClaranAIM/internal/msg-core-service/eventconsumer"
	"ClaranAIM/internal/msg-core-service/handler"
	"ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/outbox"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"net"
	"net/http"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 IM 消息核心服务。
// 它同时暴露 Kitex RPC、DTM 分支 HTTP，并启动 Outbox/Kafka 事件链路。
func main() {
	logger.InitService("msg-core-service")

	// msg-core-service 是 IM 的核心写路径：会话、消息事实、用户级消息状态、
	// 已读游标和 WebSocket 推送目标都从这里统一处理。启动时只做非破坏性
	// AutoMigrate，避免服务重启清空聊天历史。
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

	var publisher eventbus.Publisher
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		publisher = eventbus.NewKafkaPublisher(cfg.Kafka.Brokers, cfg.Kafka.ClientID)
		defer publisher.Close()
		outboxWorker := outbox.NewWorker(outbox.NewGormStore(db), publisher)
		go outboxWorker.Run(context.Background())
		logger.Info("Kafka消息事件发布已启用", "brokers", cfg.Kafka.Brokers)
	}

	msgService := service.NewMessageServiceWithPublisher(msgRepo, redisClient, cfg.Etcd.Endpoints)
	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	settingsRPCClient, err := settingsservice.NewClient(
		"settings-service",
		append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)...,
	)
	if err != nil {
		logger.Fatal("创建settings-service客户端失败", "error", err)
	}
	if impl, ok := msgService.(interface {
		SetTranslationDependencies(service.TranslationSettings, service.TranslationLLM)
	}); ok {
		impl.SetTranslationDependencies(settingsclient.NewRPCClient(settingsRPCClient), service.NewOpenAICompatibleTranslator())
	}
	msgHandler := handler.NewMessageServiceImpl(msgService)
	if cfg.DTM.Enabled && cfg.DTM.MsgCoreBranchAddress != "" {
		mux := http.NewServeMux()
		dtmbranch.NewHandler(msgService).RegisterRoutes(mux)
		go func() {
			logger.Info("DTM消息分支HTTP服务已启动", "address", cfg.DTM.MsgCoreBranchAddress)
			if err := http.ListenAndServe(cfg.DTM.MsgCoreBranchAddress, mux); err != nil {
				logger.Error("DTM消息分支HTTP服务停止", "error", err)
			}
		}()
	}

	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		consumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicGroupEvents, "msg-core-service")
		defer consumer.Close()
		eventconsumer.StartGroupEventConsumer(context.Background(), consumer, msgService)
		logger.Info("Kafka群组事件消费已启用", "topic", events.TopicGroupEvents)
	}

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
		Name:    "msg-core-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
