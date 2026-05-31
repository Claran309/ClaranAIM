namespace go bot

struct CreateBotReq {
    1: string name
    2: string type
    3: string description
    4: string model_name
    5: string api_key
    6: string base_url
    7: string system_prompt
    8: string skills_dir
    9: string agent_root
    10: i64 owner_id
    11: string avatar
    12: string signature
    13: string workspace_root
    14: string tool_policy
    15: i64 context_message_limit
    16: i64 memory_recall_limit
    17: i64 max_output_tokens
    18: double temperature
    19: string group_trigger_mode
    20: bool auto_reply_enabled
}

struct CreateBotResp {
    1: bool success
    2: i64 bot_id
    3: string msg
}

struct UpdateBotReq {
    1: i64 bot_id
    2: i64 operator_id
    3: string name
    4: string description
    5: string model_name
    6: string api_key
    7: string base_url
    8: string system_prompt
    9: string skills_dir
    10: string agent_root
    11: bool is_active
    12: string avatar
    13: string signature
    14: string workspace_root
    15: string tool_policy
    16: bool is_active_set
    17: i64 context_message_limit
    18: i64 memory_recall_limit
    19: i64 max_output_tokens
    20: double temperature
    21: string group_trigger_mode
    22: bool auto_reply_enabled
}

struct UpdateBotResp {
    1: bool success
    2: string msg
}

struct GetBotReq {
    1: i64 bot_id
}

struct BotInfo {
    1: i64 id
    2: string name
    3: string type
    4: string description
    5: string model_name
    6: string base_url
    7: string system_prompt
    8: string skills_dir
    9: string agent_root
    10: i64 owner_id
    11: bool is_active
    12: string created_at
    13: string updated_at
    14: i64 agent_user_id
    15: string avatar
    16: string signature
    17: string workspace_root
    18: string tool_policy
    19: i64 context_message_limit
    20: i64 memory_recall_limit
    21: i64 max_output_tokens
    22: double temperature
    23: string group_trigger_mode
    24: bool auto_reply_enabled
}

struct BotPermission {
    1: i64 id
    2: i64 bot_id
    3: i64 user_id
    4: string role
    5: string created_at
    6: string updated_at
}

struct GetBotResp {
    1: bool success
    2: BotInfo bot
    3: string msg
}

struct ListBotsReq {
    1: i64 owner_id
    2: string type
}

struct ListBotsResp {
    1: bool success
    2: list<BotInfo> bots
    3: string msg
}

struct DeleteBotReq {
    1: i64 bot_id
    2: i64 operator_id
}

struct DeleteBotResp {
    1: bool success
    2: string msg
}

struct ChatWithBotReq {
    1: i64 bot_id
    2: i64 user_id
    3: i64 conversation_id
    4: string message
}

struct ChatWithBotResp {
    1: bool success
    2: string reply
    3: i64 input_tokens
    4: i64 output_tokens
    5: double cost
    6: string msg
}

struct AgentTaskReq {
    1: i64 bot_id
    2: i64 user_id
    3: i64 conversation_id
    4: string question
}

struct AgentTaskResp {
    1: bool success
    2: string result
    3: string msg
}

struct GrantPermissionReq {
    1: i64 bot_id
    2: i64 operator_id
    3: i64 user_id
    4: string role
}

struct GrantPermissionResp {
    1: bool success
    2: string msg
}

struct RevokePermissionReq {
    1: i64 bot_id
    2: i64 operator_id
    3: i64 user_id
}

struct RevokePermissionResp {
    1: bool success
    2: string msg
}

struct ListPermissionsReq {
    1: i64 bot_id
    2: i64 operator_id
}

struct ListPermissionsResp {
    1: bool success
    2: list<BotPermission> permissions
    3: string msg
}

struct CreateRouteReq {
    1: i64 bot_id
    2: string route_pattern
    3: string route_type
    4: i64 priority
}

struct CreateRouteResp {
    1: bool success
    2: i64 route_id
    3: string msg
}

struct BotRoute {
    1: i64 id
    2: i64 bot_id
    3: string route_pattern
    4: string route_type
    5: i64 priority
    6: string created_at
}

struct ListRoutesReq {
    1: i64 bot_id
}

struct ListRoutesResp {
    1: bool success
    2: list<BotRoute> routes
    3: string msg
}

struct DeleteRouteReq {
    1: i64 route_id
    2: i64 operator_id
}

struct DeleteRouteResp {
    1: bool success
    2: string msg
}

struct GetBillingReq {
    1: i64 bot_id
    2: i64 user_id
    3: i64 limit
    4: i64 offset
}

struct BillingRecord {
    1: i64 id
    2: i64 bot_id
    3: i64 user_id
    4: i64 conversation_id
    5: i64 input_tokens
    6: i64 output_tokens
    7: double cost
    8: string model_name
    9: string created_at
}

struct GetBillingResp {
    1: bool success
    2: list<BillingRecord> records
    3: i64 total
    4: string msg
}

service BotService {
    CreateBotResp CreateBot(1: CreateBotReq req)
    UpdateBotResp UpdateBot(1: UpdateBotReq req)
    GetBotResp GetBot(1: GetBotReq req)
    ListBotsResp ListBots(1: ListBotsReq req)
    DeleteBotResp DeleteBot(1: DeleteBotReq req)
    ChatWithBotResp ChatWithBot(1: ChatWithBotReq req)
    AgentTaskResp SummarizeConversation(1: AgentTaskReq req)
    AgentTaskResp AskConversation(1: AgentTaskReq req)
    AgentTaskResp ExtractInsights(1: AgentTaskReq req)
    AgentTaskResp GenerateReplyCandidates(1: AgentTaskReq req)
    GrantPermissionResp GrantPermission(1: GrantPermissionReq req)
    RevokePermissionResp RevokePermission(1: RevokePermissionReq req)
    ListPermissionsResp ListPermissions(1: ListPermissionsReq req)
    CreateRouteResp CreateRoute(1: CreateRouteReq req)
    ListRoutesResp ListRoutes(1: ListRoutesReq req)
    DeleteRouteResp DeleteRoute(1: DeleteRouteReq req)
    GetBillingResp GetBilling(1: GetBillingReq req)
}
