package main

import (
	convdao "ClaranAIM/internal/conversation-intelligence-service/dao"
	"ClaranAIM/internal/conversation-intelligence-service/eventconsumer"
	convhandler "ClaranAIM/internal/conversation-intelligence-service/handler"
	convsvc "ClaranAIM/internal/conversation-intelligence-service/service"
	"ClaranAIM/kitex_gen/conversation_intelligence/conversationintelligenceservice"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/ragclient"
	"context"
	"net"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动 conversation-intelligence-service。
func main() {
	logger.InitService("conversation-intelligence-service")

	cfg, err := config.Load("config/conversation-intelligence-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := convdao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "conversation-intelligence-service")

	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	options := append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)
	messageClient, err := messageservice.NewClient("msg-core-service", options...)
	if err != nil {
		logger.Fatal("创建msg-core-service客户端失败", "error", err)
	}
	ragRPCClient, err := ragservice.NewClient("rag-service", options...)
	if err != nil {
		logger.Fatal("创建rag-service客户端失败", "error", err)
	}
	memoryRPCClient, err := memoryservice.NewClient("memory-service", options...)
	if err != nil {
		logger.Fatal("创建memory-service客户端失败", "error", err)
	}

	extractor := convsvc.ArtifactExtractor(convsvc.RuleArtifactExtractor{})
	if cfg.ConversationIntelligence.LLMEnabled {
		extractor = convsvc.NewFallbackArtifactExtractor(
			convsvc.NewLLMArtifactExtractor(convsvc.LLMArtifactExtractorConfig{
				BaseURL: cfg.ConversationIntelligence.LLMBaseURL,
				APIKey:  cfg.ConversationIntelligence.LLMAPIKey,
				Model:   cfg.ConversationIntelligence.LLMModel,
			}),
			convsvc.RuleArtifactExtractor{},
		)
	}
	service := convsvc.NewConversationIntelligenceService(
		convdao.NewRepository(db),
		convsvc.NewMessageRPCWindowFetcher(messageClient),
		convsvc.NewRAGClientSink(ragclient.NewRPCClient(ragRPCClient)),
		convsvc.NewMemoryClientSink(memoryclient.NewRPCClient(memoryRPCClient)),
		extractor,
		convsvc.ConversationIntelligenceOptions{
			WindowMessageLimit:  cfg.ConversationIntelligence.WindowMessageLimit,
			MinValuableMessages: cfg.ConversationIntelligence.MinValuableMessages,
			MaxRetries:          cfg.ConversationIntelligence.MaxRetries,
			RetryDelaySeconds:   cfg.ConversationIntelligence.RetryDelaySeconds,
		},
	)
	serviceCtx, cancelService := context.WithCancel(context.Background())
	defer cancelService()
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		messageConsumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicMessageEvents, "conversation-intelligence-message-activity")
		defer messageConsumer.Close()
		eventconsumer.StartConversationActivityConsumer(serviceCtx, messageConsumer, service)
		imConsumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicIMEvents, "conversation-intelligence-im-activity")
		defer imConsumer.Close()
		eventconsumer.StartConversationActivityConsumer(serviceCtx, imConsumer, service)
		logger.Info("会话智能归档事件消费已启用", "message_topic", events.TopicMessageEvents, "im_topic", events.TopicIMEvents)
	}
	convsvc.StartDigestScheduler(serviceCtx, service, convsvc.DigestScheduleOptions{
		MessageThreshold: cfg.ConversationIntelligence.WindowMessageLimit,
		WindowMinutes:    cfg.ConversationIntelligence.WindowMinutes,
		Limit:            20,
	}, time.Duration(cfg.ConversationIntelligence.ScheduleIntervalSeconds)*time.Second)

	registry, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "conversation-intelligence-service")
	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}
	svr := conversationintelligenceservice.NewServer(
		convhandler.NewConversationIntelligenceServiceImpl(service),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(registry),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)
	health.LogStartup(health.ServiceInfo{Name: "conversation-intelligence-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := svr.Run(); err != nil {
		logger.Fatal("conversation-intelligence-service RPC服务停止", "error", err)
	}
}
