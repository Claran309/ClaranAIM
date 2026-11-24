package service

import (
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/logger"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var embeddingFallbackOnce sync.Once

type chunkPlan struct {
	parentTitle   string
	parentContent string
	parentSummary string
	children      []string
}

func buildHierarchicalChunks(input IngestInput, content string) ([]model.Chunk, error) {
	plans := planHierarchicalChunks(defaultString(input.SourceType, "text"), content)
	if len(plans) == 0 {
		return nil, errors.New("未能生成有效知识分块")
	}
	chunks := make([]model.Chunk, 0, len(plans)*3)
	index := 0
	for _, plan := range plans {
		parentID, err := idgen.NextID()
		if err != nil {
			return nil, err
		}
		parentContent := strings.TrimSpace(plan.parentContent)
		parentSummary := parentSummary(plan.parentTitle, plan.parentSummary, parentContent)
		parentKeywords := strings.Join(extractKeywords(plan.parentTitle+" "+parentSummary+" "+parentContent, 24), ",")
		chunks = append(chunks, model.Chunk{
			ID:            parentID,
			ParentChunkID: 0,
			ChunkLevel:    model.ChunkLevelParent,
			ChunkIndex:    index,
			Content:       parentContent,
			Summary:       parentSummary,
			Keywords:      parentKeywords,
			EmbeddingRef:  fmt.Sprintf("mysql://rag_chunks/%d/parent", parentID),
			QualityScore:  qualityScore(parentContent),
		})
		index++
		children := plan.children
		if len(children) == 0 {
			children = splitIntoChunks(parentContent, 420)
		}
		for _, child := range children {
			child = strings.TrimSpace(child)
			if child == "" {
				continue
			}
			chunks = append(chunks, model.Chunk{
				ParentChunkID: parentID,
				ChunkLevel:    model.ChunkLevelChild,
				ChunkIndex:    index,
				Content:       child,
				Summary:       parentSummary,
				Keywords:      strings.Join(extractKeywords(plan.parentTitle+" "+child, 16), ","),
				EmbeddingRef:  fmt.Sprintf("milvus://claran_rag_chunks/pending/%d", index),
				QualityScore:  qualityScore(child),
			})
			index++
		}
	}
	return chunks, nil
}

func planHierarchicalChunks(sourceType, content string) []chunkPlan {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "md", "markdown":
		return planMarkdownChunks(content)
	case "go", "code", "golang":
		return planGoCodeChunks(content)
	case "conversation", "chat":
		return planConversationChunks(content)
	default:
		return planParagraphChunks(content)
	}
}

func planMarkdownChunks(content string) []chunkPlan {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	documentTitle := ""
	type section struct {
		title string
		lines []string
	}
	var sections []section
	current := section{title: "正文"}
	flush := func() {
		body := strings.TrimSpace(strings.Join(current.lines, "\n"))
		if body == "" && strings.TrimSpace(current.title) == "" {
			return
		}
		sections = append(sections, current)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && documentTitle == "" {
			documentTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			flush()
			current = section{title: strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))}
			continue
		}
		current.lines = append(current.lines, line)
	}
	flush()
	plans := make([]chunkPlan, 0, len(sections))
	for _, sec := range sections {
		body := strings.TrimSpace(strings.Join(sec.lines, "\n"))
		if body == "" {
			continue
		}
		title := strings.TrimSpace(sec.title)
		if title == "" || title == "正文" {
			title = defaultString(documentTitle, "正文")
		}
		parentContent := strings.TrimSpace("## " + title + "\n" + body)
		children := splitMarkdownChildren(body, title)
		plans = append(plans, chunkPlan{parentTitle: title, parentContent: parentContent, children: children})
	}
	return plans
}

func splitMarkdownChildren(body, title string) []string {
	var children []string
	var current []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(current, "\n"))
		if text != "" {
			children = append(children, splitIntoChunks(text, 420)...)
		}
		current = nil
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			flush()
			current = append(current, "## "+title, line)
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return children
}

func planParagraphChunks(content string) []chunkPlan {
	paragraphs := splitParagraphs(content)
	if len(paragraphs) == 0 {
		return nil
	}
	var plans []chunkPlan
	var current []string
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		text := strings.Join(current, "\n\n")
		plans = append(plans, chunkPlan{parentTitle: firstLine(text, "段落"), parentContent: text, children: splitIntoChunks(text, 420)})
		current = nil
		currentLen = 0
	}
	for _, paragraph := range paragraphs {
		l := len([]rune(paragraph))
		if currentLen > 0 && currentLen+l > 1500 {
			flush()
		}
		current = append(current, paragraph)
		currentLen += l
	}
	flush()
	return plans
}

func splitParagraphs(content string) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	parts := regexp.MustCompile(`\n\s*\n`).Split(normalized, -1)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, splitLongParagraph(part, 1100)...)
		}
	}
	if len(out) == 0 && strings.TrimSpace(content) != "" {
		out = splitIntoChunks(content, 1200)
	}
	return out
}

func splitLongParagraph(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 1100
	}
	if len([]rune(text)) <= maxRunes {
		return []string{text}
	}
	lines := strings.Split(text, "\n")
	nonEmptyLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}
	if len(nonEmptyLines) <= 1 {
		return splitIntoChunks(text, maxRunes)
	}
	out := make([]string, 0, len(nonEmptyLines)/4+1)
	var current []string
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, strings.Join(current, "\n"))
		current = nil
		currentLen = 0
	}
	for _, line := range nonEmptyLines {
		lineLen := len([]rune(line))
		if currentLen > 0 && currentLen+lineLen > maxRunes {
			flush()
		}
		if lineLen > maxRunes {
			flush()
			out = append(out, splitIntoChunks(line, maxRunes)...)
			continue
		}
		current = append(current, line)
		currentLen += lineLen
	}
	flush()
	return out
}

func planConversationChunks(content string) []chunkPlan {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var messages []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			messages = append(messages, line)
		}
	}
	if len(messages) == 0 {
		return nil
	}
	var plans []chunkPlan
	for start := 0; start < len(messages); start += 20 {
		end := start + 20
		if end > len(messages) {
			end = len(messages)
		}
		parent := strings.Join(messages[start:end], "\n")
		children := make([]string, 0, end-start)
		for i := start; i < end; i += 3 {
			j := i + 3
			if j > end {
				j = end
			}
			children = append(children, strings.Join(messages[i:j], "\n"))
		}
		plans = append(plans, chunkPlan{parentTitle: fmt.Sprintf("聊天记录 %d-%d", start+1, end), parentContent: parent, children: children})
	}
	return plans
}

func planGoCodeChunks(content string) []chunkPlan {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var prelude []string
	var pendingComments []string
	var blocks []chunkPlan
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if isGoCommentLine(line) {
			pendingComments = append(pendingComments, lines[i])
			i++
			continue
		}
		if line == "" {
			if len(pendingComments) > 0 {
				pendingComments = append(pendingComments, lines[i])
			} else {
				prelude = append(prelude, lines[i])
			}
			i++
			continue
		}
		if isGoDeclarationStart(line) {
			blockLines := append([]string{}, pendingComments...)
			pendingComments = nil
			braceBalance := strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
			blockLines = append(blockLines, lines[i])
			i++
			for braceBalance > 0 && i < len(lines) {
				braceBalance += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
				blockLines = append(blockLines, lines[i])
				i++
			}
			block := strings.TrimSpace(strings.Join(blockLines, "\n"))
			blocks = append(blocks, chunkPlan{parentTitle: firstLine(block, "代码块"), parentContent: block, children: splitCodeChildren(block)})
			continue
		}
		if len(pendingComments) > 0 {
			prelude = append(prelude, pendingComments...)
			pendingComments = nil
		}
		if strings.HasPrefix(line, "package ") || strings.HasPrefix(line, "import ") {
			prelude = append(prelude, lines[i])
		}
		i++
	}
	if len(pendingComments) > 0 {
		prelude = append(prelude, pendingComments...)
	}
	if text := strings.TrimSpace(strings.Join(prelude, "\n")); text != "" {
		blocks = append([]chunkPlan{{parentTitle: "package/import", parentContent: text, children: splitIntoChunks(text, 420)}}, blocks...)
	}
	if len(blocks) == 0 {
		return planParagraphChunks(content)
	}
	return blocks
}

func isGoCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*")
}

func isGoDeclarationStart(line string) bool {
	return strings.HasPrefix(line, "func ") ||
		strings.HasPrefix(line, "type ") ||
		strings.HasPrefix(line, "var ") ||
		strings.HasPrefix(line, "const ")
}

func splitCodeChildren(block string) []string {
	if len([]rune(block)) <= 900 {
		return []string{block}
	}
	return splitIntoChunks(block, 700)
}

func parentSummary(title, existing, content string) string {
	if strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing)
	}
	paragraphs := splitParagraphs(content)
	if len(paragraphs) == 0 {
		return truncate(title, 240)
	}
	summary := strings.TrimSpace(paragraphs[0])
	if title != "" && !strings.Contains(summary, title) {
		summary = title + "： " + summary
	}
	return truncate(summary, 420)
}

func countChildChunks(chunks []model.Chunk) int {
	count := 0
	for _, chunk := range chunks {
		if normalizedChunkLevel(chunk) == model.ChunkLevelChild {
			count++
		}
	}
	return count
}

func normalizedChunkLevel(chunk model.Chunk) string {
	if strings.TrimSpace(chunk.ChunkLevel) == "" {
		return model.ChunkLevelChild
	}
	return chunk.ChunkLevel
}

func (s *ragServiceImpl) embedding(ctx context.Context, text string) []float32 {
	if s.embedder != nil {
		if vector, err := s.embedder.Embed(ctx, text); err == nil && len(vector) > 0 {
			return normalizeVectorDim(vector, s.embeddingDim)
		} else {
			embeddingFallbackOnce.Do(func() {
				logger.Warn("RAG embedding调用失败，已回退hash；该模式仅适用于测试/离线评估", "error", err)
			})
		}
	} else {
		embeddingFallbackOnce.Do(func() {
			logger.Warn("RAG embedding未配置，已回退hash；该模式仅适用于测试/离线评估")
		})
	}
	return hashEmbedding(text, s.embeddingDim)
}

func normalizeVectorDim(vector []float32, dim int) []float32 {
	if dim <= 0 || len(vector) == dim {
		return vector
	}
	out := make([]float32, dim)
	copy(out, vector)
	return out
}

type graphExtractInput struct {
	DocumentID        int64
	Chunk             model.Chunk
	GraphRelationMode string
}

type extractedEntity struct {
	Name        string
	Type        string
	Description string
	Aliases     []string
}

type extractedRelationship struct {
	Source      string
	Target      string
	Type        string
	Description string
	Evidence    string
	Confidence  float64
}

type graphExtractResult struct {
	Entities      []extractedEntity
	Relationships []extractedRelationship
}

type graphEntityMention struct {
	Name  string
	Start int
	End   int
}

type graphRelationTrigger struct {
	Type  string
	Index int
	End   int
}

type graphExtractor interface {
	Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error)
}

type graphCommunitySummarizer interface {
	Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error)
}

type ruleGraphExtractor struct{}
