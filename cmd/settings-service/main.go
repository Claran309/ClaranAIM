package main

import (
	settingsdao "ClaranAIM/internal/settings-service/dao"
	settingssvc "ClaranAIM/internal/settings-service/service"
	"ClaranAIM/internal/settings-service/transport"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"net/http"
)

// main 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func main() {
	logger.InitService("settings-service")

	cfg, err := config.Load("config/settings-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	db, err := settingsdao.InitDB(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("初始化数据库失败", "error", err)
	}
	sqlDB, _ := db.DB()
	health.CheckMySQL(sqlDB, "settings-service")
	settingsService := settingssvc.NewSettingsService(settingsdao.NewSettingsRepo(db), settingssvc.DefaultLLMConfig{
		APIKey:  cfg.LLM.DefaultAPIKey,
		BaseURL: cfg.LLM.DefaultBaseURL,
		Model:   cfg.LLM.DefaultModel,
	})
	health.LogStartup(health.ServiceInfo{Name: "settings-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := http.ListenAndServe(cfg.Service.Address, transport.NewHTTPHandler(settingsService)); err != nil {
		logger.Fatal("settings-service HTTP服务停止", "error", err)
	}
}
