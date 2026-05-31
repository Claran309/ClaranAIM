package main

import (
	"ClaranAIM/internal/agent-runtime-service/handler"
	"ClaranAIM/internal/agent-runtime-service/service"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
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

// main 启动 Agent 运行时服务。
// runtime 负责 Eino DeepAgent、长会话 JSONL、工作目录和工具/Skill 执行，不直接管理 Agent 权限。
func main() {
	logger.InitService("agent-runtime-service")

	cfg, err := config.Load("config/agent-runtime-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	runtimeSvc := service.NewAgentRuntimeService(service.RuntimeConfig{
		SessionDir:          cfg.Agent.SessionDir,
		DefaultWorkspaceDir: cfg.Agent.AgentRoot,
		CozeloopToken:       cfg.Agent.CozeloopToken,
		CozeloopWorkspaceID: cfg.Agent.CozeloopWSID,
	})
	runtimeHandler := handler.NewAgentRuntimeServiceImpl(runtimeSvc)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "agent-runtime-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := botruntimeservice.NewServer(
		runtimeHandler,
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
		Name:    "agent-runtime-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
