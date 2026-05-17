// Package governance 提供跨服务 RPC 治理的公共配置转换。
// 当前项目的 Kitex 客户端和服务端分散在多个入口文件中，这里集中处理
// 超时、熔断、负载均衡和服务端限流，避免每个服务重复拼装同一套 option。
package governance

import (
	"ClaranAIM/pkg/config"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/circuitbreak"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/loadbalance"
	"github.com/cloudwego/kitex/server"
	"github.com/cloudwego/kitex/transport"
)

const defaultRPCTimeoutMS = 5000

// ClientOptions 返回 API 网关调用内部 Kitex 服务时使用的治理选项。
// 熔断 key 使用 Kitex 的 RPCInfo 派生函数，按下游服务/方法维度隔离故障；
// 加权轮询负载均衡为后续多实例注册到 Etcd 后的流量分摊做准备。
func ClientOptions(cfg config.RPCGovernanceConfig) []client.Option {
	timeoutMS := cfg.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultRPCTimeoutMS
	}

	opts := []client.Option{
		client.WithTransportProtocol(transport.TTHeader),
		client.WithRPCTimeout(time.Duration(timeoutMS) * time.Millisecond),
		client.WithLoadBalancer(loadbalance.NewWeightedRoundRobinBalancer()),
	}
	if cfg.CircuitBreaker {
		opts = append(opts, client.WithCircuitBreaker(circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key)))
	}
	return opts
}

// ServerOptions 返回 Kitex 服务端统一治理选项。
// MaxConnections 限制长连接数量，MaxQPS 限制服务入口吞吐；二者都小于等于 0
// 时不追加 WithLimit，让 Kitex 使用默认行为。
func ServerOptions(cfg config.RPCGovernanceConfig) []server.Option {
	if cfg.MaxConnections <= 0 && cfg.MaxQPS <= 0 {
		return nil
	}
	return []server.Option{
		server.WithLimit(&limit.Option{
			MaxConnections: cfg.MaxConnections,
			MaxQPS:         cfg.MaxQPS,
		}),
	}
}
