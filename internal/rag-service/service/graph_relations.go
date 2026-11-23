package service

import (
	"ClaranAIM/internal/rag-service/model"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func splitGraphSentences(text string) []string {
	fields := regexp.MustCompile(`[。！？!?；;\n]+`).Split(text, -1)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func entityNamesInText(sentence string, entities []extractedEntity) []string {
	mentions := entityMentionsInText(sentence, entities)
	names := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		names = append(names, mention.Name)
	}
	return names
}

func entityMentionsInText(sentence string, entities []extractedEntity) []graphEntityMention {
	lowerSentence := strings.ToLower(sentence)
	var mentions []graphEntityMention
	seen := map[string]bool{}
	for _, entity := range entities {
		candidates := append([]string{entity.Name}, entity.Aliases...)
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			needle := strings.ToLower(candidate)
			searchFrom := 0
			for {
				index := strings.Index(lowerSentence[searchFrom:], needle)
				if index < 0 {
					break
				}
				start := searchFrom + index
				key := fmt.Sprintf("%s:%d:%d", canonicalEntityKey(entity.Name), start, start+len(candidate))
				if !seen[key] {
					seen[key] = true
					mentions = append(mentions, graphEntityMention{Name: entity.Name, Start: start, End: start + len(candidate)})
				}
				searchFrom = start + len(needle)
			}
		}
	}
	sort.SliceStable(mentions, func(i, j int) bool {
		return mentions[i].Start < mentions[j].Start
	})
	return mentions
}

func relationTypeFromSentence(sentence string) string {
	lower := strings.ToLower(sentence)
	switch {
	case strings.Contains(sentence, "写入") || strings.Contains(sentence, "写") || strings.Contains(lower, "write"):
		return "WRITES"
	case strings.Contains(sentence, "读取") || strings.Contains(lower, "read"):
		return "READS"
	case strings.Contains(sentence, "消费") || strings.Contains(lower, "consume"):
		return "CONSUMES"
	case strings.Contains(sentence, "发布") || strings.Contains(lower, "publish"):
		return "PUBLISHES"
	case strings.Contains(sentence, "调用") || strings.Contains(lower, "call"):
		return "CALLS"
	case strings.Contains(sentence, "存储") || strings.Contains(sentence, "保存") || strings.Contains(lower, "store"):
		return "STORES"
	case strings.Contains(sentence, "依赖") || strings.Contains(lower, "depend"):
		return "DEPENDS_ON"
	case strings.Contains(sentence, "配置") || strings.Contains(lower, "config"):
		return "CONFIGURES"
	case strings.Contains(sentence, "触发") || strings.Contains(lower, "trigger"):
		return "TRIGGERS"
	case strings.Contains(sentence, "拥有") || strings.Contains(sentence, "负责") || strings.Contains(lower, "own"):
		return "OWNS"
	default:
		return ""
	}
}

func relationTriggersInSentence(sentence string) []graphRelationTrigger {
	lower := strings.ToLower(sentence)
	relationTypes := []string{"WRITES", "READS", "CONSUMES", "PUBLISHES", "CALLS", "STORES", "DEPENDS_ON", "CONFIGURES", "TRIGGERS", "OWNS", "包含", "使用", "生成", "实现", "支撑", "约束", "属于", "面向", "展示", "校验", "筛选", "检索", "召回"}
	triggers := make([]graphRelationTrigger, 0, 2)
	for _, relationType := range relationTypes {
		for _, term := range relationTriggerTerms(relationType) {
			needle := strings.ToLower(term)
			searchFrom := 0
			for {
				idx := strings.Index(lower[searchFrom:], needle)
				if idx < 0 {
					break
				}
				absolute := searchFrom + idx
				if isASCIIWord(needle) && !isRelationTermBoundary(lower, absolute, absolute+len(needle)) {
					searchFrom = absolute + len(needle)
					continue
				}
				triggers = append(triggers, graphRelationTrigger{Type: relationType, Index: absolute, End: absolute + len(needle)})
				searchFrom = absolute + len(needle)
			}
		}
	}
	sort.SliceStable(triggers, func(i, j int) bool {
		if triggers[i].Index == triggers[j].Index {
			return triggers[i].End > triggers[j].End
		}
		return triggers[i].Index < triggers[j].Index
	})
	return dedupeRelationTriggers(triggers)
}

func dedupeRelationTriggers(triggers []graphRelationTrigger) []graphRelationTrigger {
	out := make([]graphRelationTrigger, 0, len(triggers))
	for _, trigger := range triggers {
		if len(out) > 0 {
			prev := out[len(out)-1]
			if trigger.Index >= prev.Index && trigger.End <= prev.End {
				continue
			}
			if trigger.Index == prev.Index && trigger.End > prev.End {
				out[len(out)-1] = trigger
				continue
			}
		}
		out = append(out, trigger)
	}
	return out
}

func isASCIIWord(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func isRelationTermBoundary(text string, start, end int) bool {
	if start > 0 {
		prev := rune(text[start-1])
		if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') || prev == '_' {
			return false
		}
	}
	if end < len(text) {
		next := rune(text[end])
		if (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') || next == '_' {
			return false
		}
	}
	return true
}

func relationDirectionAt(sentence string, mentions []graphEntityMention, trigger graphRelationTrigger, defaultSubject string) (string, string) {
	if len(mentions) < 2 {
		return "", ""
	}
	before := -1
	after := -1
	for i, mention := range mentions {
		if mention.End <= trigger.Index {
			before = i
		}
		if mention.Start >= trigger.Index && after == -1 {
			after = i
		}
	}
	if before < 0 || after < 0 || before == after {
		return "", ""
	}
	if isPassiveRelation(sentence, mentions[before], trigger) {
		return mentions[after].Name, mentions[before].Name
	}
	if shouldUseDefaultSubject(sentence, mentions[before], trigger, defaultSubject) {
		return defaultSubject, mentions[after].Name
	}
	return mentions[before].Name, mentions[after].Name
}

func relationPairsAt(sentence string, mentions []graphEntityMention, trigger graphRelationTrigger, defaultSubject string, segmentEnd int) [][2]string {
	source, target := relationDirectionAt(sentence, mentions, trigger, defaultSubject)
	if source == "" || target == "" {
		return nil
	}
	if segmentEnd <= trigger.End || segmentEnd > len(sentence) {
		segmentEnd = len(sentence)
	}
	pairs := [][2]string{{source, target}}
	sourceKey := canonicalEntityKey(source)
	for _, mention := range mentions {
		if mention.Start < trigger.End || mention.Start >= segmentEnd || canonicalEntityKey(mention.Name) == sourceKey {
			continue
		}
		pairs = append(pairs, [2]string{source, mention.Name})
	}
	return uniqueRelationPairs(pairs)
}

func uniqueRelationPairs(pairs [][2]string) [][2]string {
	seen := map[string]bool{}
	out := make([][2]string, 0, len(pairs))
	for _, pair := range pairs {
		sourceKey := canonicalEntityKey(pair[0])
		targetKey := canonicalEntityKey(pair[1])
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey {
			continue
		}
		key := sourceKey + "->" + targetKey
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, pair)
	}
	return out
}

func isPassiveRelation(sentence string, target graphEntityMention, trigger graphRelationTrigger) bool {
	if target.End < 0 || trigger.Index <= target.End || trigger.Index > len(sentence) {
		return false
	}
	between := sentence[target.End:trigger.Index]
	return strings.Contains(between, "被") || strings.Contains(strings.ToLower(between), " by ")
}

func shouldUseDefaultSubject(sentence string, before graphEntityMention, trigger graphRelationTrigger, defaultSubject string) bool {
	if defaultSubject == "" || canonicalEntityKey(before.Name) == canonicalEntityKey(defaultSubject) {
		return false
	}
	if before.End < 0 || trigger.Index <= before.End || trigger.Index > len(sentence) {
		return false
	}
	between := strings.TrimSpace(sentence[before.End:trigger.Index])
	if between == "" {
		return false
	}
	return regexp.MustCompile(`^[，,、\s]*(并|并且|然后|同时)?[，,、\s]*$`).MatchString(between)
}

func relationDirection(sentence string, mentions []graphEntityMention, relationType string) (string, string) {
	if len(mentions) < 2 {
		return "", ""
	}
	verbIndex := relationVerbIndex(sentence, relationType)
	if verbIndex >= 0 {
		before := -1
		after := -1
		for i, mention := range mentions {
			if mention.End <= verbIndex {
				before = i
			}
			if mention.Start >= verbIndex && after == -1 {
				after = i
			}
		}
		if before >= 0 && after >= 0 && before != after {
			return mentions[before].Name, mentions[after].Name
		}
	}
	return mentions[0].Name, mentions[1].Name
}

func relationVerbIndex(sentence, relationType string) int {
	terms := relationTriggerTerms(relationType)
	lower := strings.ToLower(sentence)
	best := -1
	for _, term := range terms {
		idx := strings.Index(lower, strings.ToLower(term))
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	return best
}

func relationTriggerTerms(relationType string) []string {
	switch relationType {
	case "WRITES":
		return []string{"写入", "写", "write", "writes"}
	case "READS":
		return []string{"读取", "read", "reads"}
	case "CONSUMES":
		return []string{"消费", "consume", "consumes"}
	case "PUBLISHES":
		return []string{"发布", "publish", "publishes"}
	case "CALLS":
		return []string{"调用", "call", "calls"}
	case "STORES":
		return []string{"存储", "保存", "store", "stores"}
	case "DEPENDS_ON":
		return []string{"依赖", "depend", "depends"}
	case "CONFIGURES":
		return []string{"配置", "config", "configures"}
	case "TRIGGERS":
		return []string{"触发", "trigger", "triggers"}
	case "OWNS":
		return []string{"拥有", "负责", "own", "owns"}
	case "包含":
		return []string{"包含", "包括", "由", "组成", "contains", "includes"}
	case "使用":
		return []string{"使用", "采用", "基于", "use", "uses"}
	case "生成":
		return []string{"生成", "产出", "输出", "produce", "produces", "generate", "generates"}
	case "实现":
		return []string{"实现", "落地", "implement", "implements"}
	case "支撑":
		return []string{"支撑", "支持", "服务于", "support", "supports"}
	case "约束":
		return []string{"约束", "限制", "要求", "constraint", "constrains", "requires"}
	case "属于":
		return []string{"属于", "归属", "隶属", "belongs to"}
	case "面向":
		return []string{"面向", "适用于", "针对", "targets", "serves"}
	case "展示":
		return []string{"展示", "显示", "呈现", "show", "shows", "display", "displays"}
	case "校验":
		return []string{"校验", "验证", "检查", "validate", "validates", "check", "checks"}
	case "筛选":
		return []string{"筛选", "过滤", "filter", "filters"}
	case "检索":
		return []string{"检索", "搜索", "查询", "retrieve", "retrieves", "search", "searches", "query", "queries"}
	case "召回":
		return []string{"召回", "匹配", "recall", "recalls", "match", "matches"}
	default:
		return nil
	}
}

func relationDescription(source, target, relationType string) string {
	verbs := map[string]string{
		"CALLS":      "调用",
		"PUBLISHES":  "发布到",
		"CONSUMES":   "消费",
		"STORES":     "存储到",
		"OWNS":       "负责",
		"DEPENDS_ON": "依赖",
		"CONFIGURES": "配置",
		"TRIGGERS":   "触发",
		"READS":      "读取",
		"WRITES":     "写入",
		"CONTAINS":   "包含",
		"RELATED_TO": "关联",
		"包含":         "包含",
		"使用":         "使用",
		"生成":         "生成",
		"实现":         "实现",
		"支撑":         "支撑",
		"约束":         "约束",
		"属于":         "属于",
		"面向":         "面向",
		"展示":         "展示",
		"校验":         "校验",
		"筛选":         "筛选",
		"检索":         "检索",
		"召回":         "召回",
	}
	verb := defaultString(verbs[relationType], relationType)
	return source + " " + verb + " " + target
}

func relationAllowedByEntityTypes(relationType, sourceType, targetType string) bool {
	relationType = cleanGraphLabel(relationType)
	if isCustomGraphRelation(relationType) {
		return true
	}
	sourceType = normalizeGraphEntityTypeForPolicy(sourceType)
	targetType = normalizeGraphEntityTypeForPolicy(targetType)
	if sourceType == "" || targetType == "" {
		return true
	}
	switch relationType {
	case "CONTAINS":
		return true
	case "CALLS":
		return isExecutableGraphEntityType(sourceType) && isCallableGraphEntityType(targetType)
	case "PUBLISHES":
		return isExecutableGraphEntityType(sourceType) && targetType == "EventTopic"
	case "CONSUMES":
		return isExecutableGraphEntityType(sourceType) && targetType == "EventTopic"
	case "WRITES", "READS", "STORES":
		return isExecutableGraphEntityType(sourceType) && isStorageGraphEntityType(targetType)
	case "CONFIGURES":
		return isExecutableGraphEntityType(sourceType) && targetType != "Person"
	case "TRIGGERS":
		return sourceType != "DatabaseTable" && targetType != "DatabaseTable"
	case "DEPENDS_ON":
		return sourceType != "DatabaseTable" || targetType != "Person"
	case "OWNS":
		return sourceType != "DatabaseTable" && targetType != "EventTopic"
	case "RELATED_TO":
		return true
	default:
		return true
	}
}

func normalizeGraphEntityTypeForPolicy(entityType string) string {
	switch strings.TrimSpace(entityType) {
	case "Service", "DatabaseTable", "EventTopic", "API", "Module", "Concept", "Person", "Organization", "Product":
		return strings.TrimSpace(entityType)
	default:
		return ""
	}
}

func isExecutableGraphEntityType(entityType string) bool {
	switch entityType {
	case "Service", "API", "Module", "Product":
		return true
	default:
		return false
	}
}

func isCallableGraphEntityType(entityType string) bool {
	switch entityType {
	case "Service", "API", "Module":
		return true
	default:
		return false
	}
}

func isStorageGraphEntityType(entityType string) bool {
	switch entityType {
	case "DatabaseTable", "Concept", "Product":
		return true
	default:
		return false
	}
}

func canonicalEntityKey(name string) string {
	alias := strings.TrimSpace(strings.ToLower(name))
	switch alias {
	case "消息核心服务", "messageservice", "message service", "msg core service":
		return "msgcoreservice"
	}
	var b strings.Builder
	for _, r := range alias {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeEntityType(entityType, name string) string {
	return normalizeEntityTypeWithContext(entityType, name, "")
}

func normalizeEntityTypeWithContext(entityType, name, contextText string) string {
	entityType = cleanGraphLabel(entityType)
	if entityType != "" {
		return entityType
	}
	return inferEntityTypeFromContext(name, contextText)
}

func normalizeRelationType(relationType string) string {
	raw := cleanGraphLabel(relationType)
	if raw == "" {
		return ""
	}
	return raw
}

func logGraphStage(stage string, ownerID, documentID int64, fields ...interface{}) {
	parts := []string{
		fmt.Sprintf("stage=%s", stage),
		fmt.Sprintf("owner_id=%d", ownerID),
		fmt.Sprintf("document_id=%d", documentID),
	}
	for i := 0; i+1 < len(fields); i += 2 {
		key := strings.TrimSpace(fmt.Sprint(fields[i]))
		if key == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, fields[i+1]))
	}
	log.Printf("GraphRAG抽取诊断: %s", strings.Join(parts, " "))
}

func cleanGraphLabel(value string) string {
	cleaned := strings.Trim(value, " \t\r\n，。,.；;:：()（）[]【】\"'")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned
}

func isGenericEntityType(entityType string) bool {
	raw := strings.TrimSpace(entityType)
	if raw == "" {
		return true
	}
	lower := strings.ToLower(raw)
	generic := []string{"entity", "node", "object", "content", "data", "information", "unknown", "实体", "节点", "对象", "内容", "数据", "信息", "未知"}
	for _, item := range generic {
		if lower == strings.ToLower(item) {
			return true
		}
	}
	if raw == "Concept" || raw == "概念" {
		return true
	}
	return false
}

func isCustomGraphRelation(relationType string) bool {
	if relationType == "" {
		return false
	}
	upper := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(relationType, "-", "_"), " ", "_"))
	switch upper {
	case "CALLS", "PUBLISHES", "CONSUMES", "STORES", "OWNS", "DEPENDS_ON", "CONFIGURES", "TRIGGERS", "READS", "WRITES", "RELATED_TO", "CONTAINS":
		return false
	default:
		return true
	}
}

func isGenericRelatedRelation(relationType string) bool {
	raw := strings.TrimSpace(relationType)
	upper := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(raw, "-", "_"), " ", "_"))
	switch upper {
	case "", "RELATED_TO", "RELATES_TO", "RELATED", "ASSOCIATED_WITH", "ASSOCIATES_WITH":
		return true
	}
	lower := strings.ToLower(raw)
	generic := []string{"相关", "关联", "有关", "提到", "提及", "同现", "一起出现", "co-occur", "mentioned"}
	for _, item := range generic {
		if lower == strings.ToLower(item) || strings.Contains(lower, strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func isWeakHomogeneousGraphRelation(relationType string) bool {
	raw := strings.TrimSpace(relationType)
	if raw == "" {
		return true
	}
	lower := strings.ToLower(raw)
	weak := []string{
		"共同说明", "共同支撑", "共同描述", "共同体现", "共同关联", "共同指向",
		"同章共现", "同章共同说明", "同章说明", "同章上下文", "同属", "并列", "补充说明", "语义相关", "上下文相关",
		"co-describes", "co_supports", "co-supports",
	}
	for _, item := range weak {
		if lower == strings.ToLower(item) || strings.Contains(lower, strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func isWeakGraphRelationType(relationType string) bool {
	return isGenericRelatedRelation(relationType) || isWeakHomogeneousGraphRelation(relationType)
}

func encodeAliases(aliases []string) string {
	aliases = mergeAliases(nil, aliases...)
	data, err := json.Marshal(aliases)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeAliases(raw string) []string {
	var aliases []string
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		return nil
	}
	return aliases
}

func mergeAliases(base []string, values ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func communityForEntity(ownerID int64, entity model.Entity) *model.Community {
	name := communityName(entity.Name)
	aliases := decodeAliases(entity.AliasesJSON)
	return &model.Community{
		OwnerID:         ownerID,
		Name:            name,
		Summary:         fmt.Sprintf("%s 社区包含 %s 等实体，用于 GraphRAG 查询时提供领域摘要和子图入口。", name, defaultString(entity.Name, "相关")),
		KeyEntitiesJSON: encodeAliases(append(aliases, entity.Name)),
		Level:           1,
	}
}
