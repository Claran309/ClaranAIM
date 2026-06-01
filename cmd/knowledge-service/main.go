package main

import (
	knowledgehandler "ClaranAIM/internal/knowledge-service/handler"
	knowledgesvc "ClaranAIM/internal/knowledge-service/service"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/knowledgeclient"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/ragclient"
	"net"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动知识图谱查询服务。
// knowledge-service 不负责入库和 GraphRAG 构建，只通过 rag-service 读取当前用户可见子图，
// 再整理为前端可视化需要的节点、边、社区、详情和过滤视图。
func main() {
	logger.InitService("knowledge-service")

	cfg, err := config.Load("config/knowledge-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	resolver, err := etcd.NewEtcdResolver(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd resolver失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "knowledge-service")

	ragRPCClient, err := ragservice.NewClient(
		"rag-service",
		append([]client.Option{client.WithResolver(resolver)}, governance.ClientOptions(cfg.Governance.RPC)...)...,
	)
	if err != nil {
		logger.Fatal("创建rag-service客户端失败", "error", err)
	}
	knowledgeService := knowledgesvc.NewKnowledgeService(
		knowledgeclient.NewRAGSource(ragclient.NewRPCClient(ragRPCClient)),
	)

	registry, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}
	svr := knowledgeservice.NewServer(
		knowledgehandler.NewKnowledgeServiceImpl(knowledgeService),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(registry),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)
	health.LogStartup(health.ServiceInfo{Name: "knowledge-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := svr.Run(); err != nil {
		logger.Fatal("knowledge-service RPC服务停止", "error", err)
	}
}
