package agent

import (
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
