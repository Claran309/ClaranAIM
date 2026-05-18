namespace go bot_runtime

struct RuntimeBotConfig {
    1: i64 bot_id
    2: i64 agent_user_id
    3: string name
    4: string description
    5: string model_name
    6: string api_key
    7: string base_url
    8: string system_prompt
    9: string skills_dir
    10: string workspace_root
    11: string tool_policy
    12: bool include_domain_tools
}

struct ContextOptions {
    1: i64 conversation_id
    2: i64 user_id
    3: i64 limit
    4: i64 before_id
    5: string start_at
    6: string end_at
    7: string keyword
    8: string topic
}

struct TokenUsage {
    1: i64 input_tokens
    2: i64 output_tokens
    3: bool usage_seen
}

struct ToolTrace {
    1: string name
    2: string arguments
    3: string result
    4: string error
}

struct RunAgentReq {
    1: RuntimeBotConfig bot
    2: i64 user_id
    3: i64 conversation_id
    4: string session_id
    5: string input
    6: ContextOptions context
}

struct RunAgentResp {
    1: bool success
    2: string reply
    3: TokenUsage usage
    4: list<ToolTrace> tool_traces
    5: string structured_result
    6: string session_id
    7: string msg
}

struct AgentTaskReq {
    1: RuntimeBotConfig bot
    2: i64 user_id
    3: i64 conversation_id
    4: string task_type
    5: string question
    6: ContextOptions context
}

struct AgentTaskResp {
    1: bool success
    2: string result
    3: string structured_result
    4: TokenUsage usage
    5: string msg
}

struct GetAgentSessionReq {
    1: i64 bot_id
    2: i64 user_id
    3: i64 conversation_id
}

struct AgentSessionInfo {
    1: string session_id
    2: string title
    3: string created_at
}

struct GetAgentSessionResp {
    1: bool success
    2: list<AgentSessionInfo> sessions
    3: string msg
}

service BotRuntimeService {
    RunAgentResp RunAgent(1: RunAgentReq req)
    AgentTaskResp SummarizeConversation(1: AgentTaskReq req)
    AgentTaskResp AskConversation(1: AgentTaskReq req)
    AgentTaskResp ExtractInsights(1: AgentTaskReq req)
    AgentTaskResp GenerateReplyCandidates(1: AgentTaskReq req)
    GetAgentSessionResp GetAgentSessions(1: GetAgentSessionReq req)
}
