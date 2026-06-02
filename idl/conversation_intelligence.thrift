namespace go conversation_intelligence

struct DigestJob {
    1: i64 id
    2: i64 conversation_id
    3: i64 viewer_id
    4: i64 agent_id
    5: string status
    6: i64 message_count
    7: i64 valuable_count
    8: string error_message
    9: i64 retry_count
    10: i64 max_retries
    11: string next_run_at
    12: string last_attempt_at
    13: string completed_at
    14: string reason
}

struct ConversationArtifact {
    1: i64 id
    2: i64 job_id
    3: i64 conversation_id
    4: string type
    5: string title
    6: string content
    7: string metadata
    8: list<i64> source_message_ids
    9: double confidence
}

struct CreateDigestJobReq {
    1: i64 conversation_id
    2: i64 viewer_id
    3: i64 agent_id
    4: i64 start_message_id
    5: i64 end_message_id
    6: string start_time
    7: string end_time
    8: string reason
}

struct CreateDigestJobResp {
    1: bool success
    2: DigestJob job
    3: string msg
}

struct ProcessDigestJobReq {
    1: i64 job_id
    2: i64 viewer_id
}

struct ProcessDigestJobResp {
    1: bool success
    2: DigestJob job
    3: list<ConversationArtifact> artifacts
    4: string msg
}

struct ListArtifactsReq {
    1: i64 viewer_id
    2: i64 conversation_id
    3: string artifact_type
    4: i64 limit
    5: i64 offset
}

struct ListArtifactsResp {
    1: bool success
    2: list<ConversationArtifact> artifacts
    3: i64 total
    4: string msg
}

struct ListDigestJobsReq {
    1: i64 viewer_id
    2: i64 conversation_id
    3: string status
    4: i64 limit
    5: i64 offset
}

struct ListDigestJobsResp {
    1: bool success
    2: list<DigestJob> jobs
    3: i64 total
    4: string msg
}

struct RetryDigestJobReq {
    1: i64 job_id
    2: i64 viewer_id
}

struct RetryDigestJobResp {
    1: bool success
    2: DigestJob job
    3: list<ConversationArtifact> artifacts
    4: string msg
}

service ConversationIntelligenceService {
    CreateDigestJobResp CreateDigestJob(1: CreateDigestJobReq req)
    ProcessDigestJobResp ProcessDigestJob(1: ProcessDigestJobReq req)
    ListArtifactsResp ListArtifacts(1: ListArtifactsReq req)
    ListDigestJobsResp ListDigestJobs(1: ListDigestJobsReq req)
    RetryDigestJobResp RetryDigestJob(1: RetryDigestJobReq req)
}
