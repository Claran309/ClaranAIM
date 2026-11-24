// Command mockllm 提供 OpenAI 兼容的本地 mock 服务。
// 它用于 ClaranAIM 压测：固定延迟、固定 token、确定性输出，避免真实模型费用和波动。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type server struct {
	addr       string
	latency    time.Duration
	dim        int
	promptTok  int
	outputTok  int
	errorRate  float64
	requests   atomic.Int64
	embeddings atomic.Int64
}

func main() {
	s := &server{}
	flag.StringVar(&s.addr, "addr", "127.0.0.1:18090", "listen address")
	latencyMS := flag.Int("latency-ms", envInt("MOCK_LLM_LATENCY_MS", 150), "fixed response latency in ms")
	flag.IntVar(&s.dim, "dim", envInt("MOCK_EMBEDDING_DIM", 256), "embedding dimension")
	flag.IntVar(&s.promptTok, "prompt-tokens", envInt("MOCK_PROMPT_TOKENS", 200), "reported prompt tokens")
	flag.IntVar(&s.outputTok, "output-tokens", envInt("MOCK_OUTPUT_TOKENS", 40), "reported output tokens")
	flag.Float64Var(&s.errorRate, "error-rate", envFloat("MOCK_ERROR_RATE", 0), "fraction of requests answered with 500")
	flag.Parse()
	s.latency = time.Duration(*latencyMS) * time.Millisecond

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/chat/completions", s.handleChat)
	mux.HandleFunc("/v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("/embeddings", s.handleEmbeddings)

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("mockllm listening on %s latency=%s dim=%d", s.addr, s.latency, s.dim)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (s *server) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"requests":   s.requests.Load(),
		"embeddings": s.embeddings.Load(),
		"latency_ms": s.latency.Milliseconds(),
		"dim":        s.dim,
	})
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.requests.Add(1)
	time.Sleep(s.latency)
	if s.shouldFail() {
		http.Error(w, `{"error":{"message":"mock injected failure"}}`, http.StatusInternalServerError)
		return
	}
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Model == "" {
		req.Model = "mock-gpt"
	}
	content := mockAnswer(req.Messages)
	if req.Stream {
		writeStream(w, req.Model, content, s.promptTok, s.outputTok)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     s.promptTok,
			"completion_tokens": s.outputTok,
			"total_tokens":      s.promptTok + s.outputTok,
		},
	})
}

func (s *server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.embeddings.Add(1)
	time.Sleep(s.latency)
	if s.shouldFail() {
		http.Error(w, `{"error":{"message":"mock injected failure"}}`, http.StatusInternalServerError)
		return
	}
	var req struct {
		Model string          `json:"model"`
		Input json.RawMessage `json:"input"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Model == "" {
		req.Model = "mock-embedding"
	}
	texts := decodeInputs(req.Input)
	data := make([]map[string]any, 0, len(texts))
	for i, text := range texts {
		data = append(data, map[string]any{
			"object":    "embedding",
			"index":     i,
			"embedding": deterministicVector(text, s.dim),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
		"model":  req.Model,
		"usage":  map[string]int{"prompt_tokens": len(texts) * 8, "total_tokens": len(texts) * 8},
	})
}

func (s *server) shouldFail() bool {
	if s.errorRate <= 0 {
		return false
	}
	return float64(time.Now().UnixNano()%1000)/1000.0 < s.errorRate
}

// deterministicVector 用文本哈希生成稳定的单位向量。
// 同一文本在任意轮次都得到相同向量，保证 RAG 结果可复现。
func deterministicVector(text string, dim int) []float32 {
	if dim <= 0 {
		dim = 256
	}
	vec := make([]float32, dim)
	var h uint64 = 1469598103934665603
	for i := 0; i < len(text); i++ {
		h ^= uint64(text[i])
		h *= 1099511628211
	}
	var norm float64
	for i := 0; i < dim; i++ {
		h ^= h << 13
		h ^= h >> 7
		h ^= h << 17
		v := float64(int64(h%2000)-1000) / 1000.0
		vec[i] = float32(v)
		norm += v * v
	}
	if norm == 0 {
		vec[0] = 1
		return vec
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec
}

func mockAnswer(messages []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}) string {
	last := ""
	for _, m := range messages {
		if strings.TrimSpace(m.Content) != "" {
			last = m.Content
		}
	}
	if len(last) > 80 {
		last = last[:80]
	}
	return "mock-answer: " + last
}

func decodeInputs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return []string{""}
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
		return many
	}
	return []string{string(raw)}
}

func writeStream(w http.ResponseWriter, model, content string, promptTok, outputTok int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	chunks := splitChunks(content, 16)
	for i, chunk := range chunks {
		payload := map[string]any{
			"id":      "chatcmpl-mock-stream",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]string{"content": chunk},
			}},
		}
		if i == 0 {
			payload["choices"].([]map[string]any)[0]["delta"] = map[string]string{"role": "assistant", "content": chunk}
		}
		encoded, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}
	final := map[string]any{
		"id":      "chatcmpl-mock-stream",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "delta": map[string]string{}, "finish_reason": "stop"}},
		"usage":   map[string]int{"prompt_tokens": promptTok, "completion_tokens": outputTok, "total_tokens": promptTok + outputTok},
	}
	encoded, _ := json.Marshal(final)
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
	flusher.Flush()
}

func splitChunks(text string, size int) []string {
	if size <= 0 {
		size = 16
	}
	runes := []rune(text)
	var out []string
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	}
	return fallback
}
