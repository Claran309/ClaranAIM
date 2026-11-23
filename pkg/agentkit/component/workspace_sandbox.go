package component

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/schema"
)

// WorkspaceSandbox hard-bounds Eino filesystem and shell backends to one Agent workspace.
type WorkspaceSandbox struct {
	backend        filesystem.Backend
	shell          filesystem.Shell
	streamingShell filesystem.StreamingShell
	root           string
}

func NewWorkspaceSandbox(backend filesystem.Backend, root string) *WorkspaceSandbox {
	rootAbs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		rootAbs = filepath.Clean(root)
	}
	rootAbs = filepath.Clean(rootAbs)
	if evaluated, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = evaluated
	}
	s := &WorkspaceSandbox{backend: backend, root: rootAbs}
	if shell, ok := backend.(filesystem.Shell); ok {
		s.shell = shell
	}
	if streamingShell, ok := backend.(filesystem.StreamingShell); ok {
		s.streamingShell = streamingShell
	}
	return s
}

func (s *WorkspaceSandbox) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	path, err := s.workspacePath(req.Path)
	if err != nil {
		return nil, err
	}
	cp := *req
	cp.Path = path
	return s.backend.LsInfo(ctx, &cp)
}

func (s *WorkspaceSandbox) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	path, err := s.workspacePath(req.FilePath)
	if err != nil {
		return nil, err
	}
	cp := *req
	cp.FilePath = path
	return s.backend.Read(ctx, &cp)
}

func (s *WorkspaceSandbox) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	path, err := s.workspacePath(req.Path)
	if err != nil {
		return nil, err
	}
	if err := rejectTraversalPattern(req.Glob); err != nil {
		return nil, err
	}
	cp := *req
	cp.Path = path
	return s.backend.GrepRaw(ctx, &cp)
}

func (s *WorkspaceSandbox) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	path, err := s.workspacePath(req.Path)
	if err != nil {
		return nil, err
	}
	if err := rejectTraversalPattern(req.Pattern); err != nil {
		return nil, err
	}
	cp := *req
	cp.Path = path
	return s.backend.GlobInfo(ctx, &cp)
}

func (s *WorkspaceSandbox) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	path, err := s.workspacePath(req.FilePath)
	if err != nil {
		return err
	}
	cp := *req
	cp.FilePath = path
	return s.backend.Write(ctx, &cp)
}

func (s *WorkspaceSandbox) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	path, err := s.workspacePath(req.FilePath)
	if err != nil {
		return err
	}
	cp := *req
	cp.FilePath = path
	return s.backend.Edit(ctx, &cp)
}

func (s *WorkspaceSandbox) Execute(ctx context.Context, input *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if err := s.validateCommand(input.Command); err != nil {
		return nil, err
	}
	if s.shell == nil {
		return nil, fmt.Errorf("workspace sandbox shell backend is unavailable")
	}
	cp := *input
	cp.Command = s.commandInWorkspace(input.Command)
	return s.shell.Execute(ctx, &cp)
}

func (s *WorkspaceSandbox) ExecuteStreaming(ctx context.Context, input *filesystem.ExecuteRequest) (*schema.StreamReader[*filesystem.ExecuteResponse], error) {
	if err := s.validateCommand(input.Command); err != nil {
		return nil, err
	}
	if s.streamingShell == nil {
		return nil, fmt.Errorf("workspace sandbox streaming shell backend is unavailable")
	}
	cp := *input
	cp.Command = s.commandInWorkspace(input.Command)
	return s.streamingShell.ExecuteStreaming(ctx, &cp)
}

func (s *WorkspaceSandbox) workspacePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = s.root
	}
	if path.IsAbs(raw) && !filepath.IsAbs(raw) {
		return "", fmt.Errorf("workspace sandbox rejected path outside workspace: %s", raw)
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.root, path)
	}
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if err := ensureUnderRoot(s.root, pathAbs); err != nil {
		return "", err
	}
	if resolved, err := evalExistingPath(pathAbs); err == nil {
		if err := ensureUnderRoot(s.root, resolved); err != nil {
			return "", err
		}
	}
	return pathAbs, nil
}

func (s *WorkspaceSandbox) validateCommand(command string) error {
	for _, token := range shellPathTokens(command) {
		if strings.Contains(token, "\x00") {
			return fmt.Errorf("workspace sandbox rejected command path with NUL byte")
		}
		if hasShellPathExpansion(token) {
			return fmt.Errorf("workspace sandbox rejected shell-expanded path: %s", token)
		}
		if err := s.rejectEmbeddedPathEscapes(token); err != nil {
			return err
		}
		if isAnyAbs(token) || hasParentTraversal(token) {
			path := token
			if !isAnyAbs(path) {
				path = filepath.Join(s.root, path)
			}
			if _, err := s.workspacePath(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *WorkspaceSandbox) rejectEmbeddedPathEscapes(token string) error {
	for _, candidate := range embeddedPathCandidates(token) {
		if hasShellPathExpansion(candidate) {
			return fmt.Errorf("workspace sandbox rejected shell-expanded path: %s", candidate)
		}
		if isAnyAbs(candidate) || hasParentTraversal(candidate) {
			path := candidate
			if !isAnyAbs(path) {
				path = filepath.Join(s.root, path)
			}
			if _, err := s.workspacePath(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *WorkspaceSandbox) commandInWorkspace(command string) string {
	if runtime.GOOS == "windows" {
		return command
	}
	return fmt.Sprintf("cd %s && %s", shellQuote(s.root), command)
}

func ensureUnderRoot(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve workspace relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("workspace sandbox rejected path outside workspace: %s", path)
	}
	return nil
}

func evalExistingPath(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(path)
	for parent != "." && parent != string(os.PathSeparator) {
		if _, err := os.Stat(parent); err == nil {
			return filepath.EvalSymlinks(parent)
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	return "", fmt.Errorf("no existing parent for %s", path)
}

func rejectTraversalPattern(pattern string) error {
	if pattern == "" {
		return nil
	}
	if isAnyAbs(pattern) || hasParentTraversal(pattern) {
		return fmt.Errorf("workspace sandbox rejected unsafe glob pattern: %s", pattern)
	}
	return nil
}

func isAnyAbs(value string) bool {
	return filepath.IsAbs(value) || path.IsAbs(strings.ReplaceAll(value, `\`, `/`))
}

func hasParentTraversal(path string) bool {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func hasShellPathExpansion(token string) bool {
	return strings.HasPrefix(token, "~") || strings.Contains(token, "$") || strings.Contains(token, "`")
}

var shellTokenRE = regexp.MustCompile(`(?:"[^"]*"|'[^']*'|[^\s;&|<>]+)`)
var (
	unixAbsPathRE    = regexp.MustCompile(`/(?:[A-Za-z0-9._@%+=:,~-]+/)*[A-Za-z0-9._@%+=:,~-]+`)
	windowsAbsPathRE = regexp.MustCompile(`[A-Za-z]:[\\/](?:[^\\/\s"'` + "`" + `;|&<>()]+[\\/]?)+`)
	parentPathRE     = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.-])\.\.(?:[\\/][^\\/\s"'` + "`" + `;|&<>()]+)*`)
)

func shellPathTokens(command string) []string {
	raw := shellTokenRE.FindAllString(command, -1)
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.Trim(token, `"'`)
		if token == "" || strings.Contains(token, "=") && !strings.Contains(token, "/") && !strings.Contains(token, `\`) {
			continue
		}
		if token == ".." || strings.Contains(token, "/") || strings.Contains(token, `\`) || isAnyAbs(token) || hasShellPathExpansion(token) {
			out = append(out, token)
		}
	}
	return out
}

func embeddedPathCandidates(token string) []string {
	var out []string
	for _, match := range unixAbsPathRE.FindAllStringIndex(token, -1) {
		if match[0] > 0 && token[match[0]-1] == '.' {
			continue
		}
		out = append(out, trimPathCandidate(token[match[0]:match[1]]))
	}
	for _, match := range windowsAbsPathRE.FindAllStringIndex(token, -1) {
		out = append(out, trimPathCandidate(token[match[0]:match[1]]))
	}
	for _, match := range parentPathRE.FindAllStringIndex(token, -1) {
		value := strings.TrimLeftFunc(token[match[0]:match[1]], func(r rune) bool {
			return r != '.'
		})
		out = append(out, trimPathCandidate(value))
	}
	return out
}

func trimPathCandidate(value string) string {
	return strings.Trim(value, `"'.,:;()[]{}<>`)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
