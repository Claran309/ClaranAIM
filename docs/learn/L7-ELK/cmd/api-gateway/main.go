package main

import (
	"context"
	"microservicesDemo/L7-ELK/internal/api-gateway/client"
	api_gateway2 "microservicesDemo/L7-ELK/internal/api-gateway/handler"
	"microservicesDemo/L7-ELK/internal/api-gateway/router"
	"microservicesDemo/L7-ELK/pkg/logging/zap"

	"github.com/cloudwego/hertz/pkg/app/server"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/provider"
	"github.com/hertz-contrib/obs-opentelemetry/tracing"
	"go.uber.org/zap"
)

func main() {
	// 初始化Zap
	log.InitLogManager("api-gateway", "./logs/api-gateway")

	// 初始化OTel Provider
	p := hertztracing.NewOpenTelemetryProvider(
		hertztracing.WithServiceName("api-gateway"),
		hertztracing.WithExportEndpoint("localhost:8079"),
		hertztracing.WithInsecure(),
	)
	defer p.Shutdown(context.Background())
	zap.L().Info("√ 初始化Hertz OTel Provider成功")

	// 初始化Tracer
	tracer, cfg := tracing.NewServerTracer()

	// 创建rpc客户端
	clients := client.InitClients()
	zap.L().Info("√ 创建rpc客户端成功")

	// 创建http处理器
	httpHandler := api_gateway2.NewHttpHandler(clients)
	zap.L().Info("√ 创建http处理器成功")

	// 启动hertz服务器
	h := server.Default(tracer)
	zap.L().Info("√ 启动hertz服务器成功")

	// 注册全局Tracing中间件
	h.Use(tracing.ServerMiddleware(cfg))

	// 注册路由
	router.InitRouters(h, httpHandler)
	zap.L().Info("√ 注册路由成功")

	h.Spin()
}
