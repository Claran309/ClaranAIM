package main

import (
	"ClaranAIM/internal/file-service/dao"
	"ClaranAIM/internal/file-service/handler"
	"ClaranAIM/internal/file-service/service"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/observability"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动文件服务：加载配置、连接 MySQL、初始化本地/MinIO 存储实现，并通过 Etcd 注册 Kitex RPC。
func main() {
	logger.InitService("file-service")

	cfg, err := config.Load("config/file-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	obsShutdown := observability.InitService(cfg.Service.Name, cfg)
	defer obsShutdown()

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "file-service")

	fileRepo := dao.NewFileRepo(db)

	storageDir := cfg.Storage.Dir
	if storageDir == "" {
		storageDir = "./storage"
	}

	fileService := service.NewFileService(
		fileRepo,
		storageDir,
		cfg.Minio.Endpoint,
		cfg.Minio.AccessKey,
		cfg.Minio.SecretKey,
		cfg.Minio.Bucket,
		cfg.Minio.UseMinio,
	)
	fileHandler := handler.NewFileServiceImpl(fileService)

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "file-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	svr := fileservice.NewServer(
		fileHandler,
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
		Name:    "file-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := svr.Run(); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}
