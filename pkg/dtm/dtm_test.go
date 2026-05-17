package dtm

import "testing"

func TestBuildURLAddsLeadingSlash(t *testing.T) {
	got := BuildURL("127.0.0.1", 8080, "dtm/action")
	want := "http://127.0.0.1:8080/dtm/action"

	if got != want {
		t.Fatalf("BuildURL() = %q, want %q", got, want)
	}
}

func TestSagaBuilderKeepsStepsAndGID(t *testing.T) {
	builder := (&SagaBuilder{}).
		WithGID("gid-1").
		AddStep("http://svc/action", "http://svc/compensate", map[string]int{"id": 1})

	if builder.GID() != "gid-1" {
		t.Fatalf("GID() = %q, want gid-1", builder.GID())
	}

	steps := builder.Steps()
	if len(steps) != 1 {
		t.Fatalf("len(Steps()) = %d, want 1", len(steps))
	}
	if steps[0].Action != "http://svc/action" || steps[0].Compensate != "http://svc/compensate" {
		t.Fatalf("unexpected step: %#v", steps[0])
	}

	steps[0].Action = "changed"
	if builder.Steps()[0].Action == "changed" {
		t.Fatal("Steps() should return a copy")
	}
}

func TestNewSagaLocalDoesNotRequireRunningDTMServer(t *testing.T) {
	manager := NewManager("http://127.0.0.1:1")

	builder, err := manager.NewSagaLocal()
	if err != nil {
		t.Fatalf("NewSagaLocal returned error: %v", err)
	}
	if builder.GID() == "" {
		t.Fatal("NewSagaLocal should assign a non-empty gid")
	}
}

func TestGenGIDConvertsDTMClientPanicToError(t *testing.T) {
	_, err := GenGID("http://127.0.0.1:1")
	if err == nil {
		t.Fatal("GenGID should return an error when DTM server is unavailable")
	}
}
