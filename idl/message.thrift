namespace go message

struct Conversation {
    1: i64 id
    2: string type
    3: i64 group_id
    4: string created_at
    5: string updated_at
}

struct Message {
    1: i64 id
    2: i64 conversation_id
    3: i64 sender_id
    4: string content
    5: string msg_type
    6: string created_at
    7: i64 reply_to_id
    8: string status
    9: bool is_edited
    10: string edited_at
    11: list<i64> mention_user_ids
    12: bool mention_all
    13: i64 read_count
    14: i64 recipient_count
    15: bool is_read_by_me
}

struct CreateConversationReq {
    1: string type
    2: list<i64> participant_ids
    3: i64 group_id
}

struct CreateConversationResp {
    1: bool success
    2: i64 conversation_id
    3: string msg
}

struct GetConversationReq {
    1: i64 conversation_id
}

struct GetConversationResp {
    1: bool success
    2: Conversation conversation
    3: string msg
}

struct GetUserConversationsReq {
    1: i64 user_id
}

struct UserConversationInfo {
    1: i64 conversation_id
    2: string type
    3: string last_message
    4: string last_message_time
    5: i64 unread_count
    6: string target_name
    7: string target_avatar
    8: list<i64> participant_ids
    9: i64 last_sender_id
    10: i64 group_id
    11: bool is_deleted_group
}

struct GetUserConversationsResp {
    1: bool success
    2: list<UserConversationInfo> conversations
    3: string msg
}

struct SendMessageReq {
    1: i64 conversation_id
    2: i64 sender_id
    3: string content
    4: string msg_type
    5: i64 reply_to_id
    6: list<i64> mention_user_ids
    7: bool mention_all
}

struct SendMessageResp {
    1: bool success
    2: i64 msg_id
    3: string send_time
    4: string msg
}

struct GetHistoryReq {
    1: i64 conversation_id
    2: i64 user_id
    3: i64 limit
    4: i64 before_id
}

struct GetHistoryResp {
    1: bool success
    2: list<Message> messages
    3: string msg
}

struct SearchMessagesReq {
    1: i64 user_id
    2: list<i64> conversation_ids
    3: string keyword
    4: i64 limit
    5: string start_at
    6: string end_at
}

struct SearchMessagesResp {
    1: bool success
    2: list<Message> messages
    3: string msg
}

struct GetConversationParticipantsReq {
    1: i64 conversation_id
}

struct GetConversationParticipantsResp {
    1: bool success
    2: list<i64> user_ids
    3: string msg
}

struct MarkConversationReadReq {
    1: i64 conversation_id
    2: i64 user_id
    3: i64 message_id
}

struct MarkConversationReadResp {
    1: bool success
    2: string msg
}

struct DeleteLocalMessageReq {
    1: i64 conversation_id
    2: i64 user_id
    3: i64 message_id
}

struct DeleteLocalMessageResp {
    1: bool success
    2: string msg
}

struct EditMessageReq {
    1: i64 message_id
    2: i64 editor_id
    3: string content
}

struct EditMessageResp {
    1: bool success
    2: Message message
    3: string msg
}

struct RecallMessageReq {
    1: i64 message_id
    2: i64 operator_id
}

struct RecallMessageResp {
    1: bool success
    2: string msg
}

// 消息核心服务
service MessageService {
    CreateConversationResp CreateConversation(1: CreateConversationReq req)
    GetConversationResp GetConversation(1: GetConversationReq req)
    GetUserConversationsResp GetUserConversations(1: GetUserConversationsReq req)
    SendMessageResp SendMessage(1: SendMessageReq req)
    MarkConversationReadResp MarkConversationRead(1: MarkConversationReadReq req)
    DeleteLocalMessageResp DeleteLocalMessage(1: DeleteLocalMessageReq req)
    EditMessageResp EditMessage(1: EditMessageReq req)
    RecallMessageResp RecallMessage(1: RecallMessageReq req)
    GetHistoryResp GetHistory(1: GetHistoryReq req)
    SearchMessagesResp SearchMessages(1: SearchMessagesReq req)
    GetConversationParticipantsResp GetConversationParticipants(1: GetConversationParticipantsReq req)
}

// ===== 消息历史服务 =====

struct SaveMessageReq {
    1: i64 conversation_id
    2: i64 sender_id
    3: string content
    4: string msg_type
}

struct SaveMessageResp {
    1: bool success
    2: i64 message_id
    3: string msg
}

struct OfflineMessage {
    1: i64 id
    2: i64 user_id
    3: i64 message_id
    4: bool is_read
    5: string created_at
    6: string read_at
}

struct GetOfflineMessagesReq {
    1: i64 user_id
}

struct GetOfflineMessagesResp {
    1: bool success
    2: list<OfflineMessage> messages
    3: string msg
}

struct MarkOfflineReadReq {
    1: i64 user_id
    2: list<i64> message_ids
}

struct MarkOfflineReadResp {
    1: bool success
    2: string msg
}

struct GetUnreadCountReq {
    1: i64 user_id
}

struct GetUnreadCountResp {
    1: bool success
    2: i64 count
    3: string msg
}

// 消息历史服务
service HistoryService {
    SaveMessageResp SaveMessage(1: SaveMessageReq req)
    GetHistoryResp GetHistory(1: GetHistoryReq req)
    SearchMessagesResp SearchHistory(1: SearchMessagesReq req)
    GetOfflineMessagesResp GetOfflineMessages(1: GetOfflineMessagesReq req)
    MarkOfflineReadResp MarkOfflineRead(1: MarkOfflineReadReq req)
    GetUnreadCountResp GetUnreadCount(1: GetUnreadCountReq req)
}
