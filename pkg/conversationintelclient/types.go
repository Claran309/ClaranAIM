// Package conversationintelclient 定义调用 conversation-intelligence-service 的稳定契约。
package conversationintelclient

import "context"

type DigestJob struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversation_id"`
	ViewerID       int64  `json:"viewer_id"`
	AgentID        int64  `json:"agent_id"`
	Status         string `json:"status"`
	MessageCount   int64  `json:"message_count"`
	ValuableCount  int64  `json:"valuable_count"`
	ErrorMessage   string `json:"error_message"`
	RetryCount     int64  `json:"retry_count"`
	MaxRetries     int64  `json:"max_retries"`
	NextRunAt      string `json:"next_run_at"`
	LastAttemptAt  string `json:"last_attempt_at"`
	CompletedAt    string `json:"completed_at"`
	Reason         string `json:"reason"`
}

type Artifact struct {
	ID             int64   `json:"id"`
	JobID          int64   `json:"job_id"`
	ConversationID int64   `json:"conversation_id"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Metadata       string  `json:"metadata"`
	SourceMsgIDs   []int64 `json:"source_message_ids"`
	Confidence     float64 `json:"confidence"`
}

type CreateDigestJobInput struct {
	ConversationID int64
	ViewerID       int64
	AgentID        int64
	StartMessageID int64
	EndMessageID   int64
	StartTime      string
	EndTime        string
	Reason         string
}

type ListArtifactsInput struct {
	ViewerID       int64
	ConversationID int64
	ArtifactType   string
	Limit          int
	Offset         int
}

type ListDigestJobsInput struct {
	ViewerID       int64
	ConversationID int64
	Status         string
	Limit          int
	Offset         int
}

type ProcessResult struct {
	Job       DigestJob  `json:"job"`
	Artifacts []Artifact `json:"artifacts"`
}

type Service interface {
	CreateDigestJob(ctx context.Context, input CreateDigestJobInput) (*DigestJob, error)
	ProcessDigestJob(ctx context.Context, jobID, viewerID int64) (ProcessResult, error)
	RetryDigestJob(ctx context.Context, jobID, viewerID int64) (ProcessResult, error)
	ListDigestJobs(ctx context.Context, input ListDigestJobsInput) ([]DigestJob, int64, error)
	ListArtifacts(ctx context.Context, input ListArtifactsInput) ([]Artifact, int64, error)
}
