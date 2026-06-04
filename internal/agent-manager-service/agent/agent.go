package agent

import (
	"ClaranAIM/internal/agent-manager-service/component"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	clc "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/coze-dev/cozeloop-go"
)

// NewDeepAgent 构造基于 Eino ADK DeepAgent 的运行实例。
//
// 这里会把本地工具执行后端、可选的 CozeLoop 追踪、Skill 中间件和项目内领域工具
// 组装进同一个 Agent。agent-manager-service 会按 Agent 配置缓存返回的实例，
// 避免每次调用都重复初始化模型和工具链。
func NewDeepAgent(ctx context.Context, model *openai.ChatModel, agentRoot string, cozeloopApiToken string, cozeloopWorkspaceID string, skillDir string, agentName string, agentDescription string, systemPrompt string, includeDomainTools bool) (adk.Agent, error) {
	// 创建LocalBackend Tools 后端工具实例
	backend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		return nil, err
	}

	// 注册全局回调，便于在本地日志中观察 Agent 每个组件的启动、结束和异常。
	handler := callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			log.Printf("[trace] %s/%s start", info.Component, info.Name)
			return ctx
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			log.Printf("[trace] %s/%s end", info.Component, info.Name)
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			log.Printf("[trace] %s/%s error: %v", info.Component, info.Name, err)
			return ctx
		}).Build()

	callbacks.AppendGlobalHandlers(handler)

	// 配制 CozeLoop 追踪
	if cozeloopApiToken != "" && cozeloopWorkspaceID != "" {
		client, err := cozeloop.NewClient(
			cozeloop.WithAPIToken(cozeloopApiToken),
			cozeloop.WithWorkspaceID(cozeloopWorkspaceID),
		)
		if err != nil {
			return nil, fmt.Errorf("初始化CozeLoop客户端失败: %v", err)
		}
		defer func() {
			time.Sleep(5 * time.Second)
			client.Close(ctx)
		}()
		callbacks.AppendGlobalHandlers(clc.NewLoopHandler(client))
		log.Println("CozeLoop追踪已启用")
	} else {
		log.Println("CozeLoop追踪未启用，缺少API Token或Workspace ID")
	}

	// 创建自定义工具集
	tools := InitTools(ctx, model, includeDomainTools)
	toolsConfig := adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
	}

	// 获取文件系统操作说明
	extInstruction := FileSystemInstruction(agentRoot)
	skillInstruction := ""

	// 加载中间件，如果没有Skill文件就不加载相关中间件
	var handlers []adk.ChatModelAgentMiddleware
	skillsDir, found := resolveSkillsDir(skillDir)
	if found {
		skillInstruction = SkillInstruction(skillsDir)
		skillBackend, sbErr := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
			Backend: backend,
			BaseDir: skillsDir,
		})
		if sbErr != nil {
			return nil, fmt.Errorf("加载Skill后端失败 skills_dir=%s: %w", skillsDir, sbErr)
		}
		skillMiddleware, smErr := skill.NewMiddleware(ctx, &skill.Config{
			Backend: skillBackend,
		})
		if smErr != nil {
			return nil, fmt.Errorf("加载Skill中间件失败 skills_dir=%s: %w", skillsDir, smErr)
		}
		handlers = append(handlers, skillMiddleware)
	}
	// 注册安全工具调用中间件
	handlers = append(handlers, &component.SafeToolMiddleware{})

	// 创建DeepAgent类型的Agent实例
	effectiveName := agentName
	if effectiveName == "" {
		effectiveName = DefaultAgentName
	}
	effectiveDesc := agentDescription
	if effectiveDesc == "" {
		effectiveDesc = DefaultAgentDescription
	}

	instruction := systemPrompt
	if instruction == "" {
		instruction = DefaultAgentInstruction
	}
	instruction = instruction + "\n\n" + extInstruction + "\n\n" + skillInstruction

	agentConfig := &deep.Config{
		Name:           effectiveName,
		Description:    effectiveDesc,
		Instruction:    instruction,
		ChatModel:      model,
		ToolsConfig:    toolsConfig,
		Backend:        backend, // 注入LocalBackend工具集
		StreamingShell: backend, // 支持流式 Shell 输出
		MaxIteration:   50,      // 最大思考/工具调用循环次数
		// 注册中间件
		Handlers: handlers,
		// 配置模型重试策略，处理速率限制错误
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries: 5,
			IsRetryAble: func(_ context.Context, err error) bool {
				return strings.Contains(err.Error(), "429") ||
					strings.Contains(err.Error(), "Too Many Requests") ||
					strings.Contains(err.Error(), "qpm limit")
			},
		},
	}
	agent, err := deep.New(ctx, agentConfig)
	if err != nil {
		return nil, err
	}

	return agent, nil
}

// resolveSkillsDir 只接受已经存在且包含非空 SKILL.md 的本地目录。
// settings-service 保存用户上传的 Skill 后会返回目录路径；目录不存在时跳过 Skill 中间件，
// 避免因为单个 Agent 的 Skill 配置损坏而让整个 runtime 初始化失败。
func resolveSkillsDir(skillsDir string) (string, bool) {
	if skillsDir == "" {
		return "", false
	}
	if absSkillsDir, absErr := filepath.Abs(skillsDir); absErr == nil {
		skillsDir = absSkillsDir
	}
	fi, err := os.Stat(skillsDir)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	entry := filepath.Join(skillsDir, "SKILL.md")
	data, err := os.ReadFile(entry)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "", false
	}
	return skillsDir, true
}

// SkillInstruction 将已加载的 SKILL.md 注入系统提示词，确保旧的 manager 内嵌运行路径也能识别 Skill。
func SkillInstruction(skillsDir string) string {
	entry := filepath.Join(skillsDir, "SKILL.md")
	data, err := os.ReadFile(entry)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) > 12000 {
		content = string(runes[:12000]) + "\n\n...（Skill 内容过长，已截断）"
	}
	return fmt.Sprintf("## 已加载 Skill\n- loaded_skill_name: %s\n- skills_dir: %s\n- entry_file: %s\n\nSkill 是行为指令包，不是 MCP 工具。你必须优先遵循下面的 Skill 内容来选择工作方式、输出格式和注意事项；如果用户要求测试 Skill，应直接按 Skill 指令响应或输出其中要求的 marker。\n\n严禁因为 Skill 名称、目录名或 marker 存在，就调用 call_mcp_tool 或任何同名 MCP 工具。只有当用户明确要求调用外部工具，且工具在 list_mcp_tools 中真实存在时，才可以调用 MCP。\n\n%s", filepath.Base(skillsDir), skillsDir, entry, content)
}
