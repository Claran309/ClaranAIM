package conversationintelclient

import (
	"ClaranAIM/kitex_gen/conversation_intelligence"
	"ClaranAIM/kitex_gen/conversation_intelligence/conversationintelligenceservice"
	"context"
	"errors"
)

type RPCClient struct {
	client conversationintelligenceservice.Client
}

func NewRPCClient(client conversationintelligenceservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

func (c *RPCClient) CreateDigestJob(ctx context.Context, input CreateDigestJobInput) (*DigestJob, error) {
	resp, err := c.client.CreateDigestJob(ctx, &conversation_intelligence.CreateDigestJobReq{
		ConversationId: input.ConversationID,
		ViewerId:       input.ViewerID,
		AgentId:        input.AgentID,
		StartMessageId: input.StartMessageID,
		EndMessageId:   input.EndMessageID,
		StartTime:      input.StartTime,
		EndTime:        input.EndTime,
		Reason:         input.Reason,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, err
	}
	job := toClientJob(resp.GetJob())
	return &job, nil
}

func (c *RPCClient) ProcessDigestJob(ctx context.Context, jobID, viewerID int64) (ProcessResult, error) {
	resp, err := c.client.ProcessDigestJob(ctx, &conversation_intelligence.ProcessDigestJobReq{JobId: jobID, ViewerId: viewerID})
	if err != nil {
		return ProcessResult{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return ProcessResult{}, err
	}
	return ProcessResult{Job: toClientJob(resp.GetJob()), Artifacts: toClientArtifacts(resp.GetArtifacts())}, nil
}

func (c *RPCClient) RetryDigestJob(ctx context.Context, jobID, viewerID int64) (ProcessResult, error) {
	resp, err := c.client.RetryDigestJob(ctx, &conversation_intelligence.RetryDigestJobReq{JobId: jobID, ViewerId: viewerID})
	if err != nil {
		return ProcessResult{}, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return ProcessResult{}, err
	}
	return ProcessResult{Job: toClientJob(resp.GetJob()), Artifacts: toClientArtifacts(resp.GetArtifacts())}, nil
}

func (c *RPCClient) ListDigestJobs(ctx context.Context, input ListDigestJobsInput) ([]DigestJob, int64, error) {
	resp, err := c.client.ListDigestJobs(ctx, &conversation_intelligence.ListDigestJobsReq{
		ViewerId:       input.ViewerID,
		ConversationId: input.ConversationID,
		Status:         input.Status,
		Limit:          int64(input.Limit),
		Offset:         int64(input.Offset),
	})
	if err != nil {
		return nil, 0, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, 0, err
	}
	return toClientJobs(resp.GetJobs()), resp.GetTotal(), nil
}

func (c *RPCClient) ListArtifacts(ctx context.Context, input ListArtifactsInput) ([]Artifact, int64, error) {
	resp, err := c.client.ListArtifacts(ctx, &conversation_intelligence.ListArtifactsReq{
		ViewerId:       input.ViewerID,
		ConversationId: input.ConversationID,
		ArtifactType:   input.ArtifactType,
		Limit:          int64(input.Limit),
		Offset:         int64(input.Offset),
	})
	if err != nil {
		return nil, 0, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return nil, 0, err
	}
	return toClientArtifacts(resp.GetArtifacts()), resp.GetTotal(), nil
}

func toClientJob(job *conversation_intelligence.DigestJob) DigestJob {
	if job == nil {
		return DigestJob{}
	}
	return DigestJob{
		ID:             job.GetId(),
		ConversationID: job.GetConversationId(),
		ViewerID:       job.GetViewerId(),
		AgentID:        job.GetAgentId(),
		Status:         job.GetStatus(),
		MessageCount:   job.GetMessageCount(),
		ValuableCount:  job.GetValuableCount(),
		ErrorMessage:   job.GetErrorMessage(),
		RetryCount:     job.GetRetryCount(),
		MaxRetries:     job.GetMaxRetries(),
		NextRunAt:      job.GetNextRunAt(),
		LastAttemptAt:  job.GetLastAttemptAt(),
		CompletedAt:    job.GetCompletedAt(),
		Reason:         job.GetReason(),
	}
}

func toClientJobs(jobs []*conversation_intelligence.DigestJob) []DigestJob {
	out := make([]DigestJob, 0, len(jobs))
	for _, job := range jobs {
		if job == nil {
			continue
		}
		out = append(out, toClientJob(job))
	}
	return out
}

func toClientArtifacts(artifacts []*conversation_intelligence.ConversationArtifact) []Artifact {
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		out = append(out, Artifact{
			ID:             artifact.GetId(),
			JobID:          artifact.GetJobId(),
			ConversationID: artifact.GetConversationId(),
			Type:           artifact.GetType(),
			Title:          artifact.GetTitle(),
			Content:        artifact.GetContent(),
			Metadata:       artifact.GetMetadata(),
			SourceMsgIDs:   artifact.GetSourceMessageIds(),
			Confidence:     artifact.GetConfidence(),
		})
	}
	return out
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	if msg == "" {
		msg = "conversation-intelligence-service RPC调用失败"
	}
	return errors.New(msg)
}
