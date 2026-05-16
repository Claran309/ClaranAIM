package dao

import (
	"ClaranAIM/internal/bot-manager-service/model"
	"context"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.Bot{},
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

type BotRepository interface {
	CreateBot(ctx context.Context, bot *model.Bot) error
	GetBotByID(ctx context.Context, id int64) (*model.Bot, error)
	ListBots(ctx context.Context, ownerID int64, botType string) ([]model.Bot, error)
	UpdateBot(ctx context.Context, bot *model.Bot) error
	DeleteBot(ctx context.Context, id int64) error
}

type botRepositoryImpl struct {
	db *gorm.DB
}

func NewBotRepo(db *gorm.DB) BotRepository {
	return &botRepositoryImpl{db: db}
}

func (r *botRepositoryImpl) CreateBot(ctx context.Context, bot *model.Bot) error {
	return r.db.WithContext(ctx).Create(bot).Error
}

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

func (r *botRepositoryImpl) UpdateBot(ctx context.Context, bot *model.Bot) error {
	return r.db.WithContext(ctx).Save(bot).Error
}

func (r *botRepositoryImpl) DeleteBot(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Bot{}, id).Error
}

type RouteRepository interface {
	CreateRoute(ctx context.Context, route *model.BotRoute) error
	ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error)
	DeleteRoute(ctx context.Context, id int64) error
}

type routeRepositoryImpl struct {
	db *gorm.DB
}

func NewRouteRepo(db *gorm.DB) RouteRepository {
	return &routeRepositoryImpl{db: db}
}

func (r *routeRepositoryImpl) CreateRoute(ctx context.Context, route *model.BotRoute) error {
	return r.db.WithContext(ctx).Create(route).Error
}

func (r *routeRepositoryImpl) ListRoutes(ctx context.Context, botID int64) ([]model.BotRoute, error) {
	var routes []model.BotRoute
	if err := r.db.WithContext(ctx).Where("bot_id = ?", botID).Order("priority DESC").Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

func (r *routeRepositoryImpl) DeleteRoute(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.BotRoute{}, id).Error
}

type BillingRepository interface {
	CreateRecord(ctx context.Context, record *model.BillingRecord) error
	ListRecords(ctx context.Context, botID, userID int64, limit, offset int64) ([]model.BillingRecord, int64, error)
}

type billingRepositoryImpl struct {
	db *gorm.DB
}

func NewBillingRepo(db *gorm.DB) BillingRepository {
	return &billingRepositoryImpl{db: db}
}

func (r *billingRepositoryImpl) CreateRecord(ctx context.Context, record *model.BillingRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

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
