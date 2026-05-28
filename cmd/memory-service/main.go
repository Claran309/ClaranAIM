package main

import (
	memorydao "ClaranAIM/internal/memory-service/dao"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/internal/memory-service/transport"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"net/http"
)

// main 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func main() {
	logger.InitService("memory-service")

	cfg, err := config.Load("config/memory-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := memorydao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "memory-service")
	memoryService := memorysvc.NewMemoryService(memorydao.NewMemoryRepo(db))
	health.LogStartup(health.ServiceInfo{
		Name:    "memory-service",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := http.ListenAndServe(cfg.Service.Address, transport.NewHTTPHandler(memoryService)); err != nil {
		logger.Fatal("memory-service HTTP服务停止", "error", err)
	}
}
