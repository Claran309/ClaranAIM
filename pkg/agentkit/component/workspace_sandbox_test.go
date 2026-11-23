package component

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePathResolvesInsideRoot(t *testing.T) {
	root := t.TempDir()
	sandbox := NewWorkspaceSandbox(nil, root)

	cases := []string{"", "notes/today.md", "a/../b.md"}
	for _, input := range cases {
		got, err := sandbox.workspacePath(input)
		if err != nil {
			t.Fatalf("workspacePath(%q) error = %v", input, err)
		}
		want := root
		if input != "" {
			want = filepath.Join(root, input)
		}
		if filepath.Clean(got) != filepath.Clean(want) {
			t.Fatalf("workspacePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWorkspacePathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	sandbox := NewWorkspaceSandbox(nil, root)

	cases := []string{
		"../secret.txt",
		"notes/../../secret.txt",
		filepath.Join(outside, "secret.txt"),
	}
	for _, input := range cases {
		if _, err := sandbox.workspacePath(input); err == nil {
			t.Fatalf("workspacePath(%q) succeeded, want rejection", input)
		}
	}
}

func TestValidateCommandRejectsPathEscapes(t *testing.T) {
	root := t.TempDir()
	sandbox := NewWorkspaceSandbox(nil, root)

	rejected := []string{
		"cat ../secret.txt",
		"cat ~/secret.txt",
		"cat /etc/passwd",
		"echo $(cat ../secret.txt)",
	}
	for _, command := range rejected {
		if err := sandbox.validateCommand(command); err == nil {
			t.Fatalf("validateCommand(%q) succeeded, want rejection", command)
		}
	}

	allowed := []string{
		"echo hello",
		"ls notes",
		"go test ./...",
	}
	for _, command := range allowed {
		if err := sandbox.validateCommand(command); err != nil {
			t.Fatalf("validateCommand(%q) error = %v, want nil", command, err)
		}
	}
}

func TestRejectTraversalPattern(t *testing.T) {
	if err := rejectTraversalPattern("**/*.go"); err != nil {
		t.Fatalf("rejectTraversalPattern() error = %v, want nil", err)
	}
	if err := rejectTraversalPattern("../**/*.go"); err == nil {
		t.Fatal("rejectTraversalPattern() accepted parent traversal")
	}
	if err := rejectTraversalPattern("/etc/**"); err == nil {
		t.Fatal("rejectTraversalPattern() accepted absolute pattern")
	}
}
