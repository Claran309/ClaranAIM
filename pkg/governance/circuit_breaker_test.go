package governance

import (
	"ClaranAIM/pkg/config"
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/kitex/pkg/circuitbreak"
	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
)

func configWithBreaker(enabled bool) config.RPCGovernanceConfig {
	return config.RPCGovernanceConfig{TimeoutMS: 1000, CircuitBreaker: enabled}
}

// fakeRPCInfo 让 RPCInfo2Key 能生成稳定的熔断 key，避免依赖真实 Kitex 调用。
type fakeRPCInfo struct {
	rpcinfo.RPCInfo
}

func (f fakeRPCInfo) From() rpcinfo.EndpointInfo     { return fakeEndpoint{} }
func (f fakeRPCInfo) To() rpcinfo.EndpointInfo       { return fakeEndpoint{} }
func (f fakeRPCInfo) Invocation() rpcinfo.Invocation { return fakeEndpoint{} }

type fakeEndpoint struct {
	rpcinfo.EndpointInfo
	rpcinfo.Invocation
}

func (fakeEndpoint) ServiceName() string { return "user-service" }
func (fakeEndpoint) Method() string      { return "GetUserInfo" }
func (fakeEndpoint) MethodName() string  { return "GetUserInfo" }

func circuitTestContext() context.Context {
	return rpcinfo.NewCtxWithRPCInfo(context.Background(), fakeRPCInfo{})
}

// TestServiceCircuitBreakerTripsAndRecovers 验证连续超时后熔断打开，关闭熔断后又恢复调用。
func TestServiceCircuitBreakerTripsAndRecovers(t *testing.T) {
	suite := circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key)
	ctx := circuitTestContext()
	key := circuitbreak.RPCInfo2Key(rpcinfo.GetRPCInfo(ctx))
	suite.UpdateServiceCBConfig(key, circuitbreak.CBConfig{Enable: true, ErrRate: 0.5, MinSample: 20})

	timeoutEndpoint := endpoint.Chain(suite.ServiceCBMW())(func(context.Context, interface{}, interface{}) error {
		return kerrors.ErrRPCTimeout.WithCause(errors.New("upstream timeout"))
	})

	tripped := false
	for i := 0; i < 300; i++ {
		if err := timeoutEndpoint(ctx, nil, nil); errors.Is(err, kerrors.ErrCircuitBreak) {
			tripped = true
			break
		}
	}
	if !tripped {
		t.Fatal("circuit breaker never tripped after repeated timeouts")
	}

	suite.UpdateServiceCBConfig(key, circuitbreak.CBConfig{Enable: false})
	successCalls := 0
	successEndpoint := endpoint.Chain(suite.ServiceCBMW())(func(context.Context, interface{}, interface{}) error {
		successCalls++
		return nil
	})
	for i := 0; i < 50; i++ {
		if err := successEndpoint(ctx, nil, nil); err != nil {
			t.Fatalf("call %d after disabling breaker error = %v", i, err)
		}
	}
	if successCalls != 50 {
		t.Fatalf("success calls = %d, want 50", successCalls)
	}
}

// TestCircuitBreakerOptionsFollowConfig 验证治理配置开关会真实影响客户端选项数量。
func TestCircuitBreakerOptionsFollowConfig(t *testing.T) {
	withoutBreaker := ClientOptions(configWithBreaker(false))
	withBreaker := ClientOptions(configWithBreaker(true))
	if len(withBreaker) <= len(withoutBreaker) {
		t.Fatalf("breaker options = %d, want more than %d", len(withBreaker), len(withoutBreaker))
	}
}
