package service

import (
	"ClaranAIM/internal/agent-manager-service/dao"
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"ClaranAIM/pkg/memoryclient"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AgentService 定义 Agent 管理、权限、路由、对话和任务执行的业务契约。
// agent-manager-service 拥有 Agent 配置、路由规则、权限和计费记录；
// 真正的模型运行由 agent-runtime-service 承担，这里负责组装配置、鉴权、调用 runtime 和记录结果。
type AgentService interface {
	CreateBot(ctx context.Context, name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, ownerID, contextMessageLimit, memoryRecallLimit, maxOutputTokens int64, temperature float64, groupTriggerMode string, autoReplyEnabled bool, defaultAPIKey, defaultBaseURL, defaultModel string) (*model.Bot, error)
	UpdateBot(ctx context.Context, botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, isActive bool, isActiveSet bool, contextMessageLimit, memoryRecallLimit, maxOutputTokens int64, temperature float64, groupTriggerMode string, autoReplyEnabled bool, defaultAPIKey, defaultBaseURL, defaultModel string) error
	GetBot(ctx context.Context, botID int64) (*model.Bot, error)
	ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error)
	DeleteBot(ctx context.Context, botID, operatorID int64) error
	ChatWithBot(ctx context.Context, botID, userID, conversationID int64, message string) (*ChatResult, error)
	CreateRoute(ctx context.Context, botID int64, routePattern, routeType string, priority int64) (*model.BotRoute, error)
	ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error)
	DeleteRoute(ctx context.Context, routeID, operatorID int64) error
	GetBilling(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error)
	GrantPermission(ctx context.Context, botID, operatorID, userID int64, role string) error
	RevokePermission(ctx context.Context, botID, operatorID, userID int64) error
	ListPermissions(ctx context.Context, botID, operatorID int64) ([]model.BotPermission, error)
	RunAgentTask(ctx context.Context, botID, userID, conversationID int64, taskType, question string) (string, error)
	GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error)
}

// ChatResult 保存一次 Agent 对话的回复和运行元数据。
// 网关和异步 Dispatcher 通过 Status 判断这是普通回复、失败，还是等待用户确认的中断状态。
type ChatResult struct {
	Reply          string
	ConversationID int64
	Status         string
	SessionID      string
	InputTokens    int64
	OutputTokens   int64
	Cost           float64
}

// agentServiceImpl 是 AgentService 的默认实现，组合 DAO、runtime RPC、user RPC 和 memory 客户端。
type agentServiceImpl struct {
	botRepo          dao.BotRepository
	permissionRepo   dao.PermissionRepository
	routeRepo        dao.RouteRepository
	billingRepo      dao.BillingRepository
	subscriptionRepo dao.AgentSubscriptionRepository
	runtimeClient    botruntimeservice.Client
	userClient       userservice.Client
	workspaceBase    string
	memoryService    AgentMemoryService
	defaultAPIKey    string
	defaultBaseURL   string
	defaultModel     string
}

const (
	// DefaultContextMessageLimit 是 Agent 未配置时读取最近会话消息的默认条数。
	DefaultContextMessageLimit int64 = 80
	// MinContextMessageLimit 防止配置过小导致“总结会话”几乎看不到上下文。
	MinContextMessageLimit int64 = 10
	// MaxContextMessageLimit 防止一次任务把过多聊天历史塞进模型输入。
	MaxContextMessageLimit int64 = 500
	// DefaultMemoryRecallLimit 是长期记忆召回的默认条数。
	DefaultMemoryRecallLimit int64 = 12
	// MaxMemoryRecallLimit 防止长期记忆过多污染当前任务。
	MaxMemoryRecallLimit int64 = 50
	// DefaultGroupTriggerMode 表示群聊中默认只响应 @ 或命令式触发。
	DefaultGroupTriggerMode = "mention"
	skillSmokePrefix        = "[[CLARAN_SKILL_SMOKE_TEST]]"
)

// AgentMemoryService 是 agent-manager-service 依赖的最小 memory-service 契约。
// 这里避免直接 import memory-service 内部实现，保持微服务边界。
type AgentMemoryService interface {
	Recall(ctx context.Context, input memoryclient.RecallInput) (memoryclient.RecallResult, error)
	CreateMemory(ctx context.Context, input memoryclient.CreateMemoryInput) (*memoryclient.MemoryFact, error)
	ListMemories(ctx context.Context, viewerID int64, filter memoryclient.Filter) ([]memoryclient.MemoryFact, int64, error)
}

// NewAgentService 创建 Agent 管理服务。
// MySQL 保存 Agent 元数据和计费记录，runtimeClient 负责真正执行模型和工具调用。
func NewAgentService(botRepo dao.BotRepository, permissionRepo dao.PermissionRepository, routeRepo dao.RouteRepository, billingRepo dao.BillingRepository, runtimeClient botruntimeservice.Client, userClient userservice.Client, workspaceBase string) AgentService {
	return &agentServiceImpl{
		botRepo:        botRepo,
		permissionRepo: permissionRepo,
		routeRepo:      routeRepo,
		billingRepo:    billingRepo,
		runtimeClient:  runtimeClient,
		userClient:     userClient,
		workspaceBase:  workspaceBase,
	}
}

// SetAgentSubscriptionRepository 注入 Phase3 Agent-Native 订阅规则仓储。
// 这样可以在不改变 Kitex 对外接口的情况下，把旧路由规则同步成统一事件订阅规则。
func (s *agentServiceImpl) SetAgentSubscriptionRepository(repo dao.AgentSubscriptionRepository) {
	s.subscriptionRepo = repo
}

// SetMemoryService 注入 Phase4 长期记忆能力。
// 当前依赖的是窄接口，后续独立 memory RPC 只要实现同一契约即可替换。
func (s *agentServiceImpl) SetMemoryService(memory AgentMemoryService) {
	s.memoryService = memory
}

// SetDefaultLLM 注入平台默认模型供应商配置。
// internal Agent 在运行时使用这里的最新配置，而不是永久依赖创建时写入 bots 表的旧密钥。
func (s *agentServiceImpl) SetDefaultLLM(apiKey, baseURL, modelName string) {
	s.defaultAPIKey = apiKey
	s.defaultBaseURL = baseURL
	s.defaultModel = modelName
}

// CreateBot 创建一个由用户拥有的 Agent 配置。
// internal 类型使用平台默认模型供应商；custom 类型必须提供自己的 API Key 和 Base URL。
// 创建成功后会为 Agent 创建真实系统用户，并授予创建者 owner 权限。
func (s *agentServiceImpl) CreateBot(ctx context.Context, name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, ownerID, contextMessageLimit, memoryRecallLimit, maxOutputTokens int64, temperature float64, groupTriggerMode string, autoReplyEnabled bool, defaultAPIKey, defaultBaseURL, defaultModel string) (*model.Bot, error) {
	if name == "" {
		return nil, errors.New("bot名称不能为空")
	}

	if botType == "" {
		botType = "internal"
	}

	effectiveAPIKey := apiKey
	effectiveBaseURL := baseURL
	effectiveModel := modelName

	if botType == "internal" {
		effectiveAPIKey = defaultAPIKey
		effectiveBaseURL = defaultBaseURL
		if effectiveModel == "" {
			effectiveModel = defaultModel
		}
	} else {
		if apiKey == "" {
			return nil, errors.New("自部署Bot必须提供API Key")
		}
		if baseURL == "" {
			return nil, errors.New("自部署Bot必须提供Base URL")
		}
		if effectiveModel == "" {
			effectiveModel = defaultModel
		}
	}

	if toolPolicy == "" {
		toolPolicy = "safe"
	}
	if signature == "" {
		signature = description
	}
	agentUserID := int64(0)
	if s.userClient != nil {
		var err error
		agentUserID, err = s.createAgentUser(ctx, name, description)
		if err != nil {
			return nil, err
		}
	}

	bot := &model.Bot{
		Name:                name,
		Type:                botType,
		Description:         description,
		ModelName:           effectiveModel,
		APIKey:              effectiveAPIKey,
		BaseURL:             effectiveBaseURL,
		SystemPrompt:        systemPrompt,
		SkillsDir:           skillsDir,
		AgentRoot:           agentRoot,
		AgentUserID:         agentUserID,
		Avatar:              avatar,
		Signature:           signature,
		WorkspaceRoot:       workspaceRoot,
		ToolPolicy:          toolPolicy,
		ContextMessageLimit: contextMessageLimit,
		MemoryRecallLimit:   memoryRecallLimit,
		MaxOutputTokens:     maxOutputTokens,
		Temperature:         temperature,
		GroupTriggerMode:    groupTriggerMode,
		AutoReplyEnabled:    true,
		OwnerID:             ownerID,
		IsActive:            true,
	}
	normalizeAgentRuntimeSettings(bot)

	if err := s.botRepo.CreateBot(ctx, bot); err != nil {
		return nil, err
	}
	if bot.WorkspaceRoot == "" {
		bot.WorkspaceRoot = defaultWorkspaceRoot(s.workspaceBase, bot.ID)
		if err := s.botRepo.UpdateBot(ctx, bot); err != nil {
			return nil, fmt.Errorf("初始化Agent工作目录失败: %w", err)
		}
	}
	_ = s.permissionRepo.UpsertPermission(ctx, &model.BotPermission{BotID: bot.ID, UserID: ownerID, Role: "owner"})

	log.Printf("Bot创建成功: %s (id=%d, type=%s)", name, bot.ID, botType)
	return bot, nil
}

// UpdateBot 修改 Agent 配置，并由权限系统保证只有 owner/admin 可以操作。
func (s *agentServiceImpl) UpdateBot(ctx context.Context, botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, isActive bool, isActiveSet bool, contextMessageLimit, memoryRecallLimit, maxOutputTokens int64, temperature float64, groupTriggerMode string, autoReplyEnabled bool, defaultAPIKey, defaultBaseURL, defaultModel string) error {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot == nil {
		return errors.New("bot不存在")
	}
	if err := s.requireRole(ctx, bot, operatorID, "admin"); err != nil {
		return err
	}

	if name != "" {
		bot.Name = name
	}
	if description != "" {
		bot.Description = description
	}
	if modelName != "" {
		bot.ModelName = modelName
	}
	customProviderRequested := apiKey != "" || (baseURL != "" && baseURL != defaultBaseURL)
	if bot.Type == "internal" && customProviderRequested {
		bot.Type = "custom"
	}
	if bot.Type == "internal" {
		bot.APIKey = defaultAPIKey
		bot.BaseURL = defaultBaseURL
		if modelName == "" && defaultModel != "" {
			bot.ModelName = defaultModel
		}
	} else {
		if apiKey != "" {
			bot.APIKey = apiKey
		}
		if baseURL != "" {
			bot.BaseURL = baseURL
		}
		if bot.APIKey == "" || bot.BaseURL == "" {
			return errors.New("自定义Agent必须配置API Key和Base URL")
		}
	}
	if systemPrompt != "" {
		bot.SystemPrompt = systemPrompt
	}
	if skillsDir != "" {
		bot.SkillsDir = skillsDir
	}
	if agentRoot != "" {
		bot.AgentRoot = agentRoot
		bot.WorkspaceRoot = agentRoot
	}
	if avatar != "" {
		bot.Avatar = avatar
	}
	if signature != "" {
		bot.Signature = signature
	}
	if workspaceRoot != "" {
		bot.WorkspaceRoot = workspaceRoot
	}
	if toolPolicy != "" {
		bot.ToolPolicy = toolPolicy
	}
	if isActiveSet {
		bot.IsActive = isActive
	}
	if contextMessageLimit > 0 {
		bot.ContextMessageLimit = contextMessageLimit
	}
	if memoryRecallLimit > 0 {
		bot.MemoryRecallLimit = memoryRecallLimit
	}
	if maxOutputTokens >= 0 {
		bot.MaxOutputTokens = maxOutputTokens
	}
	if temperature >= 0 {
		bot.Temperature = temperature
	}
	if groupTriggerMode != "" {
		bot.GroupTriggerMode = groupTriggerMode
	}
	bot.AutoReplyEnabled = autoReplyEnabled
	normalizeAgentRuntimeSettings(bot)

	if err := s.botRepo.UpdateBot(ctx, bot); err != nil {
		return err
	}

	log.Printf("Bot更新成功: id=%d", botID)
	return nil
}

// GetBot 查询单个 Agent 配置。
func (s *agentServiceImpl) GetBot(ctx context.Context, botID int64) (*model.Bot, error) {
	return s.botRepo.GetBotByID(ctx, botID)
}

// ListBots 查询某个用户拥有的 Agent，可按类型过滤。
func (s *agentServiceImpl) ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error) {
	return s.botRepo.ListBots(ctx, ownerID, botType)
}

// DeleteBot 删除创建者拥有的 Agent 配置。
func (s *agentServiceImpl) DeleteBot(ctx context.Context, botID, operatorID int64) error {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot == nil {
		return errors.New("bot不存在")
	}
	if bot.OwnerID != operatorID {
		return errors.New("只能删除自己创建的bot")
	}

	if err := s.botRepo.DeleteBot(ctx, botID); err != nil {
		return err
	}

	log.Printf("Bot删除成功: id=%d", botID)
	return nil
}

// ChatWithBot 将一轮用户输入交给配置好的 Agent runtime 执行。
// 记忆按 Agent、用户、会话隔离，确保同一个 Agent 在不同 IM 会话中不会串上下文；
// token 计费严格使用 runtime/Eino 返回的 usage，缺失时记录 usage_missing 且 token 为 0。
func (s *agentServiceImpl) ChatWithBot(ctx context.Context, botID, userID, conversationID int64, message string) (*ChatResult, error) {
	botInfo, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return nil, err
	}
	if botInfo == nil {
		return nil, errors.New("bot不存在")
	}
	if !botInfo.IsActive {
		return nil, errors.New("bot已停用")
	}
	if botInfo.AgentUserID > 0 && userID == botInfo.AgentUserID {
		log.Printf("Agent自回声静默: trigger_path=ChatWithBot bot_id=%d user_id=%d agent_user_id=%d conversation_id=%d", botID, userID, botInfo.AgentUserID, conversationID)
		return &ChatResult{ConversationID: conversationID, Status: "silent_agent_echo"}, nil
	}
	s.applyDefaultProvider(botInfo)
	if botInfo.APIKey == "" {
		return nil, errors.New("bot未配置API Key，请联系管理员或配置自部署Bot的API Key")
	}
	if botInfo.BaseURL == "" {
		return nil, errors.New("bot未配置Base URL，请联系管理员或配置自部署Bot的Base URL")
	}

	if err := s.requireRole(ctx, botInfo, userID, "operator"); err != nil {
		return nil, err
	}
	if s.runtimeClient == nil {
		return nil, errors.New("agent-runtime-service未配置")
	}
	sessionID := defaultAgentSessionID(botID, userID, conversationID)
	skillSmoke := shouldRunSkillSmoke(message, botInfo.SkillsDir)
	if skillSmoke {
		sessionID = fmt.Sprintf("skill_smoke_%d_user_%d_%d", botID, userID, time.Now().UnixNano())
	}
	runtimeInput := s.buildInputWithMemory(ctx, botInfo, userID, conversationID, sessionID, message)
	runtimeBot := s.runtimeConfig(botInfo)
	if skillSmoke {
		runtimeBot.IncludeDomainTools = false
		runtimeBot.ToolPolicy = "skill_only"
		runtimeInput = buildSkillSmokeInput(cleanSkillSmokeInput(message), runtimeBot.SkillsDir)
		log.Printf("Skill smoke test runtime: bot_id=%d user_id=%d skills_dir=%s tool_policy=%s include_domain_tools=false session_id=%s", botID, userID, runtimeBot.SkillsDir, runtimeBot.ToolPolicy, sessionID)
	}
	resp, err := s.runtimeClient.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            runtimeBot,
		UserId:         userID,
		ConversationId: conversationID,
		SessionId:      sessionID,
		Input:          runtimeInput,
	})
	if err != nil {
		s.recordBilling(ctx, botID, userID, conversationID, "chat_error", 0, 0, 0, botInfo.ModelName)
		return nil, err
	}
	if resp == nil || !resp.Success {
		s.recordBilling(ctx, botID, userID, conversationID, "chat_error", 0, 0, 0, botInfo.ModelName)
		if resp != nil && resp.Msg != "" {
			return nil, errors.New(resp.Msg)
		}
		return nil, errors.New("Agent执行失败")
	}
	inputTokens := int64(0)
	outputTokens := int64(0)
	usageSeen := false
	if resp.Usage != nil {
		inputTokens = resp.Usage.InputTokens
		outputTokens = resp.Usage.OutputTokens
		usageSeen = resp.Usage.UsageSeen
	}
	actualCost := tokenCost(botInfo.ModelName, inputTokens, outputTokens)
	action := "chat"
	if !usageSeen {
		action = "chat_usage_missing"
	}
	s.recordBilling(ctx, botID, userID, conversationID, action, inputTokens, outputTokens, actualCost, botInfo.ModelName)
	resultSessionID := resp.SessionId
	if resultSessionID == "" {
		resultSessionID = sessionID
	}
	if !skillSmoke {
		s.recordAgentRunMemory(ctx, botID, userID, conversationID, resultSessionID, message, resp.Reply)
	}

	log.Printf("Bot对话完成: bot_id=%d, user_id=%d, input_tokens=%d, output_tokens=%d, cost=%.6f, usage_seen=%v",
		botID, userID, inputTokens, outputTokens, actualCost, usageSeen)

	return &ChatResult{
		Reply:          resp.Reply,
		ConversationID: conversationID,
		Status:         resp.Msg,
		SessionID:      resultSessionID,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		Cost:           actualCost,
	}, nil
}

const defaultSkillSmokeMarker = "skill-smoke-ok"

func shouldRunSkillSmoke(message, skillsDir string) bool {
	text := strings.TrimSpace(message)
	if strings.HasPrefix(text, skillSmokePrefix) {
		return true
	}
	if strings.TrimSpace(skillsDir) == "" {
		return false
	}
	lower := strings.ToLower(text)
	hasSkill := strings.Contains(lower, "skill") || strings.Contains(text, "技能")
	if !hasSkill {
		return false
	}
	return strings.Contains(text, "测试") ||
		strings.Contains(text, "验证") ||
		strings.Contains(text, "运行") ||
		strings.Contains(text, "执行") ||
		strings.Contains(lower, "smoke")
}

func cleanSkillSmokeInput(message string) string {
	text := strings.TrimSpace(message)
	text = strings.TrimSpace(strings.TrimPrefix(text, skillSmokePrefix))
	return extractTriggeredContentForMemory(text)
}

func buildSkillSmokeInput(raw, skillsDir string) string {
	raw = strings.TrimSpace(raw)
	marker := skillSmokeMarkerFromDir(skillsDir)
	if raw == "" {
		raw = "请按已加载 Skill 的测试要求输出 marker"
	}
	return fmt.Sprintf("%s\n\n这是 Skill 读取测试。不要调用任何工具，不要列出工具执行结果，不要创建文件，不要生成 SKILL.md 模板，不要介绍 skill_creator。请只根据系统提示中已经加载的 SKILL.md 行为指令回答；如果系统提示中没有加载到 SKILL.md，就直接说明未加载。最终回复必须原样包含 marker：%s。", raw, marker)
}

func skillSmokeMarkerFromDir(skillsDir string) string {
	content := readSkillMarkdownForSmoke(skillsDir)
	if content == "" {
		return defaultSkillSmokeMarker
	}
	if marker := extractSkillSmokeMarker(content); marker != "" {
		return marker
	}
	return defaultSkillSmokeMarker
}

func readSkillMarkdownForSmoke(skillsDir string) string {
	skillsDir = strings.TrimSpace(skillsDir)
	if skillsDir == "" {
		return ""
	}
	if absSkillsDir, absErr := filepath.Abs(skillsDir); absErr == nil {
		skillsDir = absSkillsDir
	}
	candidates := []string{filepath.Join(skillsDir, "SKILL.md")}
	_ = filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		rel, relErr := filepath.Rel(skillsDir, path)
		if relErr != nil {
			return nil
		}
		if len(strings.Split(filepath.ToSlash(rel), "/")) <= 5 {
			candidates = append(candidates, path)
		}
		return nil
	})
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(data)) != "" {
			return string(data)
		}
	}
	return ""
}

func extractSkillSmokeMarker(content string) string {
	lines := strings.Split(content, "\n")
	backtickToken := regexp.MustCompile("`([A-Za-z0-9][A-Za-z0-9_-]{5,})`")
	plainToken := regexp.MustCompile(`\b([A-Z][A-Z0-9]+(?:[_-][A-Z0-9]+){1,})\b`)
	for _, line := range lines {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "marker") {
			continue
		}
		if match := backtickToken.FindStringSubmatch(line); len(match) == 2 {
			return match[1]
		}
		if match := plainToken.FindStringSubmatch(line); len(match) == 2 {
			return match[1]
		}
	}
	for _, line := range lines {
		if match := plainToken.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

// defaultAgentSessionID 生成 Agent 长会话 key，按 Agent、用户、会话三元组隔离。
func defaultAgentSessionID(botID, userID, conversationID int64) string {
	return fmt.Sprintf("agent_%d_user_%d_conv_%d", botID, userID, conversationID)
}

// buildInputWithMemory 在用户输入前注入可召回的长期记忆。
// 记忆只作为辅助背景，用户当前输入仍具有更高优先级。
func (s *agentServiceImpl) buildInputWithMemory(ctx context.Context, botInfo *model.Bot, userID, conversationID int64, sessionID, message string) string {
	if s.memoryService == nil {
		return message
	}
	if botInfo == nil {
		return message
	}
	normalizeAgentRuntimeSettings(botInfo)
	result, err := s.memoryService.Recall(ctx, memoryclient.RecallInput{
		BotID:          botInfo.ID,
		UserID:         userID,
		ConversationID: conversationID,
		SessionID:      sessionID,
		Limit:          int(botInfo.MemoryRecallLimit),
		Query:          message,
	})
	if err != nil || strings.TrimSpace(result.ContextText) == "" {
		if err != nil {
			log.Printf("召回Agent长期记忆失败: bot_id=%d user_id=%d err=%v", botInfo.ID, userID, err)
		}
		return message
	}
	return fmt.Sprintf("%s\n\n注入策略：以下记忆只是可能相关的长期背景；如果和当前问题无关，不要强行使用；用户当前输入优先级高于记忆。\n\n用户本次输入：\n%s", result.ContextText, message)
}

// recordAgentRunMemory 将一次 Agent 交互摘要写入会话记忆，供后续跨轮召回。
func (s *agentServiceImpl) recordAgentRunMemory(ctx context.Context, botID, userID, conversationID int64, sessionID, userMessage, reply string) {
	if s.memoryService == nil {
		return
	}
	content := summarizeAgentRunMemory(userMessage, reply)
	if content == "" {
		return
	}
	if s.agentRunMemoryExists(ctx, botID, userID, conversationID, sessionID, content) {
		return
	}
	_, err := s.memoryService.CreateMemory(ctx, memoryclient.CreateMemoryInput{
		BotID:          botID,
		UserID:         userID,
		OwnerUserID:    userID,
		ConversationID: conversationID,
		SessionID:      sessionID,
		Scope:          memoryclient.ScopeConversation,
		Type:           memoryclient.TypeAgentRun,
		Content:        content,
		Source:         "agent_run",
		Visibility:     memoryclient.VisibilityPrivate,
		VectorStatus:   memoryclient.VectorPending,
		Confidence:     0.5,
	})
	if err != nil {
		log.Printf("写入Agent运行摘要记忆失败: bot_id=%d user_id=%d err=%v", botID, userID, err)
	}
}

// agentRunMemoryExists 查询最近同类运行摘要，避免 Kafka 重试或前端重复触发时写入完全相同的记忆。
func (s *agentServiceImpl) agentRunMemoryExists(ctx context.Context, botID, userID, conversationID int64, sessionID, content string) bool {
	if s.memoryService == nil || strings.TrimSpace(content) == "" {
		return false
	}
	memories, _, err := s.memoryService.ListMemories(ctx, userID, memoryclient.Filter{
		BotID:           botID,
		UserID:          userID,
		ConversationID:  conversationID,
		SessionID:       sessionID,
		Scopes:          []string{memoryclient.ScopeConversation},
		Types:           []string{memoryclient.TypeAgentRun},
		IncludeDisabled: false,
		Limit:           10,
	})
	if err != nil {
		return false
	}
	for _, memory := range memories {
		if strings.TrimSpace(memory.Content) == strings.TrimSpace(content) {
			return true
		}
	}
	return false
}

// summarizeAgentRunMemory 把用户输入和 Agent 回复压缩成适合长期记忆保存的短文本。
func summarizeAgentRunMemory(userMessage, reply string) string {
	userMessage = extractTriggeredContentForMemory(userMessage)
	userMessage = truncateRunMemoryText(userMessage, 240)
	reply = truncateRunMemoryText(reply, 360)
	if !agentRunHasLongTermValue(userMessage, reply) {
		return ""
	}
	if userMessage == "" && reply == "" {
		return ""
	}
	if reply == "" {
		return "用户请求：" + userMessage
	}
	if userMessage == "" {
		return "Agent回复：" + reply
	}
	return fmt.Sprintf("用户请求：%s\nAgent回复：%s", userMessage, reply)
}

func agentRunHasLongTermValue(userMessage, reply string) bool {
	combined := strings.ToLower(strings.TrimSpace(userMessage + "\n" + reply))
	if len([]rune(combined)) < 28 {
		return false
	}
	noise := []string{"你好", "hello", "hi", "好的", "收到", "谢谢", "测试", "在吗", "嗯", "可以", "没事", "总结一下", "帮我看看", "解释一下"}
	for _, item := range noise {
		if combined == item || (strings.Contains(combined, item) && len([]rune(combined)) < 40) {
			return false
		}
	}
	negativeSignals := []string{
		"system prompt", "系统提示词", "会话材料", "以下是可能相关的长期记忆", "当前触发内容",
		"根据上文", "本轮对话", "最近消息", "搜索结果", "检索结果", "命中来源", "参考资料",
		"这段会话", "总结如下", "临时", "刚才", "今天先", "等会", "稍后",
	}
	for _, signal := range negativeSignals {
		if strings.Contains(combined, strings.ToLower(signal)) {
			return false
		}
	}
	explicitSignals := []string{
		"长期记住", "请记住", "帮我记住", "记住：", "我的偏好", "我偏好", "我习惯", "我总是",
		"我的长期目标", "长期目标", "以后都", "下次请", "我正在学习", "我已经掌握", "我不懂", "我困惑",
		"我的项目", "项目状态", "我的配置", "我的工作目录", "我的 api key", "我的apikey",
	}
	for _, signal := range explicitSignals {
		if strings.Contains(combined, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

// extractTriggeredContentForMemory 从 Agent-Native 包装输入中抽出真正的用户触发内容。
// 运行摘要不应该保存 system prompt、会话材料和长期记忆注入，否则会污染后续召回。
func extractTriggeredContentForMemory(text string) string {
	text = strings.TrimSpace(text)
	marker := "当前触发内容："
	idx := strings.Index(text, marker)
	if idx < 0 {
		return text
	}
	rest := strings.TrimSpace(text[idx+len(marker):])
	for _, endMarker := range []string{"\n\n会话材料", "\n会话材料", "\n\n事件信息", "\n事件信息"} {
		if end := strings.Index(rest, endMarker); end >= 0 {
			rest = strings.TrimSpace(rest[:end])
			break
		}
	}
	return rest
}

// truncateRunMemoryText 清理换行并截断记忆文本，避免单条记忆过长。
func truncateRunMemoryText(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// recordBilling 记录一次 Agent 行为的实际 token 用量、费用和模型名称。
func (s *agentServiceImpl) recordBilling(ctx context.Context, botID, userID, conversationID int64, action string, inputTokens, outputTokens int64, cost float64, modelName string) {
	record := &model.BillingRecord{
		BotID:          botID,
		UserID:         userID,
		ConversationID: conversationID,
		Action:         action,
		TokenCount:     inputTokens + outputTokens,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		Cost:           cost,
		ModelName:      modelName,
	}
	if err := s.billingRepo.CreateRecord(ctx, record); err != nil {
		log.Printf("记录计费信息失败: %v", err)
	}
}

// modelTokenUsage 表示从模型响应中提取到的 token 用量。
type modelTokenUsage struct {
	InputTokens  int64
	OutputTokens int64
	Seen         bool
}

// createAgentUser 通过 user-service 为 Agent 创建真实系统用户。
// 该用户不可正常登录，但可以作为 IM 消息发送者、好友和群成员存在。
func (s *agentServiceImpl) createAgentUser(ctx context.Context, name, description string) (int64, error) {
	username := fmt.Sprintf("agent_%d", timeNowUnixNano())
	resp, err := s.userClient.Register(ctx, &user.RegisterReq{
		Username: username,
		Password: fmt.Sprintf("agent-%d", timeNowUnixNano()),
		Nickname: name,
		IsSystem: true,
	})
	if err != nil || resp == nil || !resp.Success {
		log.Printf("创建Agent系统用户失败: err=%v resp=%v", err, resp)
		if err != nil {
			return 0, fmt.Errorf("创建Agent系统用户失败: %w", err)
		}
		msg := "user-service返回失败"
		if resp != nil && resp.Msg != "" {
			msg = resp.Msg
		}
		return 0, fmt.Errorf("创建Agent系统用户失败: %s", msg)
	}
	if description != "" {
		_, _ = s.userClient.UpdateUserInfo(ctx, &user.UpdateUserInfoReq{
			UserId:     resp.UserId,
			Nickname:   name,
			Signature:  description,
			FullUpdate: false,
		})
	}
	return resp.UserId, nil
}

// requireRole 校验用户是否拥有指定最低 Agent 协作角色。
// 创建者天然拥有最高权限；非创建者必须在权限表中存在对应角色。
func (s *agentServiceImpl) requireRole(ctx context.Context, bot *model.Bot, userID int64, minRole string) error {
	if bot.OwnerID == userID {
		return nil
	}
	permission, err := s.permissionRepo.GetPermission(ctx, bot.ID, userID)
	if err != nil {
		return err
	}
	if permission == nil {
		return errors.New("无权操作该Agent")
	}
	if roleRank(permission.Role) < roleRank(minRole) {
		return errors.New("Agent权限不足")
	}
	return nil
}

// roleRank 将 Agent 角色映射为可比较的等级：owner > admin > operator > viewer。
func roleRank(role string) int {
	switch role {
	case "owner":
		return 4
	case "admin":
		return 3
	case "operator":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

// validPermissionRole 校验可被授予的协作角色，owner 只能由创建者天然拥有，不能手动授予。
func validPermissionRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}

// runtimeConfig 将数据库中的 Agent 配置转换为 agent-runtime-service 的运行配置。
func (s *agentServiceImpl) runtimeConfig(bot *model.Bot) *bot_runtime.RuntimeBotConfig {
	normalizeAgentRuntimeSettings(bot)
	workspace := bot.WorkspaceRoot
	if workspace == "" {
		workspace = defaultWorkspaceRoot(s.workspaceBase, bot.ID)
	}
	return &bot_runtime.RuntimeBotConfig{
		BotId:               bot.ID,
		AgentUserId:         bot.AgentUserID,
		Name:                bot.Name,
		Description:         bot.Description,
		ModelName:           bot.ModelName,
		ApiKey:              bot.APIKey,
		BaseUrl:             bot.BaseURL,
		SystemPrompt:        bot.SystemPrompt,
		SkillsDir:           bot.SkillsDir,
		WorkspaceRoot:       workspace,
		ToolPolicy:          bot.ToolPolicy,
		IncludeDomainTools:  true,
		ContextMessageLimit: bot.ContextMessageLimit,
		MemoryRecallLimit:   bot.MemoryRecallLimit,
		MaxOutputTokens:     bot.MaxOutputTokens,
		Temperature:         bot.Temperature,
		GroupTriggerMode:    bot.GroupTriggerMode,
		AutoReplyEnabled:    bot.AutoReplyEnabled,
	}
}

// applyDefaultProvider 为 internal Agent 覆盖最新平台默认供应商配置。
// 这避免平台换 Key/BaseURL 后，历史 Agent 继续使用 bots 表里的旧快照而触发 401。
func (s *agentServiceImpl) applyDefaultProvider(bot *model.Bot) {
	if s == nil || bot == nil || bot.Type != "internal" {
		return
	}
	configuredBaseURL := bot.BaseURL
	if s.defaultAPIKey != "" {
		bot.APIKey = s.defaultAPIKey
	}
	if s.defaultBaseURL != "" {
		bot.BaseURL = s.defaultBaseURL
	}
	if s.defaultModel != "" && (strings.TrimSpace(bot.ModelName) == "" || strings.TrimSpace(configuredBaseURL) == "" || sameProviderEndpoint(configuredBaseURL, s.defaultBaseURL)) {
		bot.ModelName = s.defaultModel
	}
}

func sameProviderEndpoint(a, b string) bool {
	a = strings.TrimRight(strings.ToLower(strings.TrimSpace(a)), "/")
	b = strings.TrimRight(strings.ToLower(strings.TrimSpace(b)), "/")
	return a != "" && b != "" && a == b
}

// normalizeAgentRuntimeSettings 对 Agent 运行参数做服务端兜底和范围裁剪。
// 前端传来的值只作为用户偏好，最终仍由这里保证不会过小、过大或落到未知触发模式。
func normalizeAgentRuntimeSettings(bot *model.Bot) {
	if bot == nil {
		return
	}
	if bot.ContextMessageLimit <= 0 {
		bot.ContextMessageLimit = DefaultContextMessageLimit
	}
	if bot.ContextMessageLimit < MinContextMessageLimit {
		bot.ContextMessageLimit = MinContextMessageLimit
	}
	if bot.ContextMessageLimit > MaxContextMessageLimit {
		bot.ContextMessageLimit = MaxContextMessageLimit
	}
	if bot.MemoryRecallLimit <= 0 {
		bot.MemoryRecallLimit = DefaultMemoryRecallLimit
	}
	if bot.MemoryRecallLimit > MaxMemoryRecallLimit {
		bot.MemoryRecallLimit = MaxMemoryRecallLimit
	}
	if bot.MaxOutputTokens < 0 {
		bot.MaxOutputTokens = 0
	}
	if bot.Temperature < 0 {
		bot.Temperature = 0
	}
	if bot.Temperature > 2 {
		bot.Temperature = 2
	}
	switch strings.ToLower(strings.TrimSpace(bot.GroupTriggerMode)) {
	case "mention", "mention_only", "keyword", "command", "all", "silent":
		bot.GroupTriggerMode = strings.ToLower(strings.TrimSpace(bot.GroupTriggerMode))
		if bot.GroupTriggerMode == "mention_only" {
			bot.GroupTriggerMode = "mention"
		}
	default:
		bot.GroupTriggerMode = DefaultGroupTriggerMode
	}
}

// defaultWorkspaceRoot 生成 Agent 默认工作目录，确保所有工作文件落在受控 storage/agent/files 下。
func defaultWorkspaceRoot(base string, botID int64) string {
	if base == "" {
		base = "storage/agent/files"
	}
	if botID <= 0 {
		return filepath.Join(base, "pending")
	}
	return filepath.Join(base, fmt.Sprintf("%d", botID))
}

// timeNowUnixNano 返回纳秒时间戳，用于生成系统用户临时用户名等低风险唯一值。
func timeNowUnixNano() int64 {
	return time.Now().UnixNano()
}

// tokenCost 按模型价格和实际输入/输出 token 计算费用。
func tokenCost(modelName string, inputTokens, outputTokens int64) float64 {
	inputYuanPerMillion, outputYuanPerMillion := modelTokenPrice(modelName)
	return float64(inputTokens)/1_000_000*inputYuanPerMillion + float64(outputTokens)/1_000_000*outputYuanPerMillion
}

// modelTokenPrice 返回指定模型每百万 token 的输入/输出价格，单位人民币。
func modelTokenPrice(modelName string) (inputYuanPerMillion, outputYuanPerMillion float64) {
	const usdToCny = 7.30
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(normalized, "glm-4.7") || strings.Contains(normalized, "glm4.7"):
		// GLM-4.7 价格按 Z.AI 官方美元计价折算：输入 $0.6/M tokens，输出 $2.2/M tokens。
		return 0.6 * usdToCny, 2.2 * usdToCny
	default:
		return 0, 0
	}
}

// CreateRoute 在确认 Agent 存在后创建路由规则，并尝试同步为 Agent-Native 订阅规则。
func (s *agentServiceImpl) CreateRoute(ctx context.Context, botID int64, routePattern, routeType string, priority int64) (*model.BotRoute, error) {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, errors.New("bot不存在")
	}

	route := &model.BotRoute{
		BotID:        botID,
		RoutePattern: routePattern,
		RouteType:    routeType,
		Priority:     priority,
	}

	if err := s.routeRepo.CreateRoute(ctx, route); err != nil {
		return nil, err
	}
	if err := s.mirrorRouteToAgentSubscription(ctx, bot, route); err != nil {
		return nil, err
	}

	log.Printf("路由创建成功: bot_id=%d, pattern=%s", botID, routePattern)
	return route, nil
}

// mirrorRouteToAgentSubscription 将旧路由规则镜像到新的 Agent 事件订阅表。
func (s *agentServiceImpl) mirrorRouteToAgentSubscription(ctx context.Context, bot *model.Bot, route *model.BotRoute) error {
	if s.subscriptionRepo == nil || bot == nil || route == nil || bot.AgentUserID <= 0 {
		return nil
	}
	rule, ok := subscriptionRuleFromRoute(bot, route)
	if !ok {
		return nil
	}
	return s.subscriptionRepo.UpsertRouteMirror(ctx, rule)
}

// subscriptionRuleFromRoute 把旧 route_type/route_pattern 翻译成统一订阅规则。
func subscriptionRuleFromRoute(bot *model.Bot, route *model.BotRoute) (*model.AgentSubscriptionRule, bool) {
	ruleType := strings.ToLower(strings.TrimSpace(route.RouteType))
	rule := &model.AgentSubscriptionRule{
		BotID:         bot.ID,
		AgentUserID:   bot.AgentUserID,
		SourceRouteID: route.ID,
		EventTypes:    "message.created",
		Action:        "trigger",
		IsActive:      true,
	}
	switch ruleType {
	case "agent_keyword", "keyword":
		rule.TriggerMode = "keyword"
		rule.Keywords = strings.TrimSpace(route.RoutePattern)
	case "agent_command", "command":
		rule.TriggerMode = "command"
		rule.CommandPrefix = strings.TrimSpace(route.RoutePattern)
	case "agent_record", "record", "silent":
		rule.EventTypes = strings.TrimSpace(route.RoutePattern)
		if rule.EventTypes == "" {
			rule.EventTypes = "message.created"
		}
		rule.TriggerMode = "all"
		rule.Action = "record"
		rule.Silent = true
	default:
		return nil, false
	}
	return rule, true
}

// ListRoutes 查询某个 Agent 的全部路由规则。
func (s *agentServiceImpl) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	return s.routeRepo.ListRoutes(ctx, botID)
}

// DeleteRoute 删除一条路由规则，并同步清理由它镜像出的 Agent 订阅规则。
// operatorID 预留给未来路由记录带 owner 元数据后的权限检查。
func (s *agentServiceImpl) DeleteRoute(ctx context.Context, routeID, operatorID int64) error {
	if s.subscriptionRepo != nil && s.routeRepo != nil {
		route, err := s.routeRepo.GetRouteByID(ctx, routeID)
		if err != nil {
			return err
		}
		if route != nil {
			_ = s.subscriptionRepo.DeleteRouteMirror(ctx, route.BotID, routeID)
		}
	}
	return s.routeRepo.DeleteRoute(ctx, routeID)
}

// GetBilling 分页查询某个 Agent 和用户组合的计费记录。
func (s *agentServiceImpl) GetBilling(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.billingRepo.ListRecords(ctx, botID, userID, limit, offset)
}

// GrantPermission 允许 owner/admin 给其他用户授予 Agent 协作权限。
func (s *agentServiceImpl) GrantPermission(ctx context.Context, botID, operatorID, userID int64, role string) error {
	if !validPermissionRole(role) {
		return errors.New("无效的Agent权限角色")
	}
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot == nil {
		return errors.New("bot不存在")
	}
	if err := s.requireRole(ctx, bot, operatorID, "admin"); err != nil {
		return err
	}
	if userID == bot.OwnerID {
		return errors.New("不能修改创建者权限")
	}
	return s.permissionRepo.UpsertPermission(ctx, &model.BotPermission{BotID: botID, UserID: userID, Role: role})
}

// RevokePermission 撤销一个协作者的 Agent 权限。
func (s *agentServiceImpl) RevokePermission(ctx context.Context, botID, operatorID, userID int64) error {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot == nil {
		return errors.New("bot不存在")
	}
	if err := s.requireRole(ctx, bot, operatorID, "admin"); err != nil {
		return err
	}
	if userID == bot.OwnerID {
		return errors.New("不能撤销创建者权限")
	}
	return s.permissionRepo.DeletePermission(ctx, botID, userID)
}

// ListPermissions 在至少 viewer 权限通过后返回协作者角色列表。
func (s *agentServiceImpl) ListPermissions(ctx context.Context, botID, operatorID int64) ([]model.BotPermission, error) {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, errors.New("bot不存在")
	}
	if err := s.requireRole(ctx, bot, operatorID, "viewer"); err != nil {
		return nil, err
	}
	return s.permissionRepo.ListPermissions(ctx, botID)
}

// RunAgentTask 将总结、问答、洞察和候选回复等结构化任务转发给 agent-runtime-service。
func (s *agentServiceImpl) RunAgentTask(ctx context.Context, botID, userID, conversationID int64, taskType, question string) (string, error) {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return "", err
	}
	if bot == nil {
		return "", errors.New("bot不存在")
	}
	if err := s.requireRole(ctx, bot, userID, "operator"); err != nil {
		return "", err
	}
	s.applyDefaultProvider(bot)
	if s.runtimeClient == nil {
		return "", errors.New("agent-runtime-service未配置")
	}
	req := &bot_runtime.AgentTaskReq{
		Bot:            s.runtimeConfig(bot),
		UserId:         userID,
		ConversationId: conversationID,
		TaskType:       taskType,
		Question:       question,
	}
	var resp *bot_runtime.AgentTaskResp
	switch taskType {
	case "summary":
		resp, err = s.runtimeClient.SummarizeConversation(ctx, req)
	case "ask":
		resp, err = s.runtimeClient.AskConversation(ctx, req)
	case "insights":
		resp, err = s.runtimeClient.ExtractInsights(ctx, req)
	case "reply_candidates":
		resp, err = s.runtimeClient.GenerateReplyCandidates(ctx, req)
	default:
		return "", errors.New("未知Agent任务类型")
	}
	if err != nil {
		return "", err
	}
	if resp == nil || !resp.Success {
		if resp != nil && resp.Msg != "" {
			return "", errors.New(resp.Msg)
		}
		return "", errors.New("Agent任务执行失败")
	}
	usage := resp.Usage
	inputTokens, outputTokens := int64(0), int64(0)
	usageSeen := false
	if usage != nil {
		inputTokens = usage.InputTokens
		outputTokens = usage.OutputTokens
		usageSeen = usage.UsageSeen
	}
	action := taskType
	if !usageSeen {
		action = taskType + "_usage_missing"
	}
	s.recordBilling(ctx, botID, userID, conversationID, action, inputTokens, outputTokens, tokenCost(bot.ModelName, inputTokens, outputTokens), bot.ModelName)
	return resp.Result_, nil
}

// GetBotByAgentUserID 通过 Agent 的 IM 系统用户 ID 反查 Agent 配置。
func (s *agentServiceImpl) GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error) {
	return s.botRepo.GetBotByAgentUserID(ctx, agentUserID)
}
