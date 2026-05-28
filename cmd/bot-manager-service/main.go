package main

import (
	"ClaranAIM/internal/bot-manager-service/dao"
	"ClaranAIM/internal/bot-manager-service/eventconsumer"
	"ClaranAIM/internal/bot-manager-service/handler"
	"ClaranAIM/internal/bot-manager-service/service"
	memorydao "ClaranAIM/internal/memory-service/dao"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"context"
	"net"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	logger.InitService("bot-manager-service")

	cfg, err := config.Load("config/bot-manager-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "bot-manager-service")

	botRepo := dao.NewBotRepo(db)
	permissionRepo := dao.NewPermissionRepo(db)
	routeRepo := dao.NewRouteRepo(db)
	billingRepo := dao.NewBillingRepo(db)
	dispatchRepo := dao.NewAgentDispatchRepo(db)
	subscriptionRepo := dao.NewAgentSubscriptionRepo(db)
	auditRepo := dao.NewAgentAuditRepo(db)

	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	clientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)
	agentClientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.LongRunningClientOptions(cfg.Governance.AgentRPC)...)
	runtimeClient, err := botruntimeservice.NewClient("bot-runtime-service", agentClientOptions...)
	if err != nil {
		logger.Fatal("创建bot-runtime-service客户端失败", "error", err)
	}
	userClient, err := userservice.NewClient("user-service", clientOptions...)
	if err != nil {
		logger.Fatal("创建user-service客户端失败", "error", err)
	}
	messageClient, err := messageservice.NewClient("msg-core-service", clientOptions...)
	if err != nil {
		logger.Fatal("创建msg-core-service客户端失败", "error", err)
	}

	botService := service.NewBotService(botRepo, permissionRepo, routeRepo, billingRepo, runtimeClient, userClient, cfg.Agent.AgentRoot)
	if impl, ok := botService.(interface {
		SetAgentSubscriptionRepository(dao.AgentSubscriptionRepository)
	}); ok {
		impl.SetAgentSubscriptionRepository(subscriptionRepo)
	}
	if impl, ok := botService.(interface {
		SetMemoryService(service.AgentMemoryService)
	}); ok {
		impl.SetMemoryService(memorysvc.NewMemoryService(memorydao.NewMemoryRepo(db)))
	}
	botHandler := handler.NewBotServiceImpl(botService, cfg)

	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		messageConsumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicMessageEvents, "bot-manager-agent-dispatcher")
		defer messageConsumer.Close()
		eventconsumer.StartAgentEventDispatcherConsumer(context.Background(), messageConsumer, botService, dispatchRepo, subscriptionRepo, auditRepo, messageClient)
		imConsumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicIMEvents, "bot-manager-agent-im-dispatcher")
		defer imConsumer.Close()
		eventconsumer.StartAgentEventDispatcherConsumer(context.Background(), imConsumer, botService, dispatchRepo, subscriptionRepo, auditRepo, messageClient)
		logger.Info("Agent原生事件消费已启用", "message_topic", events.TopicMessageEvents, "im_topic", events.TopicIMEvents)
	}

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "bot-manager-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := botservice.NewServer(
		botHandler,
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
		Name:    "bot-manager-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
