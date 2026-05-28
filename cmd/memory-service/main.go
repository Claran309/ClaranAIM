package main

import (
	memorydao "ClaranAIM/internal/memory-service/dao"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
)

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
	health.LogStartup(health.ServiceInfo{
		Name:    "memory-service",
		Version: "1.0.0",
		Port:    "local-facade",
	})

	select {}
}
