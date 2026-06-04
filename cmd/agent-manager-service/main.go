package main

import (
	"ClaranAIM/internal/agent-manager-service/dao"
	"ClaranAIM/internal/agent-manager-service/eventconsumer"
	"ClaranAIM/internal/agent-manager-service/handler"
	"ClaranAIM/internal/agent-manager-service/service"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/observability"
	"context"
	"net"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 Agent 管理服务。
// 该服务持有 Agent 配置、权限、路由、审计和事件调度，并通过 Kitex 调用 runtime/user/msg 服务。
func main() {
	logger.InitService("agent-manager-service")

	cfg, err := config.Load("config/agent-manager-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "agent-manager-service")

	botRepo := dao.NewBotRepo(db)
	permissionRepo := dao.NewPermissionRepo(db)
	routeRepo := dao.NewRouteRepo(db)
	billingRepo := dao.NewBillingRepo(db)
	dispatchRepo := dao.NewAgentDispatchRepo(db)
	subscriptionRepo := dao.NewAgentSubscriptionRepo(db)
	auditRepo := dao.NewAgentAuditRepo(db)
	taskRepo := dao.NewAgentTaskRepo(db)
	if err := eventbus.AutoMigrateReliabilityStore(db); err != nil {
		logger.Fatal("初始化事件消费可靠性表失败", "error", err)
	}
	reliabilityStore := eventbus.NewGormReliabilityStore(db)

	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	clientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)
	agentClientOptions := append([]client.Option{client.WithResolver(resolver)}, governance.LongRunningClientOptions(cfg.Governance.AgentRPC)...)
	runtimeClient, err := botruntimeservice.NewClient("agent-runtime-service", agentClientOptions...)
	if err != nil {
		logger.Fatal("创建agent-runtime-service客户端失败", "error", err)
	}
	userClient, err := userservice.NewClient("user-service", clientOptions...)
	if err != nil {
		logger.Fatal("创建user-service客户端失败", "error", err)
	}
	messageClient, err := messageservice.NewClient("msg-core-service", clientOptions...)
	if err != nil {
		logger.Fatal("创建msg-core-service客户端失败", "error", err)
	}
	memoryClient, err := memoryservice.NewClient("memory-service", clientOptions...)
	if err != nil {
		logger.Fatal("创建memory-service客户端失败", "error", err)
	}

	agentService := service.NewAgentService(botRepo, permissionRepo, routeRepo, billingRepo, runtimeClient, userClient, cfg.Agent.AgentRoot)
	if impl, ok := agentService.(interface {
		SetDefaultLLM(string, string, string)
	}); ok {
		impl.SetDefaultLLM(cfg.LLM.DefaultAPIKey, cfg.LLM.DefaultBaseURL, cfg.LLM.DefaultModel)
	}
	if impl, ok := agentService.(interface {
		SetAgentSubscriptionRepository(dao.AgentSubscriptionRepository)
	}); ok {
		impl.SetAgentSubscriptionRepository(subscriptionRepo)
	}
	if impl, ok := agentService.(interface {
		SetMemoryService(service.AgentMemoryService)
	}); ok {
		impl.SetMemoryService(memoryclient.NewRPCClient(memoryClient))
	}
	agentHandler := handler.NewAgentServiceImpl(agentService, cfg)

	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		messageConsumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicMessageEvents, "agent-manager-dispatcher")
		defer messageConsumer.Close()
		eventconsumer.StartAgentEventDispatcherConsumerWithOptions(context.Background(), messageConsumer, agentService, dispatchRepo, subscriptionRepo, auditRepo, taskRepo, messageClient, reliabilityStore)
		imConsumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicIMEvents, "agent-manager-im-dispatcher")
		defer imConsumer.Close()
		eventconsumer.StartAgentEventDispatcherConsumerWithOptions(context.Background(), imConsumer, agentService, dispatchRepo, subscriptionRepo, auditRepo, taskRepo, messageClient, reliabilityStore)
		logger.Info("Agent原生事件消费已启用", "message_topic", events.TopicMessageEvents, "im_topic", events.TopicIMEvents)
	}

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "agent-manager-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := botservice.NewServer(
		agentHandler,
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
		Name:    "agent-manager-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
