package handler

import (
	"ClaranAIM/pkg/mcpclient"
	"ClaranAIM/pkg/response"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// MCPHandler 暴露 MCP Gateway 的工具发现、调试调用和审计查询入口。
type MCPHandler struct {
	svc mcpclient.Service
}

var gatewayMCPService mcpclient.Service

// InitMCPService 注册 mcp-gateway-service RPC 客户端。
func InitMCPService(svc mcpclient.Service) {
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
	tools, err := h.svc.ListTools(ctx, mcpclient.ListToolsInput{
		UserID:         userID,
		AgentID:        parseMCPInt64(c.DefaultQuery("agent_id", "0")),
		ConversationID: parseMCPInt64(c.DefaultQuery("conversation_id", "0")),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "tools": tools})
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
	result, err := h.svc.CallTool(ctx, mcpclient.CallToolInput{
		UserID:         userID,
		AgentID:        req.AgentID,
		ConversationID: req.ConversationID,
		ToolName:       req.ToolName,
		ArgumentsJSON:  req.ArgumentsJSON,
		TraceID:        req.TraceID,
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
	traces, total, err := h.svc.ListToolCalls(ctx, mcpclient.ListToolCallsInput{
		UserID:         userID,
		AgentID:        parseMCPInt64(c.DefaultQuery("agent_id", "0")),
		ConversationID: parseMCPInt64(c.DefaultQuery("conversation_id", "0")),
		Limit:          int(parseMCPInt64(c.DefaultQuery("limit", "50"))),
		Offset:         int(parseMCPInt64(c.DefaultQuery("offset", "0"))),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "traces": traces, "total": total})
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
	trace, err := h.svc.GetToolCallTrace(ctx, userID, traceID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "trace": trace})
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
