package service

import (
	"ClaranAIM/internal/rag-service/model"
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

func (s *ragServiceImpl) buildGraph(ctx context.Context, ownerID, documentID int64, documentTitle, sourceType string, chunks []model.Chunk, relationMode string) (int64, int64) {
	if !shouldBuildGraphForSourceType(sourceType) {
		return 0, 0
	}
	relationMode = normalizeGraphRelationMode(relationMode)
	extractor, hasConfiguredExtractor := s.resolveGraphExtractor(ctx, ownerID)
	if extractor == nil {
		extractor = ruleGraphExtractor{}
	}
	candidates := graphExtractionCandidates(chunks)
	var documentEntity *model.Entity
	entityByCanonical := map[string]*model.Entity{}
	orderedEntities := make([]*model.Entity, 0)
	semanticDegree := map[int64]int{}
	seenRelations := map[string]bool{}
	entityCount := int64(0)
	relationCount := int64(0)
	logGraphStage("start", ownerID, documentID, "source_type", sourceType, "relation_mode", relationMode, "candidate_chunks", len(candidates), "llm_enabled", hasConfiguredExtractor)
	for _, chunk := range candidates {
		result, err := extractor.Extract(ctx, graphExtractInput{DocumentID: documentID, Chunk: chunk, GraphRelationMode: relationMode})
		if err != nil {
			logGraphStage("extract_error", ownerID, documentID, "chunk_id", chunk.ID, "err", err)
			continue
		}
		rawEntities := len(result.Entities)
		rawRelations := len(result.Relationships)
		result = filterGraphExtractResult(result, chunk.Content)
		logGraphStage("extract_result", ownerID, documentID, "chunk_id", chunk.ID, "raw_entities", rawEntities, "raw_relations", rawRelations, "kept_entities", len(result.Entities), "kept_relations", len(result.Relationships))
		for _, extracted := range result.Entities {
			entity := s.upsertGraphEntity(ctx, ownerID, extracted)
			if entity == nil {
				continue
			}
			if _, exists := entityByCanonical[entity.CanonicalKey]; !exists {
				entityCount++
				orderedEntities = append(orderedEntities, entity)
			}
			entityByCanonical[entity.CanonicalKey] = entity
			if documentEntity == nil {
				documentEntity = s.upsertDocumentGraphEntity(ctx, ownerID, documentID, documentTitle)
				if documentEntity != nil {
					entityCount++
				}
			}
			if documentEntity != nil && entity.ID != documentEntity.ID {
				relationKey := fmt.Sprintf("%d:%d:CONTAINS:%d", documentEntity.ID, entity.ID, documentID)
				if !seenRelations[relationKey] {
					seenRelations[relationKey] = true
					relation := &model.Relation{
						OwnerID:         ownerID,
						SourceID:        documentEntity.ID,
						TargetID:        entity.ID,
						Relation:        "CONTAINS",
						Description:     fmt.Sprintf("文档《%s》包含核心实体 %s", defaultString(documentTitle, "未命名文档"), entity.Name),
						Weight:          0.95,
						Confidence:      0.95,
						Evidence:        truncate(defaultString(chunk.Summary, chunk.Content), 500),
						EvidenceChunkID: chunk.ID,
						DocumentID:      documentID,
					}
					_ = s.graphStore.SaveRelation(ctx, relation)
					relationCount++
				}
			}
		}
		chunkSpecificRelations := int64(0)
		for _, extracted := range result.Relationships {
			source := entityByCanonical[canonicalEntityKey(extracted.Source)]
			target := entityByCanonical[canonicalEntityKey(extracted.Target)]
			if source == nil || target == nil || source.ID == 0 || target.ID == 0 || source.ID == target.ID {
				continue
			}
			confidence := extracted.Confidence
			if confidence <= 0 {
				confidence = 0.72
			}
			relationType := normalizeRelationType(extracted.Type)
			relationKey := fmt.Sprintf("%d:%d:%s:%d", source.ID, target.ID, relationType, chunk.ID)
			if seenRelations[relationKey] {
				continue
			}
			seenRelations[relationKey] = true
			relation := &model.Relation{
				OwnerID:         ownerID,
				SourceID:        source.ID,
				TargetID:        target.ID,
				Relation:        relationType,
				Description:     defaultString(extracted.Description, source.Name+" 与 "+target.Name+" 存在知识图谱关系"),
				Weight:          confidence,
				Confidence:      confidence,
				Evidence:        truncate(defaultString(extracted.Evidence, chunk.Content), 500),
				EvidenceChunkID: chunk.ID,
				DocumentID:      documentID,
			}
			_ = s.graphStore.SaveRelation(ctx, relation)
			relationCount++
			semanticDegree[source.ID]++
			semanticDegree[target.ID]++
			if relationType != "CONTAINS" && !isGenericRelatedRelation(relationType) {
				chunkSpecificRelations++
			}
		}
		if chunkSpecificRelations == 0 && len(result.Entities) >= 2 {
			backfilled := s.saveRuleBackfilledSpecificRelations(ctx, ownerID, documentID, documentTitle, chunk, result.Entities, entityByCanonical, seenRelations, semanticDegree)
			if backfilled == 0 {
				backfilled = s.saveVisibleEntityCooccurrenceRelations(ctx, ownerID, documentID, documentTitle, chunk, result.Entities, entityByCanonical, seenRelations, semanticDegree)
			}
			relationCount += backfilled
			if backfilled > 0 {
				logGraphStage("rule_specific_backfill", ownerID, documentID, "chunk_id", chunk.ID, "relations", backfilled)
			}
		}
	}
	if entityCount == 0 && relationCount == 0 {
		logGraphStage("fallback_start", ownerID, documentID, "has_configured_extractor", hasConfiguredExtractor, "candidate_chunks", len(candidates))
		fallbackEntities, fallbackRelations := s.buildFallbackTopicGraph(ctx, ownerID, documentID, documentTitle, candidates, relationMode)
		entityCount += fallbackEntities
		relationCount += fallbackRelations
		logGraphStage("fallback_done", ownerID, documentID, "entities", fallbackEntities, "relations", fallbackRelations)
	}
	if documentEntity != nil && len(entityByCanonical) > 0 {
		orphanRelations := s.ensureDocumentEntityRelations(ctx, ownerID, documentID, documentTitle, orderedEntities, semanticDegree, seenRelations, relationMode)
		relationCount += orphanRelations
		if orphanRelations > 0 {
			logGraphStage("generic_orphan_links", ownerID, documentID, "relations", orphanRelations)
		}
		crossDocRelations := s.linkCrossDocumentSharedEntities(ctx, ownerID, documentID, documentTitle, documentEntity, entityByCanonical, seenRelations)
		relationCount += crossDocRelations
		if crossDocRelations > 0 {
			logGraphStage("cross_document_links", ownerID, documentID, "relations", crossDocRelations)
		}
	}
	s.rebuildGraphCommunities(ctx, ownerID)
	logGraphStage("done", ownerID, documentID, "entities", entityCount, "relations", relationCount)
	return entityCount, relationCount
}

func (s *ragServiceImpl) linkCrossDocumentSharedEntities(ctx context.Context, ownerID, documentID int64, documentTitle string, documentEntity *model.Entity, entityByCanonical map[string]*model.Entity, seenRelations map[string]bool) int64 {
	if s == nil || s.graphStore == nil || documentEntity == nil || documentEntity.ID == 0 || len(entityByCanonical) == 0 {
		return 0
	}
	entities, relations, err := s.graphStore.ListOwnerGraph(ctx, ownerID)
	if err != nil {
		return 0
	}
	entityByID := make(map[int64]model.Entity, len(entities))
	for _, entity := range entities {
		entityByID[entity.ID] = entity
	}
	sharedEntityIDs := map[int64]string{}
	for _, entity := range entityByCanonical {
		if entity == nil || entity.ID == 0 || entity.ID == documentEntity.ID {
			continue
		}
		sharedEntityIDs[entity.ID] = entity.Name
	}
	if len(sharedEntityIDs) == 0 {
		return 0
	}
	type priorSharedDoc struct {
		doc        model.Entity
		entityName string
		docID      int64
	}
	priorByDocID := map[int64]priorSharedDoc{}
	existing := map[string]bool{}
	for _, relation := range relations {
		key := fmt.Sprintf("%d:%d:%s:%d", relation.SourceID, relation.TargetID, relation.Relation, relation.DocumentID)
		existing[key] = true
		reverseKey := fmt.Sprintf("%d:%d:%s:%d", relation.TargetID, relation.SourceID, relation.Relation, relation.DocumentID)
		existing[reverseKey] = true
		if relation.Relation != "CONTAINS" || relation.DocumentID == documentID {
			continue
		}
		entityName, ok := sharedEntityIDs[relation.TargetID]
		if !ok {
			continue
		}
		priorDoc := entityByID[relation.SourceID]
		if priorDoc.ID == 0 || priorDoc.ID == documentEntity.ID || priorDoc.Type != "文档" {
			continue
		}
		if _, ok := priorByDocID[priorDoc.ID]; !ok {
			priorByDocID[priorDoc.ID] = priorSharedDoc{doc: priorDoc, entityName: entityName, docID: relation.DocumentID}
		}
	}
	count := int64(0)
	currentTitle := defaultString(documentTitle, documentEntity.Name)
	for _, prior := range priorByDocID {
		relationKey := fmt.Sprintf("%d:%d:跨文档同一实体:%d", documentEntity.ID, prior.doc.ID, documentID)
		if seenRelations[relationKey] || existing[relationKey] {
			continue
		}
		seenRelations[relationKey] = true
		relation := &model.Relation{
			OwnerID:     ownerID,
			SourceID:    documentEntity.ID,
			TargetID:    prior.doc.ID,
			Relation:    "跨文档同一实体",
			Description: fmt.Sprintf("文档《%s》和文档《%s》都围绕实体 %s 建立了图谱关系。", currentTitle, prior.doc.Name, prior.entityName),
			Weight:      0.74,
			Confidence:  0.74,
			Evidence:    fmt.Sprintf("共享实体：%s；既有文档：《%s》；当前文档：《%s》。", prior.entityName, prior.doc.Name, currentTitle),
			DocumentID:  documentID,
		}
		_ = s.graphStore.SaveRelation(ctx, relation)
		count++
	}
	return count
}

func (s *ragServiceImpl) ensureDocumentEntityRelations(ctx context.Context, ownerID, documentID int64, documentTitle string, entities []*model.Entity, semanticDegree map[int64]int, seenRelations map[string]bool, relationMode string) int64 {
	_ = ctx
	_ = ownerID
	_ = documentID
	_ = documentTitle
	_ = entities
	_ = semanticDegree
	_ = seenRelations
	_ = relationMode
	return 0
}

func (s *ragServiceImpl) saveGenericRelatedRelation(ctx context.Context, ownerID, documentID int64, documentTitle string, source, target *model.Entity, seenRelations map[string]bool, semanticDegree map[int64]int, relationMode string) bool {
	relationKey := fmt.Sprintf("%d:%d:RELATED_TO:%d", source.ID, target.ID, documentID)
	reverseKey := fmt.Sprintf("%d:%d:RELATED_TO:%d", target.ID, source.ID, documentID)
	if seenRelations[relationKey] || seenRelations[reverseKey] {
		return false
	}
	seenRelations[relationKey] = true
	confidence := 0.42
	if normalizeGraphRelationMode(relationMode) == "generic" {
		confidence = 0.5
	}
	relation := &model.Relation{
		OwnerID:     ownerID,
		SourceID:    source.ID,
		TargetID:    target.ID,
		Relation:    "RELATED_TO",
		Description: fmt.Sprintf("%s 与 %s 同属文档《%s》的核心实体，用于在%s关系抽取模式下保留弱语义连接。", source.Name, target.Name, defaultString(documentTitle, "未命名文档"), graphRelationModeLabel(relationMode)),
		Weight:      confidence,
		Confidence:  confidence,
		Evidence:    fmt.Sprintf("文档《%s》同时包含实体：%s、%s。", defaultString(documentTitle, "未命名文档"), source.Name, target.Name),
		DocumentID:  documentID,
	}
	_ = s.graphStore.SaveRelation(ctx, relation)
	semanticDegree[source.ID]++
	semanticDegree[target.ID]++
	return true
}

func nearbyGraphRelationTargets(entities []*model.Entity, index int, anchor *model.Entity, degree map[int64]int, relationMode string) []*model.Entity {
	if normalizeGraphRelationMode(relationMode) != "generic" {
		target := nearestGraphEntity(entities, index, anchor, degree)
		if target == nil {
			return nil
		}
		return []*model.Entity{target}
	}
	out := make([]*model.Entity, 0, 3)
	for _, offset := range []int{-1, 1, -2, 2} {
		pos := index + offset
		if pos >= 0 && pos < len(entities) && entities[pos] != nil && entities[pos].ID != entities[index].ID {
			out = append(out, entities[pos])
		}
		if len(out) >= 3 {
			return out
		}
	}
	if anchor != nil && entities[index] != nil && anchor.ID != entities[index].ID {
		out = append(out, anchor)
	}
	return out
}

func firstConnectedGraphEntity(entities []*model.Entity, degree map[int64]int) *model.Entity {
	for _, entity := range entities {
		if entity != nil && degree[entity.ID] > 0 {
			return entity
		}
	}
	if len(entities) == 0 {
		return nil
	}
	return entities[0]
}

func nearestGraphEntity(entities []*model.Entity, index int, anchor *model.Entity, degree map[int64]int) *model.Entity {
	for left, right := index-1, index+1; left >= 0 || right < len(entities); left, right = left-1, right+1 {
		if left >= 0 && entities[left] != nil && entities[left].ID != entities[index].ID && degree[entities[left].ID] > 0 {
			return entities[left]
		}
		if right < len(entities) && entities[right] != nil && entities[right].ID != entities[index].ID && degree[entities[right].ID] > 0 {
			return entities[right]
		}
	}
	if anchor != nil && entities[index] != nil && anchor.ID != entities[index].ID {
		return anchor
	}
	if index > 0 {
		return entities[index-1]
	}
	if len(entities) > 1 {
		return entities[1]
	}
	return nil
}

func (s *ragServiceImpl) resolveGraphExtractor(ctx context.Context, ownerID int64) (graphExtractor, bool) {
	if s == nil {
		return nil, false
	}
	if s.llmGraphEnabled && s.graphExtractor != nil {
		return s.graphExtractor, true
	}
	if s.settings == nil || ownerID <= 0 {
		return s.graphExtractor, false
	}
	profiles, err := s.listRAGRouterProfiles(ctx, ownerID)
	if err != nil || len(profiles) == 0 {
		return s.graphExtractor, false
	}
	selected := profiles[0]
	for _, profile := range profiles {
		if profile.GetIsDefault() {
			selected = profile
			break
		}
	}
	if selected.GetId() <= 0 {
		return s.graphExtractor, false
	}
	resolved, err := s.resolveLLMProfile(ctx, ownerID, selected.GetId())
	if err != nil {
		return s.graphExtractor, false
	}
	if strings.TrimSpace(resolved.APIKey) == "" || strings.TrimSpace(resolved.BaseURL) == "" || strings.TrimSpace(resolved.ModelName) == "" {
		return s.graphExtractor, false
	}
	return fallbackGraphExtractor{primary: NewLLMGraphExtractor(resolved.APIKey, resolved.BaseURL, resolved.ModelName), fallback: ruleGraphExtractor{}}, true
}

func shouldUseSparseGraphFallback(chunks []model.Chunk) bool {
	if len(chunks) >= 4 {
		return true
	}
	total := 0
	for _, chunk := range chunks {
		total += len([]rune(strings.TrimSpace(chunk.Content)))
	}
	return total >= 1800
}

func (s *ragServiceImpl) buildFallbackTopicGraph(ctx context.Context, ownerID, documentID int64, documentTitle string, chunks []model.Chunk, relationMode string) (int64, int64) {
	if s == nil || s.graphStore == nil || len(chunks) == 0 {
		return 0, 0
	}
	documentEntity := s.upsertDocumentGraphEntity(ctx, ownerID, documentID, documentTitle)
	if documentEntity == nil {
		return 0, 0
	}
	entityByKey := map[string]*model.Entity{}
	orderedEntities := make([]*model.Entity, 0)
	entityByName := map[string]*model.Entity{}
	entityCount := int64(1)
	relationCount := int64(0)
	semanticDegree := map[int64]int{}
	seenRelations := map[string]bool{}
	for _, chunk := range chunks {
		entities := fallbackTopicEntities(chunk)
		if len(entities) == 0 {
			continue
		}
		for _, extracted := range entities {
			entity := s.upsertGraphEntity(ctx, ownerID, extracted)
			if entity == nil || entity.ID == documentEntity.ID {
				continue
			}
			if _, ok := entityByKey[entity.CanonicalKey]; !ok {
				entityCount++
				orderedEntities = append(orderedEntities, entity)
			}
			entityByKey[entity.CanonicalKey] = entity
			entityByName[canonicalEntityKey(extracted.Name)] = entity
			relationKey := fmt.Sprintf("%d:%d:CONTAINS:%d", documentEntity.ID, entity.ID, documentID)
			if seenRelations[relationKey] {
				continue
			}
			seenRelations[relationKey] = true
			relation := &model.Relation{
				OwnerID:         ownerID,
				SourceID:        documentEntity.ID,
				TargetID:        entity.ID,
				Relation:        "CONTAINS",
				Description:     fmt.Sprintf("文档《%s》包含主题 %s；这是 GraphRAG LLM 未配置或抽取为空时的最小可视化骨架。", defaultString(documentTitle, "未命名文档"), entity.Name),
				Weight:          0.72,
				Confidence:      0.72,
				Evidence:        truncate(chunk.Content, 500),
				EvidenceChunkID: chunk.ID,
				DocumentID:      documentID,
			}
			_ = s.graphStore.SaveRelation(ctx, relation)
			relationCount++
		}
		relationCount += s.saveFallbackSemanticRelations(ctx, ownerID, documentID, documentTitle, chunk, entities, entityByName, seenRelations, semanticDegree)
	}
	relationCount += s.ensureDocumentEntityRelations(ctx, ownerID, documentID, documentTitle, orderedEntities, semanticDegree, seenRelations, relationMode)
	return entityCount, relationCount
}

func (s *ragServiceImpl) saveFallbackSemanticRelations(ctx context.Context, ownerID, documentID int64, documentTitle string, chunk model.Chunk, extracted []extractedEntity, entityByName map[string]*model.Entity, seenRelations map[string]bool, semanticDegree map[int64]int) int64 {
	if s == nil || s.graphStore == nil || len(extracted) < 2 {
		return 0
	}
	relationships := extractGraphRelationships(chunk.Content, extracted)
	if len(relationships) == 0 {
		return 0
	}
	const maxRelationsPerChunk = 18
	count := int64(0)
	for _, relationship := range relationships {
		if count >= maxRelationsPerChunk {
			break
		}
		relationName := normalizeRelationType(relationship.Type)
		if relationName == "" {
			continue
		}
		source := entityByName[canonicalEntityKey(relationship.Source)]
		target := entityByName[canonicalEntityKey(relationship.Target)]
		if source == nil || target == nil || source.ID == target.ID {
			continue
		}
		relationKey := fmt.Sprintf("%d:%d:%s:%d", source.ID, target.ID, relationName, documentID)
		if seenRelations[relationKey] {
			continue
		}
		seenRelations[relationKey] = true
		confidence := relationship.Confidence
		if confidence <= 0 {
			confidence = 0.7
		}
		relation := &model.Relation{
			OwnerID:         ownerID,
			SourceID:        source.ID,
			TargetID:        target.ID,
			Relation:        relationName,
			Description:     fmt.Sprintf("%s。该关系来自文档《%s》的规则兜底抽取。", defaultString(relationship.Description, relationDescription(source.Name, target.Name, relationName)), defaultString(documentTitle, "未命名文档")),
			Weight:          0.7,
			Confidence:      confidence,
			Evidence:        truncate(defaultString(relationship.Evidence, chunk.Content), 500),
			EvidenceChunkID: chunk.ID,
			DocumentID:      documentID,
		}
		_ = s.graphStore.SaveRelation(ctx, relation)
		if semanticDegree != nil {
			semanticDegree[source.ID]++
			semanticDegree[target.ID]++
		}
		count++
	}
	return count
}

func (s *ragServiceImpl) saveRuleBackfilledSpecificRelations(ctx context.Context, ownerID, documentID int64, documentTitle string, chunk model.Chunk, extracted []extractedEntity, entityByCanonical map[string]*model.Entity, seenRelations map[string]bool, semanticDegree map[int64]int) int64 {
	if s == nil || s.graphStore == nil || len(extracted) < 2 || len(entityByCanonical) == 0 {
		return 0
	}
	relationships := extractGraphRelationships(chunk.Content, extracted)
	if len(relationships) == 0 {
		return 0
	}
	const maxRelationsPerChunk = 24
	count := int64(0)
	for _, relationship := range relationships {
		if count >= maxRelationsPerChunk {
			break
		}
		relationName := normalizeRelationType(relationship.Type)
		if relationName == "" || relationName == "CONTAINS" || isGenericRelatedRelation(relationName) {
			continue
		}
		source := entityByCanonical[canonicalEntityKey(relationship.Source)]
		target := entityByCanonical[canonicalEntityKey(relationship.Target)]
		if source == nil || target == nil || source.ID == target.ID {
			continue
		}
		relationKey := fmt.Sprintf("%d:%d:%s:%d", source.ID, target.ID, relationName, chunk.ID)
		if seenRelations[relationKey] {
			continue
		}
		seenRelations[relationKey] = true
		confidence := relationship.Confidence
		if confidence <= 0 {
			confidence = 0.78
		}
		relation := &model.Relation{
			OwnerID:         ownerID,
			SourceID:        source.ID,
			TargetID:        target.ID,
			Relation:        relationName,
			Description:     fmt.Sprintf("%s。该关系来自文档《%s》的规则关系回填。", defaultString(relationship.Description, relationDescription(source.Name, target.Name, relationName)), defaultString(documentTitle, "未命名文档")),
			Weight:          confidence,
			Confidence:      confidence,
			Evidence:        truncate(defaultString(relationship.Evidence, chunk.Content), 500),
			EvidenceChunkID: chunk.ID,
			DocumentID:      documentID,
		}
		_ = s.graphStore.SaveRelation(ctx, relation)
		if semanticDegree != nil {
			semanticDegree[source.ID]++
			semanticDegree[target.ID]++
		}
		count++
	}
	return count
}

func (s *ragServiceImpl) saveVisibleEntityCooccurrenceRelations(ctx context.Context, ownerID, documentID int64, documentTitle string, chunk model.Chunk, extracted []extractedEntity, entityByCanonical map[string]*model.Entity, seenRelations map[string]bool, semanticDegree map[int64]int) int64 {
	if s == nil || s.graphStore == nil || len(extracted) < 2 || len(entityByCanonical) == 0 {
		return 0
	}
	ordered := make([]*model.Entity, 0, len(extracted))
	seenEntity := map[int64]bool{}
	for _, item := range extracted {
		entity := entityByCanonical[canonicalEntityKey(item.Name)]
		if entity == nil || entity.ID == 0 || seenEntity[entity.ID] {
			continue
		}
		seenEntity[entity.ID] = true
		ordered = append(ordered, entity)
	}
	if len(ordered) < 2 {
		return 0
	}
	targetRelations := int(math.Ceil(math.Sqrt(float64(len(ordered)))))
	if targetRelations < 1 {
		targetRelations = 1
	}
	if targetRelations > len(ordered)-1 {
		targetRelations = len(ordered) - 1
	}
	count := int64(0)
	usedPair := map[string]bool{}
	for i := 0; i < targetRelations; i++ {
		sourceIndex := int(math.Floor(float64(i) * float64(len(ordered)-1) / float64(targetRelations)))
		if sourceIndex < 0 {
			sourceIndex = 0
		}
		if sourceIndex >= len(ordered)-1 {
			sourceIndex = len(ordered) - 2
		}
		targetIndex := sourceIndex + 1
		source := ordered[sourceIndex]
		target := ordered[targetIndex]
		if source == nil || target == nil || source.ID == target.ID {
			continue
		}
		relationName := "同章共现"
		pairKey := fmt.Sprintf("%d:%d", source.ID, target.ID)
		if usedPair[pairKey] {
			continue
		}
		usedPair[pairKey] = true
		relationKey := fmt.Sprintf("%d:%d:%s:%d", source.ID, target.ID, relationName, chunk.ID)
		reverseKey := fmt.Sprintf("%d:%d:%s:%d", target.ID, source.ID, relationName, chunk.ID)
		if seenRelations[relationKey] || seenRelations[reverseKey] {
			continue
		}
		seenRelations[relationKey] = true
		relation := &model.Relation{
			OwnerID:         ownerID,
			SourceID:        source.ID,
			TargetID:        target.ID,
			Relation:        relationName,
			Description:     fmt.Sprintf("%s 与 %s 在文档《%s》的同一章节中共同构成知识图谱上下文。", source.Name, target.Name, defaultString(documentTitle, "未命名文档")),
			Weight:          0.52,
			Confidence:      0.52,
			Evidence:        truncate(defaultString(chunk.Summary, chunk.Content), 500),
			EvidenceChunkID: chunk.ID,
			DocumentID:      documentID,
		}
		_ = s.graphStore.SaveRelation(ctx, relation)
		if semanticDegree != nil {
			semanticDegree[source.ID]++
			semanticDegree[target.ID]++
		}
		count++
	}
	return count
}

func inferFallbackSemanticRelation(source, target string) string {
	joined := strings.ToLower(source + " " + target)
	switch {
	case strings.Contains(joined, "service") || strings.Contains(joined, "api") || strings.Contains(joined, "接口"):
		return "调用或支撑"
	case strings.Contains(joined, "表") || strings.Contains(joined, "库") || strings.Contains(joined, "database"):
		return "读写"
	case strings.Contains(joined, "任务") || strings.Contains(joined, "流程") || strings.Contains(joined, "步骤"):
		return "推进"
	default:
		return ""
	}
}

func fallbackTopicEntities(chunk model.Chunk) []extractedEntity {
	text := strings.TrimSpace(chunk.Summary + " " + chunk.Content)
	keywords := extractKeywords(text, 24)
	out := make([]extractedEntity, 0, 8)
	seen := map[string]bool{}
	for _, keyword := range keywords {
		name := normalizeFallbackTopicName(keyword)
		if name == "" || seen[canonicalEntityKey(name)] {
			continue
		}
		seen[canonicalEntityKey(name)] = true
		out = append(out, extractedEntity{
			Name:        name,
			Type:        "主题",
			Description: "文档章节中的核心主题词，用于辅助知识图谱可视化和后续人工审核",
			Aliases:     []string{name},
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func normalizeFallbackTopicName(keyword string) string {
	name := strings.Trim(strings.TrimSpace(keyword), "，。；;,.、:：()（）[]【】")
	name = cleanFallbackTopicPhrase(name)
	if name == "" || len([]rune(name)) < 3 {
		return ""
	}
	lower := strings.ToLower(name)
	if regexp.MustCompile(`^\d+$`).MatchString(lower) || regexp.MustCompile(`^p?\d+[-_]\d+$`).MatchString(lower) {
		return ""
	}
	blocked := map[string]bool{
		"文档": true, "内容": true, "信息": true, "数据": true, "图片": true, "文件": true, "页面": true,
		"用户": true, "系统": true, "示例": true, "步骤": true, "章节": true, "标题": true, "正文": true,
		"document": true, "content": true, "image": true, "file": true, "example": true,
	}
	if blocked[lower] || isGenericDocumentSectionEntity(name) || isGraphStopEntityName(name) {
		return ""
	}
	return name
}

func cleanFallbackTopicPhrase(name string) string {
	name = strings.TrimSpace(name)
	for _, prefix := range []string{"并", "且", "和", "与", "及", "以及"} {
		name = strings.TrimPrefix(name, prefix)
	}
	separators := []string{"包括", "包含", "使用", "采用", "生成", "产出", "输出", "依赖", "读取", "解析", "校验", "约束", "决定", "携带", "支撑", "支持", "面向", "用于", "需要"}
	for i := 0; i < 4; i++ {
		changed := false
		for _, separator := range separators {
			if idx := strings.LastIndex(name, separator); idx >= 0 {
				tail := strings.TrimSpace(name[idx+len(separator):])
				if len([]rune(tail)) >= 3 {
					name = tail
					changed = true
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	return strings.Trim(strings.TrimSpace(name), "，。；;,.、:：()（）[]【】")
}

func graphExtractionCandidates(chunks []model.Chunk) []model.Chunk {
	const maxParentCandidates = 36
	const maxChildFallbackCandidates = 36
	parents := make([]model.Chunk, 0)
	children := make([]model.Chunk, 0)
	for _, chunk := range chunks {
		content := strings.TrimSpace(chunk.Content)
		if len([]rune(content)) < 80 {
			continue
		}
		switch normalizedChunkLevel(chunk) {
		case model.ChunkLevelParent:
			enriched := chunk
			if summary := strings.TrimSpace(chunk.Summary); summary != "" && !strings.Contains(content, summary) {
				enriched.Content = strings.TrimSpace("章节摘要： " + summary + "\n\n章节正文：\n" + content)
			}
			parents = append(parents, enriched)
		case model.ChunkLevelChild:
			children = append(children, chunk)
		}
	}
	if len(parents) == 0 && len(children) == 0 {
		for _, chunk := range chunks {
			if strings.TrimSpace(chunk.Content) == "" {
				continue
			}
			if normalizedChunkLevel(chunk) == model.ChunkLevelParent {
				parents = append(parents, chunk)
			} else {
				children = append(children, chunk)
			}
			if len(parents)+len(children) >= 8 {
				break
			}
		}
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return parents[i].QualityScore > parents[j].QualityScore
	})
	sort.SliceStable(children, func(i, j int) bool {
		return children[i].QualityScore > children[j].QualityScore
	})
	if len(parents) > 0 {
		if len(parents) > maxParentCandidates {
			parents = parents[:maxParentCandidates]
		}
		return parents
	}
	if len(children) > maxChildFallbackCandidates {
		children = children[:maxChildFallbackCandidates]
	}
	return children
}

func (s *ragServiceImpl) upsertGraphEntity(ctx context.Context, ownerID int64, extracted extractedEntity) *model.Entity {
	if s == nil || s.graphStore == nil {
		return nil
	}
	name := strings.TrimSpace(extracted.Name)
	if name == "" {
		return nil
	}
	canonical := canonicalEntityKey(name)
	if canonical == "" {
		return nil
	}
	entity, err := s.graphStore.GetEntityByCanonicalKey(ctx, ownerID, canonical)
	if err != nil {
		return nil
	}
	if entity == nil {
		entity = &model.Entity{
			OwnerID:      ownerID,
			Name:         name,
			CanonicalKey: canonical,
			Type:         normalizeEntityType(extracted.Type, name),
			Summary:      defaultString(extracted.Description, "从文档分块中抽取的知识图谱实体"),
			AliasesJSON:  encodeAliases(mergeAliases(nil, append(extracted.Aliases, name)...)),
			Score:        1,
		}
	} else {
		entity.Score += 0.2
		entity.AliasesJSON = encodeAliases(mergeAliases(decodeAliases(entity.AliasesJSON), append(extracted.Aliases, name)...))
		if strings.TrimSpace(entity.Summary) == "" || entity.Summary == "从文档分块中抽取的知识图谱实体" {
			entity.Summary = defaultString(extracted.Description, entity.Summary)
		}
		if strings.TrimSpace(entity.Type) == "" || isGenericEntityType(entity.Type) {
			entity.Type = normalizeEntityType(extracted.Type, name)
		}
	}
	_ = s.graphStore.SaveEntity(ctx, entity)
	return entity
}

func (s *ragServiceImpl) upsertDocumentGraphEntity(ctx context.Context, ownerID, documentID int64, title string) *model.Entity {
	if s == nil || s.graphStore == nil || ownerID <= 0 || documentID <= 0 {
		return nil
	}
	name := strings.TrimSpace(title)
	if name == "" {
		name = fmt.Sprintf("文档 %d", documentID)
	}
	canonical := fmt.Sprintf("document:%d", documentID)
	entity, err := s.graphStore.GetEntityByCanonicalKey(ctx, ownerID, canonical)
	if err != nil {
		return nil
	}
	if entity == nil {
		entity = &model.Entity{
			OwnerID:      ownerID,
			Name:         name,
			CanonicalKey: canonical,
			Type:         "文档",
			Summary:      fmt.Sprintf("知识库文档《%s》的标题节点，用于承载该文档内抽取出的核心实体。", name),
			AliasesJSON:  encodeAliases([]string{name}),
			Score:        1.2,
		}
	} else {
		entity.Name = name
		entity.Type = "文档"
		entity.Score += 0.1
		entity.AliasesJSON = encodeAliases(mergeAliases(decodeAliases(entity.AliasesJSON), name))
		if strings.TrimSpace(entity.Summary) == "" {
			entity.Summary = fmt.Sprintf("知识库文档《%s》的标题节点，用于承载该文档内抽取出的核心实体。", name)
		}
	}
	_ = s.graphStore.SaveEntity(ctx, entity)
	return entity
}

type fallbackGraphExtractor struct {
	primary  graphExtractor
	fallback graphExtractor
}
