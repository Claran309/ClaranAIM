// Package handler 实现 mcp-gateway-service 的 Kitex RPC 入口。
package handler

import (
	mcpsvc "ClaranAIM/internal/mcp-gateway-service/service"
	"ClaranAIM/kitex_gen/mcp_gateway"
	"context"
)

// MCPGatewayServiceImpl 将 Kitex 请求转给 MCP Gateway 业务层。
type MCPGatewayServiceImpl struct {
	svc mcpsvc.MCPGatewayService
}

// NewMCPGatewayServiceImpl 创建 MCP Gateway Kitex handler。
func NewMCPGatewayServiceImpl(svc mcpsvc.MCPGatewayService) mcp_gateway.MCPGatewayService {
	return &MCPGatewayServiceImpl{svc: svc}
}

// ListTools 返回当前用户、Agent 和会话上下文中可用的内置/远程 MCP 工具。
func (h *MCPGatewayServiceImpl) ListTools(ctx context.Context, req *mcp_gateway.ListToolsReq) (*mcp_gateway.ListToolsResp, error) {
	tools, err := h.svc.ListTools(ctx, mcpsvc.ToolContext{
		UserID:         req.GetUserId(),
		AgentID:        req.GetAgentId(),
		ConversationID: req.GetConversationId(),
	})
	if err != nil {
		return &mcp_gateway.ListToolsResp{Success: false, Msg: err.Error()}, nil
	}
	return &mcp_gateway.ListToolsResp{Success: true, Tools: toRPCTools(tools), Msg: "ok"}, nil
}

// CallTool 调用一个 MCP 工具，业务层负责审计和远程/内置工具路由。
func (h *MCPGatewayServiceImpl) CallTool(ctx context.Context, req *mcp_gateway.CallToolReq) (*mcp_gateway.CallToolResp, error) {
	resp, err := h.svc.CallTool(ctx, mcpsvc.CallToolInput{
		UserID:         req.GetUserId(),
		AgentID:        req.GetAgentId(),
		ConversationID: req.GetConversationId(),
		ToolName:       req.GetToolName(),
		ArgumentsJSON:  req.GetArgumentsJson(),
		TraceID:        req.GetTraceId(),
	})
	if err != nil {
		return &mcp_gateway.CallToolResp{Success: false, ToolName: req.GetToolName(), Msg: err.Error()}, nil
	}
	return &resp, nil
}

// GetToolSchema 返回单个工具的 JSON Schema，供前端和 Agent 工具注册层展示。
func (h *MCPGatewayServiceImpl) GetToolSchema(ctx context.Context, req *mcp_gateway.GetToolSchemaReq) (*mcp_gateway.GetToolSchemaResp, error) {
	tool, err := h.svc.GetToolSchema(ctx, mcpsvc.ToolSchemaInput{
		UserID:         req.GetUserId(),
		AgentID:        req.GetAgentId(),
		ConversationID: req.GetConversationId(),
		ToolName:       req.GetToolName(),
	})
	if err != nil {
		return &mcp_gateway.GetToolSchemaResp{Success: false, Msg: err.Error()}, nil
	}
	return &mcp_gateway.GetToolSchemaResp{Success: true, Tool: toRPCTool(tool), Msg: "ok"}, nil
}

// ListToolCalls 返回当前用户可见的 MCP 工具调用审计列表。
func (h *MCPGatewayServiceImpl) ListToolCalls(ctx context.Context, req *mcp_gateway.ListToolCallsReq) (*mcp_gateway.ListToolCallsResp, error) {
	traces, total, err := h.svc.ListToolCalls(ctx, mcpsvc.TraceListInput{
		UserID:         req.GetUserId(),
		AgentID:        req.GetAgentId(),
		ConversationID: req.GetConversationId(),
		Limit:          int(req.GetLimit()),
		Offset:         int(req.GetOffset()),
	})
	if err != nil {
		return &mcp_gateway.ListToolCallsResp{Success: false, Msg: err.Error()}, nil
	}
	return &mcp_gateway.ListToolCallsResp{Success: true, Traces: toRPCTraces(traces), Total: total, Msg: "ok"}, nil
}

// GetToolCallTrace 返回单条 MCP 工具调用审计。
func (h *MCPGatewayServiceImpl) GetToolCallTrace(ctx context.Context, req *mcp_gateway.GetToolCallTraceReq) (*mcp_gateway.GetToolCallTraceResp, error) {
	trace, err := h.svc.GetToolCallTrace(ctx, req.GetUserId(), req.GetTraceId())
	if err != nil {
		return &mcp_gateway.GetToolCallTraceResp{Success: false, Msg: err.Error()}, nil
	}
	return &mcp_gateway.GetToolCallTraceResp{Success: true, Trace: toRPCTrace(trace), Msg: "ok"}, nil
}

func toRPCTools(tools []mcpsvc.Tool) []*mcp_gateway.MCPTool {
	out := make([]*mcp_gateway.MCPTool, 0, len(tools))
	for i := range tools {
		out = append(out, toRPCTool(&tools[i]))
	}
	return out
}

func toRPCTool(tool *mcpsvc.Tool) *mcp_gateway.MCPTool {
	if tool == nil {
		return nil
	}
	return &mcp_gateway.MCPTool{
		Name:             tool.Name,
		Description:      tool.Description,
		Source:           tool.Source,
		ServerName:       tool.ServerName,
		InputSchemaJson:  tool.InputSchemaJson,
		RequiresApproval: tool.RequiresApproval,
	}
}

func toRPCTraces(traces []mcpsvc.TraceDTO) []*mcp_gateway.MCPToolCallTrace {
	out := make([]*mcp_gateway.MCPToolCallTrace, 0, len(traces))
	for i := range traces {
		out = append(out, toRPCTrace(&traces[i]))
	}
	return out
}

func toRPCTrace(trace *mcpsvc.TraceDTO) *mcp_gateway.MCPToolCallTrace {
	if trace == nil {
		return nil
	}
	return &mcp_gateway.MCPToolCallTrace{
		Id:             trace.Id,
		UserId:         trace.UserId,
		AgentId:        trace.AgentId,
		ConversationId: trace.ConversationId,
		ToolName:       trace.ToolName,
		Source:         trace.Source,
		ServerName:     trace.ServerName,
		TraceId:        trace.TraceId,
		Status:         trace.Status,
		LatencyMs:      trace.LatencyMs,
		ErrorMessage:   trace.ErrorMessage,
		CreatedAt:      trace.CreatedAt,
	}
}
