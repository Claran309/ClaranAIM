package governance

import (
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/observability"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOrdinaryRPCUsesDefaultTimeoutWhenConfigIsEmpty(t *testing.T) {
	timeout, enabled := clientTimeout(config.RPCGovernanceConfig{}, false)
	if !enabled {
		t.Fatal("ordinary RPC timeout disabled, want default timeout")
	}
	if timeout != 5*time.Second {
		t.Fatalf("ordinary RPC timeout = %v, want 5s", timeout)
	}
}

func TestLongRunningRPCCanDisableTimeout(t *testing.T) {
	timeout, enabled := clientTimeout(config.RPCGovernanceConfig{TimeoutMS: 0}, true)
	if enabled {
		t.Fatalf("long-running RPC timeout enabled with %v, want disabled", timeout)
	}
}

func TestLongRunningRPCUsesConfiguredPositiveTimeout(t *testing.T) {
	timeout, enabled := clientTimeout(config.RPCGovernanceConfig{TimeoutMS: 120000}, true)
	if !enabled {
		t.Fatal("long-running RPC timeout disabled, want configured timeout")
	}
	if timeout != 120*time.Second {
		t.Fatalf("long-running RPC timeout = %v, want 120s", timeout)
	}
}

func TestObservabilityMiddlewareRecordsRPCResult(t *testing.T) {
	runtime, err := observability.Init("governance-test", config.ObservabilityConfig{
		Enabled:        true,
		Environment:    "test",
		MetricsAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("init observability: %v", err)
	}
	defer runtime.Shutdown(context.Background())

	mw := observabilityMiddleware("client")
	next := mw(func(ctx context.Context, req, resp interface{}) error {
		return errors.New("boom")
	})
	_ = next(context.Background(), nil, nil)

	body := readGovernanceMetrics(t, runtime.MetricsAddress)
	if !contains(body, `claran_rpc_requests_total{method="unknown",service="unknown",status="error"}`) {
		t.Fatalf("rpc metric not found in metrics body:\n%s", body)
	}
}

func readGovernanceMetrics(t *testing.T, address string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get("http://" + address + "/metrics")
		if err == nil && resp != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return string(body)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("metrics endpoint not ready: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func contains(value, needle string) bool {
	return strings.Contains(value, needle)
}
