package agent

import (
	"ClaranAIM/internal/agent-runtime-service/component"
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

// NewDeepAgent 构建项目使用的 Eino ADK DeepAgent。
// 这里把本地文件/命令后端、可选 CozeLoop 追踪、Skill 中间件、安全工具策略和项目领域工具组合成一个 Agent 实例；
// agent-runtime-service 会按配置缓存返回的实例，避免每轮对话重复初始化。
func NewDeepAgent(ctx context.Context, model *openai.ChatModel, agentRoot string, cozeloopApiToken string, cozeloopWorkspaceID string, skillDir string, agentName string, agentDescription string, systemPrompt string, toolPolicy string, includeDomainTools bool) (adk.Agent, error) {
	// 创建LocalBackend Tools 后端工具实例
	backend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		return nil, err
	}
	sandboxBackend := component.NewWorkspaceSandbox(backend, agentRoot)

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

	// 配置 CozeLoop 追踪；未提供 token/workspace 时只关闭链路追踪，不影响 Agent 执行。
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

	// 创建自定义工具集，包含 WebSearch、RAG、代码分析等项目内工具。
	toolPolicy = strings.TrimSpace(toolPolicy)
	tools := InitTools(ctx, model, includeDomainTools && !toolPolicyDisablesTools(toolPolicy))
	toolsConfig := adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
	}

	// 获取文件系统操作说明，并把受控工作目录注入系统提示词。
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
	if strings.TrimSpace(skillInstruction) != "" {
		instruction = skillInstruction + "\n\n" + instruction
		log.Printf("Skill加载成功: loaded_skill_name=%s skills_dir=%s entry_file=%s include_domain_tools=%v", filepath.Base(skillsDir), skillsDir, filepath.Join(skillsDir, "SKILL.md"), includeDomainTools)
	} else if strings.TrimSpace(skillDir) != "" {
		log.Printf("Skill加载失败或为空: configured_skill_dir=%s include_domain_tools=%v", skillDir, includeDomainTools)
	}
	instruction = instruction + "\n\n" + ToolPolicyInstruction(toolPolicy) + "\n\n" + extInstruction

	agentConfig := &deep.Config{
		Name:           effectiveName,
		Description:    effectiveDesc,
		Instruction:    instruction,
		ChatModel:      model,
		ToolsConfig:    toolsConfig,
		Backend:        sandboxBackend, // 注入受工作目录硬约束的文件工具集
		StreamingShell: sandboxBackend, // 支持流式 Shell 输出，并限制在 Agent 工作目录内
		MaxIteration:   50,             // 最大思考/工具调用循环次数
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

func toolPolicyDisablesTools(toolPolicy string) bool {
	switch strings.ToLower(strings.TrimSpace(toolPolicy)) {
	case "disabled", "none", "no_tools", "skill_only", "skill-only":
		return true
	default:
		return false
	}
}

// ToolPolicyInstruction 根据 Agent 工具策略生成系统提示词片段。
// 这只是模型侧约束，真正的文件越权和高风险工具拦截仍必须由 SafeToolMiddleware 等服务端逻辑兜底。
func ToolPolicyInstruction(toolPolicy string) string {
	switch strings.ToLower(strings.TrimSpace(toolPolicy)) {
	case "approval_required", "approval", "confirm":
		return `## 工具审批策略
- 你可以读取、分析和列出工作目录内的文件。
- 在执行写文件、删除文件、覆盖文件、运行命令、安装依赖、修改配置等可能改变环境的动作前，必须先向用户说明你准备做什么、影响哪些路径、为什么需要这样做，并等待用户明确同意。
- 用户明确表示“确认”“可以”“继续”“同意”后，你可以在下一轮对话中继续执行刚才说明过的动作。`
	case "readonly", "read_only":
		return `## 工具审批策略
- 当前 Agent 只能读取和分析信息，不应执行写文件、删除文件、覆盖文件、运行命令、安装依赖或修改配置等会改变环境的动作。
- 如果用户要求修改环境，请先说明当前工具策略不允许直接执行，并给出可读的建议方案。`
	case "skill_only", "skill-only", "no_tools":
		return `## 工具审批策略
- 当前 Agent 处于 Skill-only 验证模式，不应主动调用任何外部工具。
- 请只根据已加载的 SKILL.md、当前用户输入和已有上下文回答。
- 如果用户要求测试 Skill，请输出已加载 Skill 要求的 marker 或按 Skill 的输出格式作答；不要创建新 Skill、生成 SKILL.md 模板、列出工具或写测试报告文件。`
	case "disabled":
		return `## 工具审批策略
- 当前 Agent 不应主动调用任何外部工具。
- 请仅基于已经提供的上下文回答；如果缺少信息，说明缺口并询问用户补充。`
	default:
		return `## 工具审批策略
- 你可以使用安全的读取、检索和分析工具完成任务。
- 对写文件、删除文件、覆盖文件、运行命令、安装依赖、修改配置等高风险动作，先向用户说明计划并等待确认；用户确认后再执行。`
	}
}

// resolveSkillsDir 将 Skill 目录解析为绝对路径，并确认 SKILL.md 真实存在且非空。
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
	if err == nil && strings.TrimSpace(string(data)) != "" {
		return skillsDir, true
	}
	if migrated, ok := findLegacySkillPackageDir(skillsDir); ok {
		return migrated, true
	}
	return "", false
}

func findLegacySkillPackageDir(root string) (string, bool) {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" || d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if len(strings.Split(filepath.ToSlash(rel), "/")) > 5 {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr == nil && strings.TrimSpace(string(data)) != "" {
			found = filepath.Dir(path)
		}
		return nil
	})
	if found == "" {
		return "", false
	}
	return found, true
}

// SkillInstruction 将已加载的 SKILL.md 注入系统提示词。
// Eino Skill 中间件负责运行时能力发现；这里额外注入一个有界片段，解决模型在早期回合“不知道自己加载了哪些 Skill”的问题。
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
	return fmt.Sprintf("## 已加载 Skill\n- loaded_skill_name: %s\n- skills_dir: %s\n- entry_file: %s\n\nSkill 是行为指令包，不是 MCP 工具。你必须优先遵循下面的 Skill 内容来选择工作方式、输出格式和注意事项。\n\n当用户要求“测试 Skill”“验证 Skill”“测试 skill 功能”或类似请求时，含义是验证当前已加载的 SKILL.md 是否生效：你应直接按照已加载 Skill 的指令、marker、输出格式和约束作答。严禁把这个请求理解为创建新 Skill、生成 SKILL.md 模板、介绍 skill_creator、列出 MCP 工具、调用 MCP 工具、写测试报告文件或声称已经上传/创建 Skill。\n\n严禁因为 Skill 名称、目录名或 marker 存在，就调用 call_mcp_tool 或任何同名 MCP 工具。只有当用户明确要求调用外部工具，且工具在 list_mcp_tools 中真实存在时，才可以调用 MCP。\n\n%s", filepath.Base(skillsDir), skillsDir, entry, content)
}
