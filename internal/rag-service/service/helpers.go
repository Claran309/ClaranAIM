package service

import (
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

func cleanRAGContextText(text string) string {
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">")
	text = replacer.Replace(text)
	text = regexp.MustCompile(`(?is)<script.*?</script>|<style.*?</style>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)</?(table|thead|tbody|tr|td|th|p|div|span|strong|em|ul|ol|li|h[1-6])[^>]*>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func escapePlainForMarkdown(text string) string {
	return strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func extractEntities(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range wordRe.FindAllString(text, -1) {
		if len([]rune(token)) < 2 {
			continue
		}
		first := []rune(token)[0]
		if unicode.IsUpper(first) || unicode.Is(unicode.Han, first) {
			if !seen[token] {
				seen[token] = true
				out = append(out, token)
			}
		}
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func inferEntityType(name string) string {
	return inferEntityTypeFromContext(name, "")
}

func inferEntityTypeFromContext(name, contextText string) string {
	lower := strings.ToLower(name)
	contextLower := strings.ToLower(contextText)
	joined := lower + " " + contextLower
	if strings.Contains(lower, "-service") || strings.Contains(lower, "-gateway") || strings.Contains(name, "服务") || strings.Contains(name, "网关") {
		return "服务"
	}
	if strings.Contains(lower, "_") || strings.HasSuffix(lower, "table") || strings.Contains(name, "表") || strings.Contains(lower, "outbox") {
		return "数据库表"
	}
	if strings.Contains(lower, ".events") || strings.Contains(lower, "topic") || strings.Contains(name, "事件") || strings.Contains(name, "主题") {
		return "事件主题"
	}
	if strings.Contains(lower, "/api/") || strings.HasPrefix(lower, "api") {
		return "接口"
	}
	if strings.Contains(lower, "module") || strings.Contains(name, "模块") {
		return "模块"
	}
	if strings.Contains(name, "部") || strings.Contains(name, "组") || strings.Contains(name, "团队") {
		return "组织"
	}
	if strings.Contains(name, "系统") || strings.Contains(name, "平台") || strings.Contains(name, "产品") {
		return "产品"
	}
	if strings.Contains(joined, "数据表") || strings.Contains(joined, "表") || strings.Contains(joined, "table") || strings.Contains(joined, "outbox") {
		return "数据库表"
	}
	if strings.Contains(joined, "事件主题") || strings.Contains(joined, "topic") || strings.Contains(joined, "event topic") {
		return "事件主题"
	}
	if strings.Contains(joined, "服务") || strings.Contains(joined, "service") || strings.Contains(joined, "gateway") {
		return "服务"
	}
	if strings.Contains(joined, "接口") || strings.Contains(joined, "api") {
		return "接口"
	}
	if strings.Contains(joined, "模块") || strings.Contains(joined, "module") {
		return "模块"
	}
	if strings.Contains(joined, "组织") || strings.Contains(joined, "公司") || strings.Contains(joined, "机构") {
		return "组织"
	}
	if strings.Contains(joined, "产品") || strings.Contains(joined, "数据库产品") {
		return "产品"
	}
	return "主题"
}

func communityName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "agent") || strings.Contains(name, "助手") || strings.Contains(name, "调度"):
		return "Agent 事件链路"
	case strings.Contains(lower, "message") || strings.Contains(lower, "msg") || strings.Contains(lower, "conversation") || strings.Contains(name, "消息") || strings.Contains(name, "会话"):
		return "IM 消息链路"
	case strings.Contains(lower, "rag") || strings.Contains(lower, "memory") || strings.Contains(lower, "milvus") || strings.Contains(name, "记忆") || strings.Contains(name, "知识"):
		return "Memory/RAG"
	case strings.Contains(lower, "file") || strings.Contains(lower, "minio") || strings.Contains(lower, "ocr") || strings.Contains(name, "文件"):
		return "文件服务"
	default:
		return "通用知识图谱"
	}
}

func firstLine(text, fallback string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncate(line, 80)
		}
	}
	return fallback
}

func truncate(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func documentToRPC(doc *model.Document) rag.RAGDocument {
	return rag.RAGDocument{Id: doc.ID, OwnerId: doc.OwnerID, Title: doc.Title, Source: doc.Source, SourceType: doc.SourceType, Visibility: doc.Visibility, GroupId: doc.GroupID, ConversationId: doc.ConversationID, Status: doc.Status, CreatedAt: doc.CreatedAt.Format(time.RFC3339), UpdatedAt: doc.UpdatedAt.Format(time.RFC3339)}
}

func entitiesToRPC(entities []model.Entity) []*rag.RAGGraphNode {
	out := make([]*rag.RAGGraphNode, 0, len(entities))
	for _, e := range entities {
		out = append(out, &rag.RAGGraphNode{Id: e.ID, Name: e.Name, Type: e.Type, Summary: e.Summary, CommunityId: e.CommunityID, Score: e.Score})
	}
	return out
}

func relationsToRPC(relations []model.Relation) []*rag.RAGGraphEdge {
	out := make([]*rag.RAGGraphEdge, 0, len(relations))
	for _, r := range relations {
		out = append(out, &rag.RAGGraphEdge{Id: r.ID, SourceId: r.SourceID, TargetId: r.TargetID, Relation: r.Relation, Weight: r.Weight, Evidence: r.Evidence, DocumentId: r.DocumentID})
	}
	return out
}

func filterGraphForDisplay(entities []model.Entity, relations []model.Relation) ([]model.Entity, []model.Relation) {
	validID := map[int64]bool{}
	filteredEntities := make([]model.Entity, 0, len(entities))
	for _, entity := range entities {
		extracted := extractedEntity{Name: entity.Name, Type: entity.Type, Description: entity.Summary, Aliases: decodeAliases(entity.AliasesJSON)}
		if !isUsefulGraphEntity(extracted, entity.Summary, true) {
			continue
		}
		entity.Type = normalizeEntityType(entity.Type, entity.Name)
		filteredEntities = append(filteredEntities, entity)
		validID[entity.ID] = true
	}
	filteredRelations := make([]model.Relation, 0, len(relations))
	seen := map[string]bool{}
	for _, relation := range relations {
		if !validID[relation.SourceID] || !validID[relation.TargetID] || relation.SourceID == relation.TargetID {
			continue
		}
		relation.Relation = normalizeRelationType(relation.Relation)
		if relation.Relation == "" {
			continue
		}
		if isExplicitBadGraphRelationText(relation.Description + " " + relation.Evidence) {
			continue
		}
		key := fmt.Sprintf("%d-%s-%d-%d-%s", relation.SourceID, relation.Relation, relation.TargetID, relation.DocumentID, canonicalEntityKey(relation.Evidence))
		if seen[key] {
			continue
		}
		seen[key] = true
		filteredRelations = append(filteredRelations, relation)
	}
	return filteredEntities, filteredRelations
}

func ensureDisplayEntityRelations(entities []model.Entity, relations []model.Relation, documentID int64) []model.Relation {
	_ = entities
	_ = documentID
	return relations
}

func displayRelationKey(sourceID, targetID int64, relation string) string {
	return fmt.Sprintf("%d:%s:%d", sourceID, normalizeRelationType(relation), targetID)
}

func isKnowledgeDocumentRelation(relation string) bool {
	relation = normalizeRelationType(relation)
	return relation == "CONTAINS" || relation == "包含"
}

func isGraphDocumentEntity(entity model.Entity) bool {
	return normalizeEntityType(entity.Type, entity.Name) == "文档" || strings.HasPrefix(entity.CanonicalKey, "document:")
}

func firstConnectedDisplayEntity(entities []model.Entity, degree map[int64]int) model.Entity {
	for _, entity := range entities {
		if degree[entity.ID] > 0 {
			return entity
		}
	}
	return entities[0]
}

func nearestDisplayEntity(entities []model.Entity, index int, anchor model.Entity, degree map[int64]int) model.Entity {
	for left, right := index-1, index+1; left >= 0 || right < len(entities); left, right = left-1, right+1 {
		if left >= 0 && entities[left].ID != entities[index].ID && degree[entities[left].ID] > 0 {
			return entities[left]
		}
		if right < len(entities) && entities[right].ID != entities[index].ID && degree[entities[right].ID] > 0 {
			return entities[right]
		}
	}
	if anchor.ID != 0 && anchor.ID != entities[index].ID {
		return anchor
	}
	if index > 0 {
		return entities[index-1]
	}
	if len(entities) > 1 {
		return entities[1]
	}
	return model.Entity{}
}

func communitiesToRPC(communities []model.Community) []*rag.RAGGraphCommunity {
	out := make([]*rag.RAGGraphCommunity, 0, len(communities))
	for _, c := range communities {
		out = append(out, &rag.RAGGraphCommunity{Id: c.ID, Name: c.Name, Summary: c.Summary, Level: c.Level})
	}
	return out
}
