package agent

import (
	"ClaranAIM/internal/agent-runtime-service/graphTool"
	"ClaranAIM/internal/agent-runtime-service/logic"
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// InitTools 初始化 Agent 可调用的 Eino 工具集合。
//
// includeDomainTools 保留历史参数名，当前含义是是否注册 ClaranAIM 通用工作工具；
// RAG 与联网搜索工具始终尝试注册。单个工具初始化失败不会中断整个 Agent 启动，
// 只记录日志并继续加载其他工具。
func InitTools(ctx context.Context, chatModel model.BaseChatModel, includeDomainTools bool) []tool.BaseTool {
	var tools []tool.BaseTool

	if includeDomainTools {
		conversationDigestTool, err := utils.InferTool(
			"conversation_digest",
			"把会话文本整理为摘要、结论、待办、风险和下一步建议",
			logic.BuildConversationDigest,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 conversation_digest 初始化成功")
			tools = append(tools, conversationDigestTool)
		}

		taskBreakdownTool, err := utils.InferTool(
			"task_breakdown",
			"把用户目标拆解为执行步骤、验收标准、风险和需要补充的信息",
			logic.BreakDownTask,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 task_breakdown 初始化成功")
			tools = append(tools, taskBreakdownTool)
		}

		textPolishTool, err := utils.InferTool(
			"text_polish",
			"按指定语气和格式润色、改写或结构化一段文本",
			logic.PolishText,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 text_polish 初始化成功")
			tools = append(tools, textPolishTool)
		}

		skillCreatorTool, err := utils.InferTool(
			"skill_creator",
			"根据用户目标生成标准 SKILL.md 模板，便于上传到 ClaranAIM 的 Skill 管理页",
			logic.CreateSkillMarkdown,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 skill_creator 初始化成功")
			tools = append(tools, skillCreatorTool)
		}

		webSearchAugmentTool, err := utils.InferTool(
			"web_search",
			"通过 MCP Gateway 查询外部实时资料、官方文档、价格、版本和 API，并返回来源。",
			logic.MCPWebSearch,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 web_search 初始化成功")
			tools = append(tools, webSearchAugmentTool)
		}

		memorySearchTool, err := utils.InferTool(
			"search_memory",
			"通过 MCP Gateway 查询用户、会话或 Agent 的长期记忆；记忆只作为可能相关的辅助信息。",
			logic.MCPSearchMemory,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 search_memory 初始化成功")
			tools = append(tools, memorySearchTool)
		}

		knowledgeSearchTool, err := utils.InferTool(
			"search_knowledge",
			"通过 MCP Gateway 检索项目文档、文件知识库和 RAG chunks，支持 Adaptive RAG、Hybrid Search、GraphRAG 和来源返回。",
			logic.MCPSearchKnowledge,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 search_knowledge 初始化成功")
			tools = append(tools, knowledgeSearchTool)
		}

		knowledgeGraphTool, err := utils.InferTool(
			"query_knowledge_graph",
			"通过 MCP Gateway 查询知识图谱实体、关系、社区摘要和邻域子图。",
			logic.MCPQueryKnowledgeGraph,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 query_knowledge_graph 初始化成功")
			tools = append(tools, knowledgeGraphTool)
		}

		conversationSummaryTool, err := utils.InferTool(
			"summarize_conversation",
			"通过 MCP Gateway 手动触发会话智能归档，总结某个会话窗口中的摘要、决策、任务、话题和候选记忆。",
			logic.MCPSummarizeConversation,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 summarize_conversation 初始化成功")
			tools = append(tools, conversationSummaryTool)
		}

		mcpListToolsTool, err := utils.InferTool(
			"list_mcp_tools",
			"列出当前用户、Agent 和会话上下文可用的全部 MCP 工具，包含内置工具和用户配置的远程 MCP 工具。",
			logic.MCPListTools,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 list_mcp_tools 初始化成功")
			tools = append(tools, mcpListToolsTool)
		}

		mcpCallTool, err := utils.InferTool(
			"call_mcp_tool",
			"调用当前上下文可用的任意 MCP 工具，主要用于调用用户配置的远程 MCP 工具。调用前应先用 list_mcp_tools 确认工具名和参数格式。",
			logic.MCPCallTool,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 call_mcp_tool 初始化成功")
			tools = append(tools, mcpCallTool)
		}
	}

	ragTool, err := graphTool.BuildTool(ctx, chatModel)
	if err != nil {
		log.Print("初始化RAG工具失败:", err)
	} else {
		log.Println("工具 rag 初始化成功")
		tools = append(tools, ragTool)
	}

	return tools
}
