// Command loadgen 是 ClaranAIM 的本地压测器。
// 不依赖 k6：直接发 HTTP、维持 WebSocket、统计分位并落盘原始结果。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type manifest struct {
	RunID   string              `json:"run_id"`
	Users   []manifestUser      `json:"users"`
	Private []manifestConv      `json:"private_conversations"`
	Groups  []manifestGroupConv `json:"group_conversations"`
}

type manifestUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type manifestConv struct {
	ID           int64   `json:"id"`
	Participants []int64 `json:"participants"`
}

type manifestGroupConv struct {
	ID           int64   `json:"id"`
	GroupID      int64   `json:"group_id"`
	Participants []int64 `json:"participants"`
}

type options struct {
	baseURL      string
	wsURL        string
	scenario     string
	runID        string
	password     string
	tokenFile    string
	concurrency  int
	duration     time.Duration
	warmup       time.Duration
	rate         int
	outDir       string
	manifest     string
	messageLen   int
	useWS        bool
	stopAt       time.Time
	groupLimit   int
	privateLimit int
	count        int64
	agentID      string
	docKB        int
}

type recorder struct {
	mu        sync.Mutex
	latencies []time.Duration
	statuses  map[int]int64
	errors    map[string]int64
	sent      atomic.Int64
	ok        atomic.Int64
	failed    atomic.Int64
	wsRecv    atomic.Int64
	wsDup     atomic.Int64
	wsSeen    sync.Map
}

func newRecorder() *recorder {
	return &recorder{statuses: map[int]int64{}, errors: map[string]int64{}}
}

func (r *recorder) record(status int, latency time.Duration, errMsg string) {
	r.mu.Lock()
	r.latencies = append(r.latencies, latency)
	r.statuses[status]++
	if errMsg != "" {
		r.errors[errMsg]++
	}
	r.mu.Unlock()
}

func main() {
	opt := options{}
	flag.StringVar(&opt.baseURL, "base", "http://127.0.0.1:18080", "api gateway base url")
	flag.StringVar(&opt.wsURL, "ws", "ws://127.0.0.1:8081/ws", "websocket url")
	flag.StringVar(&opt.scenario, "scenario", "c3", "c1|c2|c3|c4|ws")
	flag.StringVar(&opt.runID, "run-id", "", "run id; defaults to scenario+timestamp")
	flag.StringVar(&opt.password, "password", envOr("BENCH_PASSWORD", "bench-local-pass"), "bench user password")
	flag.StringVar(&opt.tokenFile, "tokens", "", "pre-generated token file (one token per line)")
	flag.IntVar(&opt.concurrency, "concurrency", 50, "concurrent workers / VUs")
	flag.DurationVar(&opt.duration, "duration", 5*time.Minute, "measurement duration")
	flag.DurationVar(&opt.warmup, "warmup", time.Minute, "warmup duration")
	flag.IntVar(&opt.rate, "rate", 0, "optional target requests/sec (0 = closed loop)")
	flag.StringVar(&opt.outDir, "out", "bench/results", "result directory")
	flag.StringVar(&opt.manifest, "manifest", "bench/results/seed/manifest.json", "seed manifest path")
	flag.IntVar(&opt.messageLen, "message-len", 64, "message body length")
	flag.BoolVar(&opt.useWS, "ws-recv", false, "open websocket per worker and count deliveries")
	flag.IntVar(&opt.groupLimit, "group-limit", 0, "limit groups used by c4 (0 = all)")
	flag.IntVar(&opt.privateLimit, "private-limit", 0, "limit private conversations used by c2/c3 (0 = all)")
	flag.Int64Var(&opt.count, "count", 0, "stop after N requests across all workers (0 = unlimited)")
	flag.StringVar(&opt.agentID, "agent-id", "", "agent id for -scenario agent")
	flag.IntVar(&opt.docKB, "doc-kb", 64, "document size in KB for -scenario rag-ingest")
	flag.Parse()

	if opt.runID == "" {
		opt.runID = fmt.Sprintf("AIM-P1-%s-%s", strings.ToUpper(opt.scenario), time.Now().Format("20060102-1504"))
	}
	m, err := loadManifest(opt.manifest)
	if err != nil {
		log.Fatalf("load manifest: %v", err)
	}
	if len(m.Users) == 0 {
		log.Fatal("manifest has no users")
	}

	runDir := filepath.Join(opt.outDir, opt.runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// Windows 临时端口有限；压测器保持少量长连接复用，
			// 避免高并发下先撞 WSAENOBUFS / 端口耗尽而不是服务瓶颈。
			MaxIdleConns:        128,
			MaxIdleConnsPerHost: 128,
			MaxConnsPerHost:     128,
			IdleConnTimeout:     120 * time.Second,
			DisableKeepAlives:   false,
		},
	}
	rec := newRecorder()
	tokens, err := loadTokens(opt.tokenFile)
	if err != nil {
		log.Fatalf("load tokens: %v", err)
	}
	if len(tokens) > 0 {
		log.Printf("using %d pre-generated tokens", len(tokens))
	}

	log.Printf("scenario=%s run=%s concurrency=%d duration=%s warmup=%s", opt.scenario, opt.runID, opt.concurrency, opt.duration, opt.warmup)
	warmCtx, warmCancel := context.WithCancel(context.Background())
	warmOpt := opt
	// warmup 前缀不能以 opt.runID 开头，否则会污染正式对账的 LIKE 'bench-<runID>-%'。
	warmOpt.runID = "warmup-" + opt.runID
	warmOpt.stopAt = time.Now().Add(opt.warmup)
	runScenario(warmCtx, client, warmOpt, m, tokens, rec, false)
	warmCancel()
	rec = newRecorder()

	// 用 stopAt 停止发新请求，而不是取消 context：
	// 否则测试结束瞬间的在途请求会被客户端取消，服务端误记为 500。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opt.stopAt = time.Now().Add(opt.duration)
	start := time.Now()
	runScenario(ctx, client, opt, m, tokens, rec, opt.useWS)
	elapsed := time.Since(start)

	summary := buildSummary(opt, rec, elapsed, start)
	writeSummary(runDir, summary)
	printSummary(summary)
}

func runScenario(ctx context.Context, client *http.Client, opt options, m manifest, tokens []string, rec *recorder, withWS bool) {
	var wg sync.WaitGroup
	for i := 0; i < opt.concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			runWorker(ctx, client, opt, m, tokens, rec, idx, withWS)
		}(i)
	}
	wg.Wait()
}

func runWorker(ctx context.Context, client *http.Client, opt options, m manifest, tokens []string, rec *recorder, idx int, withWS bool) {
	var conv manifestConv
	private := m.Private
	if opt.privateLimit > 0 && len(private) > opt.privateLimit {
		private = private[:opt.privateLimit]
	}
	if len(private) > 0 {
		conv = private[idx%len(private)]
	}
	if opt.scenario == "agent" && len(private) > 0 {
		conv = private[0]
	}
	var group manifestGroupConv
	groups := m.Groups
	if opt.groupLimit > 0 && len(groups) > opt.groupLimit {
		groups = groups[:opt.groupLimit]
	}
	if len(groups) > 0 {
		group = groups[idx%len(groups)]
	}

	userIndex := idx % len(m.Users)
	user := m.Users[userIndex]
	// Agent / RAG 场景的资源属于第一个 bench 用户，必须统一用其 token，
	// 否则会返回“无权操作该 Agent”或看不到私有文档。
	if opt.scenario == "agent" || opt.scenario == "rag-ingest" || opt.scenario == "rag-search" {
		userIndex = 0
		user = m.Users[0]
	}
	// 压测必须用会话参与者的身份发消息，否则 msg-core 会返回“无权访问该会话”。
	if len(conv.Participants) > 0 && opt.scenario != "agent" {
		participantID := conv.Participants[idx%len(conv.Participants)]
		if mapped, ok := userIndexByID(m, participantID); ok {
			userIndex = mapped
			user = m.Users[mapped]
		}
	}
	if opt.scenario == "c4" && len(group.Participants) > 0 {
		// 群聊发送者必须是该群成员，否则会被 msg-core 拒绝。
		senderID := group.Participants[idx%len(group.Participants)]
		if mapped, ok := userIndexByID(m, senderID); ok {
			userIndex = mapped
			user = m.Users[mapped]
		}
	}

	var token string
	if len(tokens) > 0 {
		token = tokens[userIndex%len(tokens)]
	} else {
		var err error
		token, err = login(ctx, client, opt.baseURL, user.Username, opt.password)
		if err != nil {
			rec.record(0, 0, "login: "+err.Error())
			return
		}
	}

	stopWS := make(chan struct{})
	if withWS {
		go readWS(ctx, opt.wsURL, token, rec, stopWS)
		defer close(stopWS)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for seq := 0; ; seq++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !opt.stopAt.IsZero() && !time.Now().Before(opt.stopAt) {
			return
		}
		if opt.count > 0 && rec.sent.Load() >= opt.count {
			return
		}
		switch opt.scenario {
		case "c1":
			doGet(ctx, client, rec, opt.baseURL+"/api/v1/user/info", token)
		case "c2":
			if conv.ID != 0 {
				doGet(ctx, client, rec, fmt.Sprintf("%s/api/v1/message/history/%d?limit=50", opt.baseURL, conv.ID), token)
			}
			doGet(ctx, client, rec, opt.baseURL+"/api/v1/message/conversations", token)
		case "c4":
			if group.ID != 0 {
				sendMessage(ctx, client, rec, opt, token, group.ID, idx, seq, opt.runID)
			}
		case "ws":
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		case "agent":
			doAgent(ctx, client, rec, opt, token, conv.ID, idx, seq)
		case "rag-ingest":
			doRagIngest(ctx, client, rec, opt, token, idx, seq)
		case "rag-search":
			doRagSearch(ctx, client, rec, opt, token, idx, seq)
		default:
			if conv.ID != 0 {
				sendMessage(ctx, client, rec, opt, token, conv.ID, idx, seq, opt.runID)
			}
		}
		if opt.rate > 0 {
			// -rate 是全局目标速率；每个 worker 的间隔按并发数平摊。
			interval := time.Duration(float64(opt.concurrency) / float64(opt.rate) * float64(time.Second))
			if interval <= 0 {
				interval = time.Millisecond
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}
}

func userIndexByID(m manifest, id int64) (int, bool) {
	for i, u := range m.Users {
		if u.ID == id {
			return i, true
		}
	}
	return 0, false
}

func login(ctx context.Context, client *http.Client, base, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/user/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var decoded struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
			Token       string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("decode login: %w body=%s", err, truncate(string(raw)))
	}
	token := decoded.Data.AccessToken
	if token == "" {
		token = decoded.Data.Token
	}
	if token == "" {
		return "", fmt.Errorf("no token status=%d body=%s", resp.StatusCode, truncate(string(raw)))
	}
	return token, nil
}

func doGet(ctx context.Context, client *http.Client, rec *recorder, target, token string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, 0, "request: "+err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, latency, classify(err))
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	rec.sent.Add(1)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		rec.ok.Add(1)
	} else {
		rec.failed.Add(1)
	}
	rec.record(resp.StatusCode, latency, "")
}

func sendMessage(ctx context.Context, client *http.Client, rec *recorder, opt options, token string, convID int64, worker, seq int, runID string) {
	clientMsgID := fmt.Sprintf("%s-%d-%d", runID, worker, seq)
	content := fmt.Sprintf("bench-%s-w%d-s%d-%s", runID, worker, seq, strings.Repeat("x", opt.messageLen))
	body, _ := json.Marshal(map[string]any{
		"conversation_id": fmt.Sprintf("%d", convID),
		"content":         content,
		"msg_type":        "text",
		"client_msg_id":   clientMsgID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opt.baseURL+"/api/v1/message/send", bytes.NewReader(body))
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, 0, "request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, latency, classify(err))
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	rec.sent.Add(1)
	// 网关对部分业务失败仍返回 HTTP 200 + success:false；
	// 压测必须按业务成功判定，否则对账会虚高。
	businessOK := resp.StatusCode >= 200 && resp.StatusCode < 300
	if businessOK {
		var decoded struct {
			Code int `json:"code"`
			Data struct {
				Success bool `json:"success"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &decoded); err == nil {
			if decoded.Code != 0 || (decoded.Data.Success == false && strings.Contains(string(raw), `"success"`)) {
				businessOK = false
			}
		}
	}
	if businessOK {
		rec.ok.Add(1)
	} else {
		rec.failed.Add(1)
	}
	rec.record(resp.StatusCode, latency, statusMessage(resp.StatusCode, raw, businessOK))
}

func readWS(ctx context.Context, wsURL, token string, rec *recorder, stop <-chan struct{}) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		rec.record(0, 0, "ws dial: "+err.Error())
		return
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Time{})
	// 每个连接单独去重：同一条群消息投递给多个在线成员是正常扇出，
	// 不能算重复投递。
	seen := map[int64]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		default:
		}
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			rec.record(0, 0, "ws read: "+classify(err))
			return
		}
		for _, line := range bytes.Split(payload, []byte("\n")) {
			var envelope struct {
				Type string `json:"type"`
				Data struct {
					MsgID int64 `json:"msg_id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(line, &envelope); err != nil {
				continue
			}
			if envelope.Data.MsgID == 0 {
				continue
			}
			if _, loaded := seen[envelope.Data.MsgID]; loaded {
				rec.wsDup.Add(1)
				continue
			}
			seen[envelope.Data.MsgID] = struct{}{}
			rec.wsRecv.Add(1)
		}
	}
}

// postBusiness 发送 JSON 请求并按业务字段判定成功。
// 网关对部分业务失败仍返回 HTTP 200 + success:false，不能只看状态码。
func postBusiness(ctx context.Context, client *http.Client, rec *recorder, token, target string, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, 0, "marshal: "+err.Error())
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, 0, "request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		rec.failed.Add(1)
		rec.record(0, latency, classify(err))
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	rec.sent.Add(1)
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if ok {
		var decoded struct {
			Code int `json:"code"`
			Data struct {
				Success bool `json:"success"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &decoded); err == nil {
			if decoded.Code != 0 || strings.Contains(string(raw), `"success":false`) {
				ok = false
			}
		}
	}
	if ok {
		rec.ok.Add(1)
		rec.record(resp.StatusCode, latency, "")
		return
	}
	rec.failed.Add(1)
	rec.record(resp.StatusCode, latency, statusMessage(resp.StatusCode, raw, false))
}

func doAgent(ctx context.Context, client *http.Client, rec *recorder, opt options, token string, convID int64, worker, seq int) {
	body := map[string]any{
		"bot_id":          opt.agentID,
		"conversation_id": fmt.Sprintf("%d", convID),
		"message":         fmt.Sprintf("bench-%s-w%d-s%d", opt.runID, worker, seq),
	}
	postBusiness(ctx, client, rec, token, opt.baseURL+"/api/v1/agent/run", body)
}

func doRagIngest(ctx context.Context, client *http.Client, rec *recorder, opt options, token string, worker, seq int) {
	kb := opt.docKB
	if kb <= 0 {
		kb = 64
	}
	paragraph := "ClaranAIM benchmark document describes outbox kafka websocket agent runtime retrieval. "
	var builder strings.Builder
	for builder.Len() < kb*1024 {
		builder.WriteString(paragraph)
	}
	body := map[string]any{
		"title":               fmt.Sprintf("bench-%s-w%d-s%d", opt.runID, worker, seq),
		"content":             builder.String(),
		"source":              "bench",
		"source_type":         "text",
		"visibility":          "private",
		"graph_relation_mode": "off",
	}
	postBusiness(ctx, client, rec, token, opt.baseURL+"/api/v1/rag/ingest", body)
}

func doRagSearch(ctx context.Context, client *http.Client, rec *recorder, opt options, token string, worker, seq int) {
	body := map[string]any{
		"query": fmt.Sprintf("outbox kafka agent runtime %d-%d", worker, seq),
		"top_k": 5,
		"mode":  "hybrid",
	}
	postBusiness(ctx, client, rec, token, opt.baseURL+"/api/v1/rag/search", body)
}

func buildSummary(opt options, rec *recorder, elapsed time.Duration, start time.Time) map[string]any {
	rec.mu.Lock()
	latencies := append([]time.Duration(nil), rec.latencies...)
	statuses := make(map[int]int64, len(rec.statuses))
	for k, v := range rec.statuses {
		statuses[k] = v
	}
	errors := make(map[string]int64, len(rec.errors))
	for k, v := range rec.errors {
		errors[k] = v
	}
	rec.mu.Unlock()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	summary := map[string]any{
		"run_id":      opt.runID,
		"scenario":    opt.scenario,
		"concurrency": opt.concurrency,
		"started_at":  start.Format(time.RFC3339),
		"elapsed_s":   math.Round(elapsed.Seconds()*1000) / 1000,
		"sent":        rec.sent.Load(),
		"ok":          rec.ok.Load(),
		"failed":      rec.failed.Load(),
		"rps":         math.Round(float64(rec.sent.Load())/elapsed.Seconds()*100) / 100,
		"statuses":    statuses,
		"errors":      errors,
		"ws_recv":     rec.wsRecv.Load(),
		"ws_dup":      rec.wsDup.Load(),
	}
	if len(latencies) > 0 {
		summary["p50_ms"] = pct(latencies, 0.50)
		summary["p90_ms"] = pct(latencies, 0.90)
		summary["p95_ms"] = pct(latencies, 0.95)
		summary["p99_ms"] = pct(latencies, 0.99)
		summary["max_ms"] = pct(latencies, 1.0)
		summary["avg_ms"] = round2(avg(latencies))
	}
	return summary
}

func writeSummary(dir string, summary map[string]any) {
	path := filepath.Join(dir, "summary.json")
	encoded, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		log.Printf("write summary: %v", err)
		return
	}
	log.Printf("summary written: %s", path)
}

func printSummary(s map[string]any) {
	log.Printf("sent=%v ok=%v failed=%v rps=%v p50=%vms p95=%vms p99=%vms ws_recv=%v ws_dup=%v",
		s["sent"], s["ok"], s["failed"], s["rps"], s["p50_ms"], s["p95_ms"], s["p99_ms"], s["ws_recv"], s["ws_dup"])
}

func loadManifest(path string) (manifest, error) {
	var m manifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(raw, &m)
	return m, err
}

func loadTokens(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tokens []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tokens = append(tokens, line)
		}
	}
	return tokens, nil
}

func pct(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return round2(float64(sorted[idx].Microseconds()) / 1000)
}

func avg(values []time.Duration) float64 {
	if len(values) == 0 {
		return 0
	}
	var total int64
	for _, v := range values {
		total += v.Microseconds()
	}
	return float64(total) / float64(len(values)) / 1000
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func statusMessage(status int, body []byte, businessOK bool) string {
	if status >= 200 && status < 300 && businessOK {
		return ""
	}
	return fmt.Sprintf("http_%d: %s", status, truncate(string(body)))
}

func classify(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "context deadline exceeded") {
		return "timeout"
	}
	if strings.Contains(msg, "connection refused") {
		return "connection_refused"
	}
	return truncate(msg)
}

func truncate(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
