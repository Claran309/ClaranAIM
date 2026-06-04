package middleware

import (
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/observability"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
)

func TestRequireRoleAllowsAdmin(t *testing.T) {
	c := app.NewContext(0)
	c.Set("role", jwt.RoleAdmin)

	RequireRole(jwt.RoleAdmin)(context.Background(), c)

	if c.IsAborted() {
		t.Fatal("admin role should be allowed")
	}
}

func TestRequireRoleRejectsUser(t *testing.T) {
	c := app.NewContext(0)
	c.Set("role", jwt.RoleUser)
	c.Request = protocol.Request{}
	c.Response = protocol.Response{}

	RequireRole(jwt.RoleAdmin)(context.Background(), c)

	if !c.IsAborted() {
		t.Fatal("user role should be rejected")
	}
	if c.Response.StatusCode() != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", c.Response.StatusCode(), http.StatusForbidden)
	}
}

func TestObservabilityMiddlewareRecordsHTTPMetrics(t *testing.T) {
	runtime, err := observability.Init("api-gateway-test", config.ObservabilityConfig{
		Enabled:        true,
		Environment:    "test",
		MetricsAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("init observability: %v", err)
	}
	defer runtime.Shutdown(context.Background())

	c := app.NewContext(1)
	c.Request = protocol.Request{}
	c.Response = protocol.Response{}
	c.Request.SetMethod(http.MethodGet)
	c.Request.SetRequestURI("/api/v1/ping?x=1")
	c.SetFullPath("/api/v1/ping")
	c.Response.SetStatusCode(http.StatusAccepted)

	ObservabilityMiddleware()(context.Background(), c)

	body := readMetricsBody(t, runtime.MetricsAddress)
	if !strings.Contains(body, `claran_http_requests_total{method="GET",route="/api/v1/ping",service="api-gateway-test",status="202"}`) {
		t.Fatalf("http metric not found in metrics body:\n%s", body)
	}
}

func readMetricsBody(t *testing.T, address string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get("http://" + address + "/metrics")
		if err == nil && resp != nil {
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return string(data)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("metrics endpoint not ready: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
