package main

import (
	"context"
	"microservicesDemo/L7-ELK/internal/user-service/dao"
	"microservicesDemo/L7-ELK/internal/user-service/handler"
	"microservicesDemo/L7-ELK/internal/user-service/service"
	"microservicesDemo/L7-ELK/kitex_gen/user/userservice"
	"microservicesDemo/L7-ELK/pkg/logging/zap"
	mq "microservicesDemo/L7-ELK/pkg/mq/kafka"
	"net"

	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/kitex-contrib/obs-opentelemetry/provider"
	"github.com/kitex-contrib/obs-opentelemetry/tracing"
	etcd "github.com/kitex-contrib/registry-etcd"
	"go.uber.org/zap"
)

func main() {
	// 初始化Zap
	log.InitLogManager("user-service", "./logs/user-service")
	zap.L().Info("√ 初始化Zap成功")

	// 初始化数据库
	db, err := dao.InitDB("claran:chr070309@tcp(:3306)/microserviceDemo?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		zap.L().Fatal("× 初始化数据库失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 初始化数据库成功")

	// 初始化Kafka生产者
	kafkaProducer, err := mq.NewProducer()
	if err != nil {
		zap.L().Fatal("× 初始化Kafka生产者失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 初始化Kafka生产者成功")

	// 初始化 OpenTelemetry provider
	otelProvider := provider.NewOpenTelemetryProvider(
		provider.WithServiceName("user-service"),
		provider.WithExportEndpoint(":8079"),
		provider.WithInsecure(),
		provider.WithEnableTracing(true),
		provider.WithEnableMetrics(true),
	)
	defer otelProvider.Shutdown(context.Background())
	zap.L().Info("√ OpenTelemetry 初始化完成")

	// 初始化仓储层
	userRepo := dao.NewUserRepo(db)
	zap.L().Info("√ 初始化仓储层成功")

	// 初始化服务层
	userService := service.NewUserService(userRepo, kafkaProducer)
	zap.L().Info("√ 初始化服务层成功")

	// 初始化处理器
	userHandler := handler.NewUserServiceImpl(userService)
	zap.L().Info("√ 初始化处理器成功")

	// 初始化etcd注册中心
	r, err := etcd.NewEtcdRegistry([]string{":2379"})
	if err != nil {
		zap.L().Fatal("× 初始化etcd注册中心失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 初始化etcd注册中心成功")

	// 解析服务地址
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:8080")
	if err != nil {
		zap.L().Fatal("× 解析TCP地址失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 解析TCP地址成功")

	// 创建Kitex服务器
	svr := userservice.NewServer(userHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "user-service",
		}),
		server.WithRegistry(r),
		server.WithLimit(&limit.Option{
			MaxConnections: 1000,
			MaxQPS:         1000,
		}),
		server.WithSuite(tracing.NewServerSuite()), // 使用Kitex的tracing
	)
	zap.L().Info("√ 创建Kitex服务器成功")

	// 打印服务启动信息
	zap.L().Info("启动user-service服务",
		zap.String("address", "127.0.0.1:8080"),
		zap.String("etcd", ":2379"),
		zap.String("otel_collector", ":8079"),
		zap.Int("max_connections", 1000),
		zap.Int("max_qps", 1000),
	)
	zap.L().Info("监控面板信息:",
		zap.String("jaeger", "http://:8085"),
		zap.String("prometheus", ""),
		zap.String("grafana", "http://:8086 (admin/admin123)"),
	)

	// 启动服务
	err = svr.Run()
	if err != nil {
		zap.L().Fatal("× 启动user-service服务失败",
			zap.Error(err),
		)
	}
}
