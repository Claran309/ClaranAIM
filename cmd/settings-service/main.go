package main

import (
	settingsdao "ClaranAIM/internal/settings-service/dao"
	"ClaranAIM/internal/settings-service/handler"
	settingssvc "ClaranAIM/internal/settings-service/service"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动系统设置服务。
// settings-service 通过 Kitex RPC 暴露 LLM Profile、Prompt 和 Agent Skill 配置，不直接参与浏览器路由。
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
	}, settingssvc.WithSkillStorageRoot(cfg.Skills.Dir))

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "settings-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}
	svr := settingsservice.NewServer(
		handler.NewSettingsServiceImpl(settingsService),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(r),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)
	health.LogStartup(health.ServiceInfo{Name: "settings-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := svr.Run(); err != nil {
		logger.Fatal("settings-service RPC服务停止", "error", err)
	}
}
