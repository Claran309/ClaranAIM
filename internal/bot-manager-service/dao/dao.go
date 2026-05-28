// Package dao contains bot-manager-service persistence adapters.
package dao

import (
	"ClaranAIM/internal/bot-manager-service/model"
	"context"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB opens MySQL and performs non-destructive migrations for bot tables.
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.Bot{},
		&model.BotPermission{},
		&model.AgentDispatchRecord{},
		&model.AgentSubscriptionRule{},
		&model.AgentAuditRecord{},
		&model.BotRoute{},
		&model.BillingRecord{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

// AgentDispatchRepository stores Kafka @Agent consumer progress for idempotency.
type AgentDispatchRepository interface {
	Start(ctx context.Context, record *model.AgentDispatchRecord) (bool, error)
	MarkCompleted(ctx context.Context, eventID string, agentUserID, replyMsgID int64) error
	MarkFailed(ctx context.Context, eventID string, agentUserID int64, message string) error
}

type agentDispatchRepositoryImpl struct {
	db *gorm.DB
}

// NewAgentDispatchRepo creates a GORM-backed Agent dispatch repository.
func NewAgentDispatchRepo(db *gorm.DB) AgentDispatchRepository {
	return &agentDispatchRepositoryImpl{db: db}
}

// Start inserts a started record. It returns false when the event was already
// completed, and true for new or retryable failed/started records.
func (r *agentDispatchRepositoryImpl) Start(ctx context.Context, record *model.AgentDispatchRecord) (bool, error) {
	var existing model.AgentDispatchRecord
	err := r.db.WithContext(ctx).
		Where("event_id = ? AND agent_user_id = ?", record.EventID, record.AgentUserID).
		First(&existing).Error
	if err == nil {
		if existing.Status == "completed" {
			return false, nil
		}
		return true, r.db.WithContext(ctx).Model(&existing).Updates(map[string]interface{}{
			"status":          "started",
			"bot_id":          record.BotID,
			"event_type":      record.EventType,
			"decision":        record.Decision,
			"source_event_id": record.SourceEventID,
			"agent_trace_id":  record.AgentTraceID,
			"source_msg_id":   record.SourceMsgID,
			"conversation_id": record.ConversationID,
			"sender_id":       record.SenderID,
			"error_message":   "",
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
		return false, err
	}
	return true, r.db.WithContext(ctx).Create(record).Error
}

// MarkCompleted stores the reply message id after a successful dispatch.
func (r *agentDispatchRepositoryImpl) MarkCompleted(ctx context.Context, eventID string, agentUserID, replyMsgID int64) error {
	return r.db.WithContext(ctx).Model(&model.AgentDispatchRecord{}).
		Where("event_id = ? AND agent_user_id = ?", eventID, agentUserID).
		Updates(map[string]interface{}{
			"status":       "completed",
			"reply_msg_id": replyMsgID,
		}).Error
}

// MarkFailed stores the last failure reason while keeping the event retryable.
func (r *agentDispatchRepositoryImpl) MarkFailed(ctx context.Context, eventID string, agentUserID int64, message string) error {
	return r.db.WithContext(ctx).Model(&model.AgentDispatchRecord{}).
		Where("event_id = ? AND agent_user_id = ?", eventID, agentUserID).
		Updates(map[string]interface{}{
			"status":        "failed",
			"error_message": message,
		}).Error
}

// AgentSubscriptionRepository stores Agent-native event subscription rules.
type AgentSubscriptionRepository interface {
	ListActiveRules(ctx context.Context, conversationID int64, eventType string) ([]model.AgentSubscriptionRule, error)
	UpsertRouteMirror(ctx context.Context, rule *model.AgentSubscriptionRule) error
	DeleteRouteMirror(ctx context.Context, botID int64, routeID int64) error
}

type agentSubscriptionRepositoryImpl struct {
	db *gorm.DB
}

// NewAgentSubscriptionRepo creates a GORM-backed subscription repository.
func NewAgentSubscriptionRepo(db *gorm.DB) AgentSubscriptionRepository {
	return &agentSubscriptionRepositoryImpl{db: db}
}

// ListActiveRules returns rules that can observe the current conversation and event.
func (r *agentSubscriptionRepositoryImpl) ListActiveRules(ctx context.Context, conversationID int64, eventType string) ([]model.AgentSubscriptionRule, error) {
	var rules []model.AgentSubscriptionRule
	query := r.db.WithContext(ctx).Where("is_active = ?", true)
	if conversationID > 0 {
		query = query.Where("conversation_id = 0 OR conversation_id = ?", conversationID)
	}
	if err := query.Find(&rules).Error; err != nil {
		return nil, err
	}
	filtered := make([]model.AgentSubscriptionRule, 0, len(rules))
	for _, rule := range rules {
		if eventType == "" || containsCSVToken(rule.EventTypes, eventType) || rule.EventTypes == "" {
			filtered = append(filtered, rule)
		}
	}
	return filtered, nil
}

// UpsertRouteMirror keeps legacy bot route rules usable by Agent-native dispatch.
func (r *agentSubscriptionRepositoryImpl) UpsertRouteMirror(ctx context.Context, rule *model.AgentSubscriptionRule) error {
	if rule == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("source_route_id = ? AND bot_id = ?", rule.SourceRouteID, rule.BotID).
		Assign(map[string]interface{}{
			"agent_user_id":     rule.AgentUserID,
			"conversation_id":   rule.ConversationID,
			"conversation_type": rule.ConversationType,
			"event_types":       rule.EventTypes,
			"keywords":          rule.Keywords,
			"command_prefix":    rule.CommandPrefix,
			"trigger_mode":      rule.TriggerMode,
			"action":            rule.Action,
			"silent":            rule.Silent,
			"is_active":         rule.IsActive,
		}).
		FirstOrCreate(rule).Error
}

// DeleteRouteMirror removes the subscription generated from a bot route.
func (r *agentSubscriptionRepositoryImpl) DeleteRouteMirror(ctx context.Context, botID int64, routeID int64) error {
	return r.db.WithContext(ctx).
		Where("bot_id = ? AND source_route_id = ?", botID, routeID).
		Delete(&model.AgentSubscriptionRule{}).Error
}

// AgentAuditRepository stores Agent-native routing and action decisions.
type AgentAuditRepository interface {
	Create(ctx context.Context, record *model.AgentAuditRecord) error
}

type agentAuditRepositoryImpl struct {
	db *gorm.DB
}

// NewAgentAuditRepo creates a GORM-backed audit repository.
func NewAgentAuditRepo(db *gorm.DB) AgentAuditRepository {
	return &agentAuditRepositoryImpl{db: db}
}

// Create inserts one audit record.
func (r *agentAuditRepositoryImpl) Create(ctx context.Context, record *model.AgentAuditRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// BotRepository stores bot configurations.
type BotRepository interface {
	CreateBot(ctx context.Context, bot *model.Bot) error
	GetBotByID(ctx context.Context, id int64) (*model.Bot, error)
	ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error)
	UpdateBot(ctx context.Context, bot *model.Bot) error
	DeleteBot(ctx context.Context, id int64) error
	GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error)
}

type botRepositoryImpl struct {
	db *gorm.DB
}

// NewBotRepo creates a GORM-backed bot repository.
func NewBotRepo(db *gorm.DB) BotRepository {
	return &botRepositoryImpl{db: db}
}

// CreateBot inserts a bot configuration.
func (r *botRepositoryImpl) CreateBot(ctx context.Context, bot *model.Bot) error {
	return r.db.WithContext(ctx).Create(bot).Error
}

// GetBotByID returns one bot or nil when it does not exist.
func (r *botRepositoryImpl) GetBotByID(ctx context.Context, id int64) (*model.Bot, error) {
	var bot model.Bot
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&bot).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &bot, nil
}

// ListBots returns bot configurations filtered by owner and optional type.
func (r *botRepositoryImpl) ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error) {
	var bots []model.Bot
	query := r.db.WithContext(ctx)
	if ownerID > 0 {
		query = query.Where("owner_id = ?", ownerID)
	}
	if botType != "" {
		query = query.Where("type = ?", botType)
	}
	if err := query.Find(&bots).Error; err != nil {
		return nil, err
	}
	return bots, nil
}

// UpdateBot saves a changed bot configuration.
func (r *botRepositoryImpl) UpdateBot(ctx context.Context, bot *model.Bot) error {
	return r.db.WithContext(ctx).Save(bot).Error
}

// DeleteBot removes a bot configuration by primary key.
func (r *botRepositoryImpl) DeleteBot(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Bot{}, id).Error
}

// GetBotByAgentUserID returns the bot bound to an Agent system user.
func (r *botRepositoryImpl) GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error) {
	var bot model.Bot
	if err := r.db.WithContext(ctx).Where("agent_user_id = ?", agentUserID).First(&bot).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &bot, nil
}

// PermissionRepository stores per-Agent collaborator roles.
type PermissionRepository interface {
	UpsertPermission(ctx context.Context, permission *model.BotPermission) error
	DeletePermission(ctx context.Context, botID, userID int64) error
	GetPermission(ctx context.Context, botID, userID int64) (*model.BotPermission, error)
	ListPermissions(ctx context.Context, botID int64) ([]model.BotPermission, error)
}

type permissionRepositoryImpl struct {
	db *gorm.DB
}

// NewPermissionRepo creates a GORM-backed permission repository.
func NewPermissionRepo(db *gorm.DB) PermissionRepository {
	return &permissionRepositoryImpl{db: db}
}

// UpsertPermission grants or updates one user's Agent role.
func (r *permissionRepositoryImpl) UpsertPermission(ctx context.Context, permission *model.BotPermission) error {
	return r.db.WithContext(ctx).
		Where("bot_id = ? AND user_id = ?", permission.BotID, permission.UserID).
		Assign(map[string]interface{}{"role": permission.Role}).
		FirstOrCreate(permission).Error
}

// DeletePermission revokes one user's Agent role.
func (r *permissionRepositoryImpl) DeletePermission(ctx context.Context, botID, userID int64) error {
	return r.db.WithContext(ctx).
		Where("bot_id = ? AND user_id = ?", botID, userID).
		Delete(&model.BotPermission{}).Error
}

// GetPermission returns one permission row or nil.
func (r *permissionRepositoryImpl) GetPermission(ctx context.Context, botID, userID int64) (*model.BotPermission, error) {
	var permission model.BotPermission
	if err := r.db.WithContext(ctx).Where("bot_id = ? AND user_id = ?", botID, userID).First(&permission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &permission, nil
}

// ListPermissions returns all collaborator roles for one Agent.
func (r *permissionRepositoryImpl) ListPermissions(ctx context.Context, botID int64) ([]model.BotPermission, error) {
	var permissions []model.BotPermission
	err := r.db.WithContext(ctx).Where("bot_id = ?", botID).Find(&permissions).Error
	return permissions, err
}

// RouteRepository stores bot routing rules.
type RouteRepository interface {
	CreateRoute(ctx context.Context, route *model.BotRoute) error
	GetRouteByID(ctx context.Context, id int64) (*model.BotRoute, error)
	ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error)
	DeleteRoute(ctx context.Context, id int64) error
}

type routeRepositoryImpl struct {
	db *gorm.DB
}

// NewRouteRepo creates a GORM-backed route repository.
func NewRouteRepo(db *gorm.DB) RouteRepository {
	return &routeRepositoryImpl{db: db}
}

// CreateRoute inserts a route rule.
func (r *routeRepositoryImpl) CreateRoute(ctx context.Context, route *model.BotRoute) error {
	return r.db.WithContext(ctx).Create(route).Error
}

// GetRouteByID returns one route or nil.
func (r *routeRepositoryImpl) GetRouteByID(ctx context.Context, id int64) (*model.BotRoute, error) {
	var route model.BotRoute
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&route).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &route, nil
}

// ListRoutes returns routes ordered by priority for one bot.
func (r *routeRepositoryImpl) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	var routes []model.BotRoute
	if err := r.db.WithContext(ctx).Where("bot_id = ?", botID).Order("priority DESC").Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

// DeleteRoute removes a route rule.
func (r *routeRepositoryImpl) DeleteRoute(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.BotRoute{}, id).Error
}

// BillingRepository stores actual token usage and cost records.
type BillingRepository interface {
	CreateRecord(ctx context.Context, record *model.BillingRecord) error
	ListRecords(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error)
}

type billingRepositoryImpl struct {
	db *gorm.DB
}

// NewBillingRepo creates a GORM-backed billing repository.
func NewBillingRepo(db *gorm.DB) BillingRepository {
	return &billingRepositoryImpl{db: db}
}

// CreateRecord inserts one billing record.
func (r *billingRepositoryImpl) CreateRecord(ctx context.Context, record *model.BillingRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// ListRecords returns a paginated billing page and total count.
func (r *billingRepositoryImpl) ListRecords(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error) {
	var records []model.BillingRecord
	var total int64
	query := r.db.WithContext(ctx).Model(&model.BillingRecord{})
	if botID > 0 {
		query = query.Where("bot_id = ?", botID)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("created_at DESC").Limit(int(limit)).Offset(int(offset)).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func containsCSVToken(csv string, token string) bool {
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}
