package main

import (
	websearchhandler "ClaranAIM/internal/web-search-service/handler"
	websearchsvc "ClaranAIM/internal/web-search-service/service"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/governance"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/logger"
	"net"
	"strings"
	"time"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// main 启动轻量 Web Search Augmentation RPC 服务。
func main() {
	logger.InitService("web-search-service")

	cfg, err := config.Load("config/web-search-service.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	r, err := etcd.NewEtcdRegistry(cfg.Etcd.Endpoints)
	if err != nil {
		logger.Fatal("创建etcd注册中心失败", "error", err)
	}
	health.CheckEtcd(cfg.Etcd.Endpoints, "web-search-service")

	addr, err := net.ResolveTCPAddr("tcp", cfg.Service.Address)
	if err != nil {
		logger.Fatal("解析服务地址失败", "error", err)
	}

	webSearchService := websearchsvc.NewWebSearchService(websearchsvc.WebSearchOptions{
		MaxResults:      cfg.WebSearch.MaxResults,
		MaxFetch:        cfg.WebSearch.MaxFetch,
		MaxPassages:     cfg.WebSearch.MaxPassages,
		MaxCharsPerPage: cfg.WebSearch.MaxCharsPerPage,
		TrustedDomains:  splitDomains(cfg.WebSearch.TrustedDomains),
		UserAgent:       cfg.WebSearch.UserAgent,
		Timeout:         time.Duration(cfg.WebSearch.TimeoutMS) * time.Millisecond,
	})
	svr := websearchservice.NewServer(
		websearchhandler.NewWebSearchServiceImpl(webSearchService),
		append([]server.Option{
			server.WithServiceAddr(addr),
			server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.Service.Name}),
			server.WithRegistry(r),
			server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
		}, governance.ServerOptions(cfg.Governance.RPC)...)...,
	)

	health.LogStartup(health.ServiceInfo{Name: "web-search-service", Version: "1.0.0", Port: cfg.Service.Address})
	if err := svr.Run(); err != nil {
		logger.Fatal("web-search-service RPC服务停止", "error", err)
	}
}

func splitDomains(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
