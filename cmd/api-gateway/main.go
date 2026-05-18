package main

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/internal/api-gateway/handler"
	"ClaranAIM/internal/api-gateway/router"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/logger"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	logger.InitService("api-gateway")

	cfg, err := config.Load("config/api-gateway.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	jwt.SetSecretKey(cfg.JWT.SecretKey)
	jwt.SetTokenExpirations(cfg.JWT.AccessExpiration, cfg.JWT.RefreshExpiration)

	client.InitClients(cfg.Etcd.Endpoints, cfg.Governance.RPC)
	health.CheckEtcd(cfg.Etcd.Endpoints, "api-gateway")

	handler.InitFileStorage(cfg)
	handler.InitDTMConfig(cfg.DTM)
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
