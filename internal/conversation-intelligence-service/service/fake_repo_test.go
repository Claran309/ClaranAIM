package service

import (
	"ClaranAIM/internal/conversation-intelligence-service/dao"
	"ClaranAIM/internal/conversation-intelligence-service/model"
	"context"
	"sort"
	"time"
)

type fakeConversationRepo struct {
	nextID     int64
	jobs       map[int64]*model.DigestJob
	artifacts  []model.ConversationArtifact
	activities map[int64]*model.ConversationActivityCursor
}

func newFakeConversationRepo() *fakeConversationRepo {
	return &fakeConversationRepo{nextID: 1, jobs: map[int64]*model.DigestJob{}, activities: map[int64]*model.ConversationActivityCursor{}}
}

func (r *fakeConversationRepo) CreateJob(ctx context.Context, job *model.DigestJob) error {
	_ = ctx
	if job.ID == 0 {
		job.ID = r.nextID
		r.nextID++
	}
	cp := *job
	r.jobs[job.ID] = &cp
	return nil
}

func (r *fakeConversationRepo) UpdateJob(ctx context.Context, job *model.DigestJob) error {
	_ = ctx
	cp := *job
	r.jobs[job.ID] = &cp
	return nil
}

func (r *fakeConversationRepo) GetJob(ctx context.Context, id int64) (*model.DigestJob, error) {
	_ = ctx
	job := r.jobs[id]
	if job == nil {
		return nil, nil
	}
	cp := *job
	return &cp, nil
}

func (r *fakeConversationRepo) ListJobs(ctx context.Context, filter dao.JobFilter) ([]model.DigestJob, int64, error) {
	_ = ctx
	var out []model.DigestJob
	for _, job := range r.jobs {
		if filter.ViewerID > 0 && job.ViewerID != filter.ViewerID {
			continue
		}
		if filter.ConversationID > 0 && job.ConversationID != filter.ConversationID {
			continue
		}
		if filter.Status != "" && job.Status != filter.Status {
			continue
		}
		out = append(out, *job)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, int64(len(out)), nil
}

func (r *fakeConversationRepo) ListRetryableJobs(ctx context.Context, options dao.RetryableJobOptions) ([]model.DigestJob, error) {
	_ = ctx
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	var out []model.DigestJob
	for _, job := range r.jobs {
		if job.Status != model.JobStatusFailed {
			continue
		}
		if job.MaxRetries > 0 && job.RetryCount >= job.MaxRetries {
			continue
		}
		if job.NextRunAt != nil && job.NextRunAt.After(now) {
			continue
		}
		out = append(out, *job)
	}
	return out, nil
}

func (r *fakeConversationRepo) CreateArtifacts(ctx context.Context, artifacts []model.ConversationArtifact) error {
	_ = ctx
	for i := range artifacts {
		if artifacts[i].ID == 0 {
			artifacts[i].ID = r.nextID
			r.nextID++
		}
		r.artifacts = append(r.artifacts, artifacts[i])
	}
	return nil
}

func (r *fakeConversationRepo) ListArtifacts(ctx context.Context, filter dao.ArtifactFilter) ([]model.ConversationArtifact, int64, error) {
	_ = ctx
	out := make([]model.ConversationArtifact, 0, len(r.artifacts))
	for _, artifact := range r.artifacts {
		if filter.ConversationID > 0 && artifact.ConversationID != filter.ConversationID {
			continue
		}
		if filter.ViewerID > 0 && artifact.ViewerID != filter.ViewerID {
			continue
		}
		if filter.ArtifactType != "" && artifact.ArtifactType != filter.ArtifactType {
			continue
		}
		out = append(out, artifact)
	}
	return out, int64(len(out)), nil
}

func (r *fakeConversationRepo) UpsertActivity(ctx context.Context, activity dao.ActivityUpsert) error {
	_ = ctx
	delta := activity.MessageCountDelta
	if delta <= 0 {
		delta = 1
	}
	now := activity.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	cursor := r.activities[activity.ConversationID]
	if cursor == nil {
		cursor = &model.ConversationActivityCursor{ID: r.nextID, ConversationID: activity.ConversationID, ViewerID: activity.ViewerID, AgentID: activity.AgentID, FirstMessageID: activity.MessageID, WindowStartedAt: now}
		r.nextID++
		r.activities[activity.ConversationID] = cursor
	}
	cursor.LastMessageID = activity.MessageID
	cursor.LastActivityAt = now
	cursor.PendingMessages += delta
	if activity.AgentID > 0 {
		cursor.AgentID = activity.AgentID
	}
	return nil
}

func (r *fakeConversationRepo) ListDueActivities(ctx context.Context, options dao.DueActivityOptions) ([]model.ConversationActivityCursor, error) {
	_ = ctx
	threshold := options.MessageThreshold
	if threshold <= 0 {
		threshold = 100
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	windowMinutes := options.WindowMinutes
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	deadline := now.Add(-time.Duration(windowMinutes) * time.Minute)
	var out []model.ConversationActivityCursor
	for _, cursor := range r.activities {
		if cursor.PendingMessages >= threshold || cursor.WindowStartedAt.Before(deadline) || cursor.WindowStartedAt.Equal(deadline) {
			out = append(out, *cursor)
		}
	}
	return out, nil
}

func (r *fakeConversationRepo) MarkActivityDigested(ctx context.Context, cursorID, jobID int64, digestedAt time.Time) error {
	_ = ctx
	for _, cursor := range r.activities {
		if cursor.ID != cursorID {
			continue
		}
		cursor.PendingMessages = 0
		cursor.FirstMessageID = 0
		cursor.LastMessageID = 0
		cursor.LastDigestJobID = jobID
		cursor.LastDigestAt = &digestedAt
		cursor.WindowStartedAt = digestedAt
	}
	return nil
}
