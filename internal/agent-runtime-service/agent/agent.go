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
	tools := InitTools(ctx, model, includeDomainTools)
	toolsConfig := adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
	}

	// 获取文件系统操作说明，并把受控工作目录注入系统提示词。
	extInstruction := FileSystemInstruction(agentRoot)

	// 加载中间件，如果没有Skill文件就不加载相关中间件
	var handlers []adk.ChatModelAgentMiddleware
	skillsDir, found := resolveSkillsDir(skillDir)
	if found {
		skillBackend, sbErr := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
			Backend: backend,
			BaseDir: skillsDir,
		})
		if sbErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, sbErr)
			os.Exit(1)
		}
		skillMiddleware, smErr := skill.NewMiddleware(ctx, &skill.Config{
			Backend: skillBackend,
		})
		if smErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, smErr)
			os.Exit(1)
		}
		handlers = append(handlers, skillMiddleware)
	}
	// 注册安全工具调用中间件
	handlers = append(handlers, &component.SafeToolMiddleware{})

	// 创建DeepAgent类型的Agent实例
	effectiveName := agentName
	if effectiveName == "" {
		effectiveName = AmiyaName
	}
	effectiveDesc := agentDescription
	if effectiveDesc == "" {
		effectiveDesc = AmiyaDescription
	}

	instruction := systemPrompt
	if instruction == "" {
		instruction = AmiyaInstruction
	}
	instruction = instruction + "\n\n" + ToolPolicyInstruction(toolPolicy) + "\n\n" + extInstruction

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

// resolveSkillsDir 将 Skill 目录解析为绝对路径，并确认目录真实存在。
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
	return skillsDir, true
}
