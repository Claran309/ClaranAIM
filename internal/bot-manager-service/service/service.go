package service

import (
	"ClaranAIM/internal/bot-manager-service/dao"
	"ClaranAIM/internal/bot-manager-service/model"
	memorymodel "ClaranAIM/internal/memory-service/model"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/kitex_gen/bot_runtime"
	"ClaranAIM/kitex_gen/bot_runtime/botruntimeservice"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// BotService defines the bot management and runtime chat contract.
//
// bot-manager-service owns bot configuration, routing rules, local conversation
// memory and billing records. The current implementation executes bot chats in
// this service; a future bot-runtime-service can reuse this interface boundary.
type BotService interface {
	CreateBot(ctx context.Context, name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, ownerID int64, defaultAPIKey, defaultBaseURL, defaultModel string) (*model.Bot, error)
	UpdateBot(ctx context.Context, botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, isActive bool, isActiveSet bool, defaultAPIKey, defaultBaseURL, defaultModel string) error
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

// ChatResult keeps runtime metadata that the HTTP gateway and asynchronous
// dispatcher need to distinguish a normal reply from a pending user approval.
type ChatResult struct {
	Reply          string
	ConversationID int64
	Status         string
	SessionID      string
	InputTokens    int64
	OutputTokens   int64
	Cost           float64
}

type botServiceImpl struct {
	botRepo          dao.BotRepository
	permissionRepo   dao.PermissionRepository
	routeRepo        dao.RouteRepository
	billingRepo      dao.BillingRepository
	subscriptionRepo dao.AgentSubscriptionRepository
	runtimeClient    botruntimeservice.Client
	userClient       userservice.Client
	workspaceBase    string
	memoryService    AgentMemoryService
}

// AgentMemoryService is the narrow memory-service contract needed by bot-manager.
type AgentMemoryService interface {
	Recall(ctx context.Context, input memorysvc.RecallInput) (memorysvc.RecallResult, error)
	CreateMemory(ctx context.Context, input memorysvc.CreateMemoryInput) (*memorymodel.MemoryFact, error)
}

// NewBotService keeps Bot metadata in MySQL and conversational memory on disk.
// The in-memory agent cache avoids rebuilding the same LLM agent for every chat
// turn, but UpdateBot/DeleteBot invalidate it when configuration changes.
func NewBotService(botRepo dao.BotRepository, permissionRepo dao.PermissionRepository, routeRepo dao.RouteRepository, billingRepo dao.BillingRepository, runtimeClient botruntimeservice.Client, userClient userservice.Client, workspaceBase string) BotService {
	return &botServiceImpl{
		botRepo:        botRepo,
		permissionRepo: permissionRepo,
		routeRepo:      routeRepo,
		billingRepo:    billingRepo,
		runtimeClient:  runtimeClient,
		userClient:     userClient,
		workspaceBase:  workspaceBase,
	}
}

// SetAgentSubscriptionRepository lets the process wire the optional Phase3
// Agent-native subscription repository without changing the public Kitex-facing
// BotService interface.
func (s *botServiceImpl) SetAgentSubscriptionRepository(repo dao.AgentSubscriptionRepository) {
	s.subscriptionRepo = repo
}

// SetMemoryService injects Phase4 long-term memory without changing the Kitex
// BotService interface. The standalone memory RPC can later implement the same
// narrow contract.
func (s *botServiceImpl) SetMemoryService(memory AgentMemoryService) {
	s.memoryService = memory
}

// CreateBot creates a bot configuration owned by one user.
//
// Internal bots inherit the system default provider credentials; custom bots
// must provide their own API key and base URL.
func (s *botServiceImpl) CreateBot(ctx context.Context, name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, ownerID int64, defaultAPIKey, defaultBaseURL, defaultModel string) (*model.Bot, error) {
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
		Name:          name,
		Type:          botType,
		Description:   description,
		ModelName:     effectiveModel,
		APIKey:        effectiveAPIKey,
		BaseURL:       effectiveBaseURL,
		SystemPrompt:  systemPrompt,
		SkillsDir:     skillsDir,
		AgentRoot:     agentRoot,
		AgentUserID:   agentUserID,
		Avatar:        avatar,
		Signature:     signature,
		WorkspaceRoot: workspaceRoot,
		ToolPolicy:    toolPolicy,
		OwnerID:       ownerID,
		IsActive:      true,
	}

	if err := s.botRepo.CreateBot(ctx, bot); err != nil {
		return nil, err
	}
	if bot.WorkspaceRoot == "" {
		bot.WorkspaceRoot = defaultWorkspaceRoot(s.workspaceBase, bot.ID)
		_ = s.botRepo.UpdateBot(ctx, bot)
	}
	_ = s.permissionRepo.UpsertPermission(ctx, &model.BotPermission{BotID: bot.ID, UserID: ownerID, Role: "owner"})

	log.Printf("Bot创建成功: %s (id=%d, type=%s)", name, bot.ID, botType)
	return bot, nil
}

// UpdateBot modifies bot settings and invalidates the cached runtime agent.
func (s *botServiceImpl) UpdateBot(ctx context.Context, botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, avatar, signature, workspaceRoot, toolPolicy string, isActive bool, isActiveSet bool, defaultAPIKey, defaultBaseURL, defaultModel string) error {
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
	if bot.Type == "internal" {
		if apiKey != "" || baseURL != "" {
			return errors.New("内部Bot不允许修改API Key和Base URL")
		}
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

	if err := s.botRepo.UpdateBot(ctx, bot); err != nil {
		return err
	}

	log.Printf("Bot更新成功: id=%d", botID)
	return nil
}

// GetBot returns one bot configuration.
func (s *botServiceImpl) GetBot(ctx context.Context, botID int64) (*model.Bot, error) {
	return s.botRepo.GetBotByID(ctx, botID)
}

// ListBots returns bots owned by a user, optionally filtered by type.
func (s *botServiceImpl) ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error) {
	return s.botRepo.ListBots(ctx, ownerID, botType)
}

// DeleteBot removes a bot configuration owned by operatorID and drops the cached
// runtime instance.
func (s *botServiceImpl) DeleteBot(ctx context.Context, botID, operatorID int64) error {
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

// ChatWithBot runs one user message through the configured Eino agent.
//
// Memory is scoped by bot ID and conversation ID so the same bot can keep
// independent context in different IM conversations. Token billing uses the
// usage metadata returned by Eino messages; when usage is absent, the billing
// record is marked as usage-missing with zero tokens.
func (s *botServiceImpl) ChatWithBot(ctx context.Context, botID, userID, conversationID int64, message string) (*ChatResult, error) {
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
		return nil, errors.New("bot-runtime-service未配置")
	}
	sessionID := defaultAgentSessionID(botID, userID, conversationID)
	runtimeInput := s.buildInputWithMemory(ctx, botID, userID, conversationID, sessionID, message)
	resp, err := s.runtimeClient.RunAgent(ctx, &bot_runtime.RunAgentReq{
		Bot:            s.runtimeConfig(botInfo),
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
	s.recordAgentRunMemory(ctx, botID, userID, conversationID, resultSessionID, message, resp.Reply)

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

func defaultAgentSessionID(botID, userID, conversationID int64) string {
	return fmt.Sprintf("agent_%d_user_%d_conv_%d", botID, userID, conversationID)
}

func (s *botServiceImpl) buildInputWithMemory(ctx context.Context, botID, userID, conversationID int64, sessionID, message string) string {
	if s.memoryService == nil {
		return message
	}
	result, err := s.memoryService.Recall(ctx, memorysvc.RecallInput{
		BotID:          botID,
		UserID:         userID,
		ConversationID: conversationID,
		SessionID:      sessionID,
		Limit:          12,
	})
	if err != nil || strings.TrimSpace(result.ContextText) == "" {
		if err != nil {
			log.Printf("召回Agent长期记忆失败: bot_id=%d user_id=%d err=%v", botID, userID, err)
		}
		return message
	}
	return fmt.Sprintf("%s\n\n请只把上面的长期记忆作为辅助背景，不要泄露记忆系统内部字段；如果用户纠正了记忆，以用户当前说法为准。\n\n用户本次输入：\n%s", result.ContextText, message)
}

func (s *botServiceImpl) recordAgentRunMemory(ctx context.Context, botID, userID, conversationID int64, sessionID, userMessage, reply string) {
	if s.memoryService == nil {
		return
	}
	content := summarizeAgentRunMemory(userMessage, reply)
	if content == "" {
		return
	}
	_, err := s.memoryService.CreateMemory(ctx, memorysvc.CreateMemoryInput{
		BotID:          botID,
		UserID:         userID,
		OwnerUserID:    userID,
		ConversationID: conversationID,
		SessionID:      sessionID,
		Scope:          memorymodel.ScopeConversation,
		Type:           memorymodel.TypeAgentRun,
		Content:        content,
		Source:         "agent_run",
		Visibility:     memorymodel.VisibilityPrivate,
		VectorStatus:   memorymodel.VectorPending,
		Confidence:     0.5,
	})
	if err != nil {
		log.Printf("写入Agent运行摘要记忆失败: bot_id=%d user_id=%d err=%v", botID, userID, err)
	}
}

func summarizeAgentRunMemory(userMessage, reply string) string {
	userMessage = truncateRunMemoryText(userMessage, 240)
	reply = truncateRunMemoryText(reply, 360)
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

func (s *botServiceImpl) recordBilling(ctx context.Context, botID, userID, conversationID int64, action string, inputTokens, outputTokens int64, cost float64, modelName string) {
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

type modelTokenUsage struct {
	InputTokens  int64
	OutputTokens int64
	Seen         bool
}

func (s *botServiceImpl) createAgentUser(ctx context.Context, name, description string) (int64, error) {
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

func (s *botServiceImpl) requireRole(ctx context.Context, bot *model.Bot, userID int64, minRole string) error {
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

func validPermissionRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}

func (s *botServiceImpl) runtimeConfig(bot *model.Bot) *bot_runtime.RuntimeBotConfig {
	workspace := bot.WorkspaceRoot
	if workspace == "" {
		workspace = defaultWorkspaceRoot(s.workspaceBase, bot.ID)
	}
	return &bot_runtime.RuntimeBotConfig{
		BotId:              bot.ID,
		AgentUserId:        bot.AgentUserID,
		Name:               bot.Name,
		Description:        bot.Description,
		ModelName:          bot.ModelName,
		ApiKey:             bot.APIKey,
		BaseUrl:            bot.BaseURL,
		SystemPrompt:       bot.SystemPrompt,
		SkillsDir:          bot.SkillsDir,
		WorkspaceRoot:      workspace,
		ToolPolicy:         bot.ToolPolicy,
		IncludeDomainTools: bot.Name == "Amiya" || bot.Type == "internal",
	}
}

func defaultWorkspaceRoot(base string, botID int64) string {
	if base == "" {
		base = "storage/agent/workspaces"
	}
	if botID <= 0 {
		return filepath.Join(base, "pending")
	}
	return filepath.Join(base, fmt.Sprintf("%d", botID))
}

func timeNowUnixNano() int64 {
	return time.Now().UnixNano()
}

func tokenCost(modelName string, inputTokens, outputTokens int64) float64 {
	inputYuanPerMillion, outputYuanPerMillion := modelTokenPrice(modelName)
	return float64(inputTokens)/1_000_000*inputYuanPerMillion + float64(outputTokens)/1_000_000*outputYuanPerMillion
}

func modelTokenPrice(modelName string) (inputYuanPerMillion, outputYuanPerMillion float64) {
	const usdToCny = 7.30
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(normalized, "glm-4.7") || strings.Contains(normalized, "glm4.7"):
		// Z.AI official pricing: GLM-4.7 input $0.6/M tokens, output $2.2/M tokens.
		return 0.6 * usdToCny, 2.2 * usdToCny
	default:
		return 0, 0
	}
}

// CreateRoute creates a routing rule for a bot after confirming the bot exists.
func (s *botServiceImpl) CreateRoute(ctx context.Context, botID int64, routePattern, routeType string, priority int64) (*model.BotRoute, error) {
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

func (s *botServiceImpl) mirrorRouteToAgentSubscription(ctx context.Context, bot *model.Bot, route *model.BotRoute) error {
	if s.subscriptionRepo == nil || bot == nil || route == nil || bot.AgentUserID <= 0 {
		return nil
	}
	rule, ok := subscriptionRuleFromRoute(bot, route)
	if !ok {
		return nil
	}
	return s.subscriptionRepo.UpsertRouteMirror(ctx, rule)
}

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

// ListRoutes returns all routing rules for one bot.
func (s *botServiceImpl) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	return s.routeRepo.ListRoutes(ctx, botID)
}

// DeleteRoute removes one routing rule. operatorID is kept for future ownership
// checks when route records carry owner metadata.
func (s *botServiceImpl) DeleteRoute(ctx context.Context, routeID, operatorID int64) error {
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

// GetBilling returns paginated billing records for one bot/user pair.
func (s *botServiceImpl) GetBilling(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.billingRepo.ListRecords(ctx, botID, userID, limit, offset)
}

// GrantPermission allows Agent owners/admins to share operation access.
func (s *botServiceImpl) GrantPermission(ctx context.Context, botID, operatorID, userID int64, role string) error {
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

// RevokePermission removes a collaborator role.
func (s *botServiceImpl) RevokePermission(ctx context.Context, botID, operatorID, userID int64) error {
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

// ListPermissions returns collaborator roles after viewer-level access.
func (s *botServiceImpl) ListPermissions(ctx context.Context, botID, operatorID int64) ([]model.BotPermission, error) {
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

// RunAgentTask delegates structured Agent tasks to bot-runtime-service.
func (s *botServiceImpl) RunAgentTask(ctx context.Context, botID, userID, conversationID int64, taskType, question string) (string, error) {
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
	if s.runtimeClient == nil {
		return "", errors.New("bot-runtime-service未配置")
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

// GetBotByAgentUserID finds an Agent config from its IM user identity.
func (s *botServiceImpl) GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error) {
	return s.botRepo.GetBotByAgentUserID(ctx, agentUserID)
}
