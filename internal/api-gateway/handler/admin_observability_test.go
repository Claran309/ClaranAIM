package handler

import (
	"ClaranAIM/pkg/config"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestObservabilityLinksReturnsConfiguredPanels(t *testing.T) {
	InitAdminObservabilityLinks(config.ObservabilityConfig{
		Enabled:        true,
		Environment:    "test",
		OTLPEndpoint:   "http://127.0.0.1:8082",
		MetricsAddress: "127.0.0.1:19080",
		GrafanaURL:     "http://grafana.local",
		JaegerURL:      "http://jaeger.local",
		KibanaURL:      "http://kibana.local",
		PrometheusURL:  "http://prometheus.local",
	})

	c := app.NewContext(0)
	NewAdminHandler().ObservabilityLinks(context.Background(), c)

	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Success     bool             `json:"success"`
			Environment string           `json:"environment"`
			Links       []map[string]any `json:"links"`
		} `json:"data"`
	}
	if err := json.Unmarshal(c.Response.Body(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 || !payload.Data.Success {
		t.Fatalf("unexpected response: %#v", payload)
	}
	if payload.Data.Environment != "test" {
		t.Fatalf("environment=%q, want test", payload.Data.Environment)
	}
	if len(payload.Data.Links) != 4 {
		t.Fatalf("links len=%d, want 4", len(payload.Data.Links))
	}
	if payload.Data.Links[0]["url"] != "http://grafana.local" {
		t.Fatalf("grafana link=%v", payload.Data.Links[0]["url"])
	}
	if payload.Data.Links[3]["url"] != "http://prometheus.local/targets" {
		t.Fatalf("prometheus target link=%v", payload.Data.Links[3]["url"])
	}
}
