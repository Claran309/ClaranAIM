package component

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
)

func TestWorkspaceSandboxRejectsPathEscapes(t *testing.T) {
	root := t.TempDir()
	backend := NewWorkspaceSandbox(filesystem.NewInMemoryBackend(), root)
	ctx := context.Background()

	if err := backend.Write(ctx, &filesystem.WriteRequest{
		FilePath: filepath.Join(root, "allowed.txt"),
		Content:  "ok",
	}); err != nil {
		t.Fatalf("write inside workspace: %v", err)
	}

	for _, tt := range []struct {
		name string
		run  func() error
	}{
		{
			name: "read parent traversal",
			run: func() error {
				_, err := backend.Read(ctx, &filesystem.ReadRequest{FilePath: filepath.Join(root, "..", "secret.txt")})
				return err
			},
		},
		{
			name: "write parent traversal",
			run: func() error {
				return backend.Write(ctx, &filesystem.WriteRequest{FilePath: filepath.Join(root, "..", "secret.txt"), Content: "no"})
			},
		},
		{
			name: "grep parent traversal",
			run: func() error {
				_, err := backend.GrepRaw(ctx, &filesystem.GrepRequest{Path: filepath.Join(root, ".."), Pattern: "secret"})
				return err
			},
		},
		{
			name: "glob parent traversal",
			run: func() error {
				_, err := backend.GlobInfo(ctx, &filesystem.GlobInfoRequest{Path: filepath.Join(root, ".."), Pattern: "*"})
				return err
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "workspace") {
				t.Fatalf("error = %v, want workspace rejection", err)
			}
		})
	}
}

func TestWorkspaceSandboxRejectsSymlinkEscapes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires privileges in many dev environments")
	}

	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	backend := NewWorkspaceSandbox(filesystem.NewInMemoryBackend(), root)
	_, err := backend.LsInfo(context.Background(), &filesystem.LsInfoRequest{Path: link})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("error = %v, want symlink workspace rejection", err)
	}
}

func TestWorkspaceSandboxRejectsShellPathEscapes(t *testing.T) {
	root := t.TempDir()
	backend := NewWorkspaceSandbox(filesystem.NewInMemoryBackend(), root)

	for _, command := range []string{
		"cat ../secret.txt",
		"cat /etc/passwd",
		"grep token ../../config.yml",
		"cd /tmp && cat secret.txt",
		"cd .. && cat secret.txt",
		"cat < /etc/passwd",
		"echo leaked > ../secret.txt",
		"cat ~/secret.txt",
		"cat $HOME/secret.txt",
		"cat ${HOME}/secret.txt",
		"python -c \"open('/etc/passwd').read()\"",
		"python -c \"open('../secret.txt').read()\"",
		"powershell -Command \"Get-Content C:\\Windows\\win.ini\"",
	} {
		t.Run(command, func(t *testing.T) {
			err := backend.validateCommand(command)
			if err == nil || !strings.Contains(err.Error(), "workspace") {
				t.Fatalf("error = %v, want workspace rejection", err)
			}
		})
	}
}

func TestWorkspaceSandboxAllowsShellPathsInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	backend := NewWorkspaceSandbox(filesystem.NewInMemoryBackend(), root)

	for _, command := range []string{
		"cat notes.txt",
		"grep token ./subdir/file.txt",
		"cat " + filepath.Join(root, "notes.txt"),
	} {
		t.Run(command, func(t *testing.T) {
			if err := backend.validateCommand(command); err != nil {
				t.Fatalf("validateCommand(%q) returned error: %v", command, err)
			}
		})
	}
}
