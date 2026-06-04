package observability

import (
	"ClaranAIM/pkg/config"
	"context"
	"net/http"
	"testing"
	"time"
)

func TestInitDisabledReturnsNoop(t *testing.T) {
	runtime, err := Init("test-service", config.ObservabilityConfig{Enabled: false})
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if runtime.Enabled {
		t.Fatal("runtime enabled = true, want false")
	}
	if runtime.MetricsAddress != "" {
		t.Fatalf("metrics address=%q, want empty", runtime.MetricsAddress)
	}
}

func TestInitStartsMetricsEndpoint(t *testing.T) {
	runtime, err := Init("test-service", config.ObservabilityConfig{
		Enabled:        true,
		Environment:    "test",
		MetricsAddress: "127.0.0.1:0",
		OTLPEndpoint:   "",
	})
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	defer runtime.Shutdown(context.Background())
	if !runtime.Enabled {
		t.Fatal("runtime enabled = false, want true")
	}
	if runtime.MetricsAddress == "" || runtime.MetricsAddress == "127.0.0.1:0" {
		t.Fatalf("metrics address=%q, want resolved listener address", runtime.MetricsAddress)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var body []byte
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+runtime.MetricsAddress+"/metrics", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp != nil {
			body = make([]byte, 256)
			n, _ := resp.Body.Read(body)
			_ = resp.Body.Close()
			body = body[:n]
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if ctx.Err() != nil {
			t.Fatalf("metrics endpoint did not become ready: %v", ctx.Err())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !containsBytes(body, []byte("claran_service_start_total")) {
		t.Fatalf("metrics body does not contain claran metric prefix: %q", string(body))
	}
}

func TestRecordHTTPAndRPCMetricsDoNotPanicWhenDisabled(t *testing.T) {
	RecordHTTPRequest("GET", "/api/v1/test", 200, 12*time.Millisecond)
	RecordRPCRequest("test-service", "Ping", "success", 3*time.Millisecond)
	RecordDependencyCheck("mysql", "ok")
}

func TestNormalizeOTLPEndpointUsesSignalSpecificPath(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		defaultPath string
		want        string
	}{
		{
			name:        "base url gets trace path",
			raw:         "http://127.0.0.1:8082",
			defaultPath: "/v1/traces",
			want:        "http://127.0.0.1:8082/v1/traces",
		},
		{
			name:        "full trace endpoint rewrites to metrics when requested",
			raw:         "http://127.0.0.1:8082/v1/traces",
			defaultPath: "/v1/metrics",
			want:        "http://127.0.0.1:8082/v1/metrics",
		},
		{
			name:        "custom path is preserved",
			raw:         "http://collector.internal/custom/otlp",
			defaultPath: "/v1/metrics",
			want:        "http://collector.internal/custom/otlp",
		},
		{
			name:        "invalid endpoint is preserved for exporter error handling",
			raw:         "collector:4318",
			defaultPath: "/v1/traces",
			want:        "collector:4318",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOTLPEndpoint(tt.raw, tt.defaultPath); got != tt.want {
				t.Fatalf("normalizeOTLPEndpoint(%q, %q)=%q, want %q", tt.raw, tt.defaultPath, got, tt.want)
			}
		})
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
