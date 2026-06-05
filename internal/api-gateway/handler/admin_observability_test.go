package handler

import (
	"ClaranAIM/kitex_gen/admin"
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

func TestParseAdminReviewItemIDAcceptsStringSnowflakeID(t *testing.T) {
	got, err := parseAdminReviewItemID("56300721120968704")
	if err != nil {
		t.Fatalf("parseAdminReviewItemID returned error: %v", err)
	}
	if got != 56300721120968704 {
		t.Fatalf("id=%d, want exact snowflake id", got)
	}
}

func TestGroupAdminMCPTracesUsesStableFingerprintWhenTraceIDDiffers(t *testing.T) {
	resp := &admin.ListMCPTracesResp{
		Success: true,
		Total:   2,
		Traces: []*admin.AdminMCPTrace{
			{Id: 1, TraceId: "mcp_1", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "web_search", Source: "builtin", ServerName: "web-search-service", Status: "success", LatencyMs: 10, CreatedAt: "2026-06-05T01:00:00+08:00"},
			{Id: 2, TraceId: "mcp_2", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "web_search", Source: "builtin", ServerName: "web-search-service", Status: "success", LatencyMs: 15, CreatedAt: "2026-06-05T01:00:01+08:00"},
		},
	}

	grouped := groupAdminMCPTraces(resp)
	traces, ok := grouped["traces"].([]map[string]interface{})
	if !ok {
		t.Fatalf("grouped traces type = %T", grouped["traces"])
	}
	if len(traces) != 1 {
		t.Fatalf("group len=%d, want duplicate MCP calls grouped by stable fingerprint", len(traces))
	}
	if traces[0]["call_count"] != int64(2) || traces[0]["total_latency_ms"] != int64(25) {
		t.Fatalf("group=%#v, want call_count=2 total_latency_ms=25", traces[0])
	}
}

func TestGroupAdminMCPTracesDoesNotSplitSameToolByStatus(t *testing.T) {
	resp := &admin.ListMCPTracesResp{
		Success: true,
		Total:   2,
		Traces: []*admin.AdminMCPTrace{
			{Id: 1, TraceId: "mcp_1", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "search_knowledge", Source: "builtin", ServerName: "mcp-gateway-service", Status: "success", LatencyMs: 10, CreatedAt: "2026-06-05T00:51:34+08:00"},
			{Id: 2, TraceId: "mcp_2", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "search_knowledge", Source: "builtin", ServerName: "mcp-gateway-service", Status: "failed", ErrorMessage: "query timeout", LatencyMs: 15, CreatedAt: "2026-06-04T02:31:07+08:00"},
		},
	}

	grouped := groupAdminMCPTraces(resp)
	traces, ok := grouped["traces"].([]map[string]interface{})
	if !ok {
		t.Fatalf("grouped traces type = %T", grouped["traces"])
	}
	if len(traces) != 1 {
		t.Fatalf("group len=%d, want same MCP tool grouped across success/failed statuses", len(traces))
	}
	if traces[0]["call_count"] != int64(2) || traces[0]["status"] != "failed" || traces[0]["error_message"] != "query timeout" {
		t.Fatalf("group=%#v, want aggregate failed status with both calls", traces[0])
	}
}

func TestGroupAdminMCPTracesDoesNotSplitSameToolAcrossBackendSources(t *testing.T) {
	resp := &admin.ListMCPTracesResp{
		Success: true,
		Total:   3,
		Traces: []*admin.AdminMCPTrace{
			{Id: 1, TraceId: "mcp_new_1", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "search_knowledge", Source: "rag-service", ServerName: "rag-service", Status: "success", LatencyMs: 10, CreatedAt: "2026-06-05T00:51:34+08:00"},
			{Id: 2, TraceId: "mcp_new_2", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "search_knowledge", Source: "mcp-gateway", ServerName: "mcp-gateway-service", Status: "success", LatencyMs: 20, CreatedAt: "2026-06-05T00:51:35+08:00"},
			{Id: 3, TraceId: "mcp_old_1", UserId: 1001, AgentId: 2, ConversationId: 3, ToolName: "search_knowledge", Source: "builtin", ServerName: "agent-runtime-service", Status: "failed", ErrorMessage: "legacy timeout", LatencyMs: 30, CreatedAt: "2026-06-04T02:31:07+08:00"},
		},
	}

	grouped := groupAdminMCPTraces(resp)
	traces, ok := grouped["traces"].([]map[string]interface{})
	if !ok {
		t.Fatalf("grouped traces type = %T", grouped["traces"])
	}
	if len(traces) != 1 {
		t.Fatalf("group len=%d, want same MCP tool grouped across backend source/server drift", len(traces))
	}
	if traces[0]["call_count"] != int64(3) || traces[0]["total_latency_ms"] != int64(60) {
		t.Fatalf("group=%#v, want all backend calls aggregated", traces[0])
	}
	if traces[0]["status"] != "failed" || traces[0]["error_message"] != "legacy timeout" {
		t.Fatalf("group=%#v, want failed legacy status preserved inside aggregate", traces[0])
	}
}
