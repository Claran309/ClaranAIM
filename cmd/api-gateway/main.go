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

	client.InitClients(cfg.Etcd.Endpoints)
	health.CheckEtcd(cfg.Etcd.Endpoints, "api-gateway")

	handler.InitFileStorage(cfg)

	h := server.Default(server.WithHostPorts(cfg.Service.Address))

	router.RegisterRoutes(h.Engine)

	health.LogStartup(health.ServiceInfo{
		Name:    "api-gateway",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	h.Spin()
}
