// Package service 负责 agent-runtime-service 的 Agent 执行链路。
package service

import (
	"ClaranAIM/internal/agent-runtime-service/agent"
	"ClaranAIM/internal/agent-runtime-service/reflection"
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/pkg/agentkit/component"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const runtimeSessionHistoryMessageLimit = 12
const maxAgentCacheEntries = 64

// AgentRuntimeService 执行由 agent-manager-service 传入配置的 Agent，并维护轻量长会话记忆。
type AgentRuntimeService interface {
	RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error)
	ResumeAgent(ctx context.Context, req *bot_runtime.ResumeAgentReq) (*bot_runtime.ResumeAgentResp, error)
	ListAgentRuns(ctx context.Context, req *bot_runtime.ListAgentRunsReq) (*bot_runtime.ListAgentRunsResp, error)
	RunTask(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error)
	GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error)
}

// RuntimeConfig 保存运行时进程级路径和可观测性配置。
type RuntimeConfig struct {
	SessionDir          string
	DefaultWorkspaceDir string
	CozeloopToken       string
	CozeloopWorkspaceID string
	SessionStore        store.SessionStore
	RunStore            store.RunStore
	CheckpointStore     store.CheckpointStore
	DefaultBudget       RunBudget
	Reflection          reflection.Config
}

// runtimeServiceImpl 是 AgentRuntimeService 的默认实现，持有会话存储、运行配置和 Agent 实例缓存。
type runtimeServiceImpl struct {
	sessionStore    store.SessionStore
	runStore        store.RunStore
	checkpointStore store.CheckpointStore
	reflection      reflection.Config
	cfg             RuntimeConfig
	agentCache      map[string]adk.Agent
	agentCacheOrder []string
	mu              sync.RWMutex
}

// NewAgentRuntimeService 创建只负责执行的 Agent runtime。
func NewAgentRuntimeService(cfg RuntimeConfig) AgentRuntimeService {
	if cfg.SessionStore == nil && cfg.SessionDir != "" {
		jsonlStore, err := store.NewJSONLStore(cfg.SessionDir)
		if err != nil {
			log.Printf("初始化Agent会话存储失败: %v", err)
		} else {
			cfg.SessionStore = jsonlStore
		}
	}
	fallback := store.NewMemoryRuntimeStore()
	if cfg.RunStore == nil {
		cfg.RunStore = fallback
	}
	if cfg.CheckpointStore == nil {
		cfg.CheckpointStore = fallback
	}
	return &runtimeServiceImpl{
		sessionStore:    cfg.SessionStore,
		runStore:        cfg.RunStore,
		checkpointStore: cfg.CheckpointStore,
		reflection:      cfg.Reflection,
		cfg:             cfg,
		agentCache:      make(map[string]adk.Agent),
	}
}

// RunAgent 使用长会话上下文执行一轮用户指令。
func (s *runtimeServiceImpl) RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error) {
	var rpcBudget *bot_runtime.RunBudget
	if req != nil {
		rpcBudget = req.GetBudget()
	}
	budget := s.runBudget(rpcBudget)
	ctx, cancel := withRunBudget(ctx, budget)
	defer cancel()
	return s.runAgent(ctx, req, true, budget)
}

func withDefaultAgentRunTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, 10*time.Minute)
}

func trimRuntimeSessionHistory(messages []*schema.Message, limit int) []*schema.Message {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	return messages[len(messages)-limit:]
}

func isTransientAgentRunError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection aborted") ||
		strings.Contains(msg, "server closed idle connection") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "timeout awaiting response headers")
}

// RunTask 将总结、问答、洞察、候选回复等上下文能力转换为确定的任务 prompt。
func (s *runtimeServiceImpl) RunTask(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	if req == nil || req.Bot == nil {
		return failTask("Agent配置不能为空"), nil
	}
	taskPrompt := buildTaskPrompt(req.TaskType, req.Question)
	budget := s.runBudget(nil)
	ctx, cancel := withRunBudget(ctx, budget)
	defer cancel()
	runResp, _ := s.runAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            req.Bot,
		UserId:         req.UserId,
		ConversationId: req.ConversationId,
		Input:          taskPrompt,
		Context:        req.Context,
	}, false, budget)
	if runResp == nil || !runResp.Success {
		msg := "Agent任务执行失败"
		if runResp != nil && runResp.Msg != "" {
			msg = runResp.Msg
		}
		return failTask(msg), nil
	}
	return &bot_runtime.AgentTaskResp{
		Success:           true,
		Result_:           runResp.Reply,
		StructuredResult_: runResp.StructuredResult_,
		Usage:             runResp.Usage,
		Msg:               "ok",
	}, nil
}

// GetAgentSessions 列出当前会话存储中的长会话元数据。
func (s *runtimeServiceImpl) GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error) {
	if s.sessionStore == nil {
		return &bot_runtime.GetAgentSessionResp{Success: true, Sessions: []*bot_runtime.AgentSessionInfo{}}, nil
	}
	owner := store.SessionOwner{}
	if req != nil {
		owner = store.SessionOwner{
			BotID:          req.BotId,
			UserID:         req.UserId,
			ConversationID: req.ConversationId,
		}
	}
	metas, err := s.sessionStore.ListSessions(ctx, owner)
	if err != nil {
		return &bot_runtime.GetAgentSessionResp{Success: false, Msg: err.Error()}, nil
	}
	out := make([]*bot_runtime.AgentSessionInfo, 0, len(metas))
	for _, meta := range metas {
		out = append(out, &bot_runtime.AgentSessionInfo{
			SessionId: meta.ID,
			Title:     meta.Title,
			CreatedAt: meta.CreatedAt.Format(time.RFC3339),
			Summary:   meta.Summary,
		})
	}
	return &bot_runtime.GetAgentSessionResp{Success: true, Sessions: out}, nil
}

// getOrCreateAgent 按配置缓存或创建 Eino DeepAgent 实例。
// 缓存 key 包含模型、密钥、系统提示词、工具策略和工作目录，配置变化会自然生成新实例。
func (s *runtimeServiceImpl) getOrCreateAgent(ctx context.Context, bot *bot_runtime.RuntimeBotConfig) (adk.Agent, error) {
	cacheKey := runtimeAgentCacheKey(bot)
	s.mu.RLock()
	if ag, ok := s.agentCache[cacheKey]; ok {
		s.mu.RUnlock()
		return ag, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if ag, ok := s.agentCache[cacheKey]; ok {
		s.touchAgentCache(cacheKey)
		return ag, nil
	}

	chatModel, err := component.NewChatModel(ctx, bot.ApiKey, bot.BaseUrl, bot.ModelName, bot.MaxOutputTokens, bot.Temperature)
	if err != nil {
		return nil, err
	}
	workspace, err := s.resolveWorkspaceRoot(bot)
	if err != nil {
		return nil, err
	}
	ag, err := agent.NewDeepAgent(ctx, chatModel, workspace, s.cfg.CozeloopToken, s.cfg.CozeloopWorkspaceID, bot.SkillsDir, bot.Name, bot.Description, bot.SystemPrompt, bot.ToolPolicy, bot.IncludeDomainTools)
	if err != nil {
		return nil, err
	}
	s.storeAgentCache(cacheKey, ag)
	return ag, nil
}

// storeAgentCache 写入有界 Agent 实例缓存；超过上限时按最近使用顺序淘汰。
func (s *runtimeServiceImpl) storeAgentCache(key string, ag adk.Agent) {
	if _, exists := s.agentCache[key]; !exists && len(s.agentCache) >= maxAgentCacheEntries {
		for len(s.agentCacheOrder) > 0 {
			oldest := s.agentCacheOrder[0]
			s.agentCacheOrder = s.agentCacheOrder[1:]
			if _, ok := s.agentCache[oldest]; ok {
				delete(s.agentCache, oldest)
				break
			}
		}
	}
	s.agentCache[key] = ag
	s.agentCacheOrder = append(s.agentCacheOrder, key)
}

// touchAgentCache 把命中项移动到队尾，调用方必须持有写锁或读锁保护。
func (s *runtimeServiceImpl) touchAgentCache(key string) {
	for i, item := range s.agentCacheOrder {
		if item != key {
			continue
		}
		s.agentCacheOrder = append(append(s.agentCacheOrder[:i], s.agentCacheOrder[i+1:]...), key)
		return
	}
	s.agentCacheOrder = append(s.agentCacheOrder, key)
}

// resolveWorkspaceRoot 解析并校验 Agent 工作目录。
// 最终路径必须落在 DefaultWorkspaceDir 允许根目录内，防止用户配置路径穿越到项目外。
func (s *runtimeServiceImpl) resolveWorkspaceRoot(bot *bot_runtime.RuntimeBotConfig) (string, error) {
	base := strings.TrimSpace(s.cfg.DefaultWorkspaceDir)
	if base == "" {
		base = "storage/agent/files"
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("解析Agent工作目录根失败: %w", err)
	}

	workspace := strings.TrimSpace(bot.WorkspaceRoot)
	if workspace == "" {
		workspace = filepath.Join(baseAbs, fmt.Sprintf("%d", bot.BotId))
	} else if filepath.IsAbs(workspace) {
		workspace = normalizeAbsoluteWorkspace(baseAbs, workspace, bot.BotId)
	} else if isPathUnderBase(baseAbs, workspace) {
		workspace = filepath.Clean(workspace)
	} else {
		workspace = filepath.Join(baseAbs, workspace)
	}

	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("解析Agent工作目录失败: %w", err)
	}
	rel, err := filepath.Rel(baseAbs, workspaceAbs)
	if err != nil {
		return "", fmt.Errorf("校验Agent工作目录失败: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("Agent工作目录必须位于允许根目录内: %s", baseAbs)
	}
	if err := os.MkdirAll(workspaceAbs, 0o755); err != nil {
		return "", fmt.Errorf("创建Agent工作目录失败: %w", err)
	}
	return workspaceAbs, nil
}

// normalizeAbsoluteWorkspace 兼容历史绝对路径配置。
// 如果旧路径末级目录就是 botID，则映射回新的受控工作目录；否则保留原路径并交给后续越权校验。
func normalizeAbsoluteWorkspace(baseAbs, workspace string, botID int64) string {
	cleaned := filepath.Clean(workspace)
	if isPathUnderBase(baseAbs, cleaned) {
		return cleaned
	}
	if botID > 0 && filepath.Base(cleaned) == fmt.Sprintf("%d", botID) {
		return filepath.Join(baseAbs, fmt.Sprintf("%d", botID))
	}
	return cleaned
}

// isPathUnderBase 判断路径是否位于允许根目录内。
func isPathUnderBase(baseAbs, path string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

// runtimeAgentCacheKey 根据影响 Agent 行为的配置生成缓存 key。
func runtimeAgentCacheKey(bot *bot_runtime.RuntimeBotConfig) string {
	if bot == nil {
		return "nil"
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", bot.BotId),
		bot.ModelName,
		bot.ApiKey,
		bot.BaseUrl,
		bot.SystemPrompt,
		bot.SkillsDir,
		runtimeSkillCacheVersion(bot.SkillsDir),
		bot.WorkspaceRoot,
		bot.ToolPolicy,
		fmt.Sprintf("%t", bot.IncludeDomainTools),
		fmt.Sprintf("%d", bot.MaxOutputTokens),
		fmt.Sprintf("%.4f", bot.Temperature),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func runtimeSkillCacheVersion(skillsDir string) string {
	skillsDir = strings.TrimSpace(skillsDir)
	if skillsDir == "" {
		return ""
	}
	root, err := filepath.Abs(skillsDir)
	if err != nil {
		root = filepath.Clean(skillsDir)
	}
	records := make([]string, 0, 4)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if path == root {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && len(strings.Split(filepath.ToSlash(rel), "/")) > 5 {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(info.Name(), "SKILL.md") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			records = append(records, filepath.ToSlash(rel)+"\x00unreadable")
			return nil
		}
		sum := sha256.Sum256(data)
		records = append(records, filepath.ToSlash(rel)+"\x00"+hex.EncodeToString(sum[:]))
		return nil
	})
	if len(records) == 0 {
		return ""
	}
	sort.Strings(records)
	sum := sha256.Sum256([]byte(strings.Join(records, "\x00")))
	return hex.EncodeToString(sum[:])
}

// validateRuntimeBot 校验运行 Agent 必需的模型供应商配置。
func validateRuntimeBot(bot *bot_runtime.RuntimeBotConfig) error {
	if bot.BotId <= 0 {
		return errors.New("bot_id不能为空")
	}
	if bot.ApiKey == "" {
		return errors.New("Agent未配置API Key")
	}
	if bot.BaseUrl == "" {
		return errors.New("Agent未配置Base URL")
	}
	if bot.ModelName == "" {
		return errors.New("Agent未配置模型")
	}
	return nil
}

// buildTaskPrompt 为不同任务生成面向真人的中文任务提示词。
// 它明确要求 Agent 基于传入会话材料回答，材料很少时也要判断“是否只是废话”，而不是拒绝总结。
func buildTaskPrompt(taskType, question string) string {
	base := "你正在帮助用户理解 IM 会话。请只基于用户提供的“会话材料”回答，不要假装看到了未提供的内容。材料很少时也要分析它的性质：如果只是闲聊、灌水、无实质信息、重复寒暄或情绪碎片，就明确告诉用户“这段会话基本是废话/没有形成有效信息”，并简短说明依据。只有完全没有会话材料时，才说明没有可分析的内容。请用自然、清晰的中文回复用户，面向真人阅读，不要输出机器处理格式或代码块。\n\n"
	switch taskType {
	case "summary":
		return base + "任务：总结会话。请覆盖：1）大家主要聊了什么；2）已经形成的结论；3）待办事项和负责人（如果能看出）；4）风险、分歧或悬而未决的问题。没有的信息不要编造。\n\n" + question
	case "ask":
		return base + "任务：回答用户关于会话的问题。请引用会话材料中的事实进行回答；如果材料不足，请说明缺少哪些信息。\n\n" + question
	case "insights":
		return base + "任务：提取会话洞察。请用面向人的小标题整理：结论、分歧、风险、待办、可能的负责人。没有的信息写“未体现”。\n\n" + question
	case "reply_candidates":
		return base + "任务：生成 3 条可直接发送的回复候选。每条回复都要贴合会话材料，语气自然。\n\n" + question
	default:
		return base + question
	}
}

// defaultSessionID 生成默认长会话 ID，按 Agent、用户、会话隔离。
func defaultSessionID(botID, userID, conversationID int64) string {
	return fmt.Sprintf("agent_%d_user_%d_conv_%d", botID, userID, conversationID)
}

// failRun 构造 RunAgent 的失败响应，避免把业务错误变成 RPC transport 错误。
func failRun(msg string) *bot_runtime.RunAgentResp {
	return &bot_runtime.RunAgentResp{Success: false, Msg: msg}
}

// pendingApprovalRun 构造等待用户确认的中断响应。
func pendingApprovalRun(sessionID string, info *adk.InterruptInfo) *bot_runtime.RunAgentResp {
	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     formatInterruptPrompt(info),
		SessionId: sessionID,
		Msg:       "pending_user_approval",
	}
}

// formatInterruptPrompt 将 Eino 中断信息转成前端可展示的确认文案。
func formatInterruptPrompt(info *adk.InterruptInfo) string {
	if info == nil || len(info.InterruptContexts) == 0 {
		return "这个操作需要你确认。确认后我会继续执行。"
	}
	return fmt.Sprintf("这个操作需要你确认。待确认步骤数：%d。确认后我会继续执行。", len(info.InterruptContexts))
}

// failTask 构造结构化 Agent 任务的失败响应。
func failTask(msg string) *bot_runtime.AgentTaskResp {
	return &bot_runtime.AgentTaskResp{Success: false, Msg: msg}
}

// tokenUsage 聚合一次 Agent 执行过程中模型消息返回的真实 token 用量。
type tokenUsage struct {
	input  int64
	output int64
	seen   bool
}

// mergeMessageUsage 从 Eino schema.Message 的 ResponseMeta 中累加 token usage。
func (u *tokenUsage) mergeMessageUsage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	u.seen = true
	u.input += int64(usage.PromptTokens)
	u.output += int64(usage.CompletionTokens)
}

// toRPC 将内部 usage 聚合结果转换为 bot_runtime RPC 结构。
func (u tokenUsage) toRPC() *bot_runtime.TokenUsage {
	return &bot_runtime.TokenUsage{InputTokens: u.input, OutputTokens: u.output, UsageSeen: u.seen}
}

// replyCollector 收集 Agent 多段 assistant 输出，并去重相邻重复片段。
type replyCollector struct {
	parts []string
}

// mergeMessage 从 Eino MessageVariant 中提取 assistant 消息并合并。
func (r *replyCollector) mergeMessage(output *adk.MessageVariant) {
	if output == nil {
		return
	}
	msg, err := output.GetMessage()
	if err != nil {
		return
	}
	r.mergeResolvedMessage(output.Role, msg)
}

// mergeResolvedMessage 合并已解析的 assistant 文本，忽略 tool/user/system 等非回复消息。
func (r *replyCollector) mergeResolvedMessage(role schema.RoleType, msg *schema.Message) {
	if msg == nil {
		return
	}
	if role != "" && role != schema.Assistant {
		return
	}
	if msg.Role != "" && msg.Role != schema.Assistant {
		return
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return
	}
	if len(r.parts) > 0 && r.parts[len(r.parts)-1] == content {
		return
	}
	r.parts = append(r.parts, content)
}

// String 将收集到的回复片段拼成最终文本，并在英文连续片段之间补空格。
func (r *replyCollector) String() string {
	if len(r.parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, part := range r.parts {
		if sb.Len() > 0 && needsReplySpace(sb.String(), part) {
			sb.WriteByte(' ')
		}
		sb.WriteString(part)
	}
	return strings.TrimSpace(sb.String())
}

// needsReplySpace 判断两个回复片段拼接时是否需要补 ASCII 空格。
func needsReplySpace(prev, next string) bool {
	if prev == "" || next == "" {
		return false
	}
	lastRunes := []rune(prev)
	nextRunes := []rune(next)
	last := lastRunes[len(lastRunes)-1]
	first := nextRunes[0]
	return isASCIIWord(last) && isASCIIWord(first)
}

// isASCIIWord 判断字符是否属于英文/数字单词字符。
func isASCIIWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
