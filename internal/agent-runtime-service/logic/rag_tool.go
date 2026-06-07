package logic

import (
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"context"
	"fmt"
	"strings"
	"sync"
)

type ragRuntimeContextKey string

const (
	ragUserIDKey         ragRuntimeContextKey = "rag_user_id"
	ragConversationIDKey ragRuntimeContextKey = "rag_conversation_id"
)

var (
	ragServiceMu sync.RWMutex
	ragService   ragservice.Client
)

// SetRAGService 注入 rag-service RPC 客户端。
func SetRAGService(svc ragservice.Client) {
	ragServiceMu.Lock()
	defer ragServiceMu.Unlock()
	ragService = svc
}

// WithRAGRuntimeContext 把当前运行用户和会话写入工具上下文。
// search_knowledge_base 工具会使用这些值做权限裁剪，模型不能自行指定 viewer_id。
func WithRAGRuntimeContext(ctx context.Context, userID, conversationID int64) context.Context {
	ctx = context.WithValue(ctx, ragUserIDKey, userID)
	ctx = context.WithValue(ctx, ragConversationIDKey, conversationID)
	return ctx
}

// KnowledgeSearchParams 是 Agentic RAG 知识库检索工具的入参。
type KnowledgeSearchParams struct {
	Query string `json:"query" jsonschema:"description=要在项目知识库中检索的问题或关键词"`
	Mode  string `json:"mode" jsonschema:"description=检索模式，可选 adaptive、hybrid、graphrag；为空时使用 adaptive"`
	Limit int    `json:"limit" jsonschema:"description=返回来源数量，建议 3 到 8；为空时使用 5"`
}

// SearchKnowledgeBase 调用 rag-service 执行 Adaptive RAG / GraphRAG 检索。
// 它是 Agentic RAG 的工具入口：Agent 可以先检索知识库，再根据结果决定是否追问、换关键词或回答。
func SearchKnowledgeBase(ctx context.Context, input *KnowledgeSearchParams) (string, error) {
	if input == nil {
		input = &KnowledgeSearchParams{}
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return "知识库检索失败：query不能为空。", nil
	}
	ragServiceMu.RLock()
	svc := ragService
	ragServiceMu.RUnlock()
	if svc == nil {
		return "知识库检索不可用：agent-runtime-service 尚未连接 rag-service。", nil
	}
	userID, _ := ctx.Value(ragUserIDKey).(int64)
	if userID <= 0 {
		return "知识库检索失败：缺少当前用户上下文，无法进行权限裁剪。", nil
	}
	conversationID, _ := ctx.Value(ragConversationIDKey).(int64)
	limit := input.Limit
	if limit <= 0 || limit > 12 {
		limit = 5
	}
	resp, err := svc.Search(ctx, &rag.SearchReq{
		ViewerId:       userID,
		Query:          query,
		Mode:           strings.TrimSpace(input.Mode),
		Limit:          int64(limit),
		ConversationId: conversationID,
	})
	if err != nil {
		return "", err
	}
	if resp == nil || !resp.GetSuccess() {
		msg := "知识库检索失败"
		if resp != nil && resp.GetMsg() != "" {
			msg = resp.GetMsg()
		}
		return msg, nil
	}
	var b strings.Builder
	b.WriteString("## 知识库检索结果\n")
	b.WriteString(fmt.Sprintf("- 路线：%s\n", resp.GetRoute()))
	b.WriteString(fmt.Sprintf("- CRAG：%s\n", resp.GetCragAction()))
	if check := resp.GetSelfCheck(); check != nil {
		b.WriteString(fmt.Sprintf("- Self-RAG：Retrieve=%t, IsRel=%t, IsSup=%t, IsUse=%t\n", check.GetRetrieve(), check.GetIsRel(), check.GetIsSup(), check.GetIsUse()))
	}
	b.WriteString("\n## 答案草稿\n")
	b.WriteString(resp.GetAnswer())
	b.WriteString("\n\n## 来源\n")
	if len(resp.GetSources()) == 0 {
		b.WriteString("- 未命中内部来源。\n")
	} else {
		for i, src := range resp.GetSources() {
			if i >= limit {
				break
			}
			b.WriteString(fmt.Sprintf("- %s (score %.3f)：%s\n", src.GetTitle(), src.GetScore(), truncateForTool(src.GetContent(), 220)))
		}
	}
	if len(resp.GetGraphNodes()) > 0 {
		b.WriteString("\n## 相关图谱实体\n")
		for i, node := range resp.GetGraphNodes() {
			if i >= 8 {
				break
			}
			b.WriteString(fmt.Sprintf("- %s：%s\n", node.GetName(), truncateForTool(node.GetSummary(), 120)))
		}
	}
	return b.String(), nil
}

func truncateForTool(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
