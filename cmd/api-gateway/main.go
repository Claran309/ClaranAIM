package main

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/internal/api-gateway/router"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/jwt"
	"log"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// api-gateway 启动入口
// api-gateway 是整个系统的HTTP入口，负责：
//   1. 接收前端HTTP请求
//   2. JWT认证和CORS中间件处理
//   3. 路由分发到对应的Handler
//   4. Handler通过RPC调用后端微服务
//
// 启动流程：加载配置 → 初始化JWT密钥 → 初始化RPC客户端 → 注册路由 → 启动HTTP服务
func main() {
	// 加载配置文件（config/api-gateway.yaml + 环境变量）
	cfg, err := config.Load("config/api-gateway.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化JWT密钥（用于认证中间件验证Token）
	jwt.SetSecretKey(cfg.JWT.SecretKey)

	// 初始化RPC客户端（通过Etcd发现后端微服务）
	// 初始化后可通过 client.UserClient / client.GroupClient / client.MessageClient 调用后端服务
	client.InitClients(cfg.Etcd.Endpoints)

	// 创建Hertz HTTP服务器
	h := server.Default(server.WithHostPorts(cfg.Service.Address))

	// 注册所有API路由（详见 router/router.go）
	router.RegisterRoutes(h.Engine)

	log.Printf("api-gateway 启动在 %s", cfg.Service.Address)
	h.Spin()
}
