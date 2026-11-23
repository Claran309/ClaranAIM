package service

import (
	"context"
	"strings"
	"testing"

	"ClaranAIM/kitex_gen/bot_runtime"

	commontool "github.com/cloudwego/eino-examples/adk/common/tool"
	"github.com/cloudwego/eino/adk"
)

type fakeResumableAgent struct {
	fakeRunAgent
	resumed    bool
	resumeData any
}

func (a *fakeResumableAgent) Resume(ctx context.Context, info *adk.ResumeInfo, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	a.resumed = true
	a.resumeData = info.ResumeData
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		generator.Send(assistantEvent("resumed answer"))
	}()
	return iterator
}

func TestRunAgentInterruptThenResumeThroughCheckpoint(t *testing.T) {
	agent := &fakeResumableAgent{fakeRunAgent: fakeRunAgent{runFn: func(ctx context.Context, _ *adk.AgentInput) []*adk.AgentEvent {
		return []*adk.AgentEvent{adk.Interrupt(ctx, &commontool.ApprovalInfo{
			ToolName:        "write_file",
			ArgumentsInJSON: `{"path":"safe.txt"}`,
		})}
	}}}
	svc := testRuntimeService(t, agent, "")
	runResp, err := svc.RunAgent(context.Background(), &bot_runtime.RunAgentReq{
		Bot:       testRuntimeBot(),
		UserId:    1,
		SessionId: "approval-resume",
		Input:     "write a file",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if !runResp.Success || runResp.Approval == nil || runResp.Approval.InterruptId == "" {
		t.Fatalf("run response = %#v", runResp)
	}
	if !strings.Contains(runResp.Approval.Description, "write_file") {
		t.Fatalf("approval description = %q", runResp.Approval.Description)
	}

	resumeResp, err := svc.ResumeAgent(context.Background(), &bot_runtime.ResumeAgentReq{
		Bot:         testRuntimeBot(),
		UserId:      1,
		RunId:       runResp.GetRunId(),
		InterruptId: runResp.Approval.InterruptId,
		ApprovalId:  runResp.Approval.ApprovalId,
		Approved:    true,
		Reason:      "approved by test",
	})
	if err != nil {
		t.Fatalf("ResumeAgent() error = %v", err)
	}
	if !resumeResp.Success || !strings.Contains(resumeResp.Reply, "resumed answer") {
		t.Fatalf("resume response = %#v", resumeResp)
	}
	if !agent.resumed {
		t.Fatal("fake agent Resume() was not called")
	}
	run, err := svc.runStore.GetRun(context.Background(), runResp.GetRunId())
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run == nil || run.Status != "completed" {
		t.Fatalf("run after resume = %#v", run)
	}
}
