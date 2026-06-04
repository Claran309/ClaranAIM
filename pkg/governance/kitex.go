// Package governance 提供跨服务 RPC 治理的公共配置转换。
// 当前项目的 Kitex 客户端和服务端分散在多个入口文件中，这里集中处理
// 超时、熔断、负载均衡和服务端限流，避免每个服务重复拼装同一套 option。
package governance

import (
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/observability"
	"context"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/circuitbreak"
	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/loadbalance"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/cloudwego/kitex/transport"
)

// 普通 RPC 默认 5 秒超时，避免下游服务异常时请求线程无限挂起。
const defaultRPCTimeoutMS = 5000

// clientTimeout 把配置里的毫秒值转换为 Kitex 需要的 Duration。
// allowDisable=true 只给 Agent 长任务使用，表示 timeout_ms<=0 时不设置固定 deadline。
func clientTimeout(cfg config.RPCGovernanceConfig, allowDisable bool) (time.Duration, bool) {
	timeoutMS := cfg.TimeoutMS
	if timeoutMS <= 0 {
		if allowDisable {
			return 0, false
		}
		timeoutMS = defaultRPCTimeoutMS
	}
	return time.Duration(timeoutMS) * time.Millisecond, true
}

// clientOptions 拼装所有 Kitex 客户端的共同治理选项。
// 它固定使用 TTHeader 以便透传元数据，并为未来多实例部署启用加权轮询负载均衡。
func clientOptions(cfg config.RPCGovernanceConfig, allowDisableTimeout bool) []client.Option {
	opts := []client.Option{
		client.WithTransportProtocol(transport.TTHeader),
		client.WithLoadBalancer(loadbalance.NewWeightedRoundRobinBalancer()),
		client.WithMiddleware(observabilityMiddleware("client")),
	}
	if timeout, enabled := clientTimeout(cfg, allowDisableTimeout); enabled {
		opts = append(opts, client.WithRPCTimeout(timeout))
	}
	if cfg.CircuitBreaker {
		opts = append(opts, client.WithCircuitBreaker(circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key)))
	}
	return opts
}

// ClientOptions 返回普通内部 Kitex RPC 客户端治理选项。
// timeout_ms<=0 时使用 5 秒兜底值，避免用户、群聊、消息等短请求无限挂起。
// 熔断 key 使用 Kitex 的 RPCInfo 派生函数，按下游服务/方法维度隔离故障；
// 加权轮询负载均衡为后续多实例注册到 Etcd 后的流量分摊做准备。
func ClientOptions(cfg config.RPCGovernanceConfig) []client.Option {
	return clientOptions(cfg, false)
}

// LongRunningClientOptions 返回 Agent 执行这类长任务 RPC 的客户端治理选项。
// timeout_ms<=0 表示不设置固定 Kitex deadline，由上层异步任务、心跳或人工取消
// 判断任务是否死亡；普通 RPC 不应使用这个选项。
func LongRunningClientOptions(cfg config.RPCGovernanceConfig) []client.Option {
	return clientOptions(cfg, true)
}

// ServerOptions 返回 Kitex 服务端统一治理选项。
// MaxConnections 限制长连接数量，MaxQPS 限制服务入口吞吐；二者都小于等于 0
// 时不追加 WithLimit，让 Kitex 使用默认行为。
func ServerOptions(cfg config.RPCGovernanceConfig) []server.Option {
	opts := []server.Option{server.WithMiddleware(observabilityMiddleware("server"))}
	if cfg.MaxConnections <= 0 && cfg.MaxQPS <= 0 {
		return opts
	}
	return append(opts,
		server.WithLimit(&limit.Option{
			MaxConnections: cfg.MaxConnections,
			MaxQPS:         cfg.MaxQPS,
		}),
	)
}

func observabilityMiddleware(kind string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp interface{}) error {
			service, method := rpcTarget(ctx)
			spanCtx, span := observability.StartSpan(
				ctx,
				"Kitex "+kind+" "+service+"."+method,
				observability.Attribute("rpc.system", "kitex"),
				observability.Attribute("rpc.service", service),
				observability.Attribute("rpc.method", method),
				observability.Attribute("rpc.kind", kind),
			)
			defer span.End()
			start := time.Now()
			err := next(spanCtx, req, resp)
			status := "success"
			if err != nil {
				status = "error"
			}
			observability.RecordRPCRequest(service, method, status, time.Since(start))
			return err
		}
	}
}

func rpcTarget(ctx context.Context) (string, string) {
	service := "unknown"
	method := "unknown"
	ri := rpcinfo.GetRPCInfo(ctx)
	if ri == nil {
		return service, method
	}
	if to := ri.To(); to != nil {
		if name := to.ServiceName(); name != "" {
			service = name
		}
		if m := to.Method(); m != "" {
			method = m
		}
	}
	if invocation := ri.Invocation(); invocation != nil {
		if service == "unknown" && invocation.ServiceName() != "" {
			service = invocation.ServiceName()
		}
		if method == "unknown" && invocation.MethodName() != "" {
			method = invocation.MethodName()
		}
	}
	return service, method
}
