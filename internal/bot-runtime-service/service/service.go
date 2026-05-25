// Package service owns Agent execution for bot-runtime-service.
package service

import (
	"ClaranAIM/internal/bot-runtime-service/agent"
	"ClaranAIM/internal/bot-runtime-service/component"
	"ClaranAIM/kitex_gen/bot_runtime"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// BotRuntimeService executes configured Agents and keeps long-session memory.
type BotRuntimeService interface {
	RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error)
	RunTask(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error)
	GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error)
}

// RuntimeConfig contains process-level paths and tracing knobs.
type RuntimeConfig struct {
	SessionDir          string
	DefaultWorkspaceDir string
	CozeloopToken       string
	CozeloopWorkspaceID string
}

type runtimeServiceImpl struct {
	sessionStore *component.Store
	cfg          RuntimeConfig
	agentCache   map[string]adk.Agent
	mu           sync.RWMutex
}

// NewBotRuntimeService creates an execution-only runtime.
func NewBotRuntimeService(cfg RuntimeConfig) BotRuntimeService {
	var store *component.Store
	if cfg.SessionDir != "" {
		var err error
		store, err = component.NewStore(cfg.SessionDir)
		if err != nil {
			log.Printf("初始化Agent会话存储失败: %v", err)
		}
	}
	return &runtimeServiceImpl{
		sessionStore: store,
		cfg:          cfg,
		agentCache:   make(map[string]adk.Agent),
	}
}

// RunAgent executes one user instruction with long-session context.
func (s *runtimeServiceImpl) RunAgent(ctx context.Context, req *bot_runtime.RunAgentReq) (*bot_runtime.RunAgentResp, error) {
	return s.runAgent(ctx, req, true)
}

func (s *runtimeServiceImpl) runAgent(ctx context.Context, req *bot_runtime.RunAgentReq, persistSession bool) (*bot_runtime.RunAgentResp, error) {
	if req == nil || req.Bot == nil {
		return failRun("Agent配置不能为空"), nil
	}
	if strings.TrimSpace(req.Input) == "" {
		return failRun("输入不能为空"), nil
	}
	if err := validateRuntimeBot(req.Bot); err != nil {
		return failRun(err.Error()), nil
	}

	ag, err := s.getOrCreateAgent(ctx, req.Bot)
	if err != nil {
		return failRun(fmt.Sprintf("创建Agent失败: %v", err)), nil
	}

	sessionID := req.SessionId
	if sessionID == "" {
		sessionID = defaultSessionID(req.Bot.BotId, req.UserId, req.ConversationId)
	}

	var historyMsgs []*schema.Message
	if persistSession && s.sessionStore != nil {
		session, sessErr := s.sessionStore.GetSession(sessionID)
		if sessErr == nil {
			historyMsgs = session.GetMessages()
		} else {
			log.Printf("加载Agent会话失败 session=%s err=%v", sessionID, sessErr)
		}
	}

	userMsg := schema.UserMessage(req.Input)
	inputMsgs := make([]adk.Message, 0, len(historyMsgs)+1)
	for _, msg := range historyMsgs {
		inputMsgs = append(inputMsgs, msg)
	}
	inputMsgs = append(inputMsgs, userMsg)

	iter := ag.Run(ctx, &adk.AgentInput{Messages: inputMsgs})
	var collector replyCollector
	var usage tokenUsage
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return failRun(fmt.Sprintf("Agent执行失败: %v", event.Err)), nil
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			return pendingApprovalRun(sessionID, event.Action.Interrupted), nil
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err == nil && msg != nil {
				collector.mergeResolvedMessage(event.Output.MessageOutput.Role, msg)
				usage.mergeMessageUsage(msg)
			}
		}
	}

	reply := collector.String()
	if reply == "" {
		return failRun("Agent返回为空"), nil
	}
	if persistSession && s.sessionStore != nil {
		if session, sessErr := s.sessionStore.GetSession(sessionID); sessErr == nil {
			_ = session.Append(userMsg)
			_ = session.Append(schema.AssistantMessage(reply, nil))
		}
	}

	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     reply,
		Usage:     usage.toRPC(),
		SessionId: sessionID,
		Msg:       "ok",
	}, nil
}

// RunTask turns context-oriented Agent abilities into deterministic prompts.
func (s *runtimeServiceImpl) RunTask(ctx context.Context, req *bot_runtime.AgentTaskReq) (*bot_runtime.AgentTaskResp, error) {
	if req == nil || req.Bot == nil {
		return failTask("Agent配置不能为空"), nil
	}
	taskPrompt := buildTaskPrompt(req.TaskType, req.Question)
	runResp, _ := s.runAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            req.Bot,
		UserId:         req.UserId,
		ConversationId: req.ConversationId,
		Input:          taskPrompt,
		Context:        req.Context,
	}, false)
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

// GetAgentSessions lists persisted sessions for the current lightweight store.
func (s *runtimeServiceImpl) GetAgentSessions(ctx context.Context, req *bot_runtime.GetAgentSessionReq) (*bot_runtime.GetAgentSessionResp, error) {
	if s.sessionStore == nil {
		return &bot_runtime.GetAgentSessionResp{Success: true, Sessions: []*bot_runtime.AgentSessionInfo{}}, nil
	}
	metas, err := s.sessionStore.ListSessions()
	if err != nil {
		return &bot_runtime.GetAgentSessionResp{Success: false, Msg: err.Error()}, nil
	}
	prefix := ""
	if req != nil {
		prefix = defaultSessionID(req.BotId, req.UserId, req.ConversationId)
	}
	out := make([]*bot_runtime.AgentSessionInfo, 0, len(metas))
	for _, meta := range metas {
		if req != nil && prefix != "" && !strings.HasPrefix(meta.ID, prefix) {
			continue
		}
		out = append(out, &bot_runtime.AgentSessionInfo{
			SessionId: meta.ID,
			Title:     meta.Title,
			CreatedAt: meta.CreatedAt.Format(time.RFC3339),
		})
	}
	return &bot_runtime.GetAgentSessionResp{Success: true, Sessions: out}, nil
}

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
		return ag, nil
	}

	chatModel, err := component.NewChatModel(ctx, bot.ApiKey, bot.BaseUrl, bot.ModelName)
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
	s.agentCache[cacheKey] = ag
	return ag, nil
}

func (s *runtimeServiceImpl) resolveWorkspaceRoot(bot *bot_runtime.RuntimeBotConfig) (string, error) {
	base := strings.TrimSpace(s.cfg.DefaultWorkspaceDir)
	if base == "" {
		base = "storage/agent/workspaces"
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
		bot.WorkspaceRoot,
		bot.ToolPolicy,
		fmt.Sprintf("%t", bot.IncludeDomainTools),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

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

func defaultSessionID(botID, userID, conversationID int64) string {
	return fmt.Sprintf("agent_%d_user_%d_conv_%d", botID, userID, conversationID)
}

func failRun(msg string) *bot_runtime.RunAgentResp {
	return &bot_runtime.RunAgentResp{Success: false, Msg: msg}
}

func pendingApprovalRun(sessionID string, info *adk.InterruptInfo) *bot_runtime.RunAgentResp {
	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     formatInterruptPrompt(info),
		SessionId: sessionID,
		Msg:       "pending_user_approval",
	}
}

func formatInterruptPrompt(info *adk.InterruptInfo) string {
	if info == nil || len(info.InterruptContexts) == 0 {
		return "这个操作需要你确认。确认后我会继续执行。"
	}
	return fmt.Sprintf("这个操作需要你确认。待确认步骤数：%d。确认后我会继续执行。", len(info.InterruptContexts))
}

func failTask(msg string) *bot_runtime.AgentTaskResp {
	return &bot_runtime.AgentTaskResp{Success: false, Msg: msg}
}

type tokenUsage struct {
	input  int64
	output int64
	seen   bool
}

func (u *tokenUsage) mergeMessageUsage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	u.seen = true
	u.input += int64(usage.PromptTokens)
	u.output += int64(usage.CompletionTokens)
}

func (u tokenUsage) toRPC() *bot_runtime.TokenUsage {
	return &bot_runtime.TokenUsage{InputTokens: u.input, OutputTokens: u.output, UsageSeen: u.seen}
}

type replyCollector struct {
	parts []string
}

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

func isASCIIWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
