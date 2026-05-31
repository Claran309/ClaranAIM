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
	}

	ragTool, err := graphTool.BuildTool(ctx, chatModel)
	if err != nil {
		log.Print("初始化RAG工具失败:", err)
	} else {
		log.Println("工具 rag 初始化成功")
		tools = append(tools, ragTool)
	}

	webSearchTool, err := graphTool.BuildWebSearchTool(ctx, chatModel)
	if err != nil {
		log.Print("初始化联网搜索工具失败:", err)
	} else {
		log.Println("工具 web_search 初始化成功")
		tools = append(tools, webSearchTool)
	}

	return tools
}
