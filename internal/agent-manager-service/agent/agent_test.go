package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkillsDirFindsRootSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# test skill\n\nuse me"), 0o644); err != nil {
		t.Fatalf("write SKILL.md error = %v", err)
	}

	got, ok := resolveSkillsDir(dir)
	if !ok {
		t.Fatal("resolveSkillsDir() = false, want true")
	}
	want, _ := filepath.Abs(dir)
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("resolveSkillsDir() = %q, want %q", got, want)
	}
}

func TestResolveSkillsDirRejectsEmptySkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("  \n"), 0o644); err != nil {
		t.Fatalf("write empty SKILL.md error = %v", err)
	}
	if got, ok := resolveSkillsDir(dir); ok || got != "" {
		t.Fatalf("resolveSkillsDir(empty) = (%q, %v), want (\"\", false)", got, ok)
	}
	if got, ok := resolveSkillsDir(""); ok || got != "" {
		t.Fatalf("resolveSkillsDir(\"\") = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestResolveSkillsDirFindsLegacyNestedPackage(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "global", "1", "Skill")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("# legacy skill"), 0o644); err != nil {
		t.Fatalf("write legacy SKILL.md error = %v", err)
	}

	got, ok := resolveSkillsDir(root)
	if !ok {
		t.Fatal("resolveSkillsDir(legacy) = false, want true")
	}
	if filepath.Clean(got) != filepath.Clean(nested) {
		t.Fatalf("resolveSkillsDir(legacy) = %q, want %q", got, nested)
	}
}

func TestSkillInstructionInjectsSkillContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# demo\n\n按步骤执行"), 0o644); err != nil {
		t.Fatalf("write SKILL.md error = %v", err)
	}

	got := SkillInstruction(dir)
	for _, want := range []string{"已加载 Skill", "loaded_skill_name", "按步骤执行"} {
		if !strings.Contains(got, want) {
			t.Fatalf("SkillInstruction() missing %q: %s", want, got)
		}
	}
	if empty := SkillInstruction(filepath.Join(dir, "missing")); empty != "" {
		t.Fatalf("SkillInstruction(missing) = %q, want empty", empty)
	}
}

func TestFileSystemInstructionUsesAgentRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	got := FileSystemInstruction(root)
	for _, want := range []string{"文件管理操作说明", root, "绝对路径", "main.go"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FileSystemInstruction() missing %q: %s", want, got)
		}
	}
}
