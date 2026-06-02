package main

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/internal/api-gateway/handler"
	"ClaranAIM/internal/api-gateway/router"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/conversationintelclient"
	"ClaranAIM/pkg/documentparser"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/knowledgeclient"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/mcpclient"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/ragclient"
	"ClaranAIM/pkg/settingsclient"
	"ClaranAIM/pkg/websearchclient"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// main 启动浏览器唯一入口 api-gateway。
// 它负责加载配置、初始化 JWT/RPC/内部 HTTP 客户端，并把所有 /api/v1 路由挂到 Hertz。
func main() {
	logger.InitService("api-gateway")

	cfg, err := config.Load("config/api-gateway.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	jwt.SetSecretKey(cfg.JWT.SecretKey)
	jwt.SetTokenExpirations(cfg.JWT.AccessExpiration, cfg.JWT.RefreshExpiration)

	client.InitClients(cfg.Etcd.Endpoints, cfg.Governance.RPC, cfg.Governance.AgentRPC)
	health.CheckEtcd(cfg.Etcd.Endpoints, "api-gateway")

	handler.InitFileStorage(cfg)
	handler.InitDTMConfig(cfg.DTM)
	handler.InitMemoryService(memoryclient.NewRPCClient(client.MemoryClient))
	ragService := ragclient.NewRPCClient(client.RAGClient)
	handler.InitRAGService(ragService)
	handler.InitKnowledgeService(knowledgeclient.NewRPCClient(client.KnowledgeClient))
	if cfg.Document.OCRProvider == "glm" && cfg.Document.OCRURL != "" && cfg.Document.OCRAPIKey != "" {
		ocrProvider := documentparser.NewGLMLayoutOCRProvider(cfg.Document.OCRURL, cfg.Document.OCRAPIKey, cfg.Document.OCRModel)
		handler.InitDocumentOCR(ocrProvider)
		handler.InitFileOCR(ocrProvider)
		logger.Info("文档OCR解析已启用", "provider", cfg.Document.OCRProvider, "model", cfg.Document.OCRModel)
	} else {
		logger.Info("文档OCR解析未启用，扫描件PDF和图片上传将无法自动抽取文本")
	}
	settingsService := settingsclient.NewRPCClient(client.SettingsClient)
	handler.InitSettingsService(settingsService)
	handler.InitAgentSettingsService(settingsService)
	handler.InitWebSearchService(websearchclient.NewRPCClient(client.WebSearchClient))
	handler.InitConversationIntelligenceService(conversationintelclient.NewRPCClient(client.ConversationIntelligenceClient))
	handler.InitMCPService(mcpclient.NewRPCClient(client.MCPGatewayClient))
	if cfg.DTM.Enabled {
		logger.Info("DTM分布式事务配置已启用", "server", cfg.DTM.Server)
	} else {
		logger.Info("DTM分布式事务配置未启用")
	}

	h := server.Default(server.WithHostPorts(cfg.Service.Address))

	router.RegisterRoutes(h.Engine, cfg)

	health.LogStartup(health.ServiceInfo{
		Name:    "api-gateway",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	h.Spin()
}
