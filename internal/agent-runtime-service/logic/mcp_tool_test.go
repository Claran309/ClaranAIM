package logic

import (
	"ClaranAIM/pkg/mcpclient"
	"context"
	"strings"
	"testing"
)

func TestMCPListToolsUsesRuntimeContext(t *testing.T) {
	old := mcpService
	t.Cleanup(func() { SetMCPService(old) })
	fake := &fakeRuntimeMCPService{
		tools: []mcpclient.Tool{{
			Name:             "remote_echo",
			Description:      "echo",
			Source:           "remote",
			ServerName:       "remote",
			RequiresApproval: true,
		}},
	}
	SetMCPService(fake)

	ctx := WithMCPRuntimeContext(context.Background(), 1001, 2002, 3003)
	out, err := MCPListTools(ctx, &MCPListToolsParams{})
	if err != nil {
		t.Fatalf("MCPListTools returned error: %v", err)
	}
	if fake.lastList.UserID != 1001 || fake.lastList.AgentID != 2002 || fake.lastList.ConversationID != 3003 {
		t.Fatalf("runtime context not forwarded: %#v", fake.lastList)
	}
	if !strings.Contains(out, "remote_echo") {
		t.Fatalf("tool list missing remote tool: %s", out)
	}
}

func TestMCPCallToolRejectsInvalidArgumentsJSON(t *testing.T) {
	old := mcpService
	t.Cleanup(func() { SetMCPService(old) })
	fake := &fakeRuntimeMCPService{}
	SetMCPService(fake)

	ctx := WithMCPRuntimeContext(context.Background(), 1001, 2002, 3003)
	out, err := MCPCallTool(ctx, &MCPCallToolParams{ToolName: "remote_echo", ArgumentsJSON: `{bad json`})
	if err != nil {
		t.Fatalf("MCPCallTool returned error: %v", err)
	}
	if fake.callCount != 0 {
		t.Fatalf("invalid JSON should not call gateway, callCount=%d", fake.callCount)
	}
	if !strings.Contains(out, "arguments_json") {
		t.Fatalf("unexpected error text: %s", out)
	}
}

func TestMCPCallToolForwardsNormalizedArguments(t *testing.T) {
	old := mcpService
	t.Cleanup(func() { SetMCPService(old) })
	fake := &fakeRuntimeMCPService{result: mcpclient.CallToolResult{Success: true, ResultText: "remote ok"}}
	SetMCPService(fake)

	ctx := WithMCPRuntimeContext(context.Background(), 1001, 2002, 3003)
	out, err := MCPCallTool(ctx, &MCPCallToolParams{ToolName: "remote_echo", ArgumentsJSON: `{"text":"hi"}`})
	if err != nil {
		t.Fatalf("MCPCallTool returned error: %v", err)
	}
	if out != "remote ok" {
		t.Fatalf("out=%q, want remote ok", out)
	}
	if fake.lastCall.ToolName != "remote_echo" || fake.lastCall.UserID != 1001 || fake.lastCall.AgentID != 2002 || fake.lastCall.ConversationID != 3003 {
		t.Fatalf("call input not forwarded: %#v", fake.lastCall)
	}
	if fake.lastCall.ArgumentsJSON != `{"text":"hi"}` {
		t.Fatalf("arguments=%q", fake.lastCall.ArgumentsJSON)
	}
}

type fakeRuntimeMCPService struct {
	mcpclient.Service
	tools     []mcpclient.Tool
	result    mcpclient.CallToolResult
	lastList  mcpclient.ListToolsInput
	lastCall  mcpclient.CallToolInput
	callCount int
}

func (f *fakeRuntimeMCPService) ListTools(ctx context.Context, input mcpclient.ListToolsInput) ([]mcpclient.Tool, error) {
	f.lastList = input
	return f.tools, nil
}

func (f *fakeRuntimeMCPService) CallTool(ctx context.Context, input mcpclient.CallToolInput) (mcpclient.CallToolResult, error) {
	f.callCount++
	f.lastCall = input
	return f.result, nil
}
