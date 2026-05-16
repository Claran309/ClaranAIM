package service

import (
	"ClaranAIM/internal/bot-manager-service/agent"
	"ClaranAIM/internal/bot-manager-service/component"
	"ClaranAIM/internal/bot-manager-service/dao"
	"ClaranAIM/internal/bot-manager-service/model"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type BotService interface {
	CreateBot(ctx context.Context, name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot string, ownerID int64, defaultAPIKey, defaultBaseURL, defaultModel string) (*model.Bot, error)
	UpdateBot(ctx context.Context, botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot string, isActive bool, defaultAPIKey, defaultBaseURL, defaultModel string) error
	GetBot(ctx context.Context, botID int64) (*model.Bot, error)
	ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error)
	DeleteBot(ctx context.Context, botID, operatorID int64) error
	ChatWithBot(ctx context.Context, botID, userID, conversationID int64, message string) (string, int64, error)
	CreateRoute(ctx context.Context, botID int64, routePattern, routeType string, priority int64) (*model.BotRoute, error)
	ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error)
	DeleteRoute(ctx context.Context, routeID, operatorID int64) error
	GetBilling(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error)
}

type botServiceImpl struct {
	botRepo      dao.BotRepository
	routeRepo    dao.RouteRepository
	billingRepo  dao.BillingRepository
	agentCache   map[int64]adk.Agent
	sessionStore *component.Store
	mu           sync.RWMutex
}

// NewBotService keeps Bot metadata in MySQL and conversational memory on disk.
// The in-memory agent cache avoids rebuilding the same LLM agent for every chat
// turn, but UpdateBot/DeleteBot invalidate it when configuration changes.
func NewBotService(botRepo dao.BotRepository, routeRepo dao.RouteRepository, billingRepo dao.BillingRepository, sessionDir string) BotService {
	var store *component.Store
	if sessionDir != "" {
		var err error
		store, err = component.NewStore(sessionDir)
		if err != nil {
			log.Printf("创建会话存储失败: %v，记忆功能将不可用", err)
		} else {
			log.Printf("会话存储已初始化: %s", sessionDir)
		}
	} else {
		log.Println("未配置session_dir，记忆功能将不可用")
	}

	return &botServiceImpl{
		botRepo:      botRepo,
		routeRepo:    routeRepo,
		billingRepo:  billingRepo,
		agentCache:   make(map[int64]adk.Agent),
		sessionStore: store,
	}
}

func (s *botServiceImpl) CreateBot(ctx context.Context, name, botType, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot string, ownerID int64, defaultAPIKey, defaultBaseURL, defaultModel string) (*model.Bot, error) {
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

	bot := &model.Bot{
		Name:         name,
		Type:         botType,
		Description:  description,
		ModelName:    effectiveModel,
		APIKey:       effectiveAPIKey,
		BaseURL:      effectiveBaseURL,
		SystemPrompt: systemPrompt,
		SkillsDir:    skillsDir,
		AgentRoot:    agentRoot,
		OwnerID:      ownerID,
		IsActive:     true,
	}

	if err := s.botRepo.CreateBot(ctx, bot); err != nil {
		return nil, err
	}

	log.Printf("Bot创建成功: %s (id=%d, type=%s)", name, bot.ID, botType)
	return bot, nil
}

func (s *botServiceImpl) UpdateBot(ctx context.Context, botID, operatorID int64, name, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot string, isActive bool, defaultAPIKey, defaultBaseURL, defaultModel string) error {
	bot, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot == nil {
		return errors.New("bot不存在")
	}
	if bot.OwnerID != operatorID {
		return errors.New("只能修改自己创建的bot")
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
	}
	bot.IsActive = isActive

	if err := s.botRepo.UpdateBot(ctx, bot); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.agentCache, botID)
	s.mu.Unlock()

	log.Printf("Bot更新成功: id=%d", botID)
	return nil
}

func (s *botServiceImpl) GetBot(ctx context.Context, botID int64) (*model.Bot, error) {
	return s.botRepo.GetBotByID(ctx, botID)
}

func (s *botServiceImpl) ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error) {
	return s.botRepo.ListBots(ctx, ownerID, botType)
}

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

	s.mu.Lock()
	delete(s.agentCache, botID)
	s.mu.Unlock()

	log.Printf("Bot删除成功: id=%d", botID)
	return nil
}

func (s *botServiceImpl) ChatWithBot(ctx context.Context, botID, userID, conversationID int64, message string) (string, int64, error) {
	botInfo, err := s.botRepo.GetBotByID(ctx, botID)
	if err != nil {
		return "", conversationID, err
	}
	if botInfo == nil {
		return "", conversationID, errors.New("bot不存在")
	}
	if !botInfo.IsActive {
		return "", conversationID, errors.New("bot已停用")
	}
	if botInfo.APIKey == "" {
		return "", conversationID, errors.New("bot未配置API Key，请联系管理员或配置自部署Bot的API Key")
	}
	if botInfo.BaseURL == "" {
		return "", conversationID, errors.New("bot未配置Base URL，请联系管理员或配置自部署Bot的Base URL")
	}

	ag, err := s.getOrCreateAgent(ctx, botInfo)
	if err != nil {
		return "", conversationID, fmt.Errorf("创建Agent失败: %w", err)
	}

	// Conversation ID scopes memory so the same Bot can behave independently in
	// different chats. This is the AIM bridge: AI replies are driven by IM context.
	sessionID := fmt.Sprintf("bot_%d_conv_%d", botID, conversationID)

	var historyMsgs []adk.Message
	if s.sessionStore != nil {
		session, sessErr := s.sessionStore.GetSession(sessionID)
		if sessErr != nil {
			log.Printf("获取会话记忆失败: %v，将不使用历史记忆", sessErr)
		} else {
			historyMsgs = session.GetMessages()
			log.Printf("加载会话记忆: session=%s, 历史消息数=%d", sessionID, len(historyMsgs))
		}
	}

	userMsg := schema.UserMessage(message)

	inputMsgs := make([]adk.Message, 0, len(historyMsgs)+1)
	inputMsgs = append(inputMsgs, historyMsgs...)
	inputMsgs = append(inputMsgs, userMsg)

	startTime := time.Now()
	iter := ag.Run(ctx, &adk.AgentInput{
		Messages: inputMsgs,
	})

	var reply string
	var replyCollector botReplyCollector
	var usage modelTokenUsage
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			s.recordBilling(ctx, botID, userID, conversationID, "chat_error", 0, 0, 0, botInfo.ModelName)
			return "", conversationID, fmt.Errorf("对话失败: %w", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err == nil && msg != nil {
				replyCollector.mergeResolvedMessage(event.Output.MessageOutput.Role, msg)
				usage.mergeMessageUsage(msg)
			}
		}
	}

	elapsed := time.Since(startTime)
	reply = replyCollector.String()

	if reply == "" {
		s.recordBilling(ctx, botID, userID, conversationID, "chat_empty", 0, 0, 0, botInfo.ModelName)
		return "", conversationID, errors.New("对话返回为空")
	}

	if s.sessionStore != nil {
		session, sessErr := s.sessionStore.GetSession(sessionID)
		if sessErr == nil {
			if appendErr := session.Append(userMsg); appendErr != nil {
				log.Printf("保存用户消息到记忆失败: %v", appendErr)
			}
			assistantMsg := schema.AssistantMessage(reply, nil)
			if appendErr := session.Append(assistantMsg); appendErr != nil {
				log.Printf("保存AI回复到记忆失败: %v", appendErr)
			}
			log.Printf("会话记忆已保存: session=%s", sessionID)
		}
	}

	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	actualCost := tokenCost(botInfo.ModelName, inputTokens, outputTokens)
	action := "chat"
	if !usage.Seen {
		action = "chat_usage_missing"
		log.Printf("Bot对话缺少模型usage，计费token按0记录: bot_id=%d, user_id=%d, session=%s", botID, userID, sessionID)
	}
	s.recordBilling(ctx, botID, userID, conversationID, action, inputTokens, outputTokens, actualCost, botInfo.ModelName)

	log.Printf("Bot对话完成: bot_id=%d, user_id=%d, session=%s, input_tokens=%d, output_tokens=%d, cost=%.6f, usage_seen=%v, elapsed=%v",
		botID, userID, sessionID, inputTokens, outputTokens, actualCost, usage.Seen, elapsed)

	return reply, conversationID, nil
}

func (s *botServiceImpl) getOrCreateAgent(ctx context.Context, botInfo *model.Bot) (adk.Agent, error) {
	s.mu.RLock()
	if ag, ok := s.agentCache[botInfo.ID]; ok {
		s.mu.RUnlock()
		return ag, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if ag, ok := s.agentCache[botInfo.ID]; ok {
		return ag, nil
	}

	chatModel, err := component.NewChatModel(ctx, botInfo.APIKey, botInfo.BaseURL, botInfo.ModelName)
	if err != nil {
		return nil, fmt.Errorf("创建ChatModel失败: %w", err)
	}

	// Internal bots receive the project's built-in tool set; custom bots can be
	// configured as lighter external assistants with their own model endpoint.
	includeDomainTools := botInfo.Name == "Amiya" || botInfo.Type == "internal"
	ag, err := agent.NewDeepAgent(ctx, chatModel, botInfo.AgentRoot, "", "", botInfo.SkillsDir, botInfo.Name, botInfo.Description, botInfo.SystemPrompt, includeDomainTools)
	if err != nil {
		return nil, fmt.Errorf("创建Agent失败: %w", err)
	}

	s.agentCache[botInfo.ID] = ag
	return ag, nil
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

type botReplyCollector struct {
	parts []string
}

func (r *botReplyCollector) mergeMessage(output *adk.MessageVariant) {
	if output == nil {
		return
	}
	msg, err := output.GetMessage()
	if err != nil {
		return
	}
	r.mergeResolvedMessage(output.Role, msg)
}

func (r *botReplyCollector) mergeResolvedMessage(role schema.RoleType, msg *schema.Message) {
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

func (r *botReplyCollector) String() string {
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
	last := []rune(prev)[len([]rune(prev))-1]
	first := []rune(next)[0]
	return isASCIIWord(last) && isASCIIWord(first)
}

func isASCIIWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func (u *modelTokenUsage) mergeMessageUsage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	u.Seen = true
	u.InputTokens += int64(usage.PromptTokens)
	u.OutputTokens += int64(usage.CompletionTokens)
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

	log.Printf("路由创建成功: bot_id=%d, pattern=%s", botID, routePattern)
	return route, nil
}

func (s *botServiceImpl) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	return s.routeRepo.ListRoutes(ctx, botID)
}

func (s *botServiceImpl) DeleteRoute(ctx context.Context, routeID, operatorID int64) error {
	return s.routeRepo.DeleteRoute(ctx, routeID)
}

func (s *botServiceImpl) GetBilling(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.billingRepo.ListRecords(ctx, botID, userID, limit, offset)
}
