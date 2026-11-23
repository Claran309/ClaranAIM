package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func (s *ragServiceImpl) hybridRetrieve(ctx context.Context, input SearchInput, limit int) ([]rankedChunk, error) {
	rowLimit := 300
	if input.DocumentID > 0 {
		rowLimit = 1500
	}
	rows, err := s.repo.ListChunks(ctx, dao.SearchFilter{ViewerID: input.ViewerID, GroupID: input.GroupID, ConversationID: input.ConversationID, DocumentID: input.DocumentID, Limit: rowLimit})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	childRows, parentRows := splitParentChildRows(rows)
	if len(childRows) == 0 {
		childRows = rows
	}
	queryVector := s.embedding(ctx, input.Query)
	vectorHits, _ := s.vectorIndex.Search(ctx, queryVector, limit)
	rowByChunkID := make(map[int64]dao.ChunkWithDocument, len(childRows))
	for _, row := range childRows {
		rowByChunkID[row.Chunk.ID] = row
	}
	denseRank := map[int64]int{}
	denseScore := map[int64]float64{}
	for _, hit := range vectorHits {
		if _, ok := rowByChunkID[hit.ChunkID]; !ok {
			continue
		}
		denseScore[hit.ChunkID] = hit.Score
		if _, exists := denseRank[hit.ChunkID]; !exists {
			denseRank[hit.ChunkID] = len(denseRank) + 1
		}
	}
	if len(denseRank) == 0 {
		denseRows := make([]rankedChunk, 0, len(childRows))
		for _, row := range childRows {
			score := cosine(queryVector, s.embedding(ctx, row.Chunk.Content))
			denseRows = append(denseRows, rankedChunk{Chunk: row.Chunk, Document: row.Document, Score: score})
		}
		sort.Slice(denseRows, func(i, j int) bool { return denseRows[i].Score > denseRows[j].Score })
		if len(denseRows) > limit {
			denseRows = denseRows[:limit]
		}
		for idx, row := range denseRows {
			denseRank[row.Chunk.ID] = idx + 1
			denseScore[row.Chunk.ID] = row.Score
		}
	}
	bm25Rows := bm25RankChunks(input.Query, childRows)
	if len(bm25Rows) > limit {
		bm25Rows = bm25Rows[:limit]
	}
	bm25Rank := map[int64]int{}
	bm25Score := map[int64]float64{}
	for idx, row := range bm25Rows {
		bm25Rank[row.Chunk.ID] = idx + 1
		bm25Score[row.Chunk.ID] = row.Score
	}
	candidates := map[int64]dao.ChunkWithDocument{}
	for chunkID := range denseRank {
		candidates[chunkID] = rowByChunkID[chunkID]
	}
	for _, row := range bm25Rows {
		candidates[row.Chunk.ID] = rowByChunkID[row.Chunk.ID]
	}
	out := make([]rankedChunk, 0, len(candidates))
	for _, row := range childRows {
		if _, ok := candidates[row.Chunk.ID]; !ok {
			continue
		}
		hierarchy := 0.05
		if input.GroupID > 0 && row.Document.GroupID == input.GroupID {
			hierarchy += 0.08
		}
		if input.ConversationID > 0 && row.Document.ConversationID == input.ConversationID {
			hierarchy += 0.08
		}
		score := reciprocalRankFusionScore(denseRank[row.Chunk.ID], bm25Rank[row.Chunk.ID]) + 0.01*hierarchy
		out = append(out, rankedChunk{
			Chunk:    row.Chunk,
			Document: row.Document,
			Score:    score,
			Reason: fmt.Sprintf(
				"hybrid rrf=%.4f dense_rank=%s dense=%.3f bm25_rank=%s bm25=%.3f",
				score,
				formatRank(denseRank[row.Chunk.ID]),
				denseScore[row.Chunk.ID],
				formatRank(bm25Rank[row.Chunk.ID]),
				bm25Score[row.Chunk.ID],
			),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	out = groupChildHitsByParent(out, parentRows)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func splitParentChildRows(rows []dao.ChunkWithDocument) ([]dao.ChunkWithDocument, map[int64]dao.ChunkWithDocument) {
	children := make([]dao.ChunkWithDocument, 0, len(rows))
	parents := map[int64]dao.ChunkWithDocument{}
	for _, row := range rows {
		switch normalizedChunkLevel(row.Chunk) {
		case model.ChunkLevelParent:
			parents[row.Chunk.ID] = row
		default:
			children = append(children, row)
		}
	}
	return children, parents
}

func groupChildHitsByParent(children []rankedChunk, parents map[int64]dao.ChunkWithDocument) []rankedChunk {
	grouped := make([]rankedChunk, 0, len(children))
	seen := map[int64]bool{}
	for _, child := range children {
		parentID := child.Chunk.ParentChunkID
		if parentID == 0 {
			parentID = child.Chunk.ID
		}
		if seen[parentID] {
			continue
		}
		seen[parentID] = true
		parentChunk := child.Chunk
		if parentRow, ok := parents[parentID]; ok {
			parentChunk = parentRow.Chunk
		}
		parentChunk.Content = parentContextContent(parentChunk, child.Chunk)
		reason := child.Reason + fmt.Sprintf("; child_chunk_id=%d parent_chunk_id=%d", child.Chunk.ID, parentID)
		grouped = append(grouped, rankedChunk{
			Chunk:    parentChunk,
			Document: child.Document,
			Score:    child.Score,
			Reason:   reason,
		})
	}
	return grouped
}

func parentContextContent(parentChunk, hitChild model.Chunk) string {
	summary := strings.TrimSpace(parentChunk.Summary)
	parentContent := strings.TrimSpace(parentChunk.Content)
	childContent := strings.TrimSpace(hitChild.Content)
	if parentContent == "" {
		return childContent
	}
	if len([]rune(parentContent)) <= 1200 {
		if summary != "" {
			return summary + "\n\n" + parentContent
		}
		return parentContent
	}
	var parts []string
	if summary != "" {
		parts = append(parts, "父块摘要：\n"+summary)
	}
	if childContent != "" {
		parts = append(parts, "命中小块：\n"+childContent)
	}
	if len(parts) == 0 {
		return truncate(parentContent, 1200)
	}
	return strings.Join(parts, "\n\n")
}

func reciprocalRankFusionScore(denseRank, bm25Rank int) float64 {
	const k = 60.0
	score := 0.0
	if denseRank > 0 {
		score += 1 / (k + float64(denseRank))
	}
	if bm25Rank > 0 {
		score += 1 / (k + float64(bm25Rank))
	}
	return score
}

func formatRank(rank int) string {
	if rank <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", rank)
}

func bm25RankChunks(query string, rows []dao.ChunkWithDocument) []rankedChunk {
	queryTerms := bm25Tokens(query)
	if len(queryTerms) == 0 || len(rows) == 0 {
		return nil
	}
	docFreq := map[string]int{}
	docTokens := make([][]string, len(rows))
	totalLen := 0
	for i, row := range rows {
		tokens := bm25Tokens(row.Document.Title + " " + row.Chunk.Content + " " + row.Chunk.Keywords)
		docTokens[i] = tokens
		totalLen += len(tokens)
		seen := map[string]bool{}
		for _, token := range tokens {
			if !seen[token] {
				docFreq[token]++
				seen[token] = true
			}
		}
	}
	avgDocLen := float64(totalLen) / float64(len(rows))
	if avgDocLen <= 0 {
		avgDocLen = 1
	}
	const k1 = 1.5
	const b = 0.75
	n := float64(len(rows))
	out := make([]rankedChunk, 0, len(rows))
	for i, row := range rows {
		tokens := docTokens[i]
		if len(tokens) == 0 {
			continue
		}
		tf := map[string]int{}
		for _, token := range tokens {
			tf[token]++
		}
		score := 0.0
		docLen := float64(len(tokens))
		for _, term := range queryTerms {
			freq := float64(tf[term])
			if freq == 0 {
				continue
			}
			df := float64(docFreq[term])
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			denominator := freq + k1*(1-b+b*docLen/avgDocLen)
			score += idf * (freq * (k1 + 1)) / denominator
		}
		if score > 0 {
			out = append(out, rankedChunk{Chunk: row.Chunk, Document: row.Document, Score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func bm25Tokens(text string) []string {
	raw := wordRe.FindAllString(strings.ToLower(text), -1)
	out := make([]string, 0, len(raw)*2)
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		out = append(out, token)
		runes := []rune(token)
		if len(runes) > 2 && containsHan(token) {
			for i := 0; i < len(runes)-1; i++ {
				out = append(out, string(runes[i:i+2]))
			}
		}
	}
	return out
}

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func chunksToSources(chunks []rankedChunk) []*rag.RAGSource {
	out := make([]*rag.RAGSource, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, &rag.RAGSource{DocumentId: chunk.Document.ID, ChunkId: chunk.Chunk.ID, Title: chunk.Document.Title, Content: truncate(chunk.Chunk.Content, 500), Source: chunk.Document.Source, Score: chunk.Score, Reason: chunk.Reason})
	}
	return out
}

func splitIntoChunks(text string, size int) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	var chunks []string
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[start:end])))
	}
	return chunks
}

var wordRe = regexp.MustCompile(`[A-Za-z0-9_\-\p{Han}]{2,}`)

func extractKeywords(text string, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for _, match := range wordRe.FindAllString(strings.ToLower(text), -1) {
		match = strings.TrimSpace(match)
		if match == "" || seen[match] {
			continue
		}
		seen[match] = true
		out = append(out, match)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func keywordScore(terms []string, text string) float64 {
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(text)
	hits := 0
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			hits++
		}
	}
	return float64(hits) / float64(len(terms))
}

func hashEmbedding(text string, dim int) []float32 {
	vec := make([]float32, dim)
	for _, token := range extractKeywords(text, 0) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum32()) % dim
		vec[idx] += 1
	}
	norm := float32(0)
	for _, v := range vec {
		norm += v * v
	}
	if norm == 0 {
		return vec
	}
	norm = float32(math.Sqrt(float64(norm)))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func cosine(a, b []float32) float64 {
	n := min(len(a), len(b))
	sum := float64(0)
	for i := 0; i < n; i++ {
		sum += float64(a[i] * b[i])
	}
	return sum
}
