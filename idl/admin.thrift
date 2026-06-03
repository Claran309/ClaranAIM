namespace go admin

struct AdminMetric {
    1: string key
    2: string label
    3: string value
    4: string hint
}

struct AdminUser {
    1: i64 id
    2: string username
    3: string nickname
    4: string avatar
    5: string status
    6: string role
    7: bool is_system
    8: string created_at
}

struct AdminGroup {
    1: i64 id
    2: string name
    3: i64 owner_id
    4: string announcement
    5: string created_at
    6: string status
}

struct AdminFile {
    1: string file_id
    2: string file_name
    3: string file_type
    4: i64 file_size
    5: string content_type
    6: string file_url
    7: i64 uploader_id
    8: string created_at
}

struct AdminAgent {
    1: i64 id
    2: string name
    3: string type
    4: string model_name
    5: i64 owner_id
    6: i64 agent_user_id
    7: bool is_active
    8: string tool_policy
    9: string created_at
}

struct AdminBillingRecord {
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

struct AdminReviewItem {
    1: i64 id
    2: string source
    3: string title
    4: string content
    5: string status
    6: string created_at
}

struct AdminMCPTrace {
    1: i64 id
    2: string trace_id
    3: i64 user_id
    4: i64 agent_id
    5: i64 conversation_id
    6: string tool_name
    7: string source
    8: string server_name
    9: string status
    10: i64 latency_ms
    11: string error_message
    12: string created_at
}

struct SystemNotice {
    1: i64 id
    2: string title
    3: string content
    4: string level
    5: string audience
    6: bool enabled
    7: i64 created_by
    8: string created_at
    9: string updated_at
}

struct AdminAuditLog {
    1: i64 id
    2: i64 admin_id
    3: string action
    4: string target_type
    5: string target_id
    6: string detail
    7: string created_at
}

struct DashboardReq {
    1: i64 admin_id
}

struct DashboardResp {
    1: bool success
    2: list<AdminMetric> metrics
    3: list<SystemNotice> notices
    4: list<AdminAuditLog> recent_audits
    5: string msg
}

struct ListUsersReq {
    1: i64 admin_id
    2: string keyword
    3: string role
    4: string status
    5: bool include_system
    6: i64 limit
    7: i64 offset
}

struct ListUsersResp {
    1: bool success
    2: list<AdminUser> users
    3: i64 total
    4: string msg
}

struct ListGroupsReq {
    1: i64 admin_id
    2: string keyword
    3: i64 owner_id
    4: i64 limit
    5: i64 offset
}

struct ListGroupsResp {
    1: bool success
    2: list<AdminGroup> groups
    3: i64 total
    4: string msg
}

struct UpdateGroupStatusReq {
    1: i64 admin_id
    2: i64 group_id
    3: string status
    4: string reason
}

struct UpdateGroupStatusResp {
    1: bool success
    2: string msg
    3: AdminGroup group
}

struct ListFilesReq {
    1: i64 admin_id
    2: i64 uploader_id
    3: string file_type
    4: i64 limit
    5: i64 offset
}

struct ListFilesResp {
    1: bool success
    2: list<AdminFile> files
    3: i64 total
    4: string msg
}

struct ListAgentsReq {
    1: i64 admin_id
    2: i64 owner_id
    3: string type
    4: i64 limit
    5: i64 offset
}

struct ListAgentsResp {
    1: bool success
    2: list<AdminAgent> agents
    3: i64 total
    4: string msg
}

struct ListBillingReq {
    1: i64 admin_id
    2: i64 bot_id
    3: i64 user_id
    4: i64 limit
    5: i64 offset
}

struct ListBillingResp {
    1: bool success
    2: list<AdminBillingRecord> records
    3: i64 total
    4: double total_cost
    5: string msg
}

struct ListReviewsReq {
    1: i64 admin_id
    2: string source
    3: string status
    4: i64 limit
    5: i64 offset
}

struct ListReviewsResp {
    1: bool success
    2: list<AdminReviewItem> items
    3: i64 total
    4: string msg
}

struct ReviewReq {
    1: i64 admin_id
    2: string source
    3: i64 item_id
    4: string action
    5: string note
}

struct ReviewResp {
    1: bool success
    2: string msg
}

struct ListMCPTracesReq {
    1: i64 admin_id
    2: i64 agent_id
    3: i64 conversation_id
    4: i64 limit
    5: i64 offset
}

struct ListMCPTracesResp {
    1: bool success
    2: list<AdminMCPTrace> traces
    3: i64 total
    4: string msg
}

struct SaveNoticeReq {
    1: i64 admin_id
    2: i64 notice_id
    3: string title
    4: string content
    5: string level
    6: string audience
    7: bool enabled
}

struct NoticeResp {
    1: bool success
    2: SystemNotice notice
    3: string msg
}

struct ListNoticesReq {
    1: i64 admin_id
    2: bool include_disabled
    3: i64 limit
    4: i64 offset
}

struct ListNoticesResp {
    1: bool success
    2: list<SystemNotice> notices
    3: i64 total
    4: string msg
}

struct ListAuditLogsReq {
    1: i64 admin_id
    2: string action
    3: string target_type
    4: i64 limit
    5: i64 offset
}

struct ListAuditLogsResp {
    1: bool success
    2: list<AdminAuditLog> logs
    3: i64 total
    4: string msg
}

service AdminService {
    DashboardResp GetDashboard(1: DashboardReq req)
    ListUsersResp ListUsers(1: ListUsersReq req)
    ListGroupsResp ListGroups(1: ListGroupsReq req)
    UpdateGroupStatusResp UpdateGroupStatus(1: UpdateGroupStatusReq req)
    ListFilesResp ListFiles(1: ListFilesReq req)
    ListAgentsResp ListAgents(1: ListAgentsReq req)
    ListBillingResp ListBilling(1: ListBillingReq req)
    ListReviewsResp ListReviews(1: ListReviewsReq req)
    ReviewResp ReviewItem(1: ReviewReq req)
    ListMCPTracesResp ListMCPTraces(1: ListMCPTracesReq req)
    NoticeResp SaveNotice(1: SaveNoticeReq req)
    ListNoticesResp ListNotices(1: ListNoticesReq req)
    ListAuditLogsResp ListAuditLogs(1: ListAuditLogsReq req)
}
