package main

import (
	"context"
	"microservicesDemo/L7-ELK/internal/post-service/dao"
	"microservicesDemo/L7-ELK/internal/post-service/handler"
	"microservicesDemo/L7-ELK/internal/post-service/service"
	"microservicesDemo/L7-ELK/kitex_gen/post/postservice"
	"microservicesDemo/L7-ELK/pkg/logging/zap"
	mq "microservicesDemo/L7-ELK/pkg/mq/kafka"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/kitex-contrib/obs-opentelemetry/provider"
	"github.com/kitex-contrib/obs-opentelemetry/tracing"
	etcd "github.com/kitex-contrib/registry-etcd"
	"go.uber.org/zap"
)

func main() {
	// 初始化Zap
	log.InitLogManager("post-service", "./logs/post-service")
	zap.L().Info("√ 初始化Zap成功")

	// 初始化数据库
	db, err := dao.InitDB("claran:chr070309@tcp(:3306)/microserviceDemo?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		zap.L().Fatal("× 初始化数据库失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 初始化数据库成功")

	// 初始化消息队列生产者
	kafkaProducer, err := mq.NewProducer()
	if err != nil {
		zap.L().Fatal("× 初始化Kafka生产者失败",
			zap.Error(err),
		)
	}
	defer kafkaProducer.Close()
	zap.L().Info("√ 初始化Kafka生产者成功")

	// 初始化消息队列消费者
	kafkaConsumer, err := mq.NewConsumer("user", "1")
	if err != nil {
		zap.L().Fatal("× 初始化Kafka消费者失败",
			zap.Error(err),
		)
	}
	defer kafkaConsumer.Close()
	zap.L().Info("√ 初始化Kafka消费者成功")

	// 初始化 OpenTelemetry provider
	otelProvider := provider.NewOpenTelemetryProvider(
		provider.WithServiceName("post-service"),
		provider.WithExportEndpoint(":8079"),
		provider.WithInsecure(),
		provider.WithEnableTracing(true),
		provider.WithEnableMetrics(true),
	)
	defer otelProvider.Shutdown(context.Background())
	zap.L().Info("√ OpenTelemetry 初始化完成")

	// 初始化仓储层
	postRepo := dao.NewPostRepositoryImpl(db)
	zap.L().Info("√ 初始化仓储层成功")

	// 初始化服务层
	postService := service.NewPostServiceImpl(postRepo, kafkaProducer, kafkaConsumer)
	zap.L().Info("√ 初始化服务层成功")

	// 初始化处理器
	postHandler := handler.NewPostServiceImpl(postService)
	zap.L().Info("√ 初始化处理器成功")

	// 启动消费者监听
	postService.StartConsumer("user", "1")
	defer postService.StopConsumer()
	zap.L().Info("√ 启动Kafka消费者监听成功")

	// 初始化etcd注册中心
	r, err := etcd.NewEtcdRegistry([]string{":2379"})
	if err != nil {
		zap.L().Fatal("× 初始化etcd注册中心失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 初始化etcd注册中心成功")

	// 解析服务地址
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:8081")
	if err != nil {
		zap.L().Fatal("× 解析TCP地址失败",
			zap.Error(err),
		)
	}
	zap.L().Info("√ 解析TCP地址成功")

	// 创建Kitex服务器
	svr := postservice.NewServer(postHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "post-service",
		}),
		server.WithSuite(tracing.NewServerSuite()), // 使用Kitex的tracing
		server.WithRegistry(r))
	zap.L().Info("√ 创建Kitex服务器成功")

	// 打印服务启动信息
	zap.L().Info("启动post-service服务",
		zap.String("address", "127.0.0.1:8081"),
		zap.String("etcd", ":2379"),
		zap.String("otel_collector", ":8079"),
	)
	zap.L().Info("监控面板信息:",
		zap.String("jaeger", "http://:8085"),
		zap.String("prometheus", ""),
		zap.String("grafana", "http://:8086 (admin/admin123)"),
	)

	// 启动服务
	err = svr.Run()
	if err != nil {
		zap.L().Fatal("× 启动post-service服务失败",
			zap.Error(err),
		)
	}
}
