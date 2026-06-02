package mcpclient

import (
	"ClaranAIM/kitex_gen/mcp_gateway"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"context"
	"errors"
)

// RPCClient 使用 Kitex 调用 mcp-gateway-service。
type RPCClient struct {
	client mcpgatewayservice.Client
}

// NewRPCClient 包装已初始化的 MCP Gateway Kitex 客户端。
func NewRPCClient(client mcpgatewayservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

// ListTools 返回当前 Agent 可用的 MCP 工具。
func (c *RPCClient) ListTools(ctx context.Context, input ListToolsInput) ([]Tool, error) {
	resp, err := c.client.ListTools(ctx, &mcp_gateway.ListToolsReq{
		UserId:         input.UserID,
		AgentId:        input.AgentID,
		ConversationId: input.ConversationID,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	return fromRPCTools(resp.GetTools()), nil
}

// CallTool 调用一个 MCP 工具。
func (c *RPCClient) CallTool(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	resp, err := c.client.CallTool(ctx, &mcp_gateway.CallToolReq{
		UserId:         input.UserID,
		AgentId:        input.AgentID,
		ConversationId: input.ConversationID,
		ToolName:       input.ToolName,
		ArgumentsJson:  input.ArgumentsJSON,
		TraceId:        input.TraceID,
	})
	if err != nil {
		return CallToolResult{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return CallToolResult{Success: false, ToolName: resp.GetToolName(), ResultText: resp.GetResultText(), ResultJSON: resp.GetResultJson(), TraceID: resp.GetTraceId(), Msg: resp.GetMsg()}, err
	}
	return CallToolResult{Success: resp.GetSuccess(), ToolName: resp.GetToolName(), ResultText: resp.GetResultText(), ResultJSON: resp.GetResultJson(), TraceID: resp.GetTraceId(), Msg: resp.GetMsg()}, nil
}

// ListToolCalls 返回当前用户可见的 MCP 工具调用审计列表。
func (c *RPCClient) ListToolCalls(ctx context.Context, input ListToolCallsInput) ([]ToolCallTrace, int64, error) {
	resp, err := c.client.ListToolCalls(ctx, &mcp_gateway.ListToolCallsReq{
		UserId:         input.UserID,
		AgentId:        input.AgentID,
		ConversationId: input.ConversationID,
		Limit:          int64(input.Limit),
		Offset:         int64(input.Offset),
	})
	if err != nil {
		return nil, 0, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, 0, err
	}
	return fromRPCTraces(resp.GetTraces()), resp.GetTotal(), nil
}

// GetToolCallTrace 读取单条 MCP 工具调用审计。
func (c *RPCClient) GetToolCallTrace(ctx context.Context, userID int64, traceID string) (*ToolCallTrace, error) {
	resp, err := c.client.GetToolCallTrace(ctx, &mcp_gateway.GetToolCallTraceReq{UserId: userID, TraceId: traceID})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	trace := fromRPCTrace(resp.GetTrace())
	return trace, nil
}

func fromRPCTools(tools []*mcp_gateway.MCPTool) []Tool {
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		out = append(out, Tool{
			Name:             tool.GetName(),
			Description:      tool.GetDescription(),
			Source:           tool.GetSource(),
			ServerName:       tool.GetServerName(),
			InputSchemaJSON:  tool.GetInputSchemaJson(),
			RequiresApproval: tool.GetRequiresApproval(),
		})
	}
	return out
}

func fromRPCTraces(traces []*mcp_gateway.MCPToolCallTrace) []ToolCallTrace {
	out := make([]ToolCallTrace, 0, len(traces))
	for _, trace := range traces {
		item := fromRPCTrace(trace)
		if item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func fromRPCTrace(trace *mcp_gateway.MCPToolCallTrace) *ToolCallTrace {
	if trace == nil {
		return nil
	}
	return &ToolCallTrace{
		ID:             trace.GetId(),
		UserID:         trace.GetUserId(),
		AgentID:        trace.GetAgentId(),
		ConversationID: trace.GetConversationId(),
		ToolName:       trace.GetToolName(),
		Source:         trace.GetSource(),
		ServerName:     trace.GetServerName(),
		TraceID:        trace.GetTraceId(),
		Status:         trace.GetStatus(),
		LatencyMS:      trace.GetLatencyMs(),
		ErrorMessage:   trace.GetErrorMessage(),
		CreatedAt:      trace.GetCreatedAt(),
	}
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	if msg == "" {
		msg = "mcp-gateway-service RPC调用失败"
	}
	return errors.New(msg)
}
