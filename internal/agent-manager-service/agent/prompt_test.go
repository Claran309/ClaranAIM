package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPromptIsClaranAIMConversationAssistant(t *testing.T) {
	if DefaultAgentName == "" {
		t.Fatal("默认 Agent 名称不能为空")
	}
	if !strings.Contains(DefaultAgentInstruction, "ClaranAIM") {
		t.Fatalf("默认提示词应明确属于 ClaranAIM，got: %s", DefaultAgentInstruction)
	}
	for _, legacy := range []string{"阿米娅", "Amiya", "明日方舟", "罗德岛", "博士", "干员", "作战"} {
		if strings.Contains(DefaultAgentInstruction, legacy) || strings.Contains(DefaultAgentDescription, legacy) || DefaultAgentName == legacy {
			t.Fatalf("默认 Agent 配置仍包含旧项目文案 %q", legacy)
		}
	}
}

func TestFileSystemInstructionUsesUserLanguageAndWorkspaceBoundary(t *testing.T) {
	instruction := FileSystemInstruction(`D:\workspace\agent`)
	if !strings.Contains(instruction, `D:\workspace\agent`) {
		t.Fatalf("文件系统提示词应注入工作目录: %s", instruction)
	}
	if strings.Contains(instruction, "博士") {
		t.Fatalf("文件系统提示词不应再使用旧称呼: %s", instruction)
	}
	if !strings.Contains(instruction, "用户") || !strings.Contains(instruction, "不要写到项目仓库根目录") {
		t.Fatalf("文件系统提示词应说明用户语义和写入边界: %s", instruction)
	}
}

func TestResolveSkillsDirAcceptsLegacyNestedSkillPackage(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "global", "1", "Skill", "skill")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("mkdir legacy skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "SKILL.md"), []byte("# Smoke Skill\n\nInclude exact marker: `SKILL_OK_7F3A`."), 0644); err != nil {
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
	if !strings.Contains(instruction, "SKILL_OK_7F3A") || !strings.Contains(instruction, "不是 MCP 工具") {
		t.Fatalf("skill instruction did not include nested content and MCP guard: %q", instruction)
	}
}
