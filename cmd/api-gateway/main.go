package main

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/internal/api-gateway/handler"
	"ClaranAIM/internal/api-gateway/router"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/messageclient"
	"ClaranAIM/pkg/settingsclient"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// main 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func main() {
	logger.InitService("api-gateway")

	cfg, err := config.Load("config/api-gateway.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	jwt.SetSecretKey(cfg.JWT.SecretKey)
	jwt.SetTokenExpirations(cfg.JWT.AccessExpiration, cfg.JWT.RefreshExpiration)

	client.InitClients(cfg.Etcd.Endpoints, cfg.Governance.RPC, cfg.Governance.AgentRPC)
	health.CheckEtcd(cfg.Etcd.Endpoints, "api-gateway")

	handler.InitFileStorage(cfg)
	handler.InitDTMConfig(cfg.DTM)
	handler.InitMemoryService(memoryclient.NewHTTPClient(cfg.Internal.MemoryServiceURL))
	settingsService := settingsclient.NewHTTPClient(cfg.Internal.SettingsServiceURL)
	handler.InitSettingsService(settingsService)
	handler.InitAgentSettingsService(settingsService)
	handler.InitMessageDomainService(messageclient.NewHTTPClient(cfg.Internal.MsgCoreServiceURL))
	if cfg.DTM.Enabled {
		logger.Info("DTM分布式事务配置已启用", "server", cfg.DTM.Server)
	} else {
		logger.Info("DTM分布式事务配置未启用")
	}

	h := server.Default(server.WithHostPorts(cfg.Service.Address))

	router.RegisterRoutes(h.Engine, cfg)

	health.LogStartup(health.ServiceInfo{
		Name:    "api-gateway",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	h.Spin()
}
