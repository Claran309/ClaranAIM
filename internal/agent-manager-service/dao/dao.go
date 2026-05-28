// Package dao 包含 agent-manager-service 的持久化适配器。
package dao

import (
	"ClaranAIM/internal/agent-manager-service/model"
	"context"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 打开 MySQL 连接，并对 Agent 相关表执行非破坏性迁移。
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

// AgentDispatchRepository 保存 Kafka 触发的 Agent 调度进度，用于幂等处理。
type AgentDispatchRepository interface {
	Start(ctx context.Context, record *model.AgentDispatchRecord) (bool, error)
	MarkCompleted(ctx context.Context, eventID string, agentUserID, replyMsgID int64) error
	MarkFailed(ctx context.Context, eventID string, agentUserID int64, message string) error
}

// agentDispatchRepositoryImpl 是基于 GORM 的 Agent 调度记录仓储。
type agentDispatchRepositoryImpl struct {
	db *gorm.DB
}

// NewAgentDispatchRepo 创建基于 GORM 的 Agent 调度仓储。
func NewAgentDispatchRepo(db *gorm.DB) AgentDispatchRepository {
	return &agentDispatchRepositoryImpl{db: db}
}

// Start 写入或重置一条 started 调度记录。
// 如果同一事件已经 completed，则返回 false 表示无需再次执行；失败或未完成记录允许重试。
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

// MarkCompleted 在调度成功后记录 Agent 回复消息 ID。
func (r *agentDispatchRepositoryImpl) MarkCompleted(ctx context.Context, eventID string, agentUserID, replyMsgID int64) error {
	return r.db.WithContext(ctx).Model(&model.AgentDispatchRecord{}).
		Where("event_id = ? AND agent_user_id = ?", eventID, agentUserID).
		Updates(map[string]interface{}{
			"status":       "completed",
			"reply_msg_id": replyMsgID,
		}).Error
}

// MarkFailed 记录最近一次失败原因，并保留该事件后续可重试。
func (r *agentDispatchRepositoryImpl) MarkFailed(ctx context.Context, eventID string, agentUserID int64, message string) error {
	return r.db.WithContext(ctx).Model(&model.AgentDispatchRecord{}).
		Where("event_id = ? AND agent_user_id = ?", eventID, agentUserID).
		Updates(map[string]interface{}{
			"status":        "failed",
			"error_message": message,
		}).Error
}

// AgentSubscriptionRepository 保存 Agent-Native 事件订阅规则。
type AgentSubscriptionRepository interface {
	ListActiveRules(ctx context.Context, conversationID int64, eventType string) ([]model.AgentSubscriptionRule, error)
	UpsertRouteMirror(ctx context.Context, rule *model.AgentSubscriptionRule) error
	DeleteRouteMirror(ctx context.Context, botID int64, routeID int64) error
}

// agentSubscriptionRepositoryImpl 是基于 GORM 的 Agent 订阅规则仓储。
type agentSubscriptionRepositoryImpl struct {
	db *gorm.DB
}

// NewAgentSubscriptionRepo 创建基于 GORM 的订阅规则仓储。
func NewAgentSubscriptionRepo(db *gorm.DB) AgentSubscriptionRepository {
	return &agentSubscriptionRepositoryImpl{db: db}
}

// ListActiveRules 返回能观察当前会话和事件类型的启用规则。
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

// UpsertRouteMirror 将历史 bot 路由规则镜像为 Agent-Native 可消费的订阅规则。
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

// DeleteRouteMirror 删除由某条历史路由生成的订阅规则。
func (r *agentSubscriptionRepositoryImpl) DeleteRouteMirror(ctx context.Context, botID int64, routeID int64) error {
	return r.db.WithContext(ctx).
		Where("bot_id = ? AND source_route_id = ?", botID, routeID).
		Delete(&model.AgentSubscriptionRule{}).Error
}

// AgentAuditRepository 保存 Agent-Native 路由和行动决策审计。
type AgentAuditRepository interface {
	Create(ctx context.Context, record *model.AgentAuditRecord) error
}

// agentAuditRepositoryImpl 是基于 GORM 的 Agent 审计仓储。
type agentAuditRepositoryImpl struct {
	db *gorm.DB
}

// NewAgentAuditRepo 创建基于 GORM 的审计仓储。
func NewAgentAuditRepo(db *gorm.DB) AgentAuditRepository {
	return &agentAuditRepositoryImpl{db: db}
}

// Create 插入一条 Agent 审计记录。
func (r *agentAuditRepositoryImpl) Create(ctx context.Context, record *model.AgentAuditRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// BotRepository 保存 Agent 配置。
// 接口名保留 Bot 前缀用于兼容历史模型，业务语义已经是 Agent。
type BotRepository interface {
	CreateBot(ctx context.Context, bot *model.Bot) error
	GetBotByID(ctx context.Context, id int64) (*model.Bot, error)
	ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error)
	UpdateBot(ctx context.Context, bot *model.Bot) error
	DeleteBot(ctx context.Context, id int64) error
	GetBotByAgentUserID(ctx context.Context, agentUserID int64) (*model.Bot, error)
}

// botRepositoryImpl 是基于 GORM 的 Agent 配置仓储。
type botRepositoryImpl struct {
	db *gorm.DB
}

// NewBotRepo 创建基于 GORM 的 Agent 配置仓储。
func NewBotRepo(db *gorm.DB) BotRepository {
	return &botRepositoryImpl{db: db}
}

// CreateBot 插入一条 Agent 配置。
func (r *botRepositoryImpl) CreateBot(ctx context.Context, bot *model.Bot) error {
	return r.db.WithContext(ctx).Create(bot).Error
}

// GetBotByID 根据 ID 查询 Agent，不存在时返回 nil。
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

// ListBots 按创建者和可选类型查询 Agent 配置。
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

// UpdateBot 保存修改后的 Agent 配置。
func (r *botRepositoryImpl) UpdateBot(ctx context.Context, bot *model.Bot) error {
	return r.db.WithContext(ctx).Save(bot).Error
}

// DeleteBot 按主键删除 Agent 配置。
func (r *botRepositoryImpl) DeleteBot(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Bot{}, id).Error
}

// GetBotByAgentUserID 根据 Agent 系统用户 ID 查询绑定的 Agent 配置。
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

// PermissionRepository 保存每个 Agent 的协作者角色。
type PermissionRepository interface {
	UpsertPermission(ctx context.Context, permission *model.BotPermission) error
	DeletePermission(ctx context.Context, botID, userID int64) error
	GetPermission(ctx context.Context, botID, userID int64) (*model.BotPermission, error)
	ListPermissions(ctx context.Context, botID int64) ([]model.BotPermission, error)
}

// permissionRepositoryImpl 是基于 GORM 的 Agent 权限仓储。
type permissionRepositoryImpl struct {
	db *gorm.DB
}

// NewPermissionRepo 创建基于 GORM 的权限仓储。
func NewPermissionRepo(db *gorm.DB) PermissionRepository {
	return &permissionRepositoryImpl{db: db}
}

// UpsertPermission 授予或更新某个用户的 Agent 角色。
func (r *permissionRepositoryImpl) UpsertPermission(ctx context.Context, permission *model.BotPermission) error {
	return r.db.WithContext(ctx).
		Where("bot_id = ? AND user_id = ?", permission.BotID, permission.UserID).
		Assign(map[string]interface{}{"role": permission.Role}).
		FirstOrCreate(permission).Error
}

// DeletePermission 撤销某个用户的 Agent 角色。
func (r *permissionRepositoryImpl) DeletePermission(ctx context.Context, botID, userID int64) error {
	return r.db.WithContext(ctx).
		Where("bot_id = ? AND user_id = ?", botID, userID).
		Delete(&model.BotPermission{}).Error
}

// GetPermission 查询单条权限记录，不存在时返回 nil。
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

// ListPermissions 查询某个 Agent 的全部协作者角色。
func (r *permissionRepositoryImpl) ListPermissions(ctx context.Context, botID int64) ([]model.BotPermission, error) {
	var permissions []model.BotPermission
	err := r.db.WithContext(ctx).Where("bot_id = ?", botID).Find(&permissions).Error
	return permissions, err
}

// RouteRepository 保存 Agent 路由规则。
type RouteRepository interface {
	CreateRoute(ctx context.Context, route *model.BotRoute) error
	GetRouteByID(ctx context.Context, id int64) (*model.BotRoute, error)
	ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error)
	DeleteRoute(ctx context.Context, id int64) error
}

// routeRepositoryImpl 是基于 GORM 的路由规则仓储。
type routeRepositoryImpl struct {
	db *gorm.DB
}

// NewRouteRepo 创建基于 GORM 的路由规则仓储。
func NewRouteRepo(db *gorm.DB) RouteRepository {
	return &routeRepositoryImpl{db: db}
}

// CreateRoute 插入一条路由规则。
func (r *routeRepositoryImpl) CreateRoute(ctx context.Context, route *model.BotRoute) error {
	return r.db.WithContext(ctx).Create(route).Error
}

// GetRouteByID 查询单条路由，不存在时返回 nil。
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

// ListRoutes 按优先级查询某个 Agent 的路由规则。
func (r *routeRepositoryImpl) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	var routes []model.BotRoute
	if err := r.db.WithContext(ctx).Where("bot_id = ?", botID).Order("priority DESC").Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

// DeleteRoute 删除一条路由规则。
func (r *routeRepositoryImpl) DeleteRoute(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.BotRoute{}, id).Error
}

// BillingRepository 保存实际 token 用量和费用记录。
type BillingRepository interface {
	CreateRecord(ctx context.Context, record *model.BillingRecord) error
	ListRecords(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error)
}

// billingRepositoryImpl 是基于 GORM 的计费仓储。
type billingRepositoryImpl struct {
	db *gorm.DB
}

// NewBillingRepo 创建基于 GORM 的计费仓储。
func NewBillingRepo(db *gorm.DB) BillingRepository {
	return &billingRepositoryImpl{db: db}
}

// CreateRecord 插入一条计费记录。
func (r *billingRepositoryImpl) CreateRecord(ctx context.Context, record *model.BillingRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// ListRecords 返回分页计费记录和总数。
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

// containsCSVToken 判断逗号分隔列表中是否包含指定 token。
func containsCSVToken(csv string, token string) bool {
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}
