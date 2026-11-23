package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

func (e fallbackGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	if e.primary != nil {
		result, err := e.primary.Extract(ctx, input)
		if err == nil && len(result.Entities) > 0 {
			return mergeGraphExtractFallback(ctx, e.fallback, input, result), nil
		}
	}
	if e.fallback == nil {
		return graphExtractResult{}, nil
	}
	return e.fallback.Extract(ctx, input)
}

func mergeGraphExtractFallback(ctx context.Context, fallback graphExtractor, input graphExtractInput, primary graphExtractResult) graphExtractResult {
	if fallback == nil {
		return primary
	}
	fallbackResult, err := fallback.Extract(ctx, input)
	if err != nil || (len(fallbackResult.Entities) == 0 && len(fallbackResult.Relationships) == 0) {
		return primary
	}
	merged := graphExtractResult{
		Entities:      make([]extractedEntity, 0, len(primary.Entities)+len(fallbackResult.Entities)),
		Relationships: make([]extractedRelationship, 0, len(primary.Relationships)+len(fallbackResult.Relationships)),
	}
	entityByKey := map[string]bool{}
	for _, entity := range primary.Entities {
		key := canonicalEntityKey(entity.Name)
		if key == "" || entityByKey[key] {
			continue
		}
		entityByKey[key] = true
		merged.Entities = append(merged.Entities, entity)
	}
	for _, entity := range fallbackResult.Entities {
		key := canonicalEntityKey(entity.Name)
		if key == "" || entityByKey[key] {
			continue
		}
		entityByKey[key] = true
		merged.Entities = append(merged.Entities, entity)
	}
	relationSeen := map[string]bool{}
	for _, relation := range append(primary.Relationships, fallbackResult.Relationships...) {
		relationType := normalizeRelationType(relation.Type)
		if relationType == "" {
			continue
		}
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey || !entityByKey[sourceKey] || !entityByKey[targetKey] {
			continue
		}
		key := sourceKey + "->" + targetKey + ":" + relationType + ":" + canonicalEntityKey(relation.Evidence)
		if relationSeen[key] {
			continue
		}
		relationSeen[key] = true
		relation.Type = relationType
		merged.Relationships = append(merged.Relationships, relation)
	}
	return merged
}

func mergeGraphExtractResults(primary, supplement graphExtractResult) graphExtractResult {
	if len(supplement.Entities) == 0 && len(supplement.Relationships) == 0 {
		return primary
	}
	merged := graphExtractResult{
		Entities:      make([]extractedEntity, 0, len(primary.Entities)+len(supplement.Entities)),
		Relationships: make([]extractedRelationship, 0, len(primary.Relationships)+len(supplement.Relationships)),
	}
	entityByKey := map[string]bool{}
	for _, entity := range append(primary.Entities, supplement.Entities...) {
		key := canonicalEntityKey(entity.Name)
		if key == "" || entityByKey[key] {
			continue
		}
		entityByKey[key] = true
		merged.Entities = append(merged.Entities, entity)
	}
	supplementConcretePairs := map[string]bool{}
	for _, relation := range supplement.Relationships {
		relationType := cleanGraphLabel(relation.Type)
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey || isWeakGraphRelationType(relationType) {
			continue
		}
		supplementConcretePairs[sourceKey+"->"+targetKey] = true
		supplementConcretePairs[targetKey+"->"+sourceKey] = true
	}
	relationSeen := map[string]bool{}
	for index, relation := range append(primary.Relationships, supplement.Relationships...) {
		relationType := cleanGraphLabel(relation.Type)
		if relationType == "" {
			continue
		}
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey || !entityByKey[sourceKey] || !entityByKey[targetKey] {
			continue
		}
		fromPrimary := index < len(primary.Relationships)
		if fromPrimary && isWeakGraphRelationType(relationType) && supplementConcretePairs[sourceKey+"->"+targetKey] {
			continue
		}
		key := sourceKey + "->" + targetKey + ":" + relationType + ":" + canonicalEntityKey(relation.Evidence)
		if relationSeen[key] {
			continue
		}
		relationSeen[key] = true
		relation.Type = relationType
		merged.Relationships = append(merged.Relationships, relation)
	}
	return merged
}

// LLMGraphExtractor 使用 OpenAI-compatible 小模型抽取 GraphRAG 实体和关系。
type LLMGraphExtractor struct {
	APIKey   string
	BaseURL  string
	Model    string
	Client   *http.Client
	MaxChars int
}

const llmGraphExtractorSystemPrompt = "你是 ClaranAIM 的 GraphRAG indexing analyst。只输出 JSON，不要解释。你的任务是自由、充分地抽取一篇文档章节/父块里的实体和有证据关系，让知识图谱尽量完整。\n\n分析步骤必须在心里完成，不要输出步骤：\n1. 先理解本章节的主题、作者真正讨论的对象，以及围绕主题出现的所有有意义概念、对象、能力、规则、流程、策略、页面、配置、模型、字段、指标、角色、接口、模块、服务、数据表、事件、工具、产品、状态、输入输出和结果。\n2. 不限制实体数量，不限制关系数量；只要原文中存在清晰依据并对理解章节有帮助，就可以抽取。\n3. 实体 type 由你自由命名，不需要使用固定枚举，也不需要套用示例类型；可以使用中文短语、业务术语、技术术语或原文措辞。\n4. 关系 type 由你自由命名，不需要使用固定枚举，也不需要转换成标准模板；优先使用原文语义短语或你对原文关系的准确概括。\n5. 尽量覆盖不同维度的关系：动作流转、所有权、组成、约束、依赖、目的、证据、时序、适用范围、角色职责、输入输出、使用工具、生成结果、展示承载、条件、属性、状态变化。\n6. 只是同段出现、举例、列表项、标题编号、页面元素，不算强关系；但如果原文说明了用途、组成、约束、展示、输入输出、适用范围、属性或状态变化，就应抽取为关系。\n7. 章节如果只是寒暄、会话摘要、目录、题号、步骤流水、截图 OCR 噪声或无长期价值内容，可以返回空数组。\n\n硬性规则：\n- 不要为了迎合固定类型而改写实体 type 或关系 type；实体类型和关系类型都由你自由发挥。\n- source/target 必须引用 entities 中的 name；关系必须有原文证据或可追溯描述；完全没有证据的关系不要输出。\n- 只过滤明显噪声：空值、纯数字、页码、孤立标点、无法指代任何对象的状态词、无意义图片名、临时变量名、普通教学占位人名；如果字段名、文件名、页面名、配置名、状态名承载业务/技术含义，可以作为实体保留。\n- 实体 description 应说明它在本文中的业务/技术含义。\n\nJSON格式：{\"entities\":[{\"name\":\"...\",\"type\":\"由你自由命名\",\"description\":\"它在本文中的具体含义\",\"aliases\":[]}],\"relationships\":[{\"source\":\"...\",\"target\":\"...\",\"type\":\"由你自由命名\",\"description\":\"说明关系含义\",\"evidence\":\"原文证据或可追溯依据\",\"confidence\":0.85}]}。"

func llmGraphExtractorPrompt(mode string) string {
	_ = mode
	return llmGraphExtractorSystemPrompt + "\n\n当前图谱关系模式：LLM 自由抽取，关系优先。不要限制实体数量、关系数量、实体类型或关系类型；不要把自由类型改成固定枚举；不要因为关系词不在枚举里就改成 RELATED_TO/相关。请特别注意：每抽出一个实体后，都要主动寻找它与其他实体之间的动作、归属、组成、约束、影响、输入输出、展示、条件、时序或属性关系；不要只输出实体列表。"
}

func llmGraphRelationSupplementPrompt(mode string, entities []extractedEntity) string {
	_ = mode
	names := make([]string, 0, len(entities))
	for _, entity := range entities {
		name := strings.TrimSpace(entity.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return "你是 ClaranAIM 的 GraphRAG 关系补抽 analyst。只输出 JSON，不要解释。\n\n上一轮已经抽到了实体，但关系数量太少。现在请只围绕这些已知实体补抽尽可能多的有证据关系，不限制关系数量，不限制关系 type，关系 type 由你自由命名，尽量使用原文语义短语。\n\n已知实体：" + strings.Join(names, "、") + "\n\n要求：\n- entities 数组必须返回上述已知实体，name 必须一致；type/description 可以沿用或简写。\n- relationships 中的 source/target 必须引用上述实体 name。\n- 不要只输出 RELATED_TO/相关；优先输出具体关系，如影响、约束、包含、支撑、生成、展示、输入、输出、面向、依赖、配置、触发、属于、组成、决定、用于、说明、承载。\n- 可以从同一句、同一段、标题与正文之间、列表项之间抽取关系，只要能从原文追溯。\n- 如果某些实体确实只有上下文联系，也可以输出你认为准确的自由关系类型，例如“同章共同说明某主题”，但不要留空。\n\nJSON格式：{\"entities\":[{\"name\":\"...\",\"type\":\"由你自由命名\",\"description\":\"...\",\"aliases\":[]}],\"relationships\":[{\"source\":\"...\",\"target\":\"...\",\"type\":\"由你自由命名\",\"description\":\"...\",\"evidence\":\"原文证据或可追溯依据\",\"confidence\":0.85}]}。"
}

func NewLLMGraphExtractor(apiKey, baseURL, model string) *LLMGraphExtractor {
	return &LLMGraphExtractor{
		APIKey:   strings.TrimSpace(apiKey),
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:    defaultString(model, "glm-4-flash"),
		Client:   &http.Client{Timeout: 20 * time.Second},
		MaxChars: 4000,
	}
}

func (e *LLMGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	if e == nil || e.APIKey == "" || e.BaseURL == "" {
		return graphExtractResult{}, errors.New("llm graph extractor未配置")
	}
	content := truncate(input.Chunk.Content, e.MaxChars)
	result, err := e.extractWithPrompt(ctx, llmGraphExtractorPrompt(input.GraphRelationMode), content)
	if err != nil {
		return graphExtractResult{}, err
	}
	if shouldSupplementLLMRelations(result) {
		supplement, err := e.extractWithPrompt(ctx, llmGraphRelationSupplementPrompt(input.GraphRelationMode, result.Entities), content)
		if err == nil {
			result = mergeGraphExtractResults(result, supplement)
		}
	}
	return result, nil
}

func (e *LLMGraphExtractor) extractWithPrompt(ctx context.Context, systemPrompt, content string) (graphExtractResult, error) {
	payload := map[string]interface{}{
		"model": e.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":    "user",
				"content": content,
			},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return graphExtractResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return graphExtractResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return graphExtractResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return graphExtractResult{}, fmt.Errorf("llm graph extractor状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return graphExtractResult{}, err
	}
	if len(decoded.Choices) == 0 {
		return graphExtractResult{}, errors.New("llm graph extractor未返回结果")
	}
	return parseGraphExtractResult(decoded.Choices[0].Message.Content)
}

func shouldSupplementLLMRelations(result graphExtractResult) bool {
	if len(result.Entities) < 2 {
		return false
	}
	if hasWeakLLMRelations(result.Relationships) {
		return true
	}
	minRelations := len(result.Entities) - 1
	if minRelations > 4 {
		minRelations = 4
	}
	if minRelations < 1 {
		minRelations = 1
	}
	return len(result.Relationships) < minRelations
}

func hasWeakLLMRelations(relations []extractedRelationship) bool {
	for _, relation := range relations {
		if isWeakGraphRelationType(relation.Type) {
			return true
		}
	}
	return false
}

func parseGraphExtractResult(content string) (graphExtractResult, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		Entities []struct {
			Name        string   `json:"name"`
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Aliases     []string `json:"aliases"`
		} `json:"entities"`
		Relationships []struct {
			Source      string  `json:"source"`
			Target      string  `json:"target"`
			Type        string  `json:"type"`
			Description string  `json:"description"`
			Evidence    string  `json:"evidence"`
			Confidence  float64 `json:"confidence"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return graphExtractResult{}, err
	}
	result := graphExtractResult{}
	entityNames := map[string]bool{}
	for _, entity := range parsed.Entities {
		name := strings.TrimSpace(entity.Name)
		if name == "" {
			continue
		}
		entityNames[canonicalEntityKey(name)] = true
		result.Entities = append(result.Entities, extractedEntity{
			Name:        name,
			Type:        strings.TrimSpace(entity.Type),
			Description: strings.TrimSpace(entity.Description),
			Aliases:     mergeAliases(entity.Aliases, name),
		})
	}
	for _, relation := range parsed.Relationships {
		source := strings.TrimSpace(relation.Source)
		target := strings.TrimSpace(relation.Target)
		if source == "" || target == "" || !entityNames[canonicalEntityKey(source)] || !entityNames[canonicalEntityKey(target)] {
			continue
		}
		confidence := relation.Confidence
		if confidence <= 0 {
			confidence = 0.72
		}
		result.Relationships = append(result.Relationships, extractedRelationship{
			Source:      source,
			Target:      target,
			Type:        cleanGraphLabel(relation.Type),
			Description: strings.TrimSpace(relation.Description),
			Evidence:    strings.TrimSpace(relation.Evidence),
			Confidence:  confidence,
		})
	}
	if len(result.Entities) == 0 {
		return graphExtractResult{}, errors.New("llm graph extractor未抽取实体")
	}
	return result, nil
}

func (ruleGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	_ = ctx
	text := strings.TrimSpace(input.Chunk.Content)
	if text == "" {
		return graphExtractResult{}, nil
	}
	entities := extractGraphEntities(text)
	entityByCanonical := map[string]extractedEntity{}
	for _, entity := range entities {
		canonical := canonicalEntityKey(entity.Name)
		if canonical == "" {
			continue
		}
		if existing, ok := entityByCanonical[canonical]; ok {
			existing.Aliases = mergeAliases(existing.Aliases, append(entity.Aliases, entity.Name)...)
			if existing.Description == "" {
				existing.Description = entity.Description
			}
			entityByCanonical[canonical] = existing
			continue
		}
		entityByCanonical[canonical] = entity
	}
	out := graphExtractResult{Entities: make([]extractedEntity, 0, len(entityByCanonical))}
	for _, entity := range entityByCanonical {
		out.Entities = append(out.Entities, entity)
	}
	sort.Slice(out.Entities, func(i, j int) bool { return out.Entities[i].Name < out.Entities[j].Name })
	out.Relationships = extractGraphRelationships(text, out.Entities)
	return out, nil
}

func filterGraphExtractResult(result graphExtractResult, evidence string) graphExtractResult {
	validEntities := make([]extractedEntity, 0, len(result.Entities))
	validKeys := map[string]bool{}
	usedInRelation := map[string]bool{}
	for _, relation := range result.Relationships {
		if strings.TrimSpace(relation.Evidence) == "" && strings.TrimSpace(relation.Description) == "" {
			continue
		}
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey {
			continue
		}
		usedInRelation[sourceKey] = true
		usedInRelation[targetKey] = true
	}
	for _, entity := range result.Entities {
		if !isUsefulGraphEntity(entity, evidence, usedInRelation[canonicalEntityKey(entity.Name)]) {
			continue
		}
		key := canonicalEntityKey(entity.Name)
		if key == "" || validKeys[key] {
			continue
		}
		entity.Type = normalizeEntityTypeWithContext(entity.Type, entity.Name, entity.Description+" "+evidence)
		validKeys[key] = true
		validEntities = append(validEntities, entity)
	}
	validRelations := make([]extractedRelationship, 0, len(result.Relationships))
	seenRelation := map[string]bool{}
	for _, relation := range result.Relationships {
		relation.Type = normalizeRelationType(relation.Type)
		if relation.Type == "" {
			continue
		}
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey || !validKeys[sourceKey] || !validKeys[targetKey] {
			continue
		}
		if !isUsefulGraphRelation(relation, evidence) {
			continue
		}
		key := sourceKey + "->" + targetKey + ":" + relation.Type + ":" + canonicalEntityKey(relation.Evidence)
		if seenRelation[key] {
			continue
		}
		seenRelation[key] = true
		validRelations = append(validRelations, relation)
	}
	if !graphExtractResultQualified(validEntities, validRelations) {
		if len(validEntities) == 0 {
			return graphExtractResult{}
		}
		return graphExtractResult{Entities: validEntities, Relationships: nil}
	}
	return graphExtractResult{Entities: validEntities, Relationships: validRelations}
}

func graphExtractResultQualified(entities []extractedEntity, relations []extractedRelationship) bool {
	if len(entities) < 2 || len(relations) == 0 {
		return false
	}
	return true
}

func isHighQualitySemanticRelation(relation extractedRelationship) bool {
	if !isGenericRelatedRelation(relation.Type) {
		return false
	}
	if relation.Confidence < 0.9 {
		return false
	}
	text := strings.TrimSpace(relation.Description + " " + relation.Evidence)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	weakPhrases := []string{"同段出现", "同时出现", "一起出现", "相关", "提到了", "mentioned", "co-occur", "cooccur"}
	for _, phrase := range weakPhrases {
		if strings.Contains(lower, phrase) || strings.Contains(text, phrase) {
			return false
		}
	}
	semanticPhrases := []string{"包含", "组成", "属于", "用于说明", "用于解释", "是", "定义", "体现", "支撑", "描述", "面向", "覆盖", "适用于", "组成部分", "核心内容", "关键维度"}
	for _, phrase := range semanticPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return len([]rune(text)) >= 18
}

func capGraphRelations(relations []extractedRelationship, limit int) []extractedRelationship {
	if limit <= 0 || len(relations) <= limit {
		return relations
	}
	sort.SliceStable(relations, func(i, j int) bool {
		leftRelated := isGenericRelatedRelation(relations[i].Type)
		rightRelated := isGenericRelatedRelation(relations[j].Type)
		if leftRelated != rightRelated {
			return !leftRelated
		}
		return relations[i].Confidence > relations[j].Confidence
	})
	return relations[:limit]
}

func isUsefulGraphRelation(relation extractedRelationship, evidence string) bool {
	if relation.Source == "" || relation.Target == "" {
		return false
	}
	if relation.Confidence > 0 && relation.Confidence < 0.45 {
		return false
	}
	relationEvidence := strings.TrimSpace(relation.Evidence)
	relationDescription := strings.TrimSpace(relation.Description)
	if relationEvidence == "" && relationDescription == "" {
		return false
	}
	if isExplicitBadGraphRelationText(relationEvidence + " " + relationDescription) {
		return false
	}
	sourceKey := canonicalEntityKey(relation.Source)
	targetKey := canonicalEntityKey(relation.Target)
	evidenceText := strings.TrimSpace(relationEvidence + " " + relationDescription + " " + evidence)
	evidenceLower := strings.ToLower(evidenceText)
	if evidenceLower != "" {
		compactEvidence := canonicalEntityKey(evidenceLower)
		sourceMentioned := strings.Contains(evidenceLower, strings.ToLower(relation.Source)) || strings.Contains(compactEvidence, sourceKey)
		targetMentioned := strings.Contains(evidenceLower, strings.ToLower(relation.Target)) || strings.Contains(compactEvidence, targetKey)
		if !sourceMentioned && !targetMentioned {
			return false
		}
	}
	return true
}

func isExplicitBadGraphRelationText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	badMarkers := []string{
		"错误", "不应", "不合理", "无意义", "噪声", "误抽取",
		"invalid", "impossible", "wrong", "noise",
	}
	for _, marker := range badMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func isUsefulGraphEntity(entity extractedEntity, evidence string, usedInRelation bool) bool {
	name := strings.TrimSpace(entity.Name)
	if name == "" {
		return false
	}
	key := canonicalEntityKey(name)
	if len([]rune(key)) < 2 {
		return false
	}
	lower := strings.ToLower(name)
	noise := []string{
		"product image file", "image file", "screenshot", "unknown", "none", "n/a", "todo",
		"正文", "标题", "内容", "图片", "文件", "截图", "页面", "文本", "示例", "未知实体", "系统", "用户", "数据", "信息", "说明", "文档",
		"在线用户", "当前用户", "普通用户", "触发用户", "用户身份", "文件内容", "文本内容", "页面内容", "图片内容",
		"会话摘要", "会话主题", "聊天摘要", "聊天主题",
		"目录", "章节", "步骤", "材料", "资料", "个人经历", "工作经历", "项目经历", "教育背景", "个人技能", "求职目标", "自我评价", "联系方式",
	}
	for _, item := range noise {
		if lower == item || strings.EqualFold(name, item) || name == item {
			return false
		}
	}
	if isGraphStopEntityName(name) {
		return false
	}
	if isWeakChineseGraphEntity(name) && !usedInRelation {
		return false
	}
	if isGenericDocumentSectionEntity(name) {
		return false
	}
	if regexp.MustCompile(`(?i)^[a-f0-9]{16,}\.(png|jpg|jpeg|webp|gif|bmp|tif|tiff)$`).MatchString(lower) {
		return false
	}
	if regexp.MustCompile(`(?i)^p?\d{1,4}[-_]\d{1,4}$`).MatchString(lower) {
		return false
	}
	if isGraphFieldOrStatusName(name) {
		return false
	}
	if isGenericEntityType(entity.Type) && len([]rune(name)) <= 2 {
		return false
	}
	if strings.TrimSpace(evidence) != "" && !usedInRelation && !entityMentionedByEvidence(entity, evidence) {
		return false
	}
	return true
}

func isGenericDocumentSectionEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	switch trimmed {
	case "个人经历", "工作经历", "实习经历", "项目经历", "教育背景", "专业技能", "个人技能", "求职目标", "自我评价", "联系方式", "获奖经历", "校园经历", "目录", "章节", "步骤":
		return true
	default:
		return false
	}
}

func isRuleExtractedGraphEntity(entity extractedEntity) bool {
	return strings.Contains(strings.TrimSpace(entity.Description), "从知识分块中识别")
}

func entityMentionedByEvidence(entity extractedEntity, evidence string) bool {
	evidenceLower := strings.ToLower(evidence)
	if strings.Contains(evidenceLower, strings.ToLower(strings.TrimSpace(entity.Name))) {
		return true
	}
	for _, alias := range entity.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" && strings.Contains(evidenceLower, strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

func isGraphFieldOrStatusName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	fieldNames := map[string]bool{
		"id": true, "user_id": true, "sender_id": true, "owner_id": true, "group_id": true, "conversation_id": true,
		"message_id": true, "msg_id": true, "reply_to_id": true, "event_id": true, "source_event_id": true,
		"client_msg_id": true, "trace_id": true, "agent_trace_id": true, "agent_user_id": true, "bot_id": true,
		"created_at": true, "updated_at": true, "deleted_at": true, "status": true, "type": true,
		"pending": true, "processing": true, "completed": true, "failed": true, "success": true, "error": true,
	}
	if fieldNames[lower] {
		return true
	}
	if regexp.MustCompile(`(?i)^(.*_)?(id|ids|status|type|time|at)$`).MatchString(lower) && !strings.HasSuffix(lower, "_service") {
		return true
	}
	return false
}

func isGraphStopEntityName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http") || strings.HasPrefix(lower, "data:") {
		return true
	}
	if regexp.MustCompile(`(?i)\.(png|jpg|jpeg|webp|gif|bmp|tif|tiff|pdf|docx?|pptx?|md|markdown|txt|go|js|ts|tsx|jsx|py|java|c|cc|cpp|h|hpp|rs|sql|json|ya?ml|toml|xml|html|css|scss|sh|bat|ps1)$`).MatchString(lower) {
		return true
	}
	if regexp.MustCompile(`(?i)^[a-z]:[\\/]|[\\/]{2,}`).MatchString(lower) {
		return true
	}
	if regexp.MustCompile(`^\d+(\.\d+)*$`).MatchString(lower) {
		return true
	}
	if regexp.MustCompile(`^[第]?\d+[章节页课]$`).MatchString(trimmed) {
		return true
	}
	return false
}

func isWeakChineseGraphEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	weakSuffixes := []string{"内容", "文本", "页面", "截图", "图片", "文件", "数据", "信息", "说明", "示例", "用户"}
	for _, suffix := range weakSuffixes {
		if strings.HasSuffix(trimmed, suffix) {
			switch trimmed {
			case "用户服务", "用户系统", "用户模块", "用户画像", "用户记忆":
				return false
			default:
				return true
			}
		}
	}
	return false
}

func extractGraphEntities(text string) []extractedEntity {
	seen := map[string]bool{}
	var out []extractedEntity
	add := func(name string, aliases ...string) {
		name = strings.Trim(strings.TrimSpace(name), "，。,.；;:：()（）[]【】")
		if name == "" || isGraphStopEntityName(name) || hasGraphRelationPrefixNoise(name) {
			return
		}
		if !isStrongRuleGraphEntity(name) {
			return
		}
		canonical := canonicalEntityKey(name)
		if canonical == "" || seen[canonical] {
			if canonical != "" && len(aliases) > 0 {
				for i := range out {
					if canonicalEntityKey(out[i].Name) == canonical {
						out[i].Aliases = mergeAliases(out[i].Aliases, aliases...)
						return
					}
				}
			}
			return
		}
		seen[canonical] = true
		out = append(out, extractedEntity{
			Name:        name,
			Type:        inferEntityType(name),
			Description: "从知识分块中识别的 " + inferEntityType(name) + " 实体",
			Aliases:     mergeAliases([]string{name}, aliases...),
		})
	}
	identifierRe := regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:[-_.][A-Za-z0-9]+)+|[a-z][a-z0-9]+(?:_[a-z0-9]+)+`)
	for _, match := range identifierRe.FindAllString(text, -1) {
		add(match)
	}
	if strings.Contains(strings.ToLower(text), "msg core service") {
		add("msg core service", "msg-core-service", "消息核心服务")
	}
	aliasRe := regexp.MustCompile(`([A-Za-z][A-Za-z0-9 _.-]{2,60})\s*(?:也叫|又称|即|alias)\s*([\p{Han}A-Za-z0-9 _.-]{2,40})`)
	for _, match := range aliasRe.FindAllStringSubmatch(text, -1) {
		if len(match) == 3 {
			add(match[1], match[2])
			add(match[2], match[1])
		}
	}
	for _, match := range extractChineseGraphEntityCandidates(text) {
		add(match)
	}
	return out
}

func extractChineseGraphEntityCandidates(text string) []string {
	entitySuffix := `(?:服务|模块|系统|平台|助手|数据库|数据表|事件|接口|知识库|知识图谱|图谱|流程|规则|能力|策略|标签|页面|视图|配置|模型|任务|结果|指标|权限|角色|字段|筛选|检索|召回|排序)`
	entityRe := regexp.MustCompile(`[\p{Han}]{2,18}` + entitySuffix)
	separatorRe := regexp.MustCompile(`(?:调用|写入|读取|消费|发布|存储|依赖|配置|触发|拥有|负责|包含|包括|组成|使用|采用|基于|生成|产出|输出|实现|落地|支撑|支持|服务于|约束|限制|要求|属于|归属|隶属|面向|适用于|针对|展示|显示|呈现|校验|验证|检查|过滤|搜索|查询|匹配|，|。|；|、|,|;|\s+)`)
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, segment := range separatorRe.Split(text, -1) {
		for _, match := range entityRe.FindAllString(segment, -1) {
			match = strings.TrimSpace(match)
			if match == "" || seen[match] {
				continue
			}
			seen[match] = true
			out = append(out, match)
		}
	}
	for _, match := range entityRe.FindAllString(text, -1) {
		match = trimChineseEntityCandidate(match)
		if match == "" || seen[match] {
			continue
		}
		seen[match] = true
		out = append(out, match)
	}
	return out
}

func trimChineseEntityCandidate(name string) string {
	name = strings.TrimSpace(name)
	for _, separator := range []string{"调用", "写入", "读取", "消费", "发布", "存储", "依赖", "配置", "触发", "拥有", "负责", "包含", "包括", "组成", "使用", "采用", "基于", "生成", "产出", "输出", "实现", "落地", "支撑", "支持", "服务于", "约束", "限制", "要求", "属于", "归属", "隶属", "面向", "适用于", "针对", "展示", "显示", "呈现", "校验", "验证", "检查", "过滤", "搜索", "查询", "匹配"} {
		if idx := strings.LastIndex(name, separator); idx >= 0 {
			name = strings.TrimSpace(name[idx+len(separator):])
		}
	}
	return strings.Trim(strings.TrimSpace(name), "，。；;,.、:：()（）[]【】")
}

func isStrongRuleGraphEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if lower == "msg core service" {
		return true
	}
	if strings.Contains(lower, "-service") ||
		strings.Contains(lower, "_service") ||
		strings.Contains(lower, "-gateway") ||
		strings.Contains(lower, "_gateway") ||
		strings.Contains(lower, "-manager") ||
		strings.Contains(lower, "_manager") ||
		strings.Contains(lower, "-runtime") ||
		strings.Contains(lower, "_runtime") ||
		strings.Contains(lower, ".events") ||
		strings.Contains(lower, ".event") ||
		strings.Contains(lower, "_events") ||
		strings.Contains(lower, "_event") ||
		strings.Contains(lower, "_records") ||
		strings.Contains(lower, "_outbox") ||
		strings.Contains(lower, "_table") ||
		strings.Contains(lower, "_tasks") ||
		strings.Contains(lower, "_messages") ||
		strings.Contains(lower, "_states") ||
		strings.Contains(lower, "_settings") ||
		strings.Contains(lower, "_memory") ||
		strings.Contains(lower, "_chunks") ||
		strings.Contains(lower, "_entities") ||
		strings.Contains(lower, "_relationships") {
		return true
	}
	if strings.HasPrefix(lower, "/api/") || strings.HasPrefix(lower, "api/") {
		return true
	}
	if regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(Service|Gateway|Manager|Runtime|Controller|Repository|Client)$`).MatchString(trimmed) {
		return true
	}
	if regexp.MustCompile(`^[a-z][a-z0-9]+(?:_[a-z0-9]+){1,}$`).MatchString(lower) {
		for _, suffix := range []string{"service", "gateway", "manager", "runtime", "events", "event", "records", "outbox", "table", "tasks", "messages", "states", "settings", "memory", "chunks", "entities", "relationships"} {
			if strings.HasSuffix(lower, "_"+suffix) || strings.Contains(lower, "_"+suffix+"_") {
				return true
			}
		}
	}
	if regexp.MustCompile(`[\p{Han}]{2,18}(服务|模块|系统|平台|助手|数据库|数据表|事件|接口|知识库|知识图谱|图谱|流程|规则|能力|策略|标签|页面|视图|配置|模型|任务|结果|指标|权限|角色|字段|筛选|检索|召回|排序)$`).MatchString(trimmed) {
		return true
	}
	return false
}

func hasGraphRelationPrefixNoise(name string) bool {
	trimmed := strings.TrimSpace(name)
	for _, prefix := range []string{"也叫", "又称", "负责把", "用于", "应该成为", "不应该成为"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func extractGraphRelationships(text string, entities []extractedEntity) []extractedRelationship {
	if len(entities) < 2 {
		return nil
	}
	var out []extractedRelationship
	seen := map[string]bool{}
	entityTypeByKey := map[string]string{}
	for _, entity := range entities {
		entityTypeByKey[canonicalEntityKey(entity.Name)] = normalizeEntityType(entity.Type, entity.Name)
	}
	for _, sentence := range splitGraphSentences(text) {
		mentions := entityMentionsInText(sentence, entities)
		if len(mentions) < 2 {
			continue
		}
		triggers := relationTriggersInSentence(sentence)
		if len(triggers) == 0 {
			continue
		}
		defaultSubject := mentions[0].Name
		for triggerIndex, trigger := range triggers {
			segmentEnd := len(sentence)
			if triggerIndex+1 < len(triggers) {
				segmentEnd = triggers[triggerIndex+1].Index
			}
			for _, pair := range relationPairsAt(sentence, mentions, trigger, defaultSubject, segmentEnd) {
				source, target := pair[0], pair[1]
				if source == "" || target == "" || canonicalEntityKey(source) == canonicalEntityKey(target) {
					continue
				}
				sourceKey := canonicalEntityKey(source)
				targetKey := canonicalEntityKey(target)
				if !relationAllowedByEntityTypes(trigger.Type, entityTypeByKey[sourceKey], entityTypeByKey[targetKey]) {
					continue
				}
				key := sourceKey + "->" + targetKey + ":" + trigger.Type
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, extractedRelationship{
					Source:      source,
					Target:      target,
					Type:        trigger.Type,
					Description: relationDescription(source, target, trigger.Type),
					Evidence:    sentence,
					Confidence:  0.84,
				})
			}
		}
	}
	return out
}
