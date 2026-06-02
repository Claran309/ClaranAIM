package handler

import (
	"ClaranAIM/pkg/conversationintelclient"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
)

type fakeConversationIntelligenceService struct {
	input         conversationintelclient.CreateDigestJobInput
	processJobID  int64
	processViewer int64
}

func (f *fakeConversationIntelligenceService) CreateDigestJob(ctx context.Context, input conversationintelclient.CreateDigestJobInput) (*conversationintelclient.DigestJob, error) {
	_ = ctx
	f.input = input
	return &conversationintelclient.DigestJob{ID: 1, ConversationID: input.ConversationID, ViewerID: input.ViewerID, AgentID: input.AgentID, Status: "pending"}, nil
}

func (f *fakeConversationIntelligenceService) ProcessDigestJob(ctx context.Context, jobID, viewerID int64) (conversationintelclient.ProcessResult, error) {
	_ = ctx
	f.processJobID = jobID
	f.processViewer = viewerID
	return conversationintelclient.ProcessResult{Job: conversationintelclient.DigestJob{ID: jobID, Status: "completed"}}, nil
}

func (f *fakeConversationIntelligenceService) RetryDigestJob(ctx context.Context, jobID, viewerID int64) (conversationintelclient.ProcessResult, error) {
	_ = ctx
	f.processJobID = jobID
	f.processViewer = viewerID
	return conversationintelclient.ProcessResult{Job: conversationintelclient.DigestJob{ID: jobID, Status: "completed"}}, nil
}

func (f *fakeConversationIntelligenceService) ListDigestJobs(ctx context.Context, input conversationintelclient.ListDigestJobsInput) ([]conversationintelclient.DigestJob, int64, error) {
	_ = ctx
	return []conversationintelclient.DigestJob{{ConversationID: input.ConversationID, ViewerID: input.ViewerID, Status: input.Status}}, 1, nil
}

func (f *fakeConversationIntelligenceService) ListArtifacts(ctx context.Context, input conversationintelclient.ListArtifactsInput) ([]conversationintelclient.Artifact, int64, error) {
	_ = ctx
	return []conversationintelclient.Artifact{{ConversationID: input.ConversationID, Type: input.ArtifactType}}, 1, nil
}

func TestCreateDigestJobUsesAuthenticatedViewer(t *testing.T) {
	fake := &fakeConversationIntelligenceService{}
	h := &ConversationIntelligenceHandler{svc: fake}
	c := app.NewContext(0)
	c.Set("userID", int64(1001))
	body, _ := json.Marshal(map[string]interface{}{
		"conversation_id": 2002,
		"viewer_id":       9999,
		"agent_id":        3003,
		"reason":          "manual",
	})
	c.Request.SetMethod(http.MethodPost)
	c.Request.SetBody(body)

	h.CreateDigestJob(context.Background(), c)

	if fake.input.ViewerID != 1001 {
		t.Fatalf("viewer id = %d, want authenticated user 1001", fake.input.ViewerID)
	}
	if fake.input.ConversationID != 2002 || fake.input.AgentID != 3003 {
		t.Fatalf("input identifiers not preserved: %#v", fake.input)
	}
	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
}

func TestConversationIntelligenceBindKeepsLargeIDs(t *testing.T) {
	raw := `{"conversation_id":49895688258818048,"agent_id":49895688258818049}`
	c := app.NewContext(0)
	c.Request.SetBody([]byte(raw))

	var req conversationDigestJobRequest
	if err := bindConversationIntelligenceJSON(c, &req); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	conversationID, err := parseConversationIntelligenceNumber(req.ConversationID)
	if err != nil {
		t.Fatalf("parse conversation id: %v", err)
	}
	if conversationID != 49895688258818048 {
		t.Fatalf("conversation id = %d", conversationID)
	}
}

func TestProcessDigestJobRejectsInvalidID(t *testing.T) {
	h := &ConversationIntelligenceHandler{svc: &fakeConversationIntelligenceService{}}
	c := app.NewContext(0)
	c.Set("userID", int64(1001))
	c.Params = append(c.Params, param.Param{Key: "id", Value: "bad"})

	h.ProcessDigestJob(context.Background(), c)

	if c.Response.StatusCode() != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
	if !bytes.Contains(c.Response.Body(), []byte("无效的归档任务ID")) {
		t.Fatalf("unexpected body: %s", c.Response.Body())
	}
}

func TestProcessDigestJobUsesAuthenticatedViewer(t *testing.T) {
	fake := &fakeConversationIntelligenceService{}
	h := &ConversationIntelligenceHandler{svc: fake}
	c := app.NewContext(0)
	c.Set("userID", int64(1001))
	c.Params = append(c.Params, param.Param{Key: "id", Value: "55"})

	h.ProcessDigestJob(context.Background(), c)

	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
	if fake.processJobID != 55 || fake.processViewer != 1001 {
		t.Fatalf("process args job=%d viewer=%d, want job=55 viewer=1001", fake.processJobID, fake.processViewer)
	}
}
