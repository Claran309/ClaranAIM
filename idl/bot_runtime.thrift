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
    13: i64 context_message_limit
    14: i64 memory_recall_limit
    15: i64 max_output_tokens
    16: double temperature
    17: string group_trigger_mode
    18: bool auto_reply_enabled
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

struct RunBudget {
    1: i64 max_steps
    2: i64 max_input_tokens
    3: i64 max_output_tokens
    4: double max_cost_usd
    5: i64 deadline_ms
}

struct AgentApprovalInfo {
    1: string approval_id
    2: string run_id
    3: string interrupt_id
    4: string session_id
    5: i64 bot_id
    6: i64 user_id
    7: i64 conversation_id
    8: string status
    9: string tool_name
    10: string arguments_json
    11: string description
    12: string created_at
    13: string updated_at
}

struct AgentRunInfo {
    1: string run_id
    2: string session_id
    3: i64 bot_id
    4: i64 user_id
    5: i64 conversation_id
    6: string status
    7: i64 step
    8: i64 input_tokens
    9: i64 output_tokens
    10: double cost_usd
    11: string interrupt_reason
    12: string approval_id
    13: string interrupted_tool
    14: string started_at
    15: string finished_at
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
    7: optional RunBudget budget
}

struct RunAgentResp {
    1: bool success
    2: string reply
    3: TokenUsage usage
    4: list<ToolTrace> tool_traces
    5: string structured_result
    6: string session_id
    7: string msg
    8: optional string run_id
    9: optional AgentApprovalInfo approval
}

struct ResumeAgentReq {
    1: RuntimeBotConfig bot
    2: i64 user_id
    3: string run_id
    4: string interrupt_id
    5: string approval_id
    6: bool approved
    7: string reason
    8: optional RunBudget budget
}

struct ResumeAgentResp {
    1: bool success
    2: string reply
    3: TokenUsage usage
    4: string run_id
    5: string session_id
    6: optional AgentApprovalInfo approval
    7: string msg
}

struct ListAgentRunsReq {
    1: i64 user_id
    2: i64 bot_id
    3: i64 conversation_id
    4: i64 limit
}

struct ListAgentRunsResp {
    1: bool success
    2: list<AgentRunInfo> runs
    3: string msg
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
    4: string summary
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
    ResumeAgentResp ResumeAgent(1: ResumeAgentReq req)
    ListAgentRunsResp ListAgentRuns(1: ListAgentRunsReq req)
}
