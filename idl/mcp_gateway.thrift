namespace go mcp_gateway

struct MCPTool {
    1: string name
    2: string description
    3: string source
    4: string server_name
    5: string input_schema_json
    6: bool requires_approval
}

struct ListToolsReq {
    1: i64 user_id
    2: i64 agent_id
    3: i64 conversation_id
}

struct ListToolsResp {
    1: bool success
    2: list<MCPTool> tools
    3: string msg
}

struct CallToolReq {
    1: i64 user_id
    2: i64 agent_id
    3: i64 conversation_id
    4: string tool_name
    5: string arguments_json
    6: string trace_id
}

struct CallToolResp {
    1: bool success
    2: string tool_name
    3: string result_text
    4: string result_json
    5: string trace_id
    6: string msg
}

struct GetToolSchemaReq {
    1: i64 user_id
    2: i64 agent_id
    3: i64 conversation_id
    4: string tool_name
}

struct GetToolSchemaResp {
    1: bool success
    2: MCPTool tool
    3: string msg
}

struct MCPToolCallTrace {
    1: i64 id
    2: i64 user_id
    3: i64 agent_id
    4: i64 conversation_id
    5: string tool_name
    6: string source
    7: string server_name
    8: string trace_id
    9: string status
    10: i64 latency_ms
    11: string error_message
    12: string created_at
}

struct ListToolCallsReq {
    1: i64 user_id
    2: i64 agent_id
    3: i64 conversation_id
    4: i64 limit
    5: i64 offset
}

struct ListToolCallsResp {
    1: bool success
    2: list<MCPToolCallTrace> traces
    3: i64 total
    4: string msg
}

struct GetToolCallTraceReq {
    1: i64 user_id
    2: string trace_id
}

struct GetToolCallTraceResp {
    1: bool success
    2: MCPToolCallTrace trace
    3: string msg
}

service MCPGatewayService {
    ListToolsResp ListTools(1: ListToolsReq req)
    CallToolResp CallTool(1: CallToolReq req)
    GetToolSchemaResp GetToolSchema(1: GetToolSchemaReq req)
    ListToolCallsResp ListToolCalls(1: ListToolCallsReq req)
    GetToolCallTraceResp GetToolCallTrace(1: GetToolCallTraceReq req)
}
