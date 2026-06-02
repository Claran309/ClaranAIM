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
    18: double importance
    19: double vector_score
    20: double final_score
    21: string score_reason
    22: string expired_at
    23: i64 superseded_by
    24: i64 previous_memory_id
    25: string created_at
    26: string updated_at
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
    18: double importance
    19: i64 previous_memory_id
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
    7: string query
    8: double min_score
    9: i64 vector_candidate_k
    10: bool use_llm_filter
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
    15: double importance
    16: bool importance_set
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

struct MemoryCandidate {
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
    13: string evidence
    14: double confidence
    15: double importance
    16: string status
    17: list<i64> conflict_memory_ids
    18: string conflict_resolution
    19: i64 accepted_memory_id
    20: string created_at
    21: string updated_at
}

struct CreateCandidateReq {
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
    12: string evidence
    13: double confidence
    14: double importance
    15: list<i64> conflict_memory_ids
    16: string conflict_resolution
}

struct CandidateFilter {
    1: i64 bot_id
    2: i64 user_id
    3: string status
    4: i64 limit
    5: i64 offset
}

struct CreateCandidateResp {
    1: bool success
    2: MemoryCandidate candidate
    3: string msg
}

struct ListCandidatesReq {
    1: i64 viewer_id
    2: CandidateFilter filter
}

struct ListCandidatesResp {
    1: bool success
    2: list<MemoryCandidate> candidates
    3: i64 total
    4: string msg
}

struct CandidateActionReq {
    1: i64 viewer_id
    2: i64 candidate_id
}

struct CandidateActionResp {
    1: bool success
    2: MemoryCandidate candidate
    3: string msg
}

service MemoryService {
    CreateMemoryResp CreateMemory(1: CreateMemoryReq req)
    ListMemoriesResp ListMemories(1: ListMemoriesReq req)
    RecallResp Recall(1: RecallReq req)
    UpdateMemoryResp UpdateMemory(1: UpdateMemoryReq req)
    DeleteMemoryResp DeleteMemory(1: DeleteMemoryReq req)
    CreateCandidateResp CreateCandidate(1: CreateCandidateReq req)
    ListCandidatesResp ListCandidates(1: ListCandidatesReq req)
    CandidateActionResp AcceptCandidate(1: CandidateActionReq req)
    CandidateActionResp RejectCandidate(1: CandidateActionReq req)
}
