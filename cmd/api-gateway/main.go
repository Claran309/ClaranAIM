package main

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/internal/api-gateway/router"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/jwt"
	"log"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	cfg, err := config.Load("config/api-gateway.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化JWT密钥
	jwt.SetSecretKey(cfg.JWT.SecretKey)

	// 初始化RPC客户端
	client.InitClients(cfg.Etcd.Endpoints)

	// 创建Hertz HTTP服务器
	h := server.Default(server.WithHostPorts(cfg.Service.Address))

	// 注册路由
	router.RegisterRoutes(h.Engine)

	log.Printf("api-gateway 启动在 %s", cfg.Service.Address)
	h.Spin()
}
