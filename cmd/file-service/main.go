package main

import (
	"ClaranAIM/internal/file-service/dao"
	"ClaranAIM/internal/file-service/handler"
	"ClaranAIM/internal/file-service/service"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/pkg/config"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	cfg, err := config.Load("config/file-service.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	db, err := dao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	log.Println("file-service 数据库初始化成功")

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
		log.Fatal("创建etcd注册中心失败:", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		log.Fatal("解析服务地址失败:", err)
	}

	svr := fileservice.NewServer(
		fileHandler,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: cfg.Service.Name,
		}),
		server.WithRegistry(r),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)

	log.Printf("file-service 启动在 %s (MinIO: %v)", cfg.Service.Address, cfg.Minio.UseMinio)
	if err := svr.Run(); err != nil {
		log.Fatal("file-service 启动失败:", err)
	}
}
