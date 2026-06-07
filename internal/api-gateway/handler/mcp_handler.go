package handler

import (
	"ClaranAIM/kitex_gen/mcp_gateway"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// MCPHandler 暴露 MCP Gateway 的工具发现、调试调用和审计查询入口。
type MCPHandler struct {
	svc mcpgatewayservice.Client
}

var gatewayMCPService mcpgatewayservice.Client

// InitMCPService 注册 mcp-gateway-service RPC 客户端。
func InitMCPService(svc mcpgatewayservice.Client) {
	gatewayMCPService = svc
}

// NewMCPHandler 创建 MCP HTTP handler。
func NewMCPHandler() *MCPHandler {
	return &MCPHandler{svc: gatewayMCPService}
}

func (h *MCPHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "mcp-gateway-service未初始化")
		return false
	}
	return true
}

// ListTools 返回当前用户/Agent/会话可用的内置工具和远程 MCP 工具。
func (h *MCPHandler) ListTools(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := h.svc.ListTools(ctx, &mcp_gateway.ListToolsReq{
		UserId:         userID,
		AgentId:        parseMCPInt64(c.DefaultQuery("agent_id", "0")),
		ConversationId: parseMCPInt64(c.DefaultQuery("conversation_id", "0")),
	})
	if err != nil || !resp.GetSuccess() {
		if err == nil {
			err = mcpStatusError(resp.GetSuccess(), resp.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "tools": resp.GetTools()})
}

// CallTool 手动调用一个 MCP 工具，主要用于前端调试和管理确认。
func (h *MCPHandler) CallTool(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req mcpCallReq
	if err := bindMCPJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if strings.TrimSpace(req.ToolName) == "" {
		response.BadRequest(c, "tool_name不能为空")
		return
	}
	result, err := h.svc.CallTool(ctx, &mcp_gateway.CallToolReq{
		UserId:         userID,
		AgentId:        req.AgentID,
		ConversationId: req.ConversationID,
		ToolName:       req.ToolName,
		ArgumentsJson:  req.ArgumentsJSON,
		TraceId:        req.TraceID,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}

// ListToolCalls 返回 MCP 工具调用审计列表。
func (h *MCPHandler) ListToolCalls(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := h.svc.ListToolCalls(ctx, &mcp_gateway.ListToolCallsReq{
		UserId:         userID,
		AgentId:        parseMCPInt64(c.DefaultQuery("agent_id", "0")),
		ConversationId: parseMCPInt64(c.DefaultQuery("conversation_id", "0")),
		Limit:          parseMCPInt64(c.DefaultQuery("limit", "50")),
		Offset:         parseMCPInt64(c.DefaultQuery("offset", "0")),
	})
	if err != nil || !resp.GetSuccess() {
		if err == nil {
			err = mcpStatusError(resp.GetSuccess(), resp.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "traces": resp.GetTraces(), "total": resp.GetTotal()})
}

// GetToolCallTrace 返回单条 MCP 工具调用审计详情。
func (h *MCPHandler) GetToolCallTrace(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	traceID := strings.TrimSpace(c.Param("trace_id"))
	if traceID == "" {
		response.BadRequest(c, "trace_id不能为空")
		return
	}
	resp, err := h.svc.GetToolCallTrace(ctx, &mcp_gateway.GetToolCallTraceReq{UserId: userID, TraceId: traceID})
	if err != nil || !resp.GetSuccess() {
		if err == nil {
			err = mcpStatusError(resp.GetSuccess(), resp.GetMsg())
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "trace": resp.GetTrace()})
}

type mcpCallReq struct {
	AgentID        int64  `json:"agent_id"`
	ConversationID int64  `json:"conversation_id"`
	ToolName       string `json:"tool_name"`
	ArgumentsJSON  string `json:"arguments_json"`
	TraceID        string `json:"trace_id"`
}

func bindMCPJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func parseMCPInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func mcpStatusError(success bool, msg string) error {
	if success {
		return nil
	}
	if strings.TrimSpace(msg) == "" {
		msg = "mcp-gateway-service RPC调用失败"
	}
	return errors.New(msg)
}
