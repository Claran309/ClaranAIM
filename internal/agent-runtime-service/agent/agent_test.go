package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkillsDirAcceptsLegacyNestedSkillPackage(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "global", "1", "Skill")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("mkdir legacy skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "SKILL.md"), []byte("# Smoke Skill\n\n请输出 skill-smoke-ok。"), 0644); err != nil {
		t.Fatalf("write legacy SKILL.md: %v", err)
	}

	got, ok := resolveSkillsDir(root)
	if !ok {
		t.Fatal("resolveSkillsDir should find legacy nested SKILL.md")
	}
	if got != legacyDir {
		t.Fatalf("skills dir = %q, want legacy package dir %q", got, legacyDir)
	}
	instruction := SkillInstruction(got)
	if !strings.Contains(instruction, "skill-smoke-ok") || !strings.Contains(instruction, "不是 MCP 工具") {
		t.Fatalf("skill instruction did not include content and MCP guard: %q", instruction)
	}
	for _, want := range []string{"严禁把这个请求理解为创建新 Skill", "生成 SKILL.md 模板", "介绍 skill_creator", "列出 MCP 工具"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("skill instruction missing guard %q: %q", want, instruction)
		}
	}
}

func TestToolPolicyDisablesToolsForSkillOnlySmoke(t *testing.T) {
	for _, value := range []string{"disabled", "skill_only", "skill-only", "no_tools"} {
		if !toolPolicyDisablesTools(value) {
			t.Fatalf("toolPolicyDisablesTools(%q)=false, want true", value)
		}
	}
	if toolPolicyDisablesTools("approval_required") {
		t.Fatal("approval_required should not disable tools")
	}
	instruction := ToolPolicyInstruction("skill_only")
	for _, want := range []string{"Skill-only", "不要创建新 Skill", "生成 SKILL.md 模板", "列出工具"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("skill_only instruction missing %q: %q", want, instruction)
		}
	}
}
