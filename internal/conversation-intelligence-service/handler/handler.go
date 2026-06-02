// Package handler 实现 conversation-intelligence-service 的 Kitex RPC 入口。
package handler

import (
	conversationintel "ClaranAIM/internal/conversation-intelligence-service/service"
	"ClaranAIM/kitex_gen/conversation_intelligence"
	"context"
	"time"
)

type ConversationIntelligenceServiceImpl struct {
	svc conversationintel.ConversationIntelligenceService
}

func NewConversationIntelligenceServiceImpl(svc conversationintel.ConversationIntelligenceService) conversation_intelligence.ConversationIntelligenceService {
	return &ConversationIntelligenceServiceImpl{svc: svc}
}

func (h *ConversationIntelligenceServiceImpl) CreateDigestJob(ctx context.Context, req *conversation_intelligence.CreateDigestJobReq) (*conversation_intelligence.CreateDigestJobResp, error) {
	startTime, _ := parseOptionalTime(req.GetStartTime())
	endTime, _ := parseOptionalTime(req.GetEndTime())
	job, err := h.svc.CreateDigestJob(ctx, conversationintel.CreateDigestJobInput{
		ConversationID: req.GetConversationId(),
		ViewerID:       req.GetViewerId(),
		AgentID:        req.GetAgentId(),
		StartMessageID: req.GetStartMessageId(),
		EndMessageID:   req.GetEndMessageId(),
		StartTime:      startTime,
		EndTime:        endTime,
		Reason:         req.GetReason(),
	})
	if err != nil {
		return &conversation_intelligence.CreateDigestJobResp{Success: false, Msg: err.Error()}, nil
	}
	return &conversation_intelligence.CreateDigestJobResp{Success: true, Job: toRPCJob(*job)}, nil
}

func (h *ConversationIntelligenceServiceImpl) ProcessDigestJob(ctx context.Context, req *conversation_intelligence.ProcessDigestJobReq) (*conversation_intelligence.ProcessDigestJobResp, error) {
	result, err := h.svc.ProcessDigestJob(ctx, req.GetJobId(), req.GetViewerId())
	if err != nil {
		return &conversation_intelligence.ProcessDigestJobResp{Success: false, Job: toRPCJob(result.Job), Msg: err.Error()}, nil
	}
	return &conversation_intelligence.ProcessDigestJobResp{Success: true, Job: toRPCJob(result.Job), Artifacts: resultToRPCArtifacts(result)}, nil
}

func (h *ConversationIntelligenceServiceImpl) ListArtifacts(ctx context.Context, req *conversation_intelligence.ListArtifactsReq) (*conversation_intelligence.ListArtifactsResp, error) {
	artifacts, total, err := h.svc.ListArtifacts(ctx, conversationintel.ListArtifactsInput{
		ConversationID: req.GetConversationId(),
		ViewerID:       req.GetViewerId(),
		ArtifactType:   req.GetArtifactType(),
		Limit:          int(req.GetLimit()),
		Offset:         int(req.GetOffset()),
	})
	if err != nil {
		return &conversation_intelligence.ListArtifactsResp{Success: false, Msg: err.Error()}, nil
	}
	return &conversation_intelligence.ListArtifactsResp{Success: true, Artifacts: toRPCArtifacts(artifacts), Total: total}, nil
}

func (h *ConversationIntelligenceServiceImpl) ListDigestJobs(ctx context.Context, req *conversation_intelligence.ListDigestJobsReq) (*conversation_intelligence.ListDigestJobsResp, error) {
	jobs, total, err := h.svc.ListDigestJobs(ctx, conversationintel.ListDigestJobsInput{
		ConversationID: req.GetConversationId(),
		ViewerID:       req.GetViewerId(),
		Status:         req.GetStatus(),
		Limit:          int(req.GetLimit()),
		Offset:         int(req.GetOffset()),
	})
	if err != nil {
		return &conversation_intelligence.ListDigestJobsResp{Success: false, Msg: err.Error()}, nil
	}
	return &conversation_intelligence.ListDigestJobsResp{Success: true, Jobs: toRPCJobs(jobs), Total: total}, nil
}

func (h *ConversationIntelligenceServiceImpl) RetryDigestJob(ctx context.Context, req *conversation_intelligence.RetryDigestJobReq) (*conversation_intelligence.RetryDigestJobResp, error) {
	result, err := h.svc.RetryDigestJob(ctx, req.GetJobId(), req.GetViewerId())
	if err != nil {
		return &conversation_intelligence.RetryDigestJobResp{Success: false, Job: toRPCJob(result.Job), Msg: err.Error()}, nil
	}
	return &conversation_intelligence.RetryDigestJobResp{Success: true, Job: toRPCJob(result.Job), Artifacts: resultToRPCArtifacts(result)}, nil
}

func resultToRPCArtifacts(result conversationintel.DigestResult) []*conversation_intelligence.ConversationArtifact {
	var artifacts []conversationintel.ArtifactDTO
	if result.Summary != nil {
		artifacts = append(artifacts, conversationintel.ArtifactDTO{ConversationID: result.Job.ConversationID, Type: "conversation_summary", Title: "会话摘要", Content: result.Summary.Summary, SourceMsgIDs: result.Summary.SourceMsgIDs, Confidence: 0.7})
	}
	for _, item := range result.Decisions {
		artifacts = append(artifacts, conversationintel.ArtifactDTO{ConversationID: result.Job.ConversationID, Type: "decision", Title: "决策", Content: item.DecisionText, SourceMsgIDs: item.SourceMsgIDs, Confidence: item.Confidence})
	}
	for _, item := range result.Tasks {
		artifacts = append(artifacts, conversationintel.ArtifactDTO{ConversationID: result.Job.ConversationID, Type: "task", Title: item.TaskTitle, Content: item.TaskTitle, SourceMsgIDs: item.SourceMsgIDs, Confidence: item.Confidence})
	}
	for _, item := range result.Topics {
		artifacts = append(artifacts, conversationintel.ArtifactDTO{ConversationID: result.Job.ConversationID, Type: "topic", Title: item.Topic, Content: item.Content, SourceMsgIDs: item.SourceMsgIDs, Confidence: item.Confidence})
	}
	for _, item := range result.Quotes {
		artifacts = append(artifacts, conversationintel.ArtifactDTO{ConversationID: result.Job.ConversationID, Type: "quote", Title: "金句/原则", Content: item.Quote, SourceMsgIDs: item.SourceMsgIDs, Confidence: item.Confidence})
	}
	for _, item := range result.MemoryCandidates {
		artifacts = append(artifacts, conversationintel.ArtifactDTO{ConversationID: result.Job.ConversationID, Type: "memory_candidate", Title: item.Title, Content: item.Content, SourceMsgIDs: item.SourceMsgIDs, Confidence: item.Confidence})
	}
	return toRPCArtifacts(artifacts)
}

func toRPCJob(job conversationintel.DigestJob) *conversation_intelligence.DigestJob {
	return &conversation_intelligence.DigestJob{
		Id:             job.ID,
		ConversationId: job.ConversationID,
		ViewerId:       job.ViewerID,
		AgentId:        job.AgentID,
		Status:         job.Status,
		MessageCount:   int64(job.MessageCount),
		ValuableCount:  int64(job.ValuableCount),
		ErrorMessage:   job.ErrorMessage,
		RetryCount:     int64(job.RetryCount),
		MaxRetries:     int64(job.MaxRetries),
		NextRunAt:      job.NextRunAt,
		LastAttemptAt:  job.LastAttemptAt,
		CompletedAt:    job.CompletedAt,
		Reason:         job.Reason,
	}
}

func toRPCJobs(jobs []conversationintel.DigestJob) []*conversation_intelligence.DigestJob {
	out := make([]*conversation_intelligence.DigestJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, toRPCJob(job))
	}
	return out
}

func toRPCArtifacts(artifacts []conversationintel.ArtifactDTO) []*conversation_intelligence.ConversationArtifact {
	out := make([]*conversation_intelligence.ConversationArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, &conversation_intelligence.ConversationArtifact{
			Id:               artifact.ID,
			JobId:            artifact.JobID,
			ConversationId:   artifact.ConversationID,
			Type:             artifact.Type,
			Title:            artifact.Title,
			Content:          artifact.Content,
			Metadata:         artifact.Metadata,
			SourceMessageIds: artifact.SourceMsgIDs,
			Confidence:       artifact.Confidence,
		})
	}
	return out
}

func parseOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}
