package dtm

import (
	"context"
	"errors"
	"testing"
)

func TestSagaBuilderKeepsStepOrderAndCopies(t *testing.T) {
	builder := &SagaBuilder{server: "http://localhost:36789", gid: "gid-1"}
	builder.AddStep("http://group/create", "http://group/create_compensate", map[string]int64{"group_id": 1}).
		AddStep("http://msg/conversation", "http://msg/conversation_compensate", map[string]int64{"group_id": 1})
	steps := builder.Steps()
	if len(steps) != 2 || steps[0].Action != "http://group/create" || steps[1].Compensate != "http://msg/conversation_compensate" {
		t.Fatalf("steps = %#v", steps)
	}
	steps[0].Action = "mutated"
	if builder.Steps()[0].Action == "mutated" {
		t.Fatal("Steps() must return a copy")
	}
	if builder.GID() != "gid-1" {
		t.Fatalf("gid = %q", builder.GID())
	}
}

func TestSagaBuilderWithGIDOverridesGeneratedID(t *testing.T) {
	builder := &SagaBuilder{server: "http://localhost:36789", gid: "generated"}
	if got := builder.WithGID("explicit").GID(); got != "explicit" {
		t.Fatalf("gid = %q, want explicit", got)
	}
}

func TestSagaSubmitRejectsCanceledContextBeforeNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	builder := &SagaBuilder{server: "http://127.0.0.1:1", gid: "gid-1"}
	err := builder.Submit(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Submit() error = %v, want context.Canceled", err)
	}
}

func TestSagaSubmitFailsWhenDTMServerUnavailable(t *testing.T) {
	builder := &SagaBuilder{server: "http://127.0.0.1:1", gid: "gid-unreachable"}
	builder.AddStep("http://example.invalid/action", "http://example.invalid/compensate", nil)
	if err := builder.Submit(context.Background()); err == nil {
		t.Fatal("Submit() succeeded against unreachable DTM server, want error")
	}
}

func TestBuildURLNormalizesPath(t *testing.T) {
	if got := BuildURL("127.0.0.1", 9002, "dtm/group/create"); got != "http://127.0.0.1:9002/dtm/group/create" {
		t.Fatalf("BuildURL() = %q", got)
	}
	if got := BuildURL("127.0.0.1", 9002, "/dtm/group/create"); got != "http://127.0.0.1:9002/dtm/group/create" {
		t.Fatalf("BuildURL() = %q", got)
	}
}

func TestManagerServerTrimsTrailingSlash(t *testing.T) {
	manager := NewManager("http://localhost:36789/")
	if manager.Server() != "http://localhost:36789" {
		t.Fatalf("Server() = %q", manager.Server())
	}
}
