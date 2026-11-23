package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ClaranAIM/internal/agent-runtime-service/logic"
	"ClaranAIM/internal/agent-runtime-service/store"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/pkg/agentkit/pricing"
	"ClaranAIM/pkg/idgen"

	commontool "github.com/cloudwego/eino-examples/adk/common/tool"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultRunMaxSteps       int64   = 30
	defaultRunMaxInputTokens int64   = 200_000
	defaultRunMaxOutputToken int64   = 32_000
	defaultRunMaxCostUSD     float64 = 2.0
)

// RunBudget 是单次 Agent 运行的硬预算。
type RunBudget struct {
	MaxSteps        int64
	MaxInputTokens  int64
	MaxOutputTokens int64
	MaxCostUSD      float64
	Deadline        time.Duration
}

func defaultRunBudget() RunBudget {
	return RunBudget{
		MaxSteps:        defaultRunMaxSteps,
		MaxInputTokens:  defaultRunMaxInputTokens,
		MaxOutputTokens: defaultRunMaxOutputToken,
		MaxCostUSD:      defaultRunMaxCostUSD,
		Deadline:        10 * time.Minute,
	}
}

// runBudget 合并进程默认预算和单次 RPC 预算；0 表示沿用默认值。
func (s *runtimeServiceImpl) runBudget(rpc *bot_runtime.RunBudget) RunBudget {
	budget := s.cfg.DefaultBudget
	defaults := defaultRunBudget()
	if budget.MaxSteps <= 0 {
		budget.MaxSteps = defaults.MaxSteps
	}
	if budget.MaxInputTokens <= 0 {
		budget.MaxInputTokens = defaults.MaxInputTokens
	}
	if budget.MaxOutputTokens <= 0 {
		budget.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if budget.MaxCostUSD <= 0 {
		budget.MaxCostUSD = defaults.MaxCostUSD
	}
	if budget.Deadline <= 0 {
		budget.Deadline = defaults.Deadline
	}
	if rpc == nil {
		return budget
	}
	if rpc.MaxSteps > 0 {
		budget.MaxSteps = rpc.MaxSteps
	}
	if rpc.MaxInputTokens > 0 {
		budget.MaxInputTokens = rpc.MaxInputTokens
	}
	if rpc.MaxOutputTokens > 0 {
		budget.MaxOutputTokens = rpc.MaxOutputTokens
	}
	if rpc.MaxCostUsd > 0 {
		budget.MaxCostUSD = rpc.MaxCostUsd
	}
	if rpc.DeadlineMs > 0 {
		budget.Deadline = time.Duration(rpc.DeadlineMs) * time.Millisecond
	}
	return budget
}

func withRunBudget(ctx context.Context, budget RunBudget) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	deadline := budget.Deadline
	if deadline <= 0 {
		deadline = 10 * time.Minute
	}
	return context.WithTimeout(ctx, deadline)
}

type budgetTracker struct {
	budget RunBudget
	model  string
	steps  int64
}

func newBudgetTracker(budget RunBudget, model string) *budgetTracker {
	return &budgetTracker{budget: budget, model: model}
}

func (t *budgetTracker) reset() {
	t.steps = 0
}

func (t *budgetTracker) observe(msg *schema.Message) {
	if msg == nil {
		return
	}
	if msg.Role == schema.Assistant || msg.Role == "" {
		t.steps++
	}
}

func (t *budgetTracker) cost(usage tokenUsage) float64 {
	return pricing.EstimateUSD(t.model, usage.input, usage.output)
}

func (t *budgetTracker) exceeded(usage tokenUsage) string {
	if t.budget.MaxSteps > 0 && t.steps > t.budget.MaxSteps {
		return "max_steps"
	}
	if t.budget.MaxInputTokens > 0 && usage.input > t.budget.MaxInputTokens {
		return "max_input_tokens"
	}
	if t.budget.MaxOutputTokens > 0 && usage.output > t.budget.MaxOutputTokens {
		return "max_output_tokens"
	}
	if t.budget.MaxCostUSD > 0 && t.cost(usage) > t.budget.MaxCostUSD {
		return "max_cost_usd"
	}
	return ""
}

// runAgent 执行 Agent 主流程：恢复会话上下文、用 Eino Runner 执行、追踪预算并持久化运行记录。
// persistSession=false 时用于一次性结构化任务，不污染普通聊天长会话。
func (s *runtimeServiceImpl) runAgent(ctx context.Context, req *bot_runtime.RunAgentReq, persistSession bool, budget RunBudget) (*bot_runtime.RunAgentResp, error) {
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
	owner := store.SessionOwner{
		BotID:          req.Bot.BotId,
		UserID:         req.UserId,
		ConversationID: req.ConversationId,
	}
	summary := ""
	var history []store.Message
	if persistSession && s.sessionStore != nil {
		summary, history, err = s.sessionStore.Context(ctx, sessionID, store.DefaultContextLimit)
		if err != nil {
			log.Printf("加载Agent会话上下文失败 session=%s err=%v", sessionID, err)
		}
	}
	inputMsgs := make([]adk.Message, 0, len(history)+2)
	if strings.TrimSpace(summary) != "" {
		inputMsgs = append(inputMsgs, schema.SystemMessage("以下是此前会话的滚动摘要，仅作为背景，不要把它当作本轮用户输入：\n"+summary))
	}
	for _, message := range history {
		inputMsgs = append(inputMsgs, schemaMessageFromStore(message))
	}
	inputMsgs = append(inputMsgs, schema.UserMessage(req.Input))
	if persistSession && s.sessionStore != nil {
		if err := s.sessionStore.AppendMessage(ctx, sessionID, owner, "user", req.Input, 0); err != nil {
			log.Printf("持久化用户消息失败 session=%s err=%v", sessionID, err)
		}
	}

	runID := newRuntimeID("run")
	run := &store.RunInfo{
		RunID:          runID,
		SessionID:      sessionID,
		BotID:          req.Bot.BotId,
		UserID:         req.UserId,
		ConversationID: req.ConversationId,
		Status:         store.RunStatusRunning,
		StartedAt:      time.Now(),
	}
	s.createRun(ctx, run)
	budgetDeadline := runBudgetDeadline(budget)
	tracker := newBudgetTracker(budget, req.Bot.ModelName)
	var collector replyCollector
	var usage tokenUsage

	for attempt := 0; attempt < 2; attempt++ {
		collector = replyCollector{}
		usage = tokenUsage{}
		tracker.reset()
		runCtx := logic.WithRAGRuntimeContext(ctx, req.UserId, req.ConversationId)
		runCtx = logic.WithMCPRuntimeContext(runCtx, req.UserId, req.Bot.BotId, req.ConversationId)
		iter := s.newRunner(ag).Run(runCtx, inputMsgs, adk.WithCheckPointID(runID))
		retry := false
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				if time.Now().After(budgetDeadline) {
					return s.budgetExceeded(ctx, run, "deadline", collector.String(), tracker, usage), nil
				}
				s.finishRun(ctx, run, store.RunStatusFailed, "", "", tracker, usage, ctxErr.Error())
				return failRun(fmt.Sprintf("Agent执行失败: %v", ctxErr)), nil
			}
			if event.Err != nil {
				if attempt == 0 && isTransientAgentRunError(event.Err) && !time.Now().After(budgetDeadline) {
					log.Printf("Agent执行遇到临时模型错误，准备重试一次: %v", event.Err)
					retry = true
					break
				}
				s.finishRun(ctx, run, store.RunStatusFailed, "", "", tracker, usage, event.Err.Error())
				return failRun(fmt.Sprintf("Agent执行失败: %v", event.Err)), nil
			}
			if event.Action != nil && event.Action.Interrupted != nil {
				return s.pendingApprovalResponse(ctx, run, req, event.Action.Interrupted, tracker, usage), nil
			}
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}
			msg, msgErr := event.Output.MessageOutput.GetMessage()
			if msgErr != nil || msg == nil {
				continue
			}
			collector.mergeResolvedMessage(event.Output.MessageOutput.Role, msg)
			usage.mergeMessageUsage(msg)
			tracker.observe(msg)
			if reason := tracker.exceeded(usage); reason != "" {
				return s.budgetExceeded(ctx, run, reason, collector.String(), tracker, usage), nil
			}
		}
		if retry {
			continue
		}
		break
	}

	reply := collector.String()
	if reply == "" {
		s.finishRun(ctx, run, store.RunStatusFailed, "", "", tracker, usage, "Agent返回为空")
		return failRun("Agent返回为空"), nil
	}
	reply = applyGroundingGate(reply)
	reflectionResult := s.runReflection(ctx, req, run, reply)
	reply = reflectionResult.Answer
	if persistSession && s.sessionStore != nil {
		if err := s.sessionStore.AppendMessage(ctx, sessionID, owner, "assistant", reply, usage.output); err != nil {
			log.Printf("持久化助手回复失败 session=%s err=%v", sessionID, err)
		}
	}
	run.Status = store.RunStatusCompleted
	run.Step = tracker.steps
	run.InputTokens = usage.input
	run.OutputTokens = usage.output
	run.CostUSD = tracker.cost(usage)
	attachReflection(run, reflectionResult)
	now := time.Now()
	run.FinishedAt = &now
	s.updateRun(ctx, run)
	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     reply,
		Usage:     usage.toRPC(),
		SessionId: sessionID,
		Msg:       "ok",
		RunId:     stringPtr(runID),
	}, nil
}

// ResumeAgent 从持久化 checkpoint 恢复被审批中断的运行。
func (s *runtimeServiceImpl) ResumeAgent(ctx context.Context, req *bot_runtime.ResumeAgentReq) (*bot_runtime.ResumeAgentResp, error) {
	if req == nil || req.Bot == nil {
		return failResume("Agent配置不能为空"), nil
	}
	if err := validateRuntimeBot(req.Bot); err != nil {
		return failResume(err.Error()), nil
	}
	if strings.TrimSpace(req.RunId) == "" || strings.TrimSpace(req.InterruptId) == "" {
		return failResume("缺少 run_id 或 interrupt_id，无法恢复"), nil
	}
	if s.runStore == nil {
		return failResume("运行记录存储未配置"), nil
	}
	run, err := s.runStore.GetRun(ctx, req.RunId)
	if err != nil {
		return failResume(err.Error()), nil
	}
	if run == nil {
		return failResume("运行记录不存在或已过期"), nil
	}
	if req.UserId > 0 && run.UserID > 0 && run.UserID != req.UserId {
		return failResume("无权恢复该运行"), nil
	}
	ag, err := s.getOrCreateAgent(ctx, req.Bot)
	if err != nil {
		return failResume(fmt.Sprintf("创建Agent失败: %v", err)), nil
	}
	budget := s.runBudget(req.GetBudget())
	ctx, cancel := withRunBudget(ctx, budget)
	defer cancel()
	budgetDeadline := runBudgetDeadline(budget)
	resumeData := &commontool.ApprovalResult{Approved: req.Approved}
	if !req.Approved && strings.TrimSpace(req.Reason) != "" {
		reason := strings.TrimSpace(req.Reason)
		resumeData.DisapproveReason = &reason
	}
	iter, err := s.newRunner(ag).ResumeWithParams(ctx, req.RunId, &adk.ResumeParams{
		Targets: map[string]any{req.InterruptId: resumeData},
	})
	if err != nil {
		s.finishRun(ctx, run, store.RunStatusFailed, req.ApprovalId, "", newBudgetTracker(budget, req.Bot.ModelName), tokenUsage{}, err.Error())
		return failResume(fmt.Sprintf("恢复执行失败: %v", err)), nil
	}
	run.Status = store.RunStatusRunning
	run.ApprovalID = req.ApprovalId
	s.updateRun(ctx, run)
	usage := tokenUsage{input: run.InputTokens, output: run.OutputTokens, seen: run.InputTokens > 0 || run.OutputTokens > 0}
	tracker := newBudgetTracker(budget, req.Bot.ModelName)
	tracker.steps = run.Step
	var collector replyCollector
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			if time.Now().After(budgetDeadline) {
				return s.resumeBudgetExceeded(ctx, run, "deadline", collector.String(), tracker, usage), nil
			}
			s.finishRun(ctx, run, store.RunStatusFailed, req.ApprovalId, "", tracker, usage, ctxErr.Error())
			return failResume(fmt.Sprintf("恢复执行失败: %v", ctxErr)), nil
		}
		if event.Err != nil {
			s.finishRun(ctx, run, store.RunStatusFailed, req.ApprovalId, "", tracker, usage, event.Err.Error())
			return failResume(fmt.Sprintf("恢复执行失败: %v", event.Err)), nil
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			return s.resumePendingApproval(ctx, run, req, event.Action.Interrupted, tracker, usage), nil
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		msg, msgErr := event.Output.MessageOutput.GetMessage()
		if msgErr != nil || msg == nil {
			continue
		}
		collector.mergeResolvedMessage(event.Output.MessageOutput.Role, msg)
		usage.mergeMessageUsage(msg)
		tracker.observe(msg)
		if reason := tracker.exceeded(usage); reason != "" {
			return s.resumeBudgetExceeded(ctx, run, reason, collector.String(), tracker, usage), nil
		}
	}
	reply := collector.String()
	if reply == "" {
		s.finishRun(ctx, run, store.RunStatusFailed, req.ApprovalId, "", tracker, usage, "Agent返回为空")
		return failResume("Agent返回为空"), nil
	}
	reply = applyGroundingGate(reply)
	reflectionResult := s.runReflectionForResume(ctx, req, run, reply)
	reply = reflectionResult.Answer
	if s.sessionStore != nil && run.SessionID != "" {
		owner := store.SessionOwner{BotID: run.BotID, UserID: run.UserID, ConversationID: run.ConversationID}
		if err := s.sessionStore.AppendMessage(ctx, run.SessionID, owner, "assistant", reply, usage.output); err != nil {
			log.Printf("持久化恢复后的助手回复失败 session=%s err=%v", run.SessionID, err)
		}
	}
	run.Status = store.RunStatusCompleted
	run.Step = tracker.steps
	run.InputTokens = usage.input
	run.OutputTokens = usage.output
	run.CostUSD = tracker.cost(usage)
	attachReflection(run, reflectionResult)
	now := time.Now()
	run.FinishedAt = &now
	s.updateRun(ctx, run)
	return &bot_runtime.ResumeAgentResp{
		Success:   true,
		Reply:     reply,
		Usage:     usage.toRPC(),
		RunId:     run.RunID,
		SessionId: run.SessionID,
		Msg:       "ok",
	}, nil
}

// ListAgentRuns 返回 runtime 运行记录，供 Agent 面板展示单次成本与中断原因。
func (s *runtimeServiceImpl) ListAgentRuns(ctx context.Context, req *bot_runtime.ListAgentRunsReq) (*bot_runtime.ListAgentRunsResp, error) {
	if s.runStore == nil {
		return &bot_runtime.ListAgentRunsResp{Success: true, Runs: []*bot_runtime.AgentRunInfo{}}, nil
	}
	filter := store.RunFilter{}
	if req != nil {
		filter = store.RunFilter{
			UserID:         req.UserId,
			BotID:          req.BotId,
			ConversationID: req.ConversationId,
			Limit:          int(req.Limit),
		}
	}
	runs, err := s.runStore.ListRuns(ctx, filter)
	if err != nil {
		return &bot_runtime.ListAgentRunsResp{Success: false, Msg: err.Error()}, nil
	}
	out := make([]*bot_runtime.AgentRunInfo, 0, len(runs))
	for _, run := range runs {
		info := &bot_runtime.AgentRunInfo{
			RunId:           run.RunID,
			SessionId:       run.SessionID,
			BotId:           run.BotID,
			UserId:          run.UserID,
			ConversationId:  run.ConversationID,
			Status:          run.Status,
			Step:            run.Step,
			InputTokens:     run.InputTokens,
			OutputTokens:    run.OutputTokens,
			CostUsd:         run.CostUSD,
			InterruptReason: run.InterruptReason,
			ApprovalId:      run.ApprovalID,
			InterruptedTool: run.InterruptedTool,
			StartedAt:       run.StartedAt.Format(time.RFC3339),
		}
		if run.FinishedAt != nil {
			info.FinishedAt = run.FinishedAt.Format(time.RFC3339)
		}
		out = append(out, info)
	}
	return &bot_runtime.ListAgentRunsResp{Success: true, Runs: out}, nil
}

func (s *runtimeServiceImpl) pendingApprovalResponse(ctx context.Context, run *store.RunInfo, req *bot_runtime.RunAgentReq, info *adk.InterruptInfo, tracker *budgetTracker, usage tokenUsage) *bot_runtime.RunAgentResp {
	approval := buildApprovalInfo(run.RunID, run.SessionID, req.Bot.BotId, req.UserId, req.ConversationId, info)
	run.Status = store.RunStatusPendingApproval
	run.ApprovalID = approval.ApprovalId
	run.InterruptedTool = approval.ToolName
	run.InterruptReason = "approval_required"
	run.Step = tracker.steps
	run.InputTokens = usage.input
	run.OutputTokens = usage.output
	run.CostUSD = tracker.cost(usage)
	setRunCheckpointJSON(run, approval)
	s.updateRun(ctx, run)
	runID := run.RunID
	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     formatInterruptPrompt(info),
		SessionId: run.SessionID,
		Msg:       "pending_user_approval",
		RunId:     &runID,
		Approval:  approval,
	}
}

func (s *runtimeServiceImpl) resumePendingApproval(ctx context.Context, run *store.RunInfo, req *bot_runtime.ResumeAgentReq, info *adk.InterruptInfo, tracker *budgetTracker, usage tokenUsage) *bot_runtime.ResumeAgentResp {
	approval := buildApprovalInfo(run.RunID, run.SessionID, req.Bot.BotId, req.UserId, run.ConversationID, info)
	run.Status = store.RunStatusPendingApproval
	run.ApprovalID = approval.ApprovalId
	run.InterruptedTool = approval.ToolName
	run.InterruptReason = "approval_required"
	run.Step = tracker.steps
	run.InputTokens = usage.input
	run.OutputTokens = usage.output
	run.CostUSD = tracker.cost(usage)
	setRunCheckpointJSON(run, approval)
	s.updateRun(ctx, run)
	return &bot_runtime.ResumeAgentResp{
		Success:   true,
		Reply:     formatInterruptPrompt(info),
		Usage:     usage.toRPC(),
		RunId:     run.RunID,
		SessionId: run.SessionID,
		Approval:  approval,
		Msg:       "pending_user_approval",
	}
}

func (s *runtimeServiceImpl) budgetExceeded(ctx context.Context, run *store.RunInfo, reason, partial string, tracker *budgetTracker, usage tokenUsage) *bot_runtime.RunAgentResp {
	s.finishRun(ctx, run, store.RunStatusBudgetExceeded, "", "", tracker, usage, reason)
	runID := run.RunID
	return &bot_runtime.RunAgentResp{
		Success:   true,
		Reply:     budgetReply(partial, reason),
		Usage:     usage.toRPC(),
		SessionId: run.SessionID,
		Msg:       "budget_exceeded:" + reason,
		RunId:     &runID,
	}
}

func (s *runtimeServiceImpl) resumeBudgetExceeded(ctx context.Context, run *store.RunInfo, reason, partial string, tracker *budgetTracker, usage tokenUsage) *bot_runtime.ResumeAgentResp {
	s.finishRun(ctx, run, store.RunStatusBudgetExceeded, run.ApprovalID, "", tracker, usage, reason)
	return &bot_runtime.ResumeAgentResp{
		Success:   true,
		Reply:     budgetReply(partial, reason),
		Usage:     usage.toRPC(),
		RunId:     run.RunID,
		SessionId: run.SessionID,
		Msg:       "budget_exceeded:" + reason,
	}
}

func budgetReply(partial, reason string) string {
	label := budgetLabel(reason)
	if strings.TrimSpace(partial) == "" {
		return "运行已中断：达到" + label + "预算。"
	}
	return partial + "\n\n[运行已中断：达到" + label + "预算，以上为已完成部分。]"
}

// applyGroundingGate 是 R5 的最后一道兜底：
// 当模型在没有说明“资料不足”的情况下给出确定性结论时，把它降级为可核实的说明，
// 避免 Self-RAG 已判定 IsSup=false 但最终答案仍然假装确定。
func applyGroundingGate(reply string) string {
	if !logic.AnswerNeedsGroundingRewrite(reply) {
		return reply
	}
	return "当前检索资料不足以支撑一个确定结论，我不会把猜测当作事实。\n\n" +
		"可以尝试：\n" +
		"- 换一个更具体的关键词或文档范围重新检索；\n" +
		"- 使用联网搜索补充外部资料；\n" +
		"- 或者补充你能提供的上下文，我再判断。\n\n" +
		"原始回答中的推测部分已隐藏，避免误导。"
}

func setRunCheckpointJSON(run *store.RunInfo, approval *bot_runtime.AgentApprovalInfo) {
	if run == nil || approval == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"run_id":       run.RunID,
		"session_id":   run.SessionID,
		"approval_id":  approval.ApprovalId,
		"interrupt_id": approval.InterruptId,
	})
	if err == nil {
		run.CheckpointJSON = string(payload)
	}
}

func budgetLabel(reason string) string {
	switch reason {
	case "max_steps":
		return "最大步数"
	case "max_input_tokens":
		return "输入 token"
	case "max_output_tokens":
		return "输出 token"
	case "max_cost_usd":
		return "成本"
	case "deadline":
		return "时间"
	default:
		return reason
	}
}

func (s *runtimeServiceImpl) finishRun(ctx context.Context, run *store.RunInfo, status, approvalID, tool string, tracker *budgetTracker, usage tokenUsage, reason string) {
	run.Status = status
	run.ApprovalID = approvalID
	run.InterruptedTool = tool
	run.InterruptReason = reason
	run.Step = tracker.steps
	run.InputTokens = usage.input
	run.OutputTokens = usage.output
	run.CostUSD = tracker.cost(usage)
	now := time.Now()
	run.FinishedAt = &now
	s.updateRun(ctx, run)
}

func (s *runtimeServiceImpl) createRun(ctx context.Context, run *store.RunInfo) {
	if s.runStore == nil || run == nil {
		return
	}
	if err := s.runStore.CreateRun(ctx, run); err != nil {
		log.Printf("创建Agent运行记录失败 run=%s err=%v", run.RunID, err)
	}
}

func (s *runtimeServiceImpl) updateRun(ctx context.Context, run *store.RunInfo) {
	if s.runStore == nil || run == nil {
		return
	}
	if err := s.runStore.UpdateRun(ctx, run); err != nil {
		log.Printf("更新Agent运行记录失败 run=%s err=%v", run.RunID, err)
	}
}

func (s *runtimeServiceImpl) newRunner(ag adk.Agent) *adk.Runner {
	return adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           ag,
		CheckPointStore: s.checkpointStore,
	})
}

func runBudgetDeadline(budget RunBudget) time.Time {
	if budget.Deadline <= 0 {
		return time.Now().Add(10 * time.Minute)
	}
	return time.Now().Add(budget.Deadline)
}

func buildApprovalInfo(runID, sessionID string, botID, userID, conversationID int64, info *adk.InterruptInfo) *bot_runtime.AgentApprovalInfo {
	approvalID := newRuntimeID("apv")
	interruptID := ""
	toolName := "unknown"
	arguments := ""
	if info != nil && len(info.InterruptContexts) > 0 && info.InterruptContexts[0] != nil {
		interruptID = info.InterruptContexts[0].ID
		if info.InterruptContexts[0].Info != nil {
			if ai, ok := info.InterruptContexts[0].Info.(*commontool.ApprovalInfo); ok {
				toolName = ai.ToolName
				arguments = ai.ArgumentsInJSON
			} else {
				arguments = fmt.Sprint(info.InterruptContexts[0].Info)
			}
		}
	}
	now := time.Now().Format(time.RFC3339)
	return &bot_runtime.AgentApprovalInfo{
		ApprovalId:     approvalID,
		RunId:          runID,
		InterruptId:    interruptID,
		SessionId:      sessionID,
		BotId:          botID,
		UserId:         userID,
		ConversationId: conversationID,
		Status:         "pending",
		ToolName:       toolName,
		ArgumentsJson:  redactApprovalArguments(arguments),
		Description:    fmt.Sprintf("工具 %s 需要你确认后才会执行。", toolName),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func redactApprovalArguments(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return truncateApprovalText(raw)
	}
	redactValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return truncateApprovalText(raw)
	}
	return truncateApprovalText(string(encoded))
}

func redactValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") ||
				strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
				strings.Contains(lower, "password") || strings.Contains(lower, "authorization") {
				typed[key] = "[REDACTED]"
				continue
			}
			redactValue(item)
		}
	case []any:
		for _, item := range typed {
			redactValue(item)
		}
	}
}

func truncateApprovalText(text string) string {
	runes := []rune(text)
	if len(runes) <= 500 {
		return text
	}
	return string(runes[:500]) + "..."
}

func schemaMessageFromStore(message store.Message) adk.Message {
	switch strings.ToLower(strings.TrimSpace(message.Role)) {
	case "assistant":
		return schema.AssistantMessage(message.Content, nil)
	case "system":
		return schema.SystemMessage(message.Content)
	case "tool":
		return &schema.Message{Role: schema.Tool, Content: message.Content}
	default:
		return schema.UserMessage(message.Content)
	}
}

func newRuntimeID(prefix string) string {
	if id, err := idgen.NextID(); err == nil {
		return fmt.Sprintf("%s_%d", prefix, id)
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func stringPtr(value string) *string {
	return &value
}

func failResume(msg string) *bot_runtime.ResumeAgentResp {
	return &bot_runtime.ResumeAgentResp{Success: false, Msg: msg}
}
