namespace go memory

struct MemoryFact {
    1: i64 id
    2: i64 bot_id
    3: i64 user_id
    4: i64 owner_user_id
    5: i64 group_id
    6: i64 conversation_id
    7: string session_id
    8: string scope
    9: string type
    10: string title
    11: string content
    12: string source
    13: string visibility
    14: bool enabled
    15: string vector_status
    16: string embedding_ref
    17: double confidence
    18: string created_at
    19: string updated_at
}

struct CreateMemoryReq {
    1: i64 bot_id
    2: i64 user_id
    3: i64 owner_user_id
    4: i64 group_id
    5: i64 conversation_id
    6: string session_id
    7: string scope
    8: string type
    9: string title
    10: string content
    11: string source
    12: string visibility
    13: bool enabled
    14: bool enabled_set
    15: string vector_status
    16: string embedding_ref
    17: double confidence
}

struct CreateMemoryResp {
    1: bool success
    2: MemoryFact memory
    3: string msg
}

struct MemoryFilter {
    1: i64 bot_id
    2: i64 user_id
    3: i64 owner_user_id
    4: i64 group_id
    5: i64 conversation_id
    6: string session_id
    7: list<string> scopes
    8: list<string> types
    9: bool include_disabled
    10: i64 limit
    11: i64 offset
}

struct ListMemoriesReq {
    1: i64 viewer_id
    2: MemoryFilter filter
}

struct ListMemoriesResp {
    1: bool success
    2: list<MemoryFact> memories
    3: i64 total
    4: string msg
}

struct RecallReq {
    1: i64 bot_id
    2: i64 user_id
    3: i64 group_id
    4: i64 conversation_id
    5: string session_id
    6: i64 limit
}

struct RecallResp {
    1: bool success
    2: list<MemoryFact> facts
    3: string context_text
    4: string msg
}

struct UpdateMemoryReq {
    1: i64 viewer_id
    2: i64 memory_id
    3: string scope
    4: string type
    5: string title
    6: string content
    7: string source
    8: string visibility
    9: bool enabled
    10: bool enabled_set
    11: string vector_status
    12: string embedding_ref
    13: double confidence
    14: bool confidence_set
}

struct UpdateMemoryResp {
    1: bool success
    2: MemoryFact memory
    3: string msg
}

struct DeleteMemoryReq {
    1: i64 viewer_id
    2: i64 memory_id
}

struct DeleteMemoryResp {
    1: bool success
    2: string msg
}

service MemoryService {
    CreateMemoryResp CreateMemory(1: CreateMemoryReq req)
    ListMemoriesResp ListMemories(1: ListMemoriesReq req)
    RecallResp Recall(1: RecallReq req)
    UpdateMemoryResp UpdateMemory(1: UpdateMemoryReq req)
    DeleteMemoryResp DeleteMemory(1: DeleteMemoryReq req)
}
