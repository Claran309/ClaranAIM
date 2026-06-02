package service

import (
	"ClaranAIM/pkg/settingsclient"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListToolsIncludesBuiltins(t *testing.T) {
	svc := NewMCPGatewayService(Dependencies{})
	tools, err := svc.ListTools(context.Background(), ToolContext{UserID: 1001})
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	want := map[string]bool{
		ToolWebSearch:             false,
		ToolSearchMemory:          false,
		ToolSearchKnowledge:       false,
		ToolQueryKnowledgeGraph:   false,
		ToolSummarizeConversation: false,
	}
	for _, tool := range tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("builtin tool %s not found in %#v", name, tools)
		}
	}
}

func TestRemoteMCPListAndCall(t *testing.T) {
	var called bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"tools":[{"name":"remote_echo","description":"echo","inputSchema":{"type":"object"}}]}}`))
		case "tools/call":
			called = true
			if req.Params["name"] != "remote_echo" {
				t.Fatalf("tool name = %v, want remote_echo", req.Params["name"])
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"remote ok"}]}}`))
		default:
			t.Fatalf("unexpected method %s", req.Method)
		}
	}))
	defer remote.Close()

	svc := NewMCPGatewayService(Dependencies{
		Settings: fakeMCPSettings{servers: []settingsclient.MCPServerConfig{{
			Name:           "remote",
			Transport:      settingsclient.MCPTransportStreamableHTTP,
			EndpointURL:    remote.URL,
			TrustLevel:     settingsclient.MCPTrustLow,
			AllowToolsJSON: `["remote_echo"]`,
		}}},
		RemoteHTTPClient: remote.Client(),
	})

	tools, err := svc.ListTools(context.Background(), ToolContext{UserID: 1001})
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	var found bool
	for _, tool := range tools {
		if tool.Name == "remote_echo" {
			found = true
			if !tool.RequiresApproval {
				t.Fatal("low trust remote tool should require approval")
			}
		}
	}
	if !found {
		t.Fatalf("remote_echo not found in %#v", tools)
	}

	result, err := svc.CallTool(context.Background(), CallToolInput{UserID: 1001, ToolName: "remote_echo", ArgumentsJSON: `{"text":"hi"}`})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if !called || !result.Success || result.ResultText != "remote ok" {
		t.Fatalf("result=%#v called=%t, want remote ok", result, called)
	}
}

func TestRemoteMCPRejectsInvalidArgumentsJSON(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method == "tools/call" {
			t.Fatal("invalid arguments_json must be rejected before remote tools/call")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"tools":[{"name":"remote_echo","description":"echo","inputSchema":{"type":"object"}}]}}`))
	}))
	defer remote.Close()

	svc := NewMCPGatewayService(Dependencies{
		Settings: fakeMCPSettings{servers: []settingsclient.MCPServerConfig{{
			Name:           "remote",
			Transport:      settingsclient.MCPTransportStreamableHTTP,
			EndpointURL:    remote.URL,
			AllowToolsJSON: `["remote_echo"]`,
		}}},
		RemoteHTTPClient: remote.Client(),
	})

	result, err := svc.CallTool(context.Background(), CallToolInput{UserID: 1001, ToolName: "remote_echo", ArgumentsJSON: `{bad json`})
	if err != nil {
		t.Fatalf("CallTool should return error inside response, got outer error: %v", err)
	}
	if result.Success || !strings.Contains(result.Msg, "arguments_json") {
		t.Fatalf("unexpected result: %#v", result)
	}
}

type fakeMCPSettings struct {
	settingsclient.Service
	servers []settingsclient.MCPServerConfig
}

func (f fakeMCPSettings) ResolveMCPServers(ctx context.Context, ownerID, agentID, conversationID int64) ([]settingsclient.MCPServerConfig, error) {
	return f.servers, nil
}
