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
	botRepo     dao.BotRepository
	routeRepo   dao.RouteRepository
	billingRepo dao.BillingRepository
	agentCache  map[int64]adk.Agent
	mu          sync.RWMutex
}

func NewBotService(botRepo dao.BotRepository, routeRepo dao.RouteRepository, billingRepo dao.BillingRepository) BotService {
	return &botServiceImpl{
		botRepo:     botRepo,
		routeRepo:   routeRepo,
		billingRepo: billingRepo,
		agentCache:  make(map[int64]adk.Agent),
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

	startTime := time.Now()
	iter := ag.Run(ctx, &adk.AgentInput{
		Messages: []adk.Message{schema.UserMessage(message)},
	})

	var reply string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			s.recordBilling(ctx, botID, userID, "chat_error", 0, 0)
			return "", conversationID, fmt.Errorf("对话失败: %w", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err == nil && msg != nil && msg.Content != "" {
				reply = msg.Content
			}
		}
	}

	elapsed := time.Since(startTime)

	if reply == "" {
		s.recordBilling(ctx, botID, userID, "chat_empty", 0, 0)
		return "", conversationID, errors.New("对话返回为空")
	}

	estimatedTokens := int64(len(message)+len(reply)) / 4
	estimatedCost := float64(estimatedTokens) * 0.0001
	s.recordBilling(ctx, botID, userID, "chat", estimatedTokens, estimatedCost)

	log.Printf("Bot对话完成: bot_id=%d, user_id=%d, tokens≈%d, cost≈%.6f, elapsed=%v",
		botID, userID, estimatedTokens, estimatedCost, elapsed)

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

	ag, err := agent.NewDeepAgent(ctx, chatModel, botInfo.AgentRoot, "", "", botInfo.SkillsDir)
	if err != nil {
		return nil, fmt.Errorf("创建Agent失败: %w", err)
	}

	s.agentCache[botInfo.ID] = ag
	return ag, nil
}

func (s *botServiceImpl) recordBilling(ctx context.Context, botID, userID int64, action string, tokenCount int64, cost float64) {
	record := &model.BillingRecord{
		BotID:      botID,
		UserID:     userID,
		Action:     action,
		TokenCount: tokenCount,
		Cost:       cost,
	}
	if err := s.billingRepo.CreateRecord(ctx, record); err != nil {
		log.Printf("记录计费信息失败: %v", err)
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
