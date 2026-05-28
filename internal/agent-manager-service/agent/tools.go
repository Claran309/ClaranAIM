package agent

import (
	"ClaranAIM/internal/agent-manager-service/graphTool"
	"ClaranAIM/internal/agent-manager-service/logic"
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// InitTools 初始化 bot 可调用的 Eino 工具集合。
//
// includeDomainTools 控制是否注册明日方舟领域演示工具；RAG 与联网搜索工具始终尝试
// 注册。单个工具初始化失败不会中断整个 bot 启动，只记录日志并继续加载其他工具。
func InitTools(ctx context.Context, chatModel model.BaseChatModel, includeDomainTools bool) []tool.BaseTool {
	var tools []tool.BaseTool

	if includeDomainTools {
		OperatorQueryTool, err := utils.InferTool(
			"operator_query",
			"根据干员名称查询其职业、特长和天赋等基本信息",
			logic.QueryOperator,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 operator_query 初始化成功")
			tools = append(tools, OperatorQueryTool)
		}

		ResourceQueryTool, err := utils.InferTool(
			"resource_query",
			"查询当前的资源状况，包括龙门币、合成玉、理智、源石等",
			logic.QueryResources,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 resource_query 初始化成功")
			tools = append(tools, ResourceQueryTool)
		}

		BattlePlanTool, err := utils.InferTool(
			"battle_plan",
			"根据关卡名称和难度偏好制定作战计划，推荐编队和作战要点",
			logic.MakeBattlePlan,
		)
		if err != nil {
			log.Print("初始化工具失败:", err)
		} else {
			log.Println("工具 battle_plan 初始化成功")
			tools = append(tools, BattlePlanTool)
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
